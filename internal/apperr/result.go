// Package apperr はCLI全体で共有する終了コードと操作結果の値型を提供する。
//
// エラーは標準の error インターフェースで表現するため、Result 自体に
// 振る舞い（Unwrap等）は持たせず、CLI終了コードを伴う結果を表す単純な値型
// として保つ。
package apperr

// ExitCode はCLIの終了コードを表す。
type ExitCode int

const (
	// ExitSuccess は正常終了を表す。
	ExitSuccess ExitCode = iota
	// ExitError は一般的なエラーによる終了を表す。
	ExitError
	// ExitInvalidInput は不正な入力による終了を表す。
	ExitInvalidInput
	// ExitDependencyError は依存ツール不足による終了を表す。
	ExitDependencyError
)

// Result は操作結果を表す値。
//
// Goの構造体は値渡しされるため、呼び出し側でポインタを共有しない限り
// 生成後に意図せず変更されることはない。フィールドを変更するメソッドを
// 提供しないことで、生成後は変更しない値として扱う契約を保つ。
type Result struct {
	Success bool
	Message string
	// ExitCode省略時のゼロ値はExitSuccessになる。
	ExitCode ExitCode
}
