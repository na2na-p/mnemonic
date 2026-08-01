package builder

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// センチネルエラー群。
//
// ErrSDL2SourceFetchNetwork / ErrSDL2SourceFetchTimeout / ErrSDL2SourceCache は
// いずれもErrSDL2SourceFetcherと二重にラップする。
// これによりerrors.Is(err, ErrSDL2SourceFetcher)がすべての具体エラーで真になる。
var (
	// ErrSDL2SourceFetcher はSDL2 Javaソース取得に関する基本エラー。
	ErrSDL2SourceFetcher = errors.New("SDL2ソースの取得に失敗しました")
	// ErrSDL2SourceFetchNetwork はネットワークエラーが発生した場合のエラー。
	ErrSDL2SourceFetchNetwork = errors.New("ネットワークエラーが発生しました")
	// ErrSDL2SourceFetchTimeout はタイムアウトが発生した場合のエラー。
	ErrSDL2SourceFetchTimeout = errors.New("タイムアウトが発生しました")
	// ErrSDL2SourceCache はキャッシュ操作に関するエラー。
	ErrSDL2SourceCache = errors.New("キャッシュ操作に失敗しました")
)

// sdlCommit はkrkrsdl2が使用しているSDLコミット。このコミットのJavaソースは
// krkrsdl2のネイティブライブラリ(libSDL2.so)と互換性がある。
const sdlCommit = "53dea9830964eee8b5c2a7ee0a65d6e268dc78a1"

// sdlCommitShortLen はSDL2CacheCurrentVersionとして使うsdlCommitの先頭文字数
// （8文字の短縮形）。
const sdlCommitShortLen = 8

// SDL2CacheCurrentVersion は現在のキャッシュバージョン（SDLコミットSHAの短縮形）。
//
// why not: コメントでsdlCommitとの同期を表明するだけでは形骸化しうるため、
// sdlCommitからのスライス派生にして構造的に同期を保証する
// （P4: 「why not」はコメントで表明するのではなくコードで保証できる場合はそちらを優先する）。
// 文字列スライスは定数式にできないため、Go言語仕様上varとせざるを得ない。
var SDL2CacheCurrentVersion = sdlCommit[:sdlCommitShortLen]

// SDL2ソースキャッシュ関連の定数。
const (
	// SDL2CacheValidityDays はキャッシュ有効期間（日）。
	SDL2CacheValidityDays = 30
	// SDL2CacheMarkerFile はキャッシュ作成日時マーカーのファイル名。
	SDL2CacheMarkerFile = ".cached_at"
	// SDL2CacheVersionFile はキャッシュバージョンのファイル名。
	SDL2CacheVersionFile = ".version"
)

// sdl2CacheMarkerTimeLayout はキャッシュ作成日時マーカーの日時フォーマット。
const sdl2CacheMarkerTimeLayout = time.RFC3339Nano

// SDL2SourceCacheInfo はキャッシュ情報を表す不変値。
type SDL2SourceCacheInfo struct {
	// CachedAt はキャッシュ作成日時。未作成の場合はnil。
	CachedAt  *time.Time
	IsValid   bool
	CachePath string
}

// SDL2SourceCache はSDL2 Javaソースのキャッシュを管理する。
type SDL2SourceCache struct {
	cacheDir string
}

// NewSDL2SourceCache はSDL2SourceCacheを初期化する。
// cacheDir配下の"sdl2_sources"ディレクトリをキャッシュ領域として使用する。
func NewSDL2SourceCache(cacheDir string) *SDL2SourceCache {
	return &SDL2SourceCache{cacheDir: filepath.Join(cacheDir, "sdl2_sources")}
}

// CachePath はキャッシュディレクトリのパスを返す。
func (c *SDL2SourceCache) CachePath() string {
	return c.cacheDir
}

// GetSourceFilesPath はキャッシュされたソースファイルのパス（org/libsdl/app）を返す。
func (c *SDL2SourceCache) GetSourceFilesPath() string {
	return filepath.Join(c.cacheDir, "org", "libsdl", "app")
}

// IsValid はキャッシュが有効か確認する。
// キャッシュが存在し、有効期限内かつバージョンが一致すればtrueを返す。
func (c *SDL2SourceCache) IsValid() bool {
	marker := filepath.Join(c.cacheDir, SDL2CacheMarkerFile)
	if _, err := os.Stat(marker); err != nil {
		return false
	}

	versionFile := filepath.Join(c.cacheDir, SDL2CacheVersionFile)
	versionContent, err := os.ReadFile(versionFile) //nolint:gosec // キャッシュディレクトリ配下の固定ファイル名を読む用途のため妥当
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(versionContent)) != SDL2CacheCurrentVersion {
		return false
	}

	cachedAt, ok := c.GetCachedAt()
	if !ok {
		return false
	}

	return time.Since(cachedAt) < SDL2CacheValidityDays*24*time.Hour
}

