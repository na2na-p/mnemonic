package builder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// センチネルエラー群。
var (
	// ErrTemplateNotFound は指定されたバージョンのテンプレートが存在しない場合のエラー。
	ErrTemplateNotFound = errors.New("指定されたバージョンのテンプレートが見つかりません")
	// ErrNetwork はネットワークエラー発生時のエラー。
	ErrNetwork = errors.New("ネットワークエラーが発生しました")
	// ErrFileIntegrity はダウンロードしたファイルの整合性チェックに失敗した場合のエラー。
	ErrFileIntegrity = errors.New("ファイルの整合性チェックに失敗しました")
	// ErrInvalidVersion はバージョン文字列の形式が不正な場合のエラー。
	ErrInvalidVersion = errors.New("バージョン形式が不正です")
)

// defaultGitHubAPIBase は既定のGitHub API向けベースURL。
const defaultGitHubAPIBase = "https://api.github.com/repos/na2na-p/mnemonic"

// defaultTimeout はHTTPリクエストの既定タイムアウト秒数。
const defaultTimeout = 30 * time.Second

// downloadTimeout はダウンロードの既定タイムアウト秒数。
const downloadTimeout = 300 * time.Second

// downloadMaxRetries はファイルダウンロードのリトライ回数
// （初回試行を含まない再試行回数）。
//
// why not: ネットワークレベルの一時的エラー（タイムアウト・接続エラー）に
// 限定してダウンロード処理にのみリトライを追加する。
const downloadMaxRetries = 2

// downloadRetryInterval はリトライ間隔。
const downloadRetryInterval = 500 * time.Millisecond

// errRetryableDownload はダウンロード処理内でリトライ対象となる、接続レベルの
// 一時的エラー（接続確立時のタイムアウト・接続エラー、ダウンロード中の接続切断）
// を示す非公開センチネル。HTTPステータスエラー（404/5xx等）やローカルファイル
// 操作エラーは、リトライしても解決しないため意図的にこのセンチネルでラップせず
// 対象外とする（downloadFileのwhy not参照）。
var errRetryableDownload = errors.New("リトライ対象の接続エラー")

// templateAssetPattern はテンプレートアセットのファイル名パターン。
var templateAssetPattern = regexp.MustCompile(`(?i)android-template\.zip$`)

// versionPattern はバージョン文字列の検証パターン
// （CalVer: template-YYYY.MM.DD、同日複数リリース時は連番サフィックス付き）。
//
// why not: 同日に複数リリースすると、GitHub上のタグ名に連番サフィックス
// （例: template-2026.07.28-12）が付く運用のため、末尾の "-数値" を任意で許容する。
var versionPattern = regexp.MustCompile(`^template-\d{4}\.\d{2}\.\d{2}(-\d+)?$`)

// TemplateInfo はテンプレート情報を表す不変値。
type TemplateInfo struct {
	Version     string
	DownloadURL string
	FileSize    int64
	FileName    string
}

// TemplateDownloader はGitHub Releasesからkrkrsdl2テンプレートをダウンロードする。
type TemplateDownloader struct {
	// CacheDir はダウンロード先ディレクトリ。空文字列の場合は
	// ~/.cache/mnemonic/templates を使用する。
	CacheDir string
	// HTTPClient はHTTPリクエストに使用するクライアント。
	HTTPClient *http.Client
	// APIBaseURL はGitHub APIのベースURL。空文字列の場合はdefaultGitHubAPIBaseを使用する。
	//
	// why not: httptestサーバーに差し替えてテストするための注入口。
	// production既定値は非公開定数(defaultGitHubAPIBase)のままにし、
	// テストのみがこのフィールドを介してURLを上書きする
	// （internal/cache.DirForOSがgoos引数でテスト容易性を確保しているのと同じ方針）。
	APIBaseURL string
}

// NewTemplateDownloader はTemplateDownloaderを初期化する。
// httpClientがnilの場合はdefaultTimeoutを持つ既定クライアントを使用する。
func NewTemplateDownloader(cacheDir string, httpClient *http.Client) *TemplateDownloader {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &TemplateDownloader{CacheDir: cacheDir, HTTPClient: httpClient}
}

func (d *TemplateDownloader) apiBase() string {
	if d.APIBaseURL != "" {
		return d.APIBaseURL
	}

	return defaultGitHubAPIBase
}

