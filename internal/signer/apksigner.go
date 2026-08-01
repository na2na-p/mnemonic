package signer

import (
	"context"
	"fmt"
	"os"
)

// KeystoreConfig はAPK署名に必要なキーストアの設定情報を表す不変値。
//
// Goの構造体は値渡しされるため、フィールドを変更するメソッドを提供しない
// ことで「フィールド代入禁止」という契約を保つ（internal/apperr.Resultと
// 同じ設計方針）。
//
// KeyPasswordがnilの場合、KeystorePasswordを使用する。空文字列と未指定(nil)
// を区別するため*stringを使う。
type KeystoreConfig struct {
	KeystorePath     string
	KeyAlias         string
	KeystorePassword string
	KeyPassword      *string
}

// String はfmt.Stringerを実装し、パスワードをマスクした表現を返す。
//
// why not: KeystoreConfigをそのまま%v/%+vでログ出力すると、構造体の全フィールド
// （KeystorePassword/KeyPasswordの平文）がそのまま出力されてしまう。将来
// この値をログに渡すコードが追加されても平文パスワードが漏れないよう、
// value receiverでStringerを実装しておく（%v/%+vはvalue/pointerどちらでも
// value receiverのStringerを使う）。
func (c KeystoreConfig) String() string {
	keyPassword := "<nil>"
	if c.KeyPassword != nil {
		keyPassword = "***"
	}

	return fmt.Sprintf(
		"KeystoreConfig{KeystorePath:%q, KeyAlias:%q, KeystorePassword:***, KeyPassword:%s}",
		c.KeystorePath, c.KeyAlias, keyPassword,
	)
}

// ApkSignerRunner はapksignerコマンドを実行するためのインターフェース。
//
// APKファイルの署名と検証を行うapksignerコマンドの実行機能を抽象化する。
type ApkSignerRunner interface {
	// Sign はapkPathのAPKファイルにkeystoreConfigを使って署名を適用する。
	// 成功時はapkPathを返す。
	// APKファイルが存在しない場合はErrApkNotFound、
	// キーストアファイルが存在しない場合はErrKeystoreNotFound、
	// apksignerコマンドが見つからない場合はErrApkSignerNotFound、
	// 署名処理に失敗した場合はErrApkSignFailedを返す。
	Sign(apkPath string, keystoreConfig KeystoreConfig) (string, error)

	// Verify はapkPathのAPKファイルの署名が有効かどうかを検証する。
	// APKファイルが存在しない場合はErrApkNotFound、
	// apksignerコマンドが見つからない場合はErrApkSignerNotFound、
	// コマンドの実行自体に失敗した場合はErrApkVerifyFailedを返す。
	Verify(apkPath string) (bool, error)

	// FindApkSigner はapksignerコマンドのパスを検索する。
	// ANDROID_HOME環境変数やシステムPATHを参照する。
	// 見つからない場合は空文字列とfalseを返す。
	FindApkSigner() (string, bool)
}

// DefaultApkSignerRunner はapksignerコマンドを実行する既定実装。
type DefaultApkSignerRunner struct {
	runner CommandRunner
}

// NewDefaultApkSignerRunner はDefaultApkSignerRunnerを初期化する。
// runnerがnilの場合はos/execベースの既定実装を使用する。
func NewDefaultApkSignerRunner(runner CommandRunner) *DefaultApkSignerRunner {
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	return &DefaultApkSignerRunner{runner: runner}
}

// Sign はapksigner signを実行してAPKファイルに署名する。
//
// why not: 失敗時のエラーメッセージにはapksigner自身のstderrのみを含め、
// 実行に使ったコマンドライン引数（--ks-pass/--key-passに平文パスワードを含む）は
// 一切含めない。エラーログ経由でのパスワード漏洩を防ぐための設計上の制約。
func (r *DefaultApkSignerRunner) Sign(apkPath string, keystoreConfig KeystoreConfig) (string, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return "", fmt.Errorf("%w: %s", ErrApkNotFound, apkPath)
	}

	if _, err := os.Stat(keystoreConfig.KeystorePath); err != nil {
		return "", fmt.Errorf("%w: %s", ErrKeystoreNotFound, keystoreConfig.KeystorePath)
	}

	apksignerPath, ok := r.FindApkSigner()
	if !ok {
		return "", ErrApkSignerNotFound
	}

	keyPassword := keystoreConfig.KeystorePassword
	if keystoreConfig.KeyPassword != nil {
		keyPassword = *keystoreConfig.KeyPassword
	}

	args := []string{
		apksignerPath,
		"sign",
		"--ks", keystoreConfig.KeystorePath,
		"--ks-key-alias", keystoreConfig.KeyAlias,
		"--ks-pass", "pass:" + keystoreConfig.KeystorePassword,
		"--key-pass", "pass:" + keyPassword,
		apkPath,
	}

	result, err := r.runner.Run(context.Background(), args)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrApkSignFailed, err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("%w: %s", ErrApkSignFailed, result.Stderr)
	}

	return apkPath, nil
}

// Verify はapksigner verifyを実行してAPKファイルの署名を検証する。
// 終了コードが0の場合にtrueを返す（非ゼロはコマンド失敗ではなく署名無効を表す）。
func (r *DefaultApkSignerRunner) Verify(apkPath string) (bool, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return false, fmt.Errorf("%w: %s", ErrApkNotFound, apkPath)
	}

	apksignerPath, ok := r.FindApkSigner()
	if !ok {
		return false, ErrApkSignerNotFound
	}

	result, err := r.runner.Run(context.Background(), []string{apksignerPath, "verify", apkPath})
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrApkVerifyFailed, err)
	}

	return result.ExitCode == 0, nil
}

// FindApkSigner はANDROID_HOME配下のbuild-toolsから最新バージョンのapksignerを検索し、
// 見つからない場合はシステムPATHから検索する。
func (r *DefaultApkSignerRunner) FindApkSigner() (string, bool) {
	return findAndroidBuildTool("apksigner")
}
