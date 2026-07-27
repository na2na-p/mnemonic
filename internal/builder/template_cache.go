package builder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/cache"
)

// ErrTemplateCache はテンプレートキャッシュ操作に関する基本エラー。
var ErrTemplateCache = errors.New("テンプレートキャッシュの操作に失敗しました")

// metadataTimeLayout はメタデータJSON内の日時フォーマット
// （常にUTC、末尾Z表記）。
const metadataTimeLayout = "2006-01-02T15:04:05Z"

// metadataFilename はメタデータファイル名。
const metadataFilename = "metadata.json"

// defaultRefreshDays はキャッシュのデフォルト有効期間（日）。
const defaultRefreshDays = 7

// CacheManager はテンプレートキャッシュディレクトリ管理を抽象化する。
//
// why not: internal/cacheパッケージの関数を直接呼ぶと、TemplateCacheの
// 単体テストが実ファイルシステムのキャッシュディレクトリ（$HOME配下）に
// 依存してしまう。Python版もCacheManager Protocolで抽象化しテストでは
// MagicMockに差し替えていたため、Go版でも同様に利用側（本パッケージ）で
// インターフェースを定義しgomockでモックする。
type CacheManager interface {
	// GetCacheDir はキャッシュのルートディレクトリを返す。
	GetCacheDir() (string, error)
	// GetTemplateCachePath はversionに対応するキャッシュパスを返す。
	GetTemplateCachePath(version string) (string, error)
	// ClearCache はキャッシュを削除する。templateOnly=trueの場合はテンプレートのみ削除する。
	ClearCache(templateOnly bool) error
}

// defaultCacheManager はinternal/cacheパッケージを利用するCacheManagerの既定実装。
type defaultCacheManager struct{}

// NewDefaultCacheManager はinternal/cacheパッケージに委譲するCacheManagerを返す。
func NewDefaultCacheManager() CacheManager {
	return defaultCacheManager{}
}

func (defaultCacheManager) GetCacheDir() (string, error) {
	return cache.Dir()
}

func (defaultCacheManager) GetTemplateCachePath(version string) (string, error) {
	return cache.TemplateCachePath(version)
}

func (defaultCacheManager) ClearCache(templateOnly bool) error {
	return cache.ClearCache(templateOnly)
}

// templateMetadata はテンプレートキャッシュのメタデータ。
type templateMetadata struct {
	Version      string `json:"version"`
	DownloadedAt string `json:"downloaded_at"`
	ExpiresAt    string `json:"expires_at"`
}

// TemplateCache はCacheManagerを利用してkrkrsdl2テンプレートのキャッシュを管理する。
type TemplateCache struct {
	cacheManager CacheManager
	refreshDays  int
}

// NewTemplateCache はTemplateCacheを初期化する。
// refreshDaysが0以下の場合はdefaultRefreshDays（7日）を使用する。
func NewTemplateCache(cacheManager CacheManager, refreshDays int) *TemplateCache {
	if refreshDays <= 0 {
		refreshDays = defaultRefreshDays
	}

	return &TemplateCache{cacheManager: cacheManager, refreshDays: refreshDays}
}

func (c *TemplateCache) metadataPath(version string) (string, error) {
	cachePath, err := c.cacheManager.GetTemplateCachePath(version)
	if err != nil {
		return "", err
	}

	return filepath.Join(cachePath, metadataFilename), nil
}

// readMetadata はversionのメタデータファイルを読み込む。
// ファイルが存在しない、またはパース不能な場合はok=falseを返す
// （Python版がOSError/JSONDecodeErrorを捕捉してNoneを返すのと同じ扱い）。
func (c *TemplateCache) readMetadata(version string) (templateMetadata, bool) {
	path, err := c.metadataPath(version)
	if err != nil {
		return templateMetadata{}, false
	}

	data, err := os.ReadFile(path) //nolint:gosec // キャッシュディレクトリ配下の固定ファイル名を読む用途のため妥当
	if err != nil {
		return templateMetadata{}, false
	}

	var metadata templateMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return templateMetadata{}, false
	}

	return metadata, true
}

func (c *TemplateCache) writeMetadata(version string, metadata templateMetadata) error {
	path, err := c.metadataPath(version)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600) //nolint:gosec // キャッシュディレクトリ配下の固定ファイル名へ書き込む用途のため妥当
}

// findTemplateFile はcachePath直下から拡張子".zip"の最初のファイルを探す。
func findTemplateFile(cachePath string) (string, bool) {
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			return filepath.Join(cachePath, entry.Name()), true
		}
	}

	return "", false
}