// httpClient はHTTPClientフィールドを返す。nilの場合（builder.TemplateDownloader{}の
// ゼロ値をNewTemplateDownloaderを介さず直接構築した場合）はdefaultTimeoutを持つ
// クライアントを都度生成して返し、nilポインタ参照によるpanicを避ける。
func (d *TemplateDownloader) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}

	return &http.Client{Timeout: defaultTimeout}
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Download は指定バージョンのテンプレートをダウンロードする。
// versionがnilの場合は最新バージョンをダウンロードする。
func (d *TemplateDownloader) Download(version *string) (string, error) {
	targetVersion := ""
	if version != nil {
		targetVersion = *version
	} else {
		latest, err := d.GetLatestVersion()
		if err != nil {
			return "", err
		}
		targetVersion = latest
	}

	if err := d.validateVersion(targetVersion); err != nil {
		return "", err
	}

	templateInfo, err := d.getReleaseInfo(targetVersion)
	if err != nil {
		return "", err
	}

	cacheDir := d.CacheDir
	if cacheDir == "" {
		cacheDir, err = defaultDownloaderCacheDir()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return "", fmt.Errorf("キャッシュディレクトリの作成に失敗しました: %w", err)
	}

	downloadPath := filepath.Join(cacheDir, targetVersion, templateInfo.FileName)
	if err := os.MkdirAll(filepath.Dir(downloadPath), 0o750); err != nil {
		return "", fmt.Errorf("ダウンロード先ディレクトリの作成に失敗しました: %w", err)
	}

	if err := d.downloadFile(templateInfo.DownloadURL, downloadPath); err != nil {
		return "", err
	}

	if err := d.verifyFileIntegrity(downloadPath, templateInfo.FileSize); err != nil {
		return "", err
	}

	return downloadPath, nil
}

func defaultDownloaderCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリを取得できません: %w", err)
	}

	return filepath.Join(home, ".cache", "mnemonic", "templates"), nil
}

// GetLatestVersion は最新バージョンを取得する。
//
// /releases/latestを試み、404の場合は/releasesから最初のリリースを取得する
// （pre-releaseのみのリポジトリに対応するため）。
func (d *TemplateDownloader) GetLatestVersion() (string, error) {
	resp, err := d.githubGet(d.apiBase() + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		releasesResp, err := d.githubGet(d.apiBase() + "/releases")
		if err != nil {
			return "", err
		}
		defer func() { _ = releasesResp.Body.Close() }()

		if err := checkGitHubStatus(releasesResp); err != nil {
			return "", err
		}

		var releases []githubRelease
		if err := json.NewDecoder(releasesResp.Body).Decode(&releases); err != nil {
			return "", fmt.Errorf("%w: APIレスポンスの形式が不正です: %w", ErrNetwork, err)
		}
		if len(releases) == 0 {
			return "", fmt.Errorf("%w: リリースが見つかりません", ErrNetwork)
		}

		return releases[0].TagName, nil
	}

	if err := checkGitHubStatus(resp); err != nil {
		return "", err
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("%w: APIレスポンスの形式が不正です: %w", ErrNetwork, err)
	}

	return release.TagName, nil
}

// GetDownloadURL は指定バージョンのダウンロードURLを構築する。
func (d *TemplateDownloader) GetDownloadURL(version string) (string, error) {
	if err := d.validateVersion(version); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://github.com/na2na-p/mnemonic/releases/download/%s/android-template.zip", version), nil
}

func (d *TemplateDownloader) validateVersion(version string) error {
	if version == "" || !versionPattern.MatchString(version) {
		return fmt.Errorf("%w: '%s'", ErrInvalidVersion, version)
	}

	return nil
}

func (d *TemplateDownloader) getReleaseInfo(version string) (TemplateInfo, error) {
	releaseURL := fmt.Sprintf("%s/releases/tags/%s", d.apiBase(), version)

	resp, err := d.githubGet(releaseURL)
	if err != nil {
		return TemplateInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return TemplateInfo{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, version)
	}

	if err := checkGitHubStatus(resp); err != nil {
		return TemplateInfo{}, err
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return TemplateInfo{}, fmt.Errorf("%w: APIレスポンスの形式が不正です: %w", ErrNetwork, err)
	}

	asset, ok := findTemplateAsset(release.Assets)
	if !ok {
		return TemplateInfo{}, fmt.Errorf("%w: バージョン'%s'に対応するテンプレートアセットが見つかりません", ErrTemplateNotFound, version)
	}

	return TemplateInfo{
		Version:     release.TagName,
		DownloadURL: asset.BrowserDownloadURL,
		FileSize:    asset.Size,
		FileName:    asset.Name,
	}, nil
}

