// Package fsutil はパッケージ横断で使うファイル操作の共通処理を提供する。
//
// エラーはOSのエラーをそのまま返す。文脈を付けたラップは呼び出し元が行う。
package fsutil

import (
	"errors"
	"io"
	"os"
)

// CopyFile はsrcの内容をdstへコピーし、dstを新規作成する場合はsrcのパーミッションを
// 引き継ぐ。既存のdstは切り詰めて上書きする。srcとdstが同一ファイル実体（同一パス、
// ハードリンク、大文字小文字を区別しないファイルシステム上の別表記など）を指す
// 場合は何もせずnilを返す。
//
// why not: 同一ファイルをエラーではなくnilで返す。ディレクトリを再帰コピーする
// 呼び出し元などはハードリンクの有無を知り得ず、望む終状態（dstがsrcの内容を持つ）
// は既に成立している。
func CopyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // srcの妥当性は呼び出し元が検証する契約のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	same, err := SameFile(info, dst)
	if err != nil {
		return err
	}
	if same {
		return nil
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // dstの妥当性は呼び出し元が検証する契約のため妥当
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // 呼び出し元が指定したファイルの複製でありサイズ上限は設けない
		_ = out.Close()

		return err
	}

	return out.Close()
}

// SameFile はsrcInfoのファイルとdstが同じファイル実体を指すかを返す。dstが存在
// しない場合は同一になり得ないためfalseを返す。
//
// why not: パス文字列の正規化（Abs + EvalSymlinks）で比べると、ハードリンクや
// 大文字小文字を区別しないファイルシステム上の表記違いを別ファイルと誤判定し、
// コピーで内容を消してしまう。os.SameFileはデバイスとinodeで比べるため、
// これらも同一と判定できる。
func SameFile(srcInfo os.FileInfo, dst string) (bool, error) {
	dstInfo, err := os.Stat(dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return os.SameFile(srcInfo, dstInfo), nil
}
