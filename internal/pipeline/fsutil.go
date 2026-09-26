package pipeline

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/na2na-p/mnemonic/internal/safepath"
)

// fileExists はpathが存在するファイル/ディレクトリかどうかを返す。
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// copyFile はsrcの内容をdstへコピーする。srcとdstが同一ファイル実体（同一パス、
// ハードリンク、大文字小文字を区別しないファイルシステム上の別表記など）を指す
// 場合は何もせずnilを返す。
//
// why not: パス文字列の比較（Abs/EvalSymlinks）ではハードリンクや大文字小文字を
// 区別しないファイルシステムを検出できない。os.SameFileはデバイスとinodeで比べる
// ため、これらも同一と判定できる。
//
// why not: 同一ファイルをエラーではなくnilで返す。copyTreeなどの呼び出し元は
// ハードリンクの有無を知り得ず、望む終状態（dstがsrcの内容を持つ）は既に
// 成立している。
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // 呼び出し元で存在検証済みのパイプライン内部生成パスを読む用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	sameFile, err := isSameFile(info, dst)
	if err != nil {
		return err
	}
	if sameFile {
		return nil
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // 一時ディレクトリ配下に限定して呼び出す用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // ビルド成果物のコピーでありサイズ上限は設けない
		return err
	}

	return nil
}

// isSameFile はsrcInfoのファイルとdstが同じファイル実体を指すかを返す。dstが存在
// しない場合は同一になり得ないためfalseを返す。
func isSameFile(srcInfo os.FileInfo, dst string) (bool, error) {
	dstInfo, err := os.Stat(dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return os.SameFile(srcInfo, dstInfo), nil
}

// copyTree はsrc配下のファイル・ディレクトリ構造を丸ごとdstへコピーする。
// dstに同名のファイル・ディレクトリが既に存在する場合は上書きする。
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}

		destPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return err
		}

		return copyFile(path, destPath)
	})
}

// withSuffix はpathの拡張子をsuffixへ置き換えたパスを返す。
func withSuffix(path, suffix string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))

	return base + suffix
}

// extractTemplateZip はtemplatePathのZIPファイルをdestDirへ展開する。
func extractTemplateZip(templatePath, destDir string) error {
	if !fileExists(templatePath) {
		return fmt.Errorf("テンプレートが見つかりません: %s", templatePath)
	}

	zr, err := zip.OpenReader(templatePath)
	if err != nil {
		return fmt.Errorf("無効なテンプレートファイルです: %s: %w", templatePath, err)
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if err := extractZipEntry(f, destDir); err != nil {
			return fmt.Errorf("テンプレートの展開に失敗しました: %w", err)
		}
	}

	return nil
}

// extractZipEntry はzip.FileをdestDir配下へ展開する。
//
// why not: エントリ名はテンプレートZIP内データに由来し外部入力として
// 信頼できないため、展開先がdestDir外に脱出しないことを検証する
// （zip slip対策。詳細はsafepath.Joinのwhy not参照）。
func extractZipEntry(f *zip.File, destDir string) error {
	destPath, err := safepath.Join(destDir, f.Name)
	if err != nil {
		return err
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(destPath, 0o750)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}

	src, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // destPathはsafepath.JoinでdestDir配下に限定済み
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil { //nolint:gosec // テンプレートZIPは信頼済みのビルド成果物であり展開サイズは無制限で許容する
		return err
	}

	return nil
}
