package builder

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/cache"
)

// ErrPluginDownload はkrkrsdl2プラグインのダウンロードに関する基本エラー。
var ErrPluginDownload = errors.New("プラグインのダウンロードに失敗しました")

// SupportedABIs はサポートするABI一覧。
var SupportedABIs = []string{"arm64-v8a", "armeabi-v7a", "x86", "x86_64"}

// pluginDownloadTimeout はプラグインダウンロードの既定タイムアウト秒数。
const pluginDownloadTimeout = 60 * time.Second

// pluginABIPlaceholder はURLTemplate内でABI名に置換されるプレースホルダー。
const pluginABIPlaceholder = "{abi}"

// PluginConfig は単一プラグインのダウンロード設定を表す不変値。
type PluginConfig struct {
	// Name はプラグイン名（例: extrans）。
	Name string
	// SourceFilename はZIP内のファイル名（例: extrans.so）。
	SourceFilename string
	// OutputFilename は出力ファイル名（例: libextrans.so）。
	OutputFilename string
	// URLTemplate はダウンロードURLテンプレート（pluginABIPlaceholderがABI名に置換される）。
	URLTemplate string
}

// DefaultPluginConfigs は既定のプラグイン設定一覧（extrans, wuvorbis）。
var DefaultPluginConfigs = []PluginConfig{
	{
		Name:           "extrans",
		SourceFilename: "extrans.so",
		OutputFilename: "libextrans.so",
		URLTemplate:    "https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-{abi}.zip",
	},
	{
		Name:           "wuvorbis",
		SourceFilename: "wuvorbis.so",
		OutputFilename: "libwuvorbis.so",
		URLTemplate:    "https://github.com/krkrsdl2/wuvorbis/releases/download/latest_krkrsdl2/wuvorbis-android-{abi}.zip",
	},
}

// PluginInfo は単一プラグイン情報を表す不変値。
type PluginInfo struct {
	// Name はプラグイン名。
	Name string
	// Paths はABI名をキー、プラグインファイルパスを値とする対応表。
	Paths map[string]string
}

// PluginsInfo は複数プラグイン情報を表す不変値。
type PluginsInfo struct {
	// Plugins はプラグイン名をキー、PluginInfoを値とする対応表。
	Plugins map[string]PluginInfo
}

// GetAllPathsForABI は指定ABIの全プラグインパスを取得する。
func (p PluginsInfo) GetAllPathsForABI(abi string) map[string]string {
	result := make(map[string]string, len(p.Plugins))

	for name, info := range p.Plugins {
		if path, ok := info.Paths[abi]; ok {
			result[name] = path
		}
	}

	return result
}

// PluginCacheDir はプラグインキャッシュディレクトリを返す。
func PluginCacheDir() (string, error) {
	dir, err := cache.Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "plugins"), nil
}

// PluginFetcher はkrkrsdl2向けのネイティブプラグイン(.so)をGitHubからダウンロードし、
// ローカルキャッシュに保存する。
type PluginFetcher struct {
	// CacheDir はプラグインの保存先ディレクトリ。空文字列の場合はPluginCacheDir()を使用する。
	CacheDir string
	// HTTPClient はHTTPリクエストに使用するクライアント。
	HTTPClient *http.Client
	// PluginConfigs はダウンロード対象のプラグイン設定一覧。
	// nilの場合はDefaultPluginConfigsを使用する。
	PluginConfigs []PluginConfig
}

// NewPluginFetcher はPluginFetcherを初期化する。
// httpClientがnilの場合はpluginDownloadTimeoutを持つ既定クライアントを使用する。
func NewPluginFetcher(cacheDir string, httpClient *http.Client) *PluginFetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: pluginDownloadTimeout}
	}

	return &PluginFetcher{CacheDir: cacheDir, HTTPClient: httpClient}
}

// httpClient はHTTPClientフィールドを返す。nilの場合（PluginFetcher{}のゼロ値を
// NewPluginFetcherを介さず直接構築した場合）はpluginDownloadTimeoutを持つクライアントを
// 都度生成して返し、nilポインタ参照によるpanicを避ける。
func (f *PluginFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}

	return &http.Client{Timeout: pluginDownloadTimeout}
}

