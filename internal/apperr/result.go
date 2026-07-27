// Package apperr はCLI全体で共有する終了コードと操作結果の値型を提供する。
//
// Pythonの Result は例外機構と併用され、成功/失敗を戻り値として明示的に扱うための
// 補助的な値だった。Goではエラーは標準的に error インターフェースで表現するため、
// Result 自体に振る舞い（Unwrap等）は持たせず、CLI終了コードを伴う結果を表す
// 単純な値型として保つ（PR7のCLI実装側でerrorからResultへ変換する想定）。
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
// Python版は @dataclass(frozen=True) で不変性を保証していたが、
// Goの構造体は値渡しされるため、呼び出し側でポインタを共有しない限り
// 生成後に意図せず変更されることはない。フィールドを変更するメソッドを
// 提供しないことで、Python版の「フィールド代入禁止」という契約を踏襲する。
type Result struct {
	Success bool
	Message string
	// ExitCode省略時のゼロ値はExitSuccess（Pythonのデフォルト値と同一）。
	ExitCode ExitCode
}
