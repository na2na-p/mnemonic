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
)

// debugKeystoreTimeout はkeytoolコマンドのタイムアウト。
const debugKeystoreTimeout = 30 * time.Second

// createDebugKeystore はデバッグ用キーストアを作成する。
//
// keytoolコマンドを使用してデバッグ用の自己署名キーストアを生成する。
func (b *BuildPipeline) createDebugKeystore() (string, error) {
	dir, err := os.MkdirTemp("", "mnemonic_keystore_")
	if err != nil {
		return "", fmt.Errorf("一時ディレクトリの作成に失敗しました: %w", err)
	}
	b.tempDirs = append(b.tempDirs, dir)

	debugKeystore := filepath.Join(dir, "debug.keystore")

	ctx, cancel := context.WithTimeout(context.Background(), debugKeystoreTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "keytool", //nolint:gosec // 固定の引数列でkeytoolを呼び出す用途のため妥当
		"-genkeypair", "-v",
		"-keystore", debugKeystore,
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
			return "", errors.New("keytoolコマンドが見つかりません。JDKをインストールしてください")
		}
		if ctx.Err() != nil {
			return "", errors.New("keytoolコマンドがタイムアウトしました")
		}

		return "", fmt.Errorf("keytoolの実行に失敗しました: %s", stderr.String())
	}

	return debugKeystore, nil
}