func (f *PluginFetcher) cacheDir() (string, error) {
	if f.CacheDir != "" {
		return f.CacheDir, nil
	}

	return PluginCacheDir()
}

func (f *PluginFetcher) pluginConfigs() []PluginConfig {
	if f.PluginConfigs != nil {
		return f.PluginConfigs
	}

	return DefaultPluginConfigs
}

// GetPlugins は全プラグインを取得する。
// キャッシュに有効なプラグインが存在する場合はそれを返し、存在しない場合はダウンロードする。
func (f *PluginFetcher) GetPlugins() (PluginsInfo, error) {
	if f.IsAllCacheValid() {
		if info, ok := f.GetAllCachedPlugins(); ok {
			return info, nil
		}
	}

	return f.DownloadAllPlugins()
}

// GetPlugin は最初に設定されたプラグイン（既定ではextrans）を取得する。
func (f *PluginFetcher) GetPlugin() (PluginInfo, error) {
	configs := f.pluginConfigs()
	if len(configs) == 0 {
		return PluginInfo{}, fmt.Errorf("%w: プラグイン設定が空です", ErrPluginDownload)
	}

	plugins, err := f.GetPlugins()
	if err != nil {
		return PluginInfo{}, err
	}

	return plugins.Plugins[configs[0].Name], nil
}

// DownloadAllPlugins は全プラグインをダウンロードする。
// GitHubから各プラグインの各ABI用.soをダウンロードしてキャッシュに保存する。
func (f *PluginFetcher) DownloadAllPlugins() (PluginsInfo, error) {
	dir, err := f.cacheDir()
	if err != nil {
		return PluginsInfo{}, err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return PluginsInfo{}, fmt.Errorf("%w: キャッシュディレクトリの作成に失敗しました: %w", ErrPluginDownload, err)
	}

	plugins := make(map[string]PluginInfo, len(f.pluginConfigs()))

	for _, config := range f.pluginConfigs() {
		info, err := f.downloadSinglePlugin(dir, config)
		if err != nil {
			return PluginsInfo{}, err
		}
		plugins[config.Name] = info
	}

	return PluginsInfo{Plugins: plugins}, nil
}

// DownloadPlugin は最初に設定されたプラグイン（既定ではextrans）をダウンロードする。
func (f *PluginFetcher) DownloadPlugin() (PluginInfo, error) {
	configs := f.pluginConfigs()
	if len(configs) == 0 {
		return PluginInfo{}, fmt.Errorf("%w: プラグイン設定が空です", ErrPluginDownload)
	}

	plugins, err := f.DownloadAllPlugins()
	if err != nil {
		return PluginInfo{}, err
	}

	return plugins.Plugins[configs[0].Name], nil
}

func (f *PluginFetcher) downloadSinglePlugin(cacheDir string, config PluginConfig) (PluginInfo, error) {
	paths := make(map[string]string, len(SupportedABIs))

	for _, abi := range SupportedABIs {
		url := strings.ReplaceAll(config.URLTemplate, pluginABIPlaceholder, abi)
		context := config.Name + "/" + abi

		zipContent, err := f.downloadZip(url, context)
		if err != nil {
			return PluginInfo{}, err
		}

		soContent, err := extractSOFromZip(zipContent, config.SourceFilename, context)
		if err != nil {
			return PluginInfo{}, err
		}

		abiDir := filepath.Join(cacheDir, abi)
		if err := os.MkdirAll(abiDir, 0o750); err != nil {
			return PluginInfo{}, fmt.Errorf("%w: キャッシュディレクトリの作成に失敗しました (%s): %w", ErrPluginDownload, context, err)
		}

		pluginPath := filepath.Join(abiDir, config.OutputFilename)
		if err := os.WriteFile(pluginPath, soContent, 0o600); err != nil {
			return PluginInfo{}, fmt.Errorf("%w: プラグインファイルの保存に失敗しました (%s): %w", ErrPluginDownload, context, err)
		}

		paths[abi] = pluginPath
	}

	return PluginInfo{Name: config.Name, Paths: paths}, nil
}

