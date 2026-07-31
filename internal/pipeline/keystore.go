package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/na2na-p/mnemonic/internal/cache"
)

// debugKeystoreTimeout はkeytoolコマンドのタイムアウト。
const debugKeystoreTimeout = 30 * time.Second

// debugKeystoreDirName はデバッグ用キーストアを永続化するキャッシュディレクトリ配下のサブディレクトリ名。
const debugKeystoreDirName = "keystore"

// debugKeystoreFileName は永続化するデバッグ用キーストアのファイル名。
const debugKeystoreFileName = "debug.keystore"

// createDebugKeystore はデバッグ用キーストアを作成、または既存のキーストアを再利用する。
//
// why not: ビルドのたびにos.MkdirTempで新しい一時ディレクトリへ鍵を生成すると、
// 再ビルドごとに署名鍵が変わり`adb install -r`がINSTALL_FAILED_UPDATE_INCOMPATIBLEで
// 失敗する（アンインストールしないと上書きインストールできず、セーブデータが
// 消える）。internal/cacheのテンプレートキャッシュと同じキャッシュディレクトリ
// 配下に固定パスで永続化し、既存かつ検証に通るキーストアがあれば再利用する。
func (b *BuildPipeline) createDebugKeystore() (string, error) {
	path, err := b.keystorePath()
	if err != nil {
		return "", err
	}

	if _, statErr := os.Stat(path); statErr == nil {
		if b.keystoreValid(path) {
			return path, nil
		}

		fmt.Fprintf(os.Stderr, "警告: 既存のデバッグキーストアの検証に失敗したため再作成します: %s\n", path) //nolint:errcheck // 警告出力の書き込み失敗は実用上ハンドリング不要
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("キーストアディレクトリの作成に失敗しました: %w", err)
	}

	if err := b.keystoreGenerate(path); err != nil {
		return "", err
	}

	return path, nil
}

// resolveDebugKeystorePath は永続化するデバッグ用キーストアのパスを解決する。
func resolveDebugKeystorePath() (string, error) {
	dir, err := cache.Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, debugKeystoreDirName, debugKeystoreFileName), nil
}

// validateDebugKeystoreFile はkeytool -listでpathのキーストアが読み取り可能かを検証する。
func validateDebugKeystoreFile(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), debugKeystoreTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "keytool", //nolint:gosec // 固定の引数列でkeytoolを呼び出す用途のため妥当
		"-list",
		"-keystore", path,
		"-storepass", "android",
		"-alias", "debug",
	)

	return cmd.Run() == nil
}

// generateDebugKeystoreFile はkeytoolコマンドを使用してdestPathにデバッグ用の自己署名キーストアを生成する。
func generateDebugKeystoreFile(destPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), debugKeystoreTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "keytool", //nolint:gosec // 固定の引数列でkeytoolを呼び出す用途のため妥当
		"-genkeypair", "-v",
		"-keystore", destPath,
		"-storepass", "android",
		"-alias", "debug",
		"-keypass", "android",
		"-keyalg", "RSA",
		"-keysize", "2048",
		"-validity", "10000",
		"-dname", "CN=Debug,OU=Debug,O=Debug,L=Debug,ST=Debug,C=US",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return errors.New("keytoolコマンドが見つかりません。JDKをインストールしてください")
		}
		if ctx.Err() != nil {
			return errors.New("keytoolコマンドがタイムアウトしました")
		}

		return fmt.Errorf("keytoolの実行に失敗しました: %s", stderr.String())
	}

	return nil
}
