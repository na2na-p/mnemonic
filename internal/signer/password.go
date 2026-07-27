package signer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"golang.org/x/term"
)

// defaultPasswordPrompt はGetPasswordのpromptが空文字列の場合に使う既定値。
const defaultPasswordPrompt = "Enter keystore password: "

// defaultPasswordEnvVar はGetPasswordFromEnvのenvVarが空文字列の場合に使う既定値。
const defaultPasswordEnvVar = "MNEMONIC_KEYSTORE_PASS" //nolint:gosec // これはパスワードを読み取る環境変数の"名前"であり、資格情報そのものではない

// PasswordProvider はキーストアパスワードを取得するためのインターフェース。
//
// APK署名時に必要なキーストアパスワードを対話的入力・環境変数などの
// 様々なソースから取得する機能を抽象化する。
type PasswordProvider interface {
	// GetPassword は対話的にパスワードを取得する。promptが空文字列の場合は
	// defaultPasswordPromptを使用する。入力されたパスワードは端末にエコーされない。
	// パスワードが空の場合はErrPasswordEmpty、ユーザー割り込みでキャンセルされた
	// 場合はErrPasswordCancelled、それ以外の読み取り失敗はErrPasswordInputFailedを返す。
	GetPassword(prompt string) (string, error)

	// GetPasswordFromEnv はenvVarで指定した環境変数からパスワードを取得する。
	// envVarが空文字列の場合はdefaultPasswordEnvVarを使用する。
	// 環境変数が未設定または空文字列の場合は空文字列とfalseを返す。
	GetPasswordFromEnv(envVar string) (string, bool)
}

// DefaultPasswordProvider はキーストアパスワードを取得する既定実装。
// 対話的入力（端末エコーなし）と環境変数からのパスワード取得をサポートする。
type DefaultPasswordProvider struct {
	// readPassword は対話的パスワード読み取りの実装。nilの場合は
	// term.ReadPasswordを使う既定実装にフォールバックする。
	//
	// why not: term.ReadPasswordは実端末のfdを要求し単体テストでの差し替えが
	// 困難（Python版はgetpass.getpassをunittest.mockでパッチしていた）。
	// パッケージ変数の書き換えはt.Parallel()配下のテスト同士で競合するため、
	// 構造体フィールドとして差し替え口を持たせる。
	readPassword func(fd uintptr) ([]byte, error)
}

// GetPassword は対話的にパスワードを取得する。標準エラー出力にpromptを表示し、
// 標準入力からエコーなしでパスワードを読み取る。
func (p DefaultPasswordProvider) GetPassword(prompt string) (string, error) {
	if prompt == "" {
		prompt = defaultPasswordPrompt
	}

	read := p.readPassword
	if read == nil {
		read = readPasswordFromTerminal
	}

	fmt.Fprint(os.Stderr, prompt)

	b, err := read(os.Stdin.Fd())

	fmt.Fprintln(os.Stderr)

	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return "", fmt.Errorf("%w: ユーザー割り込みにより入力がキャンセルされました", ErrPasswordCancelled)
		case errors.Is(err, io.EOF):
			return "", fmt.Errorf("%w: EOFを受信しました", ErrPasswordInputFailed)
		default:
			return "", fmt.Errorf("%w: %w", ErrPasswordInputFailed, err)
		}
	}

	if len(b) == 0 {
		return "", ErrPasswordEmpty
	}

	return string(b), nil
}

// GetPasswordFromEnv は環境変数からパスワードを取得する。
func (p DefaultPasswordProvider) GetPasswordFromEnv(envVar string) (string, bool) {
	if envVar == "" {
		envVar = defaultPasswordEnvVar
	}

	password := os.Getenv(envVar)
	if password == "" {
		return "", false
	}

	return password, true
}

// readPasswordFromTerminal はterm.ReadPasswordを使った既定の読み取り実装。
//
// why not: term.ReadPasswordは端末のECHOビットのみを落としISIG（Ctrl-CでSIGINTを
// 発生させる設定）は維持したままである。素のterm.ReadPasswordを待っている間に
// Ctrl-Cが押されると、SIGINTの既定動作でプロセスが即座に終了し、
// term.ReadPasswordが完了時に行うはずのtermios復元（エコー再有効化）が
// 実行されないまま、ユーザーのシェルにecho offの端末が残ってしまう。
// signal.NotifyでSIGINTを横取りし、届いたらこの関数の責任でtermios状態を
// 復元してからcontext.Canceledを返す（GetPasswordがErrPasswordCancelledへ
// 変換する）。読み取り中のgoroutineはstdinのブロッキングreadに残り続けるが、
// os.Stdinをcloseしない限り安全に無視できる（呼び出し元はキャンセル後に
// 処理を終了する想定）。
func readPasswordFromTerminal(fd uintptr) ([]byte, error) {
	intFd := int(fd)

	state, err := term.GetState(intFd)
	if err != nil {
		// 端末でない(パイプ・リダイレクト等)場合はSIGINT横取りに意味がないため
		// term.ReadPasswordへそのまま委ねる。
		return term.ReadPassword(intFd)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	type readResult struct {
		password []byte
		err      error
	}

	resultCh := make(chan readResult, 1)

	go func() {
		password, readErr := term.ReadPassword(intFd)
		resultCh <- readResult{password: password, err: readErr}
	}()

	select {
	case r := <-resultCh:
		return r.password, r.err
	case <-sigCh:
		_ = term.Restore(intFd, state)

		return nil, context.Canceled
	}
}
