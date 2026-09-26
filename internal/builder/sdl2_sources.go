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

// IsValid はキャッシュが有効か確認する。マーカーとバージョンが一致し、有効期限内で、
// org/libsdl/app配下のSDL2RequiredFilesがすべて通常ファイルならtrueを返す。
//
// why not: マーカーは保存完了、バージョンは保存時の版を示すだけで、その後にソースが
// 削除・破損していないことまでは保証しない。無効なキャッシュはダウンロードと上書きで
// 自己修復できるため、必須ファイルも確認する。
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

	if time.Since(cachedAt) >= SDL2CacheValidityDays*24*time.Hour {
		return false
	}

	sdlAppDir := filepath.Join(c.cacheDir, "org", "libsdl", "app")
	for _, filename := range SDL2RequiredFiles {
		info, err := os.Stat(filepath.Join(sdlAppDir, filename))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}

	return true
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

// sdl2CacheTempPrefix はSaveが新しいキャッシュを組み立てる一時ディレクトリ名の接頭辞。
const sdl2CacheTempPrefix = ".sdl2-cache-"

// Save はソースをキャッシュに保存する。sourcesDirはorgディレクトリを含む
// コピー元ディレクトリ。
//
// 既存キャッシュの削除より前（ディレクトリの作成、コピー、マーカー・バージョンの
// 書き込み）で失敗した場合は、既存のキャッシュが残る。既存キャッシュの削除または
// 置き換えで失敗した場合は、既存のキャッシュが消えているか一部だけ消えた状態に
// なりうるため、有効なキャッシュが残るとは限らない。残ったキャッシュが無効なら、
// 次回の取得で再ダウンロードされる。
//
// why not: キャッシュディレクトリへ直接書き込むと、書き込み中の不完全なキャッシュを
// 並行するビルドが読み、保存の途中で落ちるとキャッシュ自体が失われる。そのため
// 同じ親ディレクトリの一時ディレクトリに組み立ててからos.Renameで置き換える。
// 同じ親に作るのは、Renameがファイルシステムをまたげないため。RemoveAllから
// Renameまでの間はキャッシュが存在しないが、既存ディレクトリとの完全なアトミック
// 入れ替えにはrenameat2(RENAME_EXCHANGE)などのOS固有機能が要り移植性が無いため採らない。
func (c *SDL2SourceCache) Save(sourcesDir string) error {
	parentDir := filepath.Dir(c.cacheDir)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	removeStaleSDL2CacheTempDirs(parentDir)

	tmpDir, err := os.MkdirTemp(parentDir, sdl2CacheTempPrefix+"*")
	if err != nil {
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	if err := populateSDL2Cache(tmpDir, sourcesDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	if err := os.RemoveAll(c.cacheDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	if err := os.Rename(tmpDir, c.cacheDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%w: %w: キャッシュ保存に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
	}

	return nil
}

// removeStaleSDL2CacheTempDirs はparentDirに残ったSave用の一時ディレクトリを削除する。
//
// why not: 保存の途中で落ちると一時ディレクトリが残り、放置すると保存のたびに溜まるため
// 次の保存時に掃除する。parentDirはほかのキャッシュと共有するため、接頭辞が一致する
// ものだけを消す。掃除に失敗しても今回の保存には影響しないため、エラーは返さない。
// 同時に走る別のSaveが組み立て中の一時ディレクトリも消しうるが、ロックは設けない。
// FetchはSaveの失敗を無視し、必須ファイルの欠けたキャッシュはIsValidが無効と判定して
// 次回の取得で再ダウンロードされるため、この競合でビルドは失敗しない。
func removeStaleSDL2CacheTempDirs(parentDir string) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), sdl2CacheTempPrefix) {
			_ = os.RemoveAll(filepath.Join(parentDir, entry.Name()))
		}
	}
}

// populateSDL2Cache はdstDirにsourcesDir/orgのコピーとキャッシュのマーカー・バージョンを書き込む。
func populateSDL2Cache(dstDir, sourcesDir string) error {
	srcOrgDir := filepath.Join(sourcesDir, "org")
	if info, err := os.Stat(srcOrgDir); err == nil && info.IsDir() {
		if err := copyDir(srcOrgDir, filepath.Join(dstDir, "org")); err != nil {
			return err
		}
	}

	marker := filepath.Join(dstDir, SDL2CacheMarkerFile)
	if err := os.WriteFile(marker, []byte(time.Now().Format(sdl2CacheMarkerTimeLayout)), 0o600); err != nil {
		return err
	}

	versionFile := filepath.Join(dstDir, SDL2CacheVersionFile)
	return os.WriteFile(versionFile, []byte(SDL2CacheCurrentVersion), 0o600)
}

// RestoreTo はキャッシュからソースを復元する。
// コピーの途中で失敗した場合は、コピー済みのdestDir/orgの削除を試みてからエラーを返す。
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
		// 部分コピーを残して呼び出し側の上書きに任せない。コピーされたファイルは
		// キャッシュ側のパーミッションを引き継ぐため、読み取り専用のファイルが残ると
		// ダウンロードへのフォールバックが上書きできずに失敗する。
		// 削除に失敗しても、上書きを妨げる残骸はダウンロード側の書き込みエラーとして
		// 表面化するため、ここではコピーのエラーだけを返す。
		_ = os.RemoveAll(destOrgDir)
		return fmt.Errorf("%w: %w: キャッシュ復元に失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceCache, err)
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

// Fetch はSDL2 Javaソースをダウンロードまたはキャッシュから復元して配置する。
// 有効なキャッシュの復元に失敗した場合はダウンロードへフォールバックする。
func (f *SDL2SourceFetcher) Fetch(destDir string) error {
	if f.Cache != nil && f.Cache.IsValid() {
		// why not: キャッシュは最適化に過ぎないため、復元や保存の失敗で
		// ソースを取得できるビルドまで失敗させるより、再ダウンロードを選ぶ。
		if err := f.Cache.RestoreTo(destDir); err == nil {
			return nil
		}
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
		_ = f.Cache.Save(destDir)
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
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return fmt.Errorf("%w: %w: SDL2ソースのダウンロードがタイムアウトしました: %w",
			ErrSDL2SourceFetcher, ErrSDL2SourceFetchTimeout, err)
	}

	return fmt.Errorf("%w: %w: SDL2ソースのダウンロードに失敗しました: %w", ErrSDL2SourceFetcher, ErrSDL2SourceFetchNetwork, err)
}
