// Package cache はテンプレートキャッシュディレクトリの解決と管理を提供する。
//
// Python版 (src/mnemonic/cache.py) をGoへ移植したもの。
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// DefaultMaxAgeDays はキャッシュ有効期限のデフォルト日数。
const DefaultMaxAgeDays = 7

// Info はキャッシュディレクトリの情報を表す。
//
// TemplateVersion / TemplateExpiresInDays はテンプレートキャッシュが存在しない場合
// Pythonの `None` に相当するためポインタで未設定を表現する。
type Info struct {
	Directory             string
	SizeBytes             int64
	TemplateVersion       *string
	TemplateExpiresInDays *int
}

// Dir は実行環境のOSに応じたキャッシュディレクトリを返す。
func Dir() (string, error) {
	return DirForOS(runtime.GOOS)
}

// DirForOS はgoos（runtime.GOOSと同じ値域）に応じたキャッシュディレクトリを返す。
//
// Python版はplatform.systemをテストでモックしていたが、Goのruntime.GOOSは
// コンパイル時定数でありモックできない。そのため実装本体をgoos引数で受け取る形に
// 分離し、Dir()はruntime.GOOSを渡す薄いラッパーとすることでテスト容易性を確保する
// （CLAUDE.mdの「外部依存は注入可能にする」方針に沿った設計）。
func DirForOS(goos string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリを取得できません: %w", err)
	}

	switch goos {
	case "linux":
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			return filepath.Join(xdg, "mnemonic"), nil
		}

		return filepath.Join(home, ".cache", "mnemonic"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "mnemonic"), nil
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "mnemonic", "cache"), nil
		}

		return filepath.Join(home, "AppData", "Local", "mnemonic", "cache"), nil
	default:
		// Python版のelse節（Linux/Darwin/Windows以外）はXDG_CACHE_HOMEを見ずhome/.cacheに
		// 固定していたため、その挙動をそのまま踏襲する。
		return filepath.Join(home, ".cache", "mnemonic"), nil
	}
}

// TemplateCachePath はversionに対応するテンプレートキャッシュパスを返す。
func TemplateCachePath(version string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "templates", version), nil
}

// IsValid はpathのキャッシュがmaxAgeDays以内に更新されているかを判定する。
func IsValid(path string, maxAgeDays int) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	age := time.Since(info.ModTime())

	return age < time.Duration(maxAgeDays)*24*time.Hour
}

// ClearCache は実行環境のキャッシュディレクトリを削除する。
func ClearCache(templateOnly bool) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	return ClearCacheDir(dir, templateOnly)
}

// ClearCacheDir はcacheDir配下を削除する。templateOnly=trueの場合はtemplatesのみ削除する。
// cacheDirが存在しない場合もエラーにはならない（os.RemoveAllの仕様に準拠）。
func ClearCacheDir(cacheDir string, templateOnly bool) error {
	target := cacheDir
	if templateOnly {
		target = filepath.Join(cacheDir, "templates")
	}

	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("キャッシュの削除に失敗しました: %w", err)
	}

	return nil
}

// GetCacheInfo は実行環境のキャッシュディレクトリの情報を取得する。
func GetCacheInfo() (Info, error) {
	dir, err := Dir()
	if err != nil {
		return Info{}, err
	}

	return InfoForDir(dir)
}

// InfoForDir はcacheDirのキャッシュ情報を取得する。
func InfoForDir(cacheDir string) (Info, error) {
	if _, err := os.Stat(cacheDir); err != nil {
		return Info{Directory: cacheDir}, nil
	}

	totalSize, err := dirSize(cacheDir)
	if err != nil {
		return Info{}, fmt.Errorf("キャッシュサイズの計算に失敗しました: %w", err)
	}

	version, expiresInDays, err := latestTemplateInfo(filepath.Join(cacheDir, "templates"))
	if err != nil {
		return Info{}, err
	}

	return Info{
		Directory:             cacheDir,
		SizeBytes:             totalSize,
		TemplateVersion:       version,
		TemplateExpiresInDays: expiresInDays,
	}, nil
}

func dirSize(root string) (int64, error) {
	var total int64

	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()

		return nil
	})
	if err != nil {
		return 0, err
	}

	return total, nil
}

// latestTemplateInfo はtemplateDir直下で最終更新日時が最も新しいエントリを
// 最新テンプレートとみなし、そのバージョン名と残り有効日数を返す。
func latestTemplateInfo(templateDir string) (*string, *int, error) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return nil, nil, nil //nolint:nilerr // templatesディレクトリ自体が無い場合は「テンプレート未取得」として扱う
	}
	if len(entries) == 0 {
		return nil, nil, nil
	}

	var (
		latestName string
		latestMod  time.Time
	)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, nil, fmt.Errorf("テンプレート情報の取得に失敗しました: %w", err)
		}
		if latestName == "" || info.ModTime().After(latestMod) {
			latestName = entry.Name()
			latestMod = info.ModTime()
		}
	}

	expires := DefaultMaxAgeDays - int(time.Since(latestMod).Hours()/24)
	if expires < 0 {
		expires = 0
	}

	return &latestName, &expires, nil
}
