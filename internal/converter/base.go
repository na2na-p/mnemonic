// Package converter はアセット変換（文字コード・スクリプト調整・画像・動画）を提供する。
package converter

import (
	"errors"
	"fmt"
	"os"
)

// センチネルエラー群。
var (
	// ErrSourceNotFound は変換元ファイルが存在しない場合のエラー。
	ErrSourceNotFound = errors.New("変換元ファイルが見つかりません")
	// ErrSourceIsDirectory は変換元がディレクトリの場合のエラー。
	ErrSourceIsDirectory = errors.New("変換元はファイルである必要があります")
)

// ConversionStatus は変換ステータスを表す。
type ConversionStatus string

// ConversionStatusの各値。
const (
	StatusSuccess ConversionStatus = "success"
	StatusSkipped ConversionStatus = "skipped"
	StatusFailed  ConversionStatus = "failed"
)

// ConversionResult は単一ファイルの変換結果を表す不変値。
//
// DestPathが空文字列の場合は変換失敗・スキップ時を表す。
type ConversionResult struct {
	SourcePath  string
	DestPath    string
	Status      ConversionStatus
	Message     string
	BytesBefore int64
	BytesAfter  int64
}

// CompressionRatio は圧縮率（BytesAfter / BytesBefore）を計算する。
// BytesBeforeが0の場合は1.0を返す。
func (r ConversionResult) CompressionRatio() float64 {
	if r.BytesBefore == 0 {
		return 1.0
	}

	return float64(r.BytesAfter) / float64(r.BytesBefore)
}

// BytesSaved は節約されたバイト数を返す（負の場合はサイズ増加）。
func (r ConversionResult) BytesSaved() int64 {
	return r.BytesBefore - r.BytesAfter
}

// IsSuccess は変換が成功したかどうかを返す。
func (r ConversionResult) IsSuccess() bool {
	return r.Status == StatusSuccess
}

// validateSource は変換元ファイルの検証を行う。
//
// ファイルが存在しない場合はErrSourceNotFound、ディレクトリの場合は
// ErrSourceIsDirectoryを返す。
//
// why not: EncodingConverter/ScriptAdjuster/VideoConverterは自前の
// 存在チェックでConversionResult{Status: StatusFailed}を返す設計のため、
// ここでのerror伝播は使わない。ImageConverterのみこの関数を使い、errorを
// 呼び出し元へ伝播させる。
func validateSource(source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSourceNotFound, source)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: %s", ErrSourceIsDirectory, source)
	}

	return nil
}

// getFileSize はpathのファイルサイズを返す。ファイルが存在しない場合は0を返す。
func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.Size()
}
