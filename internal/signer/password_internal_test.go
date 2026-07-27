package signer

import (
	"context"
	"errors"
	"io"
	"testing"

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
