package builder

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/na2na-p/mnemonic/internal/fsutil"
)

func copyFile(src, dst string) error {
	return fsutil.CopyFile(src, dst)
}

// copyDir はsrcディレクトリの内容を再帰的にdstへコピーする。
// dstが既に存在する場合、同名ファイルは上書きされる。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		return copyFile(path, target)
	})
}
