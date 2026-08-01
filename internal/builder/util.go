package builder

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyFile はsrcの内容をdstへコピーする。
// 既存のdstは上書きされる。
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // 呼び出し元で存在検証済みのファイルを読む用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // 呼び出し元がキャッシュ/プロジェクトディレクトリ配下に限定して呼び出す用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // テンプレート/アセットファイルのコピーでありサイズ上限は設けない
		return err
	}

	return nil
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
