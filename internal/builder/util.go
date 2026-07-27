package builder

import (
	"io"
	"os"
)

// copyFile はsrcの内容をdstへコピーする（Pythonのshutil.copy2相当）。
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