// GetCachedAt はキャッシュ作成日時を取得する。
// マーカーファイルが存在しない、または内容が不正な場合はok=falseを返す。
func (c *SDL2SourceCache) GetCachedAt() (time.Time, bool) {
	marker := filepath.Join(c.cacheDir, SDL2CacheMarkerFile)

	content, err := os.ReadFile(marker) //nolint:gosec // キャッシュディレクトリ配下の固定ファイル名を読む用途のため妥当
	if err != nil {
		return time.Time{}, false
	}

	cachedAt, err := time.Parse(sdl2CacheMarkerTimeLayout, strings.TrimSpace(string(content)))
	if err != nil {
		return time.Time{}, false
	}

	return cachedAt, true
}

// GetCacheInfo はキャッシュ情報を取得する。
func (c *SDL2SourceCache) GetCacheInfo() SDL2SourceCacheInfo {
	info := SDL2SourceCacheInfo{
		IsValid:   c.IsValid(),
		CachePath: c.cacheDir,
	}

	if cachedAt, ok := c.GetCachedAt(); ok {
		info.CachedAt = &cachedAt
	}

	return info
}

// Save はソースをキャッシュに保存する。sourcesDirはorgディレクトリを含む
// コピー元ディレクトリ。
func (c *SDL2SourceCache) Save(sourcesDir string) error {
	if err := os.RemoveAll(c.cacheDir); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	if err := os.MkdirAll(c.cacheDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	srcOrgDir := filepath.Join(sourcesDir, "org")
	if info, err := os.Stat(srcOrgDir); err == nil && info.IsDir() {
		if err := copyDir(srcOrgDir, filepath.Join(c.cacheDir, "org")); err != nil {
			return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
		}
	}

	marker := filepath.Join(c.cacheDir, SDL2CacheMarkerFile)
	if err := os.WriteFile(marker, []byte(time.Now().Format(sdl2CacheMarkerTimeLayout)), 0o600); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	versionFile := filepath.Join(c.cacheDir, SDL2CacheVersionFile)
	if err := os.WriteFile(versionFile, []byte(SDL2CacheCurrentVersion), 0o600); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	return nil
}

// RestoreTo はキャッシュからソースを復元する。
func (c *SDL2SourceCache) RestoreTo(destDir string) error {
	if !c.IsValid() {
		return fmt.Errorf("%w: %w: 有効なキャッシュがありません", ErrSDL2SourceFetcher, ErrSDL2SourceCache)
	}

	srcOrgDir := filepath.Join(c.cacheDir, "org")
	destOrgDir := filepath.Join(destDir, "org")

	if err := os.RemoveAll(destOrgDir); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ復元に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	if err := copyDir(srcOrgDir, destOrgDir); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ復元に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	return nil
}

// Clear はキャッシュを削除する。キャッシュが存在しない場合もエラーにはならない
// （os.RemoveAllの仕様に準拠）。
func (c *SDL2SourceCache) Clear() error {
	if err := os.RemoveAll(c.cacheDir); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ削除に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	return nil
}

// defaultSDL2BaseURL は既定のSDL2 Javaソース取得元ベースURL。
const defaultSDL2BaseURL = "https://raw.githubusercontent.com/libsdl-org/SDL/" + sdlCommit +
	"/android-project/app/src/main/java/org/libsdl/app"

// defaultSDL2Timeout はHTTPリクエストの既定タイムアウト秒数。
const defaultSDL2Timeout = 30 * time.Second

// SDL2RequiredFiles は取得対象のJavaファイル一覧（krkrsdl2互換コミット）。
// SDLSurfaceとSDLInputConnectionはSDLActivity.javaの内部クラスとして定義されている。
var SDL2RequiredFiles = []string{
	"SDLActivity.java",
	"SDL.java",
	"SDLAudioManager.java",
	"SDLControllerManager.java",
	"HIDDevice.java",
	"HIDDeviceManager.java",
	"HIDDeviceUSB.java",
	"HIDDeviceBLESteamController.java",
}

// SDL2SourceFetcher はSDL2のJavaソースコードをダウンロードし、キャッシュを管理する。
type SDL2SourceFetcher struct {
	// Timeout はHTTPリクエストのタイムアウト。
	Timeout time.Duration
	// Cache はキャッシュマネージャー。nilの場合はキャッシュを使用しない。
	Cache *SDL2SourceCache
	// HTTPClient はHTTPリクエストに使用するクライアント。
	HTTPClient *http.Client
	// BaseURL はJavaソースの取得元ベースURL。
	//
	// why not: httptestサーバーに差し替えてテストするための注入口
	// （TemplateDownloader.APIBaseURLと同じ方針）。空文字列の場合は
	// defaultSDL2BaseURLを使用する。
	BaseURL string
}

// NewSDL2SourceFetcher はSDL2SourceFetcherを初期化する。
// timeoutが0以下の場合はdefaultSDL2Timeoutを使用する。
//
// why not: HTTPClientをここで生成せずhttpClient()に都度生成させると、Fetch1回
// あたり8ファイルぶんhttp.Clientを毎回新規アロケートすることになる
// （NewFontFetcher/NewPluginFetcherは既定クライアントをコンストラクタで確保して
// いるのと同じ理由で、ここでも1つのクライアントをFetch全体で使い回す）。
func NewSDL2SourceFetcher(timeout time.Duration, cache *SDL2SourceCache) *SDL2SourceFetcher {
	if timeout <= 0 {
		timeout = defaultSDL2Timeout
	}

	return &SDL2SourceFetcher{
		Timeout:    timeout,
		Cache:      cache,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// httpClient はHTTPClientフィールドを返す。nilの場合（SDL2SourceFetcher{}のゼロ値を
// NewSDL2SourceFetcherを介さず直接構築した場合）はTimeoutを反映したクライアントを
// 都度生成して返し、nilポインタ参照によるpanicを避ける。
func (f *SDL2SourceFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}

	timeout := f.Timeout
	if timeout <= 0 {
		timeout = defaultSDL2Timeout
	}

	return &http.Client{Timeout: timeout}
}

func (f *SDL2SourceFetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}

	return defaultSDL2BaseURL
}

// Fetch はSDL2 Javaソースをダウンロードして配置する。
// キャッシュが有効な場合はキャッシュから復元し、そうでない場合はGitHubからダウンロードする。
func (f *SDL2SourceFetcher) Fetch(destDir string) error {
	if f.Cache != nil && f.Cache.IsValid() {
		return f.Cache.RestoreTo(destDir)
	}

	sdlAppDir := filepath.Join(destDir, "org", "libsdl", "app")
	if err := os.MkdirAll(sdlAppDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w: 配置先ディレクトリの作成に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, err)
	}

	for _, filename := range SDL2RequiredFiles {
		content, err := f.downloadJavaFile(filename)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(sdlAppDir, filename), content, 0o600); err != nil {
			return fmt.Errorf("%w: %w: ファイルの書き込みに失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, err)
		}
	}

	if f.Cache != nil {
		if err := f.Cache.Save(destDir); err != nil {
			return err
		}
	}

	return nil
}

