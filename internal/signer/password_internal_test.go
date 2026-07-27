package signer

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetPasswordのwhite-boxテスト。term.ReadPasswordは実端末のfdを要求するため、
// Python版がgetpass.getpassをunittest.mockでパッチしていたのと同様に、
// readPasswordフィールドへ差し替えて検証する。
func TestDefaultPasswordProvider_GetPassword(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 対話的入力でパスワードを取得", func(t *testing.T) {
		t.Parallel()

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return []byte("interactive_password"), nil
			},
		}

		result, err := p.GetPassword("")

		require.NoError(t, err)
		assert.Equal(t, "interactive_password", result)
	})

	t.Run("正常系: カスタムプロンプトでも読み取り結果は変わらない", func(t *testing.T) {
		t.Parallel()

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return []byte("custom_prompt_password"), nil
			},
		}

		result, err := p.GetPassword("Custom prompt: ")

		require.NoError(t, err)
		assert.Equal(t, "custom_prompt_password", result)
	})

	t.Run("異常系: 空入力の場合にErrPasswordEmpty", func(t *testing.T) {
		t.Parallel()

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return []byte(""), nil
			},
		}

		_, err := p.GetPassword("")

		assert.ErrorIs(t, err, ErrPasswordEmpty)
	})

	// readPasswordFromTerminal（既定実装）はSIGINT受信時に実際にcontext.Canceledを
	// 返す（下記TestReadPasswordFromTerminalのSIGINT横取り実装を参照）。
	// ここではGetPassword側のエラー変換ロジックのみをreadPasswordフィールド経由で
	// 単体検証する。
	t.Run("異常系: ユーザー割り込みでキャンセルされた場合にErrPasswordCancelled", func(t *testing.T) {
		t.Parallel()

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return nil, context.Canceled
			},
		}

		_, err := p.GetPassword("")

		assert.ErrorIs(t, err, ErrPasswordCancelled)
	})

	t.Run("異常系: EOFの場合にErrPasswordInputFailed", func(t *testing.T) {
		t.Parallel()

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return nil, io.EOF
			},
		}

		_, err := p.GetPassword("")

		assert.ErrorIs(t, err, ErrPasswordInputFailed)
	})

	t.Run("異常系: その他の読み取りエラーもErrPasswordInputFailed", func(t *testing.T) {
		t.Parallel()

		readErr := errors.New("device error")
		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return nil, readErr
			},
		}

		_, err := p.GetPassword("")

		require.ErrorIs(t, err, ErrPasswordInputFailed)
		assert.ErrorIs(t, err, readErr)
	})
}

func TestDefaultPasswordProvider_Priority(t *testing.T) {
	t.Run("正常系: 環境変数が設定されている場合は対話的入力を呼ばない", func(t *testing.T) {
		t.Setenv("MNEMONIC_KEYSTORE_PASS", "env_password")

		called := false
		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				called = true

				return []byte("interactive_password"), nil
			},
		}

		result, ok := p.GetPasswordFromEnv("")

		assert.True(t, ok)
		assert.Equal(t, "env_password", result)
		assert.False(t, called)
	})

	t.Run("正常系: 環境変数が未設定の場合は対話的入力にフォールバック", func(t *testing.T) {
		t.Setenv("MNEMONIC_KEYSTORE_PASS", "")

		p := DefaultPasswordProvider{
			readPassword: func(uintptr) ([]byte, error) {
				return []byte("interactive_fallback"), nil
			},
		}

		_, ok := p.GetPasswordFromEnv("")
		require.False(t, ok)

		result, err := p.GetPassword("")

		require.NoError(t, err)
		assert.Equal(t, "interactive_fallback", result)
	})
}

// readPasswordFromTerminalの回帰テスト。
//
// why not: term.GetStateが成功する経路（実端末にアタッチされた場合のSIGINT横取り）は
// 疑似端末(pty)が無いと再現できず、pty用の追加依存を導入するコストに見合わない
// （レビューで許容された「テスト困難なら本番経路の正しさを優先する」方針に従う）。
// ここではterm.GetStateが失敗する経路（パイプ等、非端末なfd）で
// goroutine/channelがハングせず正しくフォールバックすることのみを検証する。
func TestReadPasswordFromTerminal_NonTerminalFallsBackWithoutHanging(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _ = readPasswordFromTerminal(r.Fd())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readPasswordFromTerminalが非端末fdに対してハングした")
	}
}
