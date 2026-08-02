package builder

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// assetCopier はゲームファイルをapp/src/main/assets/dataへコピーする。
type assetCopier struct {
	projectDir string
}

// newAssetCopier はassetCopierを初期化する。
func newAssetCopier(projectDir string) *assetCopier {
	return &assetCopier{projectDir: projectDir}
}

// Copy はゲームファイルをapp/src/main/assets/dataにコピーする（既存ファイルはマージ）。
func (c *assetCopier) Copy(assetsDir string) error {
	destDir := filepath.Join(c.projectDir, "app", "src", "main", "assets", "data")
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	err := filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return err
		}

		return copyFile(path, destPath)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}
