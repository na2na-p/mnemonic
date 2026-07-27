// Package parser はゲーム入力ファイル（EXE / XP3アーカイブ）の解析、
// ゲームエンジン構成の検出、アセットファイルの分類を行う。
package parser

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// XP3Magic はXP3アーカイブの完全なマジックバイト列（11バイト）。
var XP3Magic = []byte{'X', 'P', '3', 0x0d, 0x0a, 0x20, 0x0a, 0x1a, 0x8b, 0x67, 0x01}

// ErrEXENotFound はEXEファイルが存在しない場合のセンチネルエラー。
var ErrEXENotFound = errors.New("EXEファイルが見つかりません")

// EmbeddedXP3Info はEXE内埋め込みXP3の情報を表す。
type EmbeddedXP3Info struct {
	// Offset はEXE内でのXP3開始オフセット。
	Offset int64
	// EstimatedSize は推定サイズ（次のXP3の開始位置またはEXE終端までのバイト数）。
	EstimatedSize int64
}

// EmbeddedXP3Extractor はEXEファイルから埋め込みXP3を抽出する。
//
// Windows EXE形式のゲームファイルには、XP3アーカイブが埋め込まれていることがある。
// このクラスは、EXEファイル内のXP3オフセットを検出し、抽出する機能を提供する。
type EmbeddedXP3Extractor struct {
	exePath string
}

// NewEmbeddedXP3Extractor はexePathを対象に初期化する。
//
// exePathが存在しない場合はErrEXENotFoundを返す。
func NewEmbeddedXP3Extractor(exePath string) (*EmbeddedXP3Extractor, error) {
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrEXENotFound, exePath)
	}

	return &EmbeddedXP3Extractor{exePath: exePath}, nil
}

// ExePath は対象EXEファイルのパスを返す。
func (e *EmbeddedXP3Extractor) ExePath() string {
	return e.exePath
}

// FindEmbeddedXP3 はEXE内の埋め込みXP3を検索する。
//
// EXEファイルをスキャンし、XP3マジックバイトを検出する。
// 複数のXP3が埋め込まれている場合、すべてをオフセット順に返す。
func (e *EmbeddedXP3Extractor) FindEmbeddedXP3() ([]EmbeddedXP3Info, error) {
	content, err := os.ReadFile(e.exePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定EXEパスを読む用途のため妥当
	if err != nil {
		return nil, fmt.Errorf("EXEファイルの読み込みに失敗しました: %w", err)
	}

	fileSize := int64(len(content))

	// XP3マジックバイトをスキャンしてオフセットを記録する。
	// posをoffset+1から再開することで、Python版と同様に1バイトずつ
	// 前進しながら重複を許容した探索を行う（bytes.Indexは重複開始位置を
	// 見逃さないため、offset+len(magic)からの再開よりも安全）。
	var offsets []int64
	pos := 0
	for {
		idx := bytes.Index(content[pos:], XP3Magic)
		if idx == -1 {
			break
		}
		offset := pos + idx
		offsets = append(offsets, int64(offset))
		pos = offset + 1
	}

	result := make([]EmbeddedXP3Info, 0, len(offsets))
	for i, offset := range offsets {
		var estimatedSize int64
		if i+1 < len(offsets) {
			estimatedSize = offsets[i+1] - offset
		} else {
			estimatedSize = fileSize - offset
		}
		result = append(result, EmbeddedXP3Info{Offset: offset, EstimatedSize: estimatedSize})
	}

	return result, nil
}

// ExtractAll は検出したすべてのXP3をoutputDirへ抽出する。
//
// 抽出されたファイルはexeファイル名のstemに連番を付与した名前
// （例: game_0.xp3）で保存される。
func (e *EmbeddedXP3Extractor) ExtractAll(outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, fmt.Errorf("出力ディレクトリの作成に失敗しました: %w", err)
	}

	xp3List, err := e.FindEmbeddedXP3()
	if err != nil {
		return nil, err
	}
	if len(xp3List) == 0 {
		return []string{}, nil
	}

	content, err := os.ReadFile(e.exePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定EXEパスを読む用途のため妥当
	if err != nil {
		return nil, fmt.Errorf("EXEファイルの読み込みに失敗しました: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(e.exePath), filepath.Ext(e.exePath))

	extracted := make([]string, 0, len(xp3List))
	for i, info := range xp3List {
		data := content[info.Offset : info.Offset+info.EstimatedSize]
		outputFile := filepath.Join(outputDir, fmt.Sprintf("%s_%d.xp3", baseName, i))
		if err := os.WriteFile(outputFile, data, 0o600); err != nil { //nolint:gosec // outputFileはoutputDirとEXEファイル名由来の固定書式で構築され外部入力を含まない
			return nil, fmt.Errorf("XP3ファイルの書き込みに失敗しました: %w", err)
		}
		extracted = append(extracted, outputFile)
	}

	return extracted, nil
}