func (f *PluginFetcher) downloadZip(url, context string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: リクエストの構築に失敗しました (%s): %w", ErrPluginDownload, context, err)
	}

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: ネットワークエラー (%s): %w", ErrPluginDownload, context, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTPエラー %d (%s)", ErrPluginDownload, resp.StatusCode, context)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: レスポンスの読み込みに失敗しました (%s): %w", ErrPluginDownload, context, err)
	}

	return content, nil
}

// extractSOFromZip はZIPファイルから対象ファイルを抽出する。
// nameがtargetFilenameで終わる最初のエントリを対象とする。
func extractSOFromZip(zipContent []byte, targetFilename, context string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipContent), int64(len(zipContent)))
	if err != nil {
		return nil, fmt.Errorf("%w: 無効なZIPファイルです (%s): %w", ErrPluginDownload, context, err)
	}

	for _, file := range zr.File {
		if !strings.HasSuffix(file.Name, targetFilename) {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: ZIP内エントリの読み込みに失敗しました (%s): %w", ErrPluginDownload, context, err)
		}
		defer func() { _ = rc.Close() }()

		content, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("%w: ZIP内エントリの読み込みに失敗しました (%s): %w", ErrPluginDownload, context, err)
		}

		return content, nil
	}

	return nil, fmt.Errorf("%w: ZIPファイル内に%sが見つかりません (%s)", ErrPluginDownload, targetFilename, context)
}

// IsAllCacheValid は全プラグインのキャッシュが有効かどうかを確認する。
// 全プラグインの全ABI用ファイルが存在する場合のみtrueを返す。
func (f *PluginFetcher) IsAllCacheValid() bool {
	dir, err := f.cacheDir()
	if err != nil {
		return false
	}

	for _, config := range f.pluginConfigs() {
		for _, abi := range SupportedABIs {
			if _, err := os.Stat(filepath.Join(dir, abi, config.OutputFilename)); err != nil {
				return false
			}
		}
	}

	return true
}

// IsCacheValid は最初に設定されたプラグインのキャッシュが有効かどうかを確認する。
func (f *PluginFetcher) IsCacheValid() bool {
	configs := f.pluginConfigs()
	if len(configs) == 0 {
		return false
	}

	dir, err := f.cacheDir()
	if err != nil {
		return false
	}

	config := configs[0]
	for _, abi := range SupportedABIs {
		if _, err := os.Stat(filepath.Join(dir, abi, config.OutputFilename)); err != nil {
			return false
		}
	}

	return true
}

// GetAllCachedPlugins は全キャッシュされたプラグインを取得する。
// キャッシュが無い場合はok=falseを返す。
func (f *PluginFetcher) GetAllCachedPlugins() (PluginsInfo, bool) {
	if !f.IsAllCacheValid() {
		return PluginsInfo{}, false
	}

	dir, err := f.cacheDir()
	if err != nil {
		return PluginsInfo{}, false
	}

	plugins := make(map[string]PluginInfo, len(f.pluginConfigs()))

	for _, config := range f.pluginConfigs() {
		paths := make(map[string]string, len(SupportedABIs))
		for _, abi := range SupportedABIs {
			paths[abi] = filepath.Join(dir, abi, config.OutputFilename)
		}
		plugins[config.Name] = PluginInfo{Name: config.Name, Paths: paths}
	}

	return PluginsInfo{Plugins: plugins}, true
}

// GetCachedPluginPaths は最初に設定されたプラグインのキャッシュパスを取得する。
// キャッシュが無い場合はok=falseを返す。
func (f *PluginFetcher) GetCachedPluginPaths() (map[string]string, bool) {
	if !f.IsCacheValid() {
		return nil, false
	}

	configs := f.pluginConfigs()
	if len(configs) == 0 {
		return nil, false
	}

	dir, err := f.cacheDir()
	if err != nil {
		return nil, false
	}

	config := configs[0]
	paths := make(map[string]string, len(SupportedABIs))
	for _, abi := range SupportedABIs {
		paths[abi] = filepath.Join(dir, abi, config.OutputFilename)
	}

	return paths, true
}
