// why not: このファイルのテストはANDROID_HOME/PATH環境変数をt.Setenvで変更する。
// t.Setenvはテスト（または親テスト）がt.Parallel()を呼んでいるとpanicするため、
// zipalign_test.goと同様に本ファイルでもt.Parallel()を使わない。
package signer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/signer"
)

func TestNewDefaultApkSignerRunner_NilRunnerFallsBackToExecCommandRunner(t *testing.T) {
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("PATH", t.TempDir())

	r := signer.NewDefaultApkSignerRunner(nil)

	assert.NotPanics(t, func() {
		_, ok := r.FindApkSigner()
		assert.False(t, ok)
	})
}

func TestKeystoreConfig_KeyPasswordDefaultsToNil(t *testing.T) {
	// KeystoreConfigは不変値として扱う。Goの構造体はフィールド変更メソッドを
	// 提供しないことで不変性の契約を保つ（internal/apperr.Resultと同じ
	// 設計方針）。
	cfg := signer.KeystoreConfig{
		KeystorePath:     "keystore.jks",
		KeyAlias:         "my_alias",
		KeystorePassword: "keystore_pass",
	}

	assert.Nil(t, cfg.KeyPassword)
}

// KeystoreConfigのStringメソッドのテスト。%v/%+vでの誤ったログ出力による
// パスワード漏洩を防ぐため、fmt経由でも平文パスワードが現れないことをピン留めする。
func TestKeystoreConfig_String_RedactsPasswords(t *testing.T) {
	keyPassword := "key_pass_should_not_leak"
	cfg := signer.KeystoreConfig{
		KeystorePath:     "keystore.jks",
		KeyAlias:         "my_alias",
		KeystorePassword: "keystore_pass_should_not_leak",
		KeyPassword:      &keyPassword,
	}

	cases := map[string]string{
		"%v ローカル変数(値)":  fmt.Sprintf("%v", cfg),
		"%+v ローカル変数(値)": fmt.Sprintf("%+v", cfg),
		"%v ポインタ経由":     fmt.Sprintf("%v", &cfg),
		"%+v ポインタ経由":    fmt.Sprintf("%+v", &cfg),
		"Sprintln":      fmt.Sprintln(cfg),
	}

	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, out, "keystore_pass_should_not_leak")
			assert.NotContains(t, out, "key_pass_should_not_leak")
			// StringerがKeystoreConfig{...}形式で呼ばれていること自体も確認する
			// （呼ばれずデフォルトのフィールド列挙にフォールバックしていないか）。
			assert.Contains(t, out, "KeystoreConfig{")
		})
	}

	// key_passwordが未設定(nil)の場合の表現も確認する。
	cfgNoKeyPassword := signer.KeystoreConfig{
		KeystorePath:     "keystore.jks",
		KeyAlias:         "my_alias",
		KeystorePassword: "keystore_pass_should_not_leak",
	}
	assert.Contains(t, cfgNoKeyPassword.String(), "KeyPassword:<nil>")
}