// getAllCachedVersions はメタデータが存在するキャッシュ済みバージョン名の一覧を返す。
func (c *TemplateCache) getAllCachedVersions() []string {
	cacheDir, err := c.cacheManager.GetCacheDir()
	if err != nil {
		return nil
	}

	templatesDir := filepath.Join(cacheDir, "templates")

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil
	}

	var versions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := c.readMetadata(entry.Name()); ok {
			versions = append(versions, entry.Name())
		}
	}

	return versions
}

// GetCachedTemplate はキャッシュ済みテンプレートのパスを取得する。
// versionがnilの場合は最新のキャッシュされたバージョンを取得する。
// キャッシュが存在しない、または無効な場合はok=falseを返す。
func (c *TemplateCache) GetCachedTemplate(version *string) (string, bool) {
	targetVersion := ""
	if version != nil {
		targetVersion = *version
	} else {
		latest, ok := c.GetCachedVersion()
		if !ok {
			return "", false
		}
		targetVersion = latest
	}

	if !c.IsCacheValid(&targetVersion) {
		return "", false
	}

	cachePath, err := c.cacheManager.GetTemplateCachePath(targetVersion)
	if err != nil {
		return "", false
	}

	return findTemplateFile(cachePath)
}

// IsCacheValid はキャッシュが有効かどうかを確認する。
// versionがnilの場合は最新のキャッシュされたバージョンを確認する。
func (c *TemplateCache) IsCacheValid(version *string) bool {
	targetVersion := ""
	if version != nil {
		targetVersion = *version
	} else {
		latest, ok := c.GetCachedVersion()
		if !ok {
			return false
		}
		targetVersion = latest
	}

	metadata, ok := c.readMetadata(targetVersion)
	if !ok || metadata.ExpiresAt == "" {
		return false
	}

	expiresAt, err := time.Parse(metadataTimeLayout, metadata.ExpiresAt)
	if err != nil {
		return false
	}

	return time.Now().UTC().Before(expiresAt)
}

// GetCachedVersion はキャッシュされている最新バージョンを取得する
// （downloaded_atが最も新しいもの）。キャッシュが存在しない場合はok=falseを返す。
func (c *TemplateCache) GetCachedVersion() (string, bool) {
	versions := c.getAllCachedVersions()
	if len(versions) == 0 {
		return "", false
	}

	var (
		latestVersion string
		latestTime    time.Time
		found         bool
	)

	for _, version := range versions {
		metadata, ok := c.readMetadata(version)
		if !ok || metadata.DownloadedAt == "" {
			continue
		}

		downloadedAt, err := time.Parse(metadataTimeLayout, metadata.DownloadedAt)
		if err != nil {
			continue
		}

		if !found || downloadedAt.After(latestTime) {
			latestTime = downloadedAt
			latestVersion = version
			found = true
		}
	}

	return latestVersion, found
}

// SaveTemplate はテンプレートをキャッシュに保存する。
// templatePathが存在しない場合はos.ErrNotExistを満たすerrorを返す。
func (c *TemplateCache) SaveTemplate(templatePath, version string) (string, error) {
	if _, err := os.Stat(templatePath); err != nil {
		return "", err
	}

	cachePath, err := c.cacheManager.GetTemplateCachePath(version)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTemplateCache, err)
	}

	if err := os.MkdirAll(cachePath, 0o750); err != nil {
		return "", fmt.Errorf("%w: テンプレートの保存に失敗しました: %w", ErrTemplateCache, err)
	}

	destination := filepath.Join(cachePath, filepath.Base(templatePath))
	if err := copyFile(templatePath, destination); err != nil {
		return "", fmt.Errorf("%w: テンプレートの保存に失敗しました: %w", ErrTemplateCache, err)
	}

	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, c.refreshDays)

	metadata := templateMetadata{
		Version:      version,
		DownloadedAt: now.Format(metadataTimeLayout),
		ExpiresAt:    expiresAt.Format(metadataTimeLayout),
	}

	if err := c.writeMetadata(version, metadata); err != nil {
		return "", fmt.Errorf("%w: テンプレートの保存に失敗しました: %w", ErrTemplateCache, err)
	}

	return destination, nil
}

// ClearCache はテンプレートキャッシュをクリアする。
func (c *TemplateCache) ClearCache() error {
	if err := c.cacheManager.ClearCache(true); err != nil {
		return fmt.Errorf("%w: キャッシュのクリアに失敗しました: %w", ErrTemplateCache, err)
	}

	return nil
}
