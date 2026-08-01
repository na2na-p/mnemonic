package signer

import "errors"

// センチネルエラー群。
//
// errors.Is比較を前提にfmt.Errorf("...: %w", err)でラップする設計
// （lang-go.md Criterion G3）のため、失敗理由ごとにセンチネルを分ける。
// ファイル未検出はalign/isAligned・sign/verifyそれぞれで対象パスが異なるだけで
// 失敗理由は同一（「操作対象のファイルが存在しない」）のため、メソッドごとに
// メッセージを分けず一つのセンチネルに統合する。
var (
	// ErrZipalignFileNotFound はzipalignの操作対象ファイル
	// （align時は入力APK、isAligned時は確認対象APK）が存在しない場合のエラー。
	ErrZipalignFileNotFound = errors.New("zipalignの操作対象ファイルが見つかりません")
	// ErrZipalignNotFound はzipalignコマンドが見つからない場合のエラー。
	ErrZipalignNotFound = errors.New("zipalignコマンドが見つかりません")
	// ErrZipalignFailed はzipalignコマンドの実行自体に失敗、または
	// align/isAlignedが非ゼロ終了コードで終了した場合のエラー。
	ErrZipalignFailed = errors.New("zipalignの実行に失敗しました")

	// ErrApkNotFound はsign/verifyの操作対象APKファイルが存在しない場合のエラー。
	ErrApkNotFound = errors.New("APKファイルが見つかりません")
	// ErrKeystoreNotFound はキーストアファイルが存在しない場合のエラー。
	ErrKeystoreNotFound = errors.New("キーストアファイルが見つかりません")
	// ErrApkSignerNotFound はapksignerコマンドが見つからない場合のエラー。
	ErrApkSignerNotFound = errors.New("apksignerコマンドが見つかりません")
	// ErrApkSignFailed はapksigner signが非ゼロ終了コードで終了、
	// またはコマンド実行自体に失敗した場合のエラー。
	ErrApkSignFailed = errors.New("apksignerの署名に失敗しました")
	// ErrApkVerifyFailed はapksigner verifyコマンドの実行自体に失敗した場合のエラー。
	// 署名検証結果が無効(non-zero終了)であることそれ自体はエラーではなく
	// Verifyの戻り値boolで表現する。
	ErrApkVerifyFailed = errors.New("apksignerの検証実行に失敗しました")

	// ErrPasswordEmpty は対話的入力で得たパスワードが空だった場合のエラー。
	ErrPasswordEmpty = errors.New("パスワードが空です")
	// ErrPasswordCancelled はパスワード入力がユーザー割り込みでキャンセルされた場合のエラー。
	ErrPasswordCancelled = errors.New("パスワード入力がキャンセルされました")
	// ErrPasswordInputFailed はパスワード入力の読み取りに失敗した場合のエラー
	// （EOFを含む）。
	ErrPasswordInputFailed = errors.New("パスワード入力に失敗しました")
)
