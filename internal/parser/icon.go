package parser

import (
	"debug/pe"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
)

// センチネルエラー群。
var (
	// ErrNoIconsAvailable はEXE内にRT_GROUP_ICON/RT_ICONリソースが
	// 見つからない場合のセンチネルエラー。
	//
	// why not: アイコン無しとPE破損を一律に握りつぶすと、呼び出し元は「なぜ
	// アイコンが取得できなかったのか」を区別できない。センチネルエラーとして
	// 呼び出し元に伝播させ、errors.Isで原因を判別できるようにする（呼び出し
	// 元のpipelineがアイコン無しを「警告して続行」、PE破損を「ビルド失敗」
	// として扱い分けたい場合に必要になる）。
	ErrNoIconsAvailable = errors.New("EXEにアイコンが含まれていません")

	// ErrIconInvalidPEFile はexePathがPE形式として解析できない場合、または
	// リソースディレクトリ構造が不正な場合のセンチネルエラー。
	ErrIconInvalidPEFile = errors.New("PEファイルとして解析できません")
)

// IconExtractor はEXEファイルからアイコンを抽出するインターフェース。
type IconExtractor interface {
	// Extract はexePathからアイコンを抽出し、outputDir/icon.pngとして保存する。
	Extract(exePath, outputDir string) (string, error)
}

// ExeIconExtractor はWindows EXEのPEリソースからアイコンを抽出し、
// PNG形式で保存するIconExtractor実装。
//
// why not(stdlib debug/pe + 手動リソース解析を選んだ理由): Go版のLibrary
// SSOT(CLAUDE.md)にはPEリソースを走査しICOを再構成するライブラリが無い。
// 新規依存を増やす前に実装難度を検証する方針に従い、RT_GROUP_ICON/RT_ICONの
// 読み取りとICOのBITMAPINFOHEADER系DIBデコードをstdlib debug/pe + 本パッケージ
// のみで実装した(icon_resource.go/icon_ico.go/icon_dib.go)。
type ExeIconExtractor struct{}

// NewExeIconExtractor はExeIconExtractorを初期化する。
func NewExeIconExtractor() *ExeIconExtractor {
	return &ExeIconExtractor{}
}

// Extract はexePathの既定アイコングループから最大サイズのフレームを抽出し、
// outputDir/icon.pngとして保存する。
//
// exePathが存在しない場合はErrEXENotFound、PEとして解析できない・
// リソースディレクトリが不正な場合はErrIconInvalidPEFileをラップした
// エラー、アイコンリソースが見つからない場合はErrNoIconsAvailableを返す。
func (e *ExeIconExtractor) Extract(exePath, outputDir string) (string, error) {
	if _, err := os.Stat(exePath); err != nil {
		return "", fmt.Errorf("%w: %s", ErrEXENotFound, exePath)
	}

	peFile, err := pe.Open(exePath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}
	defer func() { _ = peFile.Close() }()

	img, err := extractBestIcon(peFile)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("出力ディレクトリの作成に失敗しました: %w", err)
	}

	outputPath := filepath.Join(outputDir, "icon.png")

	f, err := os.Create(outputPath) //nolint:gosec // outputDirとこのメソッド内で固定した"icon.png"のみで構築される出力パスのため妥当
	if err != nil {
		return "", fmt.Errorf("出力ファイルの作成に失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := png.Encode(f, img); err != nil {
		return "", fmt.Errorf("PNGエンコードに失敗しました: %w", err)
	}

	return outputPath, nil
}