func TestDefaultApkSignerRunner_Sign(t *testing.T) {
	t.Run("正常系: 署名成功時にAPKパスを返す", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		keyPassword := "key_pass"
		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "keystore_pass",
			KeyPassword:      &keyPassword,
		}

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, args []string) (signer.RunResult, error) {
				assert.Contains(t, args[0], "apksigner")
				assert.Contains(t, args, "sign")
				assert.Contains(t, args, "--ks")
				assert.Contains(t, args, keystore)
				assert.Contains(t, args, "--ks-key-alias")
				assert.Contains(t, args, "my_alias")
				assert.Contains(t, args, "--ks-pass")
				assert.Contains(t, args, "pass:keystore_pass")
				assert.Contains(t, args, "--key-pass")
				assert.Contains(t, args, "pass:key_pass")
				assert.Contains(t, args, apk)

				return signer.RunResult{ExitCode: 0}, nil
			})

		r := signer.NewDefaultApkSignerRunner(runner)

		result, err := r.Sign(apk, cfg)

		require.NoError(t, err)
		assert.Equal(t, apk, result)
	})

	t.Run("正常系: key_password未指定時はkeystore_passwordを使用", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "shared_password",
		}

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, args []string) (signer.RunResult, error) {
				assert.Contains(t, args, "pass:shared_password")

				keyPassIdx := -1
				for i, a := range args {
					if a == "--key-pass" {
						keyPassIdx = i
					}
				}
				require.NotEqual(t, -1, keyPassIdx)
				assert.Equal(t, "pass:shared_password", args[keyPassIdx+1])

				return signer.RunResult{ExitCode: 0}, nil
			})

		r := signer.NewDefaultApkSignerRunner(runner)

		_, err := r.Sign(apk, cfg)
		require.NoError(t, err)
	})

	t.Run("異常系: APKファイルが存在しない場合にErrApkNotFound", func(t *testing.T) {
		dir := t.TempDir()
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "keystore_pass",
		}

		r := signer.NewDefaultApkSignerRunner(nil)

		_, err := r.Sign(filepath.Join(dir, "missing.apk"), cfg)

		assert.ErrorIs(t, err, signer.ErrApkNotFound)
	})

	t.Run("異常系: キーストアファイルが存在しない場合にErrKeystoreNotFound", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		cfg := signer.KeystoreConfig{
			KeystorePath:     filepath.Join(dir, "missing.jks"),
			KeyAlias:         "my_alias",
			KeystorePassword: "keystore_pass",
		}

		r := signer.NewDefaultApkSignerRunner(nil)

		_, err := r.Sign(apk, cfg)

		assert.ErrorIs(t, err, signer.ErrKeystoreNotFound)
	})

	t.Run("異常系: apksignerコマンドが見つからない場合にErrApkSignerNotFound", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", t.TempDir())

		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "keystore_pass",
		}

		r := signer.NewDefaultApkSignerRunner(nil)

		_, err := r.Sign(apk, cfg)

		assert.ErrorIs(t, err, signer.ErrApkSignerNotFound)
	})

	t.Run("異常系: 不正なパスワードで非ゼロ終了した場合にErrApkSignFailed", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "wrong_password",
		}

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{
				ExitCode: 1,
				Stderr:   "Failed to load signer: keystore password was incorrect",
			}, nil)

		r := signer.NewDefaultApkSignerRunner(runner)

		_, err := r.Sign(apk, cfg)

		require.ErrorIs(t, err, signer.ErrApkSignFailed)
		require.ErrorContains(t, err, "keystore password was incorrect")
		// CRITICAL: エラーメッセージに平文パスワードそのものを含めない
		// （--ks-pass/--key-pass引数は含めず、apksigner自身のstderrのみを使う）。
		assert.NotContains(t, err.Error(), "wrong_password")
	})

	t.Run("異常系: コマンド実行自体に失敗した場合にErrApkSignFailed", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		keystore := filepath.Join(dir, "keystore.jks")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))
		require.NoError(t, os.WriteFile(keystore, []byte("keystore"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		cfg := signer.KeystoreConfig{
			KeystorePath:     keystore,
			KeyAlias:         "my_alias",
			KeystorePassword: "super_secret_pw",
		}

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{}, assert.AnError)

		r := signer.NewDefaultApkSignerRunner(runner)

		_, err := r.Sign(apk, cfg)

		require.ErrorIs(t, err, signer.ErrApkSignFailed)
		assert.NotContains(t, err.Error(), "super_secret_pw")
	})
}

func TestDefaultApkSignerRunner_Verify(t *testing.T) {
	t.Run("正常系: 終了コード0で署名有効", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, args []string) (signer.RunResult, error) {
				assert.Contains(t, args[0], "apksigner")
				assert.Contains(t, args, "verify")
				assert.Contains(t, args, apk)

				return signer.RunResult{ExitCode: 0}, nil
			})

		r := signer.NewDefaultApkSignerRunner(runner)

		valid, err := r.Verify(apk)

		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("正常系: 終了コード非ゼロは署名無効でfalse(エラーなし)", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{ExitCode: 1}, nil)

		r := signer.NewDefaultApkSignerRunner(runner)

		valid, err := r.Verify(apk)

		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("異常系: APKファイルが存在しない場合にErrApkNotFound", func(t *testing.T) {
		dir := t.TempDir()

		r := signer.NewDefaultApkSignerRunner(nil)

		_, err := r.Verify(filepath.Join(dir, "missing.apk"))

		assert.ErrorIs(t, err, signer.ErrApkNotFound)
	})

	t.Run("異常系: apksignerコマンドが見つからない場合にErrApkSignerNotFound", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", t.TempDir())

		r := signer.NewDefaultApkSignerRunner(nil)

		_, err := r.Verify(apk)

		assert.ErrorIs(t, err, signer.ErrApkSignerNotFound)
	})

	t.Run("異常系: コマンド実行自体に失敗した場合にErrApkVerifyFailed", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "apksigner")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{}, assert.AnError)

		r := signer.NewDefaultApkSignerRunner(runner)

		_, err := r.Verify(apk)

		assert.ErrorIs(t, err, signer.ErrApkVerifyFailed)
	})
}

func TestDefaultApkSignerRunner_FindApkSigner(t *testing.T) {
	t.Run("正常系: 複数バージョンから最新を選択", func(t *testing.T) {
		androidHome := t.TempDir()
		for _, v := range []string{"30.0.0", "33.0.0", "34.0.0"} {
			dir := filepath.Join(androidHome, "build-tools", v)
			require.NoError(t, os.MkdirAll(dir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "apksigner"), nil, 0o600))
		}
		t.Setenv("ANDROID_HOME", androidHome)

		r := signer.NewDefaultApkSignerRunner(nil)

		result, ok := r.FindApkSigner()

		require.True(t, ok)
		assert.Contains(t, result, "34.0.0")
	})
}
