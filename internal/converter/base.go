// Package converter はアセット変換（文字コード・スクリプト調整・画像・動画）を提供する。
package converter

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// センチネルエラー群。
var (
	// ErrSourceNotFound は変換元ファイルが存在しない場合のエラー。
	ErrSourceNotFound = errors.New("変換元ファイルが見つかりません")
	// ErrSourceUnreadable は存在しない以外の理由で変換元ファイルを読み込めない場合のエラー。
	// OSのエラーを%wで保持するため、呼び出し側はerrors.Isで原因を判別できる。
	ErrSourceUnreadable = errors.New("変換元ファイルを読み込めません")
	// ErrSourceIsDirectory は変換元がディレクトリの場合のエラー。
	ErrSourceIsDirectory = errors.New("変換元はファイルである必要があります")
	// ErrPermanentFailure は同じ入力を再試行しても解消しない変換失敗を表す。
	// Converterはこれを%wでラップしたerrを返し、呼び出し側にリトライ不要を伝える。
	ErrPermanentFailure = errors.New("再試行しても解消しない変換失敗です")
	// ErrDestinationCollision は複数の変換元が同じ出力先へ変換される場合のエラー。
	// 出力先と同じ拡張子の変換元がちょうど1件でない場合、ConversionManagerは
	// これをErrPermanentFailureでラップし、該当する変換元をいずれも変換しない。
	ErrDestinationCollision = errors.New("出力先が重複しています")
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
// Converter.Convertがerr=nilで返す結果のStatusはStatusSuccessかStatusSkippedに
// 限られる。StatusFailedはConversionManagerが組み立てる失敗の要約にだけ現れ、
// DestPathは空文字列となる。そのMessageはConvertのerrの文言、出力先が重複した
// 場合のErrDestinationCollisionを含むエラーの文言、または
// RetryConfig.MaxAttemptsが0以下で一度も変換を試みなかった場合の
// 「変換に失敗しました」となる。
//
// 出力先が重複し別の変換元を優先したためConversionManagerが変換しなかった
// 変換元は、DestPathに優先した変換元の出力先を持つStatusSkippedとなる。
//
// ConversionManager.ConvertDirectoryの結果では、Messageの文頭か空白の直後に
// 置かれた変換元・出力先のパスは、それぞれのルートからの相対パスとなる。
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
// os.Statの失敗はclassifyStatErrorで分類して返す。変換元がディレクトリの場合は
// ErrSourceIsDirectoryをErrPermanentFailureでラップして返す。
func validateSource(source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return classifyStatError(source, err)
	}
	if info.IsDir() {
		return permanentError(fmt.Errorf("%w: %s", ErrSourceIsDirectory, source))
	}

	return nil
}

// ensureSourceExists はsourceをos.Statで確認できない場合、その失敗を
// classifyStatErrorで分類したエラーを返す。
//
// why not: validateSourceは使わない。validateSourceはディレクトリも拒否するが、
// Encoding/Script/Video/Midiの各Converterはディレクトリをこの時点では弾かず、
// 後続の読み込みや外部コマンドの失敗（再試行対象）として扱う。
func ensureSourceExists(source string) error {
	if _, err := os.Stat(source); err != nil {
		return classifyStatError(source, err)
	}

	return nil
}

// classifyStatError はsourceに対するos.Statの失敗errを、変換元のセンチネル
// (ErrSourceNotFound/ErrSourceUnreadable)でclassifyPathErrorに従って分類する。
func classifyStatError(source string, err error) error {
	return classifyPathError(source, err, ErrSourceNotFound, ErrSourceUnreadable)
}

// classifyPathError はpathに対するos.Statの失敗errを分類する。
//
// 存在しない場合はnotFoundをpathとともに、権限不足の場合はOSのエラーを包んだ
// unreadableを、いずれもErrPermanentFailureでラップして返す。
// それ以外（親がファイル・パス名が長すぎる・I/Oエラーなど）はOSのエラーを包んだ
// unreadableを再試行対象として返す。
//
// why not: Statの失敗を一律にnotFoundにはしない。権限不足まで
// 「見つかりません」と報告すると利用者が原因を探せず、EIOなどの一時的な失敗まで
// 恒久扱いになって再試行されなくなる。
func classifyPathError(path string, err, notFound, unreadable error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return permanentError(fmt.Errorf("%w: %s", notFound, path))
	case errors.Is(err, fs.ErrPermission):
		return permanentError(fmt.Errorf("%w: %w", unreadable, err))
	default:
		return fmt.Errorf("%w: %w", unreadable, err)
	}
}

// permanentError はerrをErrPermanentFailureでラップし、呼び出し側がerrors.Isで
// 再試行不要と判定できるようにする。
func permanentError(err error) error {
	return fmt.Errorf("%w: %w", ErrPermanentFailure, err)
}

// getFileSize はpathのファイルサイズを返す。ファイルが存在しない場合は0を返す。
func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.Size()
}