func findTemplateAsset(assets []githubAsset) (githubAsset, bool) {
	for _, asset := range assets {
		if templateAssetPattern.MatchString(asset.Name) {
			return asset, true
		}
	}

	return githubAsset{}, false
}

// githubGet はGitHub APIへGETリクエストを送る。
func (d *TemplateDownloader) githubGet(targetURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: リクエストの構築に失敗しました: %w", ErrNetwork, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := d.httpClient().Do(req)
	if err != nil {
		return nil, classifyHTTPError(err)
	}

	return resp, nil
}

// checkGitHubStatus はGitHub APIレスポンスのステータスコードを検証する。
func checkGitHubStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: HTTPステータス %d", ErrTemplateNotFound, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTPエラーが発生しました: %d", ErrNetwork, resp.StatusCode)
	}

	return nil
}

// classifyHTTPError はhttp.Client.Doが返したerror（接続確立の失敗やタイムアウト等、
// リクエストがサーバーへ到達すらしなかったエラー）をNetworkError系へ分類する。
// errRetryableDownloadでもラップするため、downloadFileのリトライ判定対象になる。
func classifyHTTPError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return fmt.Errorf("%w: %w: リクエストがタイムアウトしました: %w", ErrNetwork, errRetryableDownload, err)
	}

	return fmt.Errorf("%w: %w: %w", ErrNetwork, errRetryableDownload, err)
}

// downloadFile はurlからdestinationへファイルをダウンロードする。
//
// 接続レベルの一時的エラー（errRetryableDownloadでラップされたエラー）に限り、
// downloadMaxRetries回まで再試行する（why notはdownloadMaxRetries定義を参照）。
// HTTPステータスエラー（404/5xx等）やローカルファイル操作エラーはリトライしても
// 解決しないため、即座にエラーを返す（errRetryableDownloadのwhy not参照）。
func (d *TemplateDownloader) downloadFile(downloadURL, destination string) error {
	var lastErr error

	for attempt := 0; attempt <= downloadMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(downloadRetryInterval)
		}

		err := d.downloadFileOnce(downloadURL, destination)
		if err == nil {
			return nil
		}

		lastErr = err

		if !errors.Is(err, errRetryableDownload) {
			return err
		}
	}

	return lastErr
}

func (d *TemplateDownloader) downloadFileOnce(downloadURL, destination string) error {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("%w: リクエストの構築に失敗しました: %w", ErrNetwork, err)
	}

	client := d.httpClient()
	if client.Timeout == 0 || client.Timeout > downloadTimeout {
		clientCopy := *client
		clientCopy.Timeout = downloadTimeout
		client = &clientCopy
	}

	resp, err := client.Do(req)
	if err != nil {
		return classifyHTTPError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrTemplateNotFound, downloadURL)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%w: ダウンロード中にHTTPエラーが発生しました: %d", ErrNetwork, resp.StatusCode)
	}

	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // 呼び出し元がキャッシュディレクトリ配下に限定して構築したパスのため妥当
	if err != nil {
		return fmt.Errorf("ダウンロード先ファイルの作成に失敗しました: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil { //nolint:gosec // テンプレートZIPのダウンロードでありサイズ上限は設けない
		return classifyHTTPError(err)
	}

	return nil
}

// verifyFileIntegrity はダウンロードしたファイルの整合性を検証する
// （ファイルサイズの一致のみを確認し、ハッシュ検証は行わない）。
func (d *TemplateDownloader) verifyFileIntegrity(filePath string, expectedSize int64) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("%w: ダウンロードしたファイルが見つかりません: %w", ErrFileIntegrity, err)
	}

	if info.Size() != expectedSize {
		return fmt.Errorf("%w: ファイルサイズが一致しません: 期待値 %d バイト、実際 %d バイト", ErrFileIntegrity, expectedSize, info.Size())
	}

	return nil
}
