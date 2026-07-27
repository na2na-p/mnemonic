package signer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/signer"
)

// why not: t.SetenvがGetPasswordFromEnvの対象環境変数を書き換えるため、
// zipalign_test.go/apksigner_test.goと同様にt.Parallel()は使わない。
func TestDefaultPasswordProvider_GetPasswordFromEnv(t *testing.T) {
	t.Run("正常系: 既定の環境変数からパスワードを取得", func(t *testing.T) {
		t.Setenv("MNEMONIC_KEYSTORE_PASS", "my_secret_password")

		p := signer.DefaultPasswordProvider{}

		result, ok := p.GetPasswordFromEnv("")

		assert.True(t, ok)
		assert.Equal(t, "my_secret_password", result)
	})

	t.Run("正常系: カスタム環境変数名からパスワードを取得", func(t *testing.T) {
		t.Setenv("CUSTOM_PASSWORD_VAR", "custom_password")

		p := signer.DefaultPasswordProvider{}

		result, ok := p.GetPasswordFromEnv("CUSTOM_PASSWORD_VAR")

		assert.True(t, ok)
		assert.Equal(t, "custom_password", result)
	})

	t.Run("異常系: 環境変数が未設定の場合はfalse", func(t *testing.T) {
		t.Setenv("MNEMONIC_KEYSTORE_PASS", "")
		t.Setenv("SOME_UNSET_VAR_FOR_TEST", "")

		p := signer.DefaultPasswordProvider{}

		result, ok := p.GetPasswordFromEnv("SOME_UNSET_VAR_FOR_TEST")

		assert.False(t, ok)
		assert.Empty(t, result)
	})

	t.Run("異常系: 環境変数が空文字列の場合はfalse", func(t *testing.T) {
		t.Setenv("MNEMONIC_KEYSTORE_PASS", "")

		p := signer.DefaultPasswordProvider{}

		result, ok := p.GetPasswordFromEnv("")

		assert.False(t, ok)
		assert.Empty(t, result)
	})
}
