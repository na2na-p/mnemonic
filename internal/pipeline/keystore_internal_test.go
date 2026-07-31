package pipeline

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKeystoreTestPipeline はkeytool実行を伴わないBuildPipelineを生成する。
// keystorePath/keystoreValid/keystoreGenerateをテストごとに差し替える。
func newKeystoreTestPipeline(t *testing.T, path string, valid bool, generate func(string) error) *BuildPipeline {
	t.Helper()

	b := &BuildPipeline{}
	b.keystorePath = func() (string, error) { return path, nil }
	b.keystoreValid = func(string) bool { return valid }
	b.keystoreGenerate = generate

	return b
}

func TestBuildPipeline_CreateDebugKeystore_NoExistingKeystoreGeneratesNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "keystore", "debug.keystore")

	generated := false
	b := newKeystoreTestPipeline(t, path, false, func(dest string) error {
		generated = true
		assert.Equal(t, path, dest)

		return os.WriteFile(dest, []byte("generated"), 0o600)
	})

	got, err := b.createDebugKeystore()

	require.NoError(t, err)
	assert.Equal(t, path, got)
	assert.True(t, generated, "既存キーストアが無い場合は生成関数が呼ばれるべき")
}

func TestBuildPipeline_CreateDebugKeystore_ValidExistingKeystoreIsReused(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "keystore", "debug.keystore")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))

	generated := false
	b := newKeystoreTestPipeline(t, path, true, func(string) error {
		generated = true

		return nil
	})

	got, err := b.createDebugKeystore()

	require.NoError(t, err)
	assert.Equal(t, path, got)
	assert.False(t, generated, "有効な既存キーストアがある場合は再生成すべきでない")
}

func TestBuildPipeline_CreateDebugKeystore_CorruptedExistingKeystoreIsRegenerated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "keystore", "debug.keystore")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("corrupted"), 0o600))

	generated := false
	b := newKeystoreTestPipeline(t, path, false, func(dest string) error {
		generated = true

		return os.WriteFile(dest, []byte("regenerated"), 0o600)
	})

	got, err := b.createDebugKeystore()

	require.NoError(t, err)
	assert.Equal(t, path, got)
	assert.True(t, generated, "検証に失敗した既存キーストアは再生成されるべき")
}

func TestBuildPipeline_CreateDebugKeystore_PathResolutionErrorIsPropagated(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("パス解決エラー")

	b := &BuildPipeline{}
	b.keystorePath = func() (string, error) { return "", wantErr }

	_, err := b.createDebugKeystore()

	require.ErrorIs(t, err, wantErr)
}

func TestBuildPipeline_CreateDebugKeystore_GenerateErrorIsPropagated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "keystore", "debug.keystore")
	wantErr := errors.New("生成エラー")

	b := newKeystoreTestPipeline(t, path, false, func(string) error { return wantErr })

	_, err := b.createDebugKeystore()

	require.ErrorIs(t, err, wantErr)
}

// why not: t.Setenvはt.Parallel()を呼んだテストでは使えない
// （並列実行中の他テストに環境変数の変更が影響しうるため）ので、
// このテストのみt.Parallel()を呼ばない。
func TestResolveDebugKeystorePath_ReturnsPathUnderCacheDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := resolveDebugKeystorePath()

	require.NoError(t, err)
	assert.Equal(t, "debug.keystore", filepath.Base(path))
	assert.Equal(t, "keystore", filepath.Base(filepath.Dir(path)))
}

// requireFunctionalKeytool はkeytoolコマンドが実際にJavaランタイムを解決できるかを
// 確認する。javaが未インストール/スタブのみの環境（このsandboxを含む）では
// keytoolはコマンドとして存在してもJavaランタイム未検出で失敗するため、
// 単なるexec.LookPathでは判定できない。呼び出せない場合はテストをスキップする。
func requireFunctionalKeytool(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, "keytool", "-help").Run(); err != nil {
		t.Skipf("keytoolが動作しない環境のためスキップします: %v", err)
	}
}

// TestGenerateDebugKeystoreFile_RegeneratesCorruptedKeystore は実際のkeytoolを
// 使い、破損した（キーストアとして読めない）既存ファイルがあってもgenerateDebugKeystoreFile
// が再生成に成功することをピン留めする。generateDebugKeystoreFile冒頭のos.Remove
// が無いと、keytool -genkeypairが既存ファイルをキーストアとしてロードしようとして
// KeyStoreExceptionで失敗する（DIスタブ経由のテストでは検出できない実物の挙動）。
func TestGenerateDebugKeystoreFile_RegeneratesCorruptedKeystore(t *testing.T) {
	t.Parallel()
	requireFunctionalKeytool(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "debug.keystore")
	require.NoError(t, os.WriteFile(path, []byte("not-a-valid-keystore"), 0o600))

	err := generateDebugKeystoreFile(path)

	require.NoError(t, err)
	assert.True(t, validateDebugKeystoreFile(path), "再生成後のキーストアはkeytool -listで検証できるべき")
}

// TestValidateDebugKeystoreFile_ValidatesGeneratedKeystore は実際のkeytoolで
// 生成したキーストアをvalidateDebugKeystoreFileが有効と判定することをピン留めする。
func TestValidateDebugKeystoreFile_ValidatesGeneratedKeystore(t *testing.T) {
	t.Parallel()
	requireFunctionalKeytool(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "debug.keystore")

	require.NoError(t, generateDebugKeystoreFile(path))
	assert.True(t, validateDebugKeystoreFile(path))
}

// TestValidateDebugKeystoreFile_RejectsCorruptedKeystore は実際のkeytoolで
// 破損したファイルがvalidateDebugKeystoreFileでfalse判定になることをピン留めする。
func TestValidateDebugKeystoreFile_RejectsCorruptedKeystore(t *testing.T) {
	t.Parallel()
	requireFunctionalKeytool(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "debug.keystore")
	require.NoError(t, os.WriteFile(path, []byte("not-a-valid-keystore"), 0o600))

	assert.False(t, validateDebugKeystoreFile(path))
}
