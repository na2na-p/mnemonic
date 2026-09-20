// Package apperr はCLI全体で共有する終了コードの型を提供する。
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
