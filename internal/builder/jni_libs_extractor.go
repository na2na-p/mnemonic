package builder

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// supportedABIs はサポートするABI一覧。
var supportedABIs = map[string]struct{}{
	"arm64-v8a":   {},
	"armeabi-v7a": {},
	"x86":         {},
	"x86_64":      {},
}

// jniLibsExtractor はkrkrsdl2_universal.apkから.soファイルを抽出し、
// app/src/main/jniLibs配下へABIごとに配置する。
type jniLibsExtractor struct {
	projectDir string
}

// newJNILibsExtractor はjniLibsExtractorを初期化する。
func newJNILibsExtractor(projectDir string) *jniLibsExtractor {
	return &jniLibsExtractor{projectDir: projectDir}
}

// Extract はkrkrsdl2_universal.apkから.soファイルを抽出する。
func (e *jniLibsExtractor) Extract() error {
	baseAPK := filepath.Join(e.projectDir, "krkrsdl2_universal.apk")
	if _, err := os.Stat(baseAPK); err != nil {
		return fmt.Errorf("%w: %w: ベースAPKが見つかりません: %s", ErrTemplatePreparer, ErrJniLibsNotFound, baseAPK)
	}

	jniLibsDir := filepath.Join(e.projectDir, "app", "src", "main", "jniLibs")
	if err := os.MkdirAll(jniLibsDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	zr, err := zip.OpenReader(baseAPK)
	if err != nil {
		return fmt.Errorf("%w: 無効なAPKファイルです: %s", ErrTemplatePreparer, baseAPK)
	}
	defer func() { _ = zr.Close() }()

	extracted := 0

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "lib/") || !strings.HasSuffix(f.Name, ".so") {
			continue
		}

		parts := strings.Split(f.Name, "/")
		if len(parts) < 3 {
			continue
		}

		abi := parts[1]
		soName := parts[2]

		if _, ok := supportedABIs[abi]; !ok {
			continue
		}

		destDir := filepath.Join(jniLibsDir, abi)
		if err := os.MkdirAll(destDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		if err := extractZipFileEntry(f, filepath.Join(destDir, soName)); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("%w: APK内に.soファイルが見つかりません: %s", ErrJniLibsNotFound, baseAPK)
	}

	return nil
}

func extractZipFileEntry(f *zip.File, destPath string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // ABI名はsupportedABIsで許可リスト検証済みのため妥当
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil { //nolint:gosec // ベースAPKは信頼済みのビルド成果物でありサイズ上限は設けない
		return err
	}

	return nil
}