func (f *SDL2SourceFetcher) downloadJavaFile(filename string) ([]byte, error) {
	targetURL := f.baseURL() + "/" + filename

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: リクエストの構築に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, err)
	}

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, classifySDL2FetchError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %w: SDL2ソースのダウンロードに失敗しました: HTTP %d",
			ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		// why not: ヘッダー受信後、ボディ転送中にタイムアウトするケース（例:
		// Content-Lengthは返るがサーバーが応答を止める）もclassifySDL2FetchErrorへ
		// 通す。ここで直接ErrSDL2SourceFetchNetworkに固定すると、ボディ読み込み中の
		// タイムアウトがErrSDL2SourceFetchTimeoutとして分類されなくなる
		// （TemplateDownloader.downloadFileOnceのio.Copy呼び出しと同じ方針）。
		return nil, classifySDL2FetchError(err)
	}

	return content, nil
}

// classifySDL2FetchError はhttp.Client.Do、およびレスポンスボディ読み込み
// （resp.Body.Read）が返したerrorをタイムアウト/ネットワークエラーへ分類する
// （TemplateDownloader.classifyHTTPErrorと同じ方針）。
//
// why not: ヘッダー受信前のタイムアウト（Client.Doが返す*url.Error）と、ヘッダー
// 受信後・ボディ転送中のタイムアウト（resp.Body.Readが返す非公開の
// *http.timeoutError）は異なる型だが、いずれもnet.Errorを実装しTimeout()が
// trueを返す。*url.Errorのみをerrors.Asで検査すると後者を取りこぼす
// （ボディ転送中のタイムアウトがErrSDL2SourceFetchNetworkに誤分類される）ため、
// net.Error単体で検査して両方のフェーズのタイムアウトを一様に扱う。
func classifySDL2FetchError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %w: SDL2ソースのダウンロードがタイムアウトしました: %w",
			ErrSDL2SourceFetcher, ErrSDL2SourceFetchTimeout, err)
	}

	return fmt.Errorf("%w: %w: SDL2ソースのダウンロードに失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, err)
}
