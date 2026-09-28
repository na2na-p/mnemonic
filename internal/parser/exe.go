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
	// EstimatedSize はOffsetからEXE終端までのバイト数。
	EstimatedSize int64
}

// EmbeddedXP3Extractor はEXEファイルから埋め込みXP3を抽出する。
//
// Windows EXE形式のゲームファイルには、XP3アーカイブが埋め込まれていることがある。
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

// FindEmbeddedXP3 はEXE内の埋め込みXP3を検索する。見つからなければfalseを返す。
//
// "MZ"で始まるファイルでは、オフセット16から16バイト刻みで最初にマジックが
// 現れた位置をアーカイブの先頭とし、EXE終端までをアーカイブとする。"MZ"で
// 始まらずマジックで始まるファイルはオフセット0のアーカイブとして扱う。
//
// why not: 1バイト刻みで探したり、2つ目以降のマジックを別のアーカイブとして
// 扱ったりしない。krkrz（base/XP3Archive.cpp のTVPGetXP3ArchiveOffset）は
// 自己完結EXEを開くとき上記の規則で1つだけ選び、索引とセグメントの位置を
// その先頭からの相対値として読み、終端はファイル終端のみで区切る。コード中の
// 偶然の一致やアーカイブ内のデータに現れるマジックで区切ると、エンジンが
// 読めるアーカイブを途中で切り詰めてしまう。
func (e *EmbeddedXP3Extractor) FindEmbeddedXP3() (EmbeddedXP3Info, bool, error) {
	info, _, found, err := e.locate()

	return info, found, err
}

func (e *EmbeddedXP3Extractor) locate() (EmbeddedXP3Info, []byte, bool, error) {
	content, err := os.ReadFile(e.exePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定EXEパスを読む用途のため妥当
	if err != nil {
		return EmbeddedXP3Info{}, nil, false, fmt.Errorf("EXEファイルの読み込みに失敗しました: %w", err)
	}

	offset, found := embeddedXP3Offset(content)
	if !found {
		return EmbeddedXP3Info{}, nil, false, nil
	}

	return EmbeddedXP3Info{Offset: offset, EstimatedSize: int64(len(content)) - offset}, content, true, nil
}

// exeXP3SearchStart と exeXP3SearchStep は、krkrzがEXE内のマジックを探す開始位置と
// 刻み幅（段落境界）。
const (
	exeXP3SearchStart = 16
	exeXP3SearchStep  = 16
)

func embeddedXP3Offset(content []byte) (int64, bool) {
	if bytes.HasPrefix(content, []byte("MZ")) {
		// why not: マジックの直後に1バイトも無い位置は採らない。krkrzの探索条件
		// （p + 11 < read）がマジックの末尾をファイルの最終バイトに置かないため。
		for off := exeXP3SearchStart; off+len(XP3Magic) < len(content); off += exeXP3SearchStep {
			if bytes.HasPrefix(content[off:], XP3Magic) {
				return int64(off), true
			}
		}

		return 0, false
	}
	if bytes.HasPrefix(content, XP3Magic) {
		return 0, true
	}

	return 0, false
}

// Extract は埋め込みXP3をoutputDirへ<EXEのファイル名のstem>_0.xp3として書き出し、
// そのパスを返す。埋め込みXP3が無ければ何も書き出さずにfalseを返す。
func (e *EmbeddedXP3Extractor) Extract(outputDir string) (string, bool, error) {
	info, content, found, err := e.locate()
	if err != nil || !found {
		return "", false, err
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", false, fmt.Errorf("出力ディレクトリの作成に失敗しました: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(e.exePath), filepath.Ext(e.exePath))
	outputFile := filepath.Join(outputDir, baseName+"_0.xp3")
	data := content[info.Offset : info.Offset+info.EstimatedSize]
	if err := os.WriteFile(outputFile, data, 0o600); err != nil { //nolint:gosec // outputFileはoutputDirとEXEファイル名由来の固定書式で構築され外部入力を含まない
		return "", false, fmt.Errorf("XP3ファイルの書き込みに失敗しました: %w", err)
	}

	return outputFile, true, nil
}
