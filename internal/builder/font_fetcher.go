package builder

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/na2na-p/mnemonic/internal/cache"
)

// ErrFontDownload はKoruriフォントのダウンロードに関する基本エラー。
var ErrFontDownload = errors.New("フォントのダウンロードに失敗しました")

// Koruriフォント関連の定数。
const (
	// KoruriVersion はKoruriフォントのバージョン識別子。
	//
	// why not: GitHubリリースにはZIPアセットが存在しないため、リポジトリのmasterブランチ
	// から直接TTFを取得する（Python版と同じ制約）。そのためバージョンはブランチ名に留まる。
	KoruriVersion = "master"
	// KoruriFontName はフォント名。
	KoruriFontName = "Koruri-Regular"
	// KoruriTTFFilename はTTFファイル名。
	KoruriTTFFilename = KoruriFontName + ".ttf"
)

// defaultKoruriDownloadURL は既定のKoruriフォントダウンロードURL。
const defaultKoruriDownloadURL = "https://raw.githubusercontent.com/Koruri/Koruri/master/" + KoruriTTFFilename

// fontDownloadTimeout はフォントダウンロードの既定タイムアウト秒数。
const fontDownloadTimeout = 60 * time.Second

// FontInfo はフォント情報を表す不変値。
type FontInfo struct {
	Name    string
	Path    string
	Version string
}

// FontCacheDir はフォントキャッシュディレクトリを返す。
func FontCacheDir() (string, error) {
	dir, err := cache.Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "fonts"), nil
}

// FontFetcher はKoruriフォントをGitHubからダウンロードし、
// ローカルキャッシュに保存する。
type FontFetcher struct {
	// CacheDir はフォントの保存先ディレクトリ。空文字列の場合はFontCacheDir()を使用する。
	CacheDir string
	// HTTPClient はHTTPリクエストに使用するクライアント。
	HTTPClient *http.Client
	// SourceURL はダウンロード元URL。
	//
	// why not: httptestサーバーに差し替えてテストするための注入口
	// （TemplateDownloader.APIBaseURLと同じ方針）。空文字列の場合は
	// defaultKoruriDownloadURLを使用する。
	SourceURL string
}

// NewFontFetcher はFontFetcherを初期化する。
// httpClientがnilの場合はfontDownloadTimeoutを持つ既定クライアントを使用する。
func NewFontFetcher(cacheDir string, httpClient *http.Client) *FontFetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: fontDownloadTimeout}
	}

	return &FontFetcher{CacheDir: cacheDir, HTTPClient: httpClient}
}

// httpClient はHTTPClientフィールドを返す。nilの場合（FontFetcher{}のゼロ値を
// NewFontFetcherを介さず直接構築した場合）はfontDownloadTimeoutを持つクライアントを
// 都度生成して返し、nilポインタ参照によるpanicを避ける。
func (f *FontFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}

	return &http.Client{Timeout: fontDownloadTimeout}
}

func (f *FontFetcher) sourceURL() string {
	if f.SourceURL != "" {
		return f.SourceURL
	}

	return defaultKoruriDownloadURL
}

func (f *FontFetcher) cacheDir() (string, error) {
	if f.CacheDir != "" {
		return f.CacheDir, nil
	}

	return FontCacheDir()
}

// GetFont はフォントを取得する。
// キャッシュに有効なフォントが存在する場合はそれを返し、存在しない場合はダウンロードする。
func (f *FontFetcher) GetFont() (FontInfo, error) {
	if f.IsCacheValid() {
		if path, ok := f.GetCachedFontPath(); ok {
			return FontInfo{Name: KoruriFontName, Path: path, Version: KoruriVersion}, nil
		}
	}

	return f.DownloadFont()
}

// DownloadFont はGitHubからKoruri-Regular.ttfを直接ダウンロードしてキャッシュに保存する。
func (f *FontFetcher) DownloadFont() (FontInfo, error) {
	dir, err := f.cacheDir()
	if err != nil {
		return FontInfo{}, err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return FontInfo{}, fmt.Errorf("%w: キャッシュディレクトリの作成に失敗しました: %w", ErrFontDownload, err)
	}

	content, err := f.downloadTTF()
	if err != nil {
		return FontInfo{}, err
	}

	fontPath := filepath.Join(dir, KoruriTTFFilename)
	if err := os.WriteFile(fontPath, content, 0o600); err != nil {
		return FontInfo{}, fmt.Errorf("%w: フォントファイルの保存に失敗しました: %w", ErrFontDownload, err)
	}

	return FontInfo{Name: KoruriFontName, Path: fontPath, Version: KoruriVersion}, nil
}

func (f *FontFetcher) downloadTTF() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, f.sourceURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: リクエストの構築に失敗しました: %w", ErrFontDownload, err)
	}

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: ネットワークエラー: %w", ErrFontDownload, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTPエラー %d", ErrFontDownload, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: レスポンスの読み込みに失敗しました: %w", ErrFontDownload, err)
	}

	return content, nil
}

// IsCacheValid はキャッシュが有効かどうかを確認する。
func (f *FontFetcher) IsCacheValid() bool {
	dir, err := f.cacheDir()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(dir, KoruriTTFFilename))

	return err == nil
}

// GetCachedFontPath はキャッシュされたフォントのパスを取得する。
// キャッシュが無い場合はok=falseを返す。
func (f *FontFetcher) GetCachedFontPath() (string, bool) {
	dir, err := f.cacheDir()
	if err != nil {
		return "", false
	}

	path := filepath.Join(dir, KoruriTTFFilename)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
}
