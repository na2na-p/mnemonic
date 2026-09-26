// Package cache はキャッシュディレクトリ（テンプレート、SDL2ソース、デバッグ用署名鍵）の
// 解決と管理を提供する。
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// TemplateMetadataFilename はテンプレートキャッシュのメタデータファイル名。
const TemplateMetadataFilename = "metadata.json"

// TemplateMetadataTimeLayout はメタデータ内の日時フォーマット（UTC）。
const TemplateMetadataTimeLayout = "2006-01-02T15:04:05Z"

// TemplateMetadata はテンプレートキャッシュ1バージョンのメタデータ。
type TemplateMetadata struct {
	Version      string `json:"version"`
	DownloadedAt string `json:"downloaded_at"`
	ExpiresAt    string `json:"expires_at"`
}

// KeystoreDirName はデバッグ用署名鍵を置くキャッシュ配下のサブディレクトリ名。
const KeystoreDirName = "keystore"

// Info はキャッシュディレクトリの情報を表す。
//
// TemplateVersion / TemplateExpiresInDays はテンプレートキャッシュが存在しない場合の
// 未設定を表現するため、ポインタとする。
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
// Goのruntime.GOOSはコンパイル時定数でありモックできない。そのため実装本体を
// goos引数で受け取る形に分離し、Dir()はruntime.GOOSを渡す薄いラッパーとする
// ことでテスト容易性を確保する（CLAUDE.mdの「外部依存は注入可能にする」
// 方針に沿った設計）。
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
		// Linux/Darwin/Windows以外のOSではXDG_CACHE_HOMEを見ずhome/.cacheに固定する。
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

// ClearCacheDir はcacheDir配下のキャッシュを削除する。
//
// templateOnly=falseの場合はKeystoreDirNameという名前のエントリを除くcacheDir直下の
// 全エントリを削除し、cacheDir自体は残す。templateOnly=trueの場合はtemplatesのみ
// 削除する。cacheDirが存在しない場合もエラーにはならない。
func ClearCacheDir(cacheDir string, templateOnly bool) error {
	if templateOnly {
		if err := os.RemoveAll(filepath.Join(cacheDir, "templates")); err != nil {
			return fmt.Errorf("キャッシュの削除に失敗しました: %w", err)
		}

		return nil
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("キャッシュの削除に失敗しました: %w", err)
	}

	for _, entry := range entries {
		// why not: 署名鍵まで消すと次回ビルドの署名鍵が変わり、既存APKへの上書き
		// インストールができなくなる（詳細はpipeline.createDebugKeystore）。
		if entry.Name() == KeystoreDirName {
			continue
		}

		if err := os.RemoveAll(filepath.Join(cacheDir, entry.Name())); err != nil {
			return fmt.Errorf("キャッシュの削除に失敗しました: %w", err)
		}
	}

	return nil
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

	version, expiresInDays := latestTemplateInfo(filepath.Join(cacheDir, "templates"))

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

// ReadTemplateMetadata はversionDir直下のメタデータを読む。無い・壊れている場合はok=false。
func ReadTemplateMetadata(versionDir string) (TemplateMetadata, bool) {
	data, err := os.ReadFile(filepath.Join(versionDir, TemplateMetadataFilename)) //nolint:gosec // キャッシュディレクトリ配下の固定ファイル名を読む用途のため妥当
	if err != nil {
		return TemplateMetadata{}, false
	}

	var metadata TemplateMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return TemplateMetadata{}, false
	}

	return metadata, true
}

// latestTemplateInfo はtemplateDir直下のバージョンのうちdownloaded_atが最も新しいものを
// 最新テンプレートとみなし、そのバージョン名とexpires_atまでの残り日数（切り上げ、
// 下限0）を返す。有効なメタデータを持つバージョンが無い場合は両方nilを返す。
//
// why not: ディレクトリの更新日時はダウンロード日時と一致せず、有効期間も
// --template-refresh-daysで変わるため、更新日時と固定日数からは求めない。
// ビルド自身が書いたメタデータを読むことで、ビルドのキャッシュ判定と一致させる。
// expires_atを解釈できない場合は、ビルドが無効なキャッシュとみなすのに合わせて0日とする。
func latestTemplateInfo(templateDir string) (*string, *int) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return nil, nil
	}

	var (
		latest       TemplateMetadata
		latestName   string
		latestLoaded time.Time
		found        bool
	)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadata, ok := ReadTemplateMetadata(filepath.Join(templateDir, entry.Name()))
		if !ok {
			continue
		}

		downloadedAt, err := time.Parse(TemplateMetadataTimeLayout, metadata.DownloadedAt)
		if err != nil {
			continue
		}

		if !found || downloadedAt.After(latestLoaded) {
			latest = metadata
			latestName = entry.Name()
			latestLoaded = downloadedAt
			found = true
		}
	}

	if !found {
		return nil, nil
	}

	return new(latestName), new(daysUntil(latest.ExpiresAt))
}

func daysUntil(expiresAt string) int {
	expires, err := time.Parse(TemplateMetadataTimeLayout, expiresAt)
	if err != nil {
		return 0
	}

	return max(0, int(math.Ceil(time.Until(expires).Hours()/24)))
}
