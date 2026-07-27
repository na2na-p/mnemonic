package builder

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// センチネルエラー群。
var (
	// ErrProjectGeneration はプロジェクト生成に関する基本エラー。
	ErrProjectGeneration = errors.New("プロジェクト生成に失敗しました")
	// ErrInvalidTemplate はテンプレートの整合性検証に失敗した場合のエラー。
	ErrInvalidTemplate = errors.New("テンプレートが無効です")
)

// requiredTemplateFiles はテンプレートZIPに必須のファイル一覧。
var requiredTemplateFiles = []string{
	"app/build.gradle",
	"app/src/main/AndroidManifest.xml",
	"settings.gradle",
	"build.gradle",
}

// javaReservedWords はJava/Kotlinの予約語一覧。
var javaReservedWords = map[string]struct{}{
	"abstract": {}, "assert": {}, "boolean": {}, "break": {}, "byte": {},
	"case": {}, "catch": {}, "char": {}, "class": {}, "const": {},
	"continue": {}, "default": {}, "do": {}, "double": {}, "else": {},
	"enum": {}, "extends": {}, "final": {}, "finally": {}, "float": {},
	"for": {}, "goto": {}, "if": {}, "implements": {}, "import": {},
	"instanceof": {}, "int": {}, "interface": {}, "long": {}, "native": {},
	"new": {}, "package": {}, "private": {}, "protected": {}, "public": {},
	"return": {}, "short": {}, "static": {}, "strictfp": {}, "super": {},
	"switch": {}, "synchronized": {}, "this": {}, "throw": {}, "throws": {},
	"transient": {}, "try": {}, "void": {}, "volatile": {}, "while": {},
	"true": {}, "false": {}, "null": {},
}

// packageSegmentPattern はパッケージ名の各セグメントの検証パターン。
var packageSegmentPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ProjectConfig はAndroidプロジェクトの設定を表す不変値。
type ProjectConfig struct {
	PackageName string
	AppName     string
	VersionCode int
	VersionName string
}

// ProjectGenerator はテンプレートからAndroidプロジェクトを生成する。
type ProjectGenerator struct {
	templatePath string
}

// NewProjectGenerator はProjectGeneratorを初期化する。
func NewProjectGenerator(templatePath string) *ProjectGenerator {
	return &ProjectGenerator{templatePath: templatePath}
}

// Generate はテンプレートからプロジェクトを生成する。
//
// テンプレートファイルまたは出力先ディレクトリが存在しない場合、
// パッケージ名が不正な場合、テンプレートの検証に失敗した場合はErrProjectGeneration
// （テンプレート検証固有の理由の場合はErrInvalidTemplate）を返す。
func (g *ProjectGenerator) Generate(outputDir string, config ProjectConfig) (string, error) {
	if _, err := os.Stat(g.templatePath); err != nil {
		return "", fmt.Errorf("%w: テンプレートファイルが見つかりません: %s", ErrProjectGeneration, g.templatePath)
	}

	if _, err := os.Stat(outputDir); err != nil {
		return "", fmt.Errorf("%w: 出力先ディレクトリが存在しません: %s", ErrProjectGeneration, outputDir)
	}

	if err := g.validatePackageName(config.PackageName); err != nil {
		return "", err
	}

	if _, err := g.ValidateTemplate(); err != nil {
		return "", err
	}

	if err := g.extractTemplate(outputDir); err != nil {
		return "", err
	}

	if err := g.updateAndroidManifest(outputDir, config); err != nil {
		return "", err
	}

	if err := g.updateBuildGradle(outputDir, config); err != nil {
		return "", err
	}

	if err := g.generateResources(outputDir, config); err != nil {
		return "", err
	}

	return outputDir, nil
}

// ValidateTemplate はテンプレートの整合性を検証する。
//
// テンプレートファイルが存在しない、ZIPとして開けない、必須ファイルが
// 欠けている場合はErrInvalidTemplateを返す。
func (g *ProjectGenerator) ValidateTemplate() (bool, error) {
	if _, err := os.Stat(g.templatePath); err != nil {
		return false, fmt.Errorf("%w: テンプレートファイルが見つかりません: %s", ErrInvalidTemplate, g.templatePath)
	}

	zr, err := zip.OpenReader(g.templatePath)
	if err != nil {
		return false, fmt.Errorf("%w: 不正なZIPファイルです: %s: %w", ErrInvalidTemplate, g.templatePath, err)
	}
	defer func() { _ = zr.Close() }()

	present := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		present[f.Name] = struct{}{}
	}

	var missing []string
	for _, required := range requiredTemplateFiles {
		if _, ok := present[required]; !ok {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return false, fmt.Errorf("%w: 必須ファイルがテンプレートにありません: %s", ErrInvalidTemplate, strings.Join(missing, ", "))
	}

	return true, nil
}

// validatePackageName はパッケージ名を検証する。
func (g *ProjectGenerator) validatePackageName(packageName string) error {
	if packageName == "" {
		return fmt.Errorf("%w: パッケージ名が空です", ErrProjectGeneration)
	}

	segments := strings.Split(packageName, ".")
	if len(segments) < 2 {
		return fmt.Errorf("%w: パッケージ名は2つ以上のセグメントが必要です: %s", ErrProjectGeneration, packageName)
	}

	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("%w: パッケージ名に空のセグメントが含まれています: %s", ErrProjectGeneration, packageName)
		}

		if !packageSegmentPattern.MatchString(segment) {
			return fmt.Errorf("%w: パッケージ名のセグメント'%s'が不正です: %s", ErrProjectGeneration, segment, packageName)
		}

		if _, reserved := javaReservedWords[segment]; reserved {
			return fmt.Errorf("%w: パッケージ名に予約語'%s'が含まれています: %s", ErrProjectGeneration, segment, packageName)
		}
	}

	return nil
}

// extractTemplate はテンプレートを展開する。
func (g *ProjectGenerator) extractTemplate(outputDir string) error {
	zr, err := zip.OpenReader(g.templatePath)
	if err != nil {
		return fmt.Errorf("%w: テンプレートの展開に失敗しました: %w", ErrProjectGeneration, err)
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if err := extractZipEntry(f, outputDir); err != nil {
			return fmt.Errorf("%w: テンプレートの展開に失敗しました: %w", ErrProjectGeneration, err)
		}
	}

	return nil
}

// extractZipEntry はzip.FileをoutputDir配下へ展開する。
//
// why not: エントリ名はテンプレートZIP内データに由来し外部入力として
// 信頼できないため、展開先がoutputDir外に脱出しないことを検証する
// （zip slip対策。internal/parser/xp3.goのsafeJoinと同じ理由）。
func extractZipEntry(f *zip.File, outputDir string) error {
	destPath, err := safeJoinPath(outputDir, f.Name)
	if err != nil {
		return err
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(destPath, 0o750)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}

	src, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // destPathはsafeJoinPathでoutputDir配下に限定済み
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil { //nolint:gosec // テンプレートZIPは信頼済みのビルド成果物であり展開サイズは無制限で許容する
		return err
	}

	return nil
}

// safeJoinPath はbaseDir配下にentryNameを結合する（zip slip対策）。
func safeJoinPath(baseDir, entryName string) (string, error) {
	cleanedName := filepath.FromSlash(strings.ReplaceAll(entryName, `\`, "/"))
	joined := filepath.Join(baseDir, cleanedName)

	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("出力ディレクトリの絶対パス解決に失敗しました: %w", err)
	}
	target, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("展開先パスの絶対パス解決に失敗しました: %w", err)
	}

	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: 展開先が出力ディレクトリの外を指しています: %s", ErrInvalidTemplate, entryName)
	}

	return target, nil
}

var (
	manifestPackagePattern = regexp.MustCompile(`package="[^"]*"`)
	manifestLabelPattern   = regexp.MustCompile(`android:label="[^"]*"`)
)

// updateAndroidManifest はAndroidManifest.xmlを更新する。
func (g *ProjectGenerator) updateAndroidManifest(outputDir string, config ProjectConfig) error {
	manifestPath := filepath.Join(outputDir, "app", "src", "main", "AndroidManifest.xml")

	content, err := os.ReadFile(manifestPath) //nolint:gosec // outputDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: AndroidManifest.xmlが見つかりません: %s", ErrProjectGeneration, manifestPath)
	}

	text := string(content)
	// why not: Go正規表現のReplaceAllStringは置換文字列中の"$"をグループ参照として
	// 解釈するため、config.PackageName/AppNameに"$"が含まれる場合に意図しない
	// 置換結果になりうる。Func版で単純な文字列連結にすることでこれを避ける
	// （Pythonのre.subは置換が単純文字列なら"$"を特別扱いしないため、この対応は
	// Go版特有の差分）。
	text = manifestPackagePattern.ReplaceAllStringFunc(text, func(string) string {
		return fmt.Sprintf(`package="%s"`, config.PackageName)
	})
	text = manifestLabelPattern.ReplaceAllStringFunc(text, func(string) string {
		return fmt.Sprintf(`android:label="%s"`, config.AppName)
	})

	if err := os.WriteFile(manifestPath, []byte(text), 0o600); err != nil { //nolint:gosec // outputDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: AndroidManifest.xmlの更新に失敗しました: %w", ErrProjectGeneration, err)
	}

	return nil
}

var (
	gradleNamespacePattern     = regexp.MustCompile(`namespace\s+["']([^"']*)["']`)
	gradleApplicationIDPattern = regexp.MustCompile(`applicationId\s+["']([^"']*)["']`)
	gradleVersionCodePattern   = regexp.MustCompile(`versionCode\s+\d+`)
	gradleVersionNamePattern   = regexp.MustCompile(`versionName\s+["']([^"']*)["']`)
)

// updateBuildGradle はapp/build.gradleまたはapp/build.gradle.ktsを更新する。
func (g *ProjectGenerator) updateBuildGradle(outputDir string, config ProjectConfig) error {
	gradlePath := filepath.Join(outputDir, "app", "build.gradle")
	if _, err := os.Stat(gradlePath); err != nil {
		gradlePath = filepath.Join(outputDir, "app", "build.gradle.kts")
	}

	content, err := os.ReadFile(gradlePath) //nolint:gosec // outputDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: build.gradleまたはbuild.gradle.ktsが見つかりません: %s", ErrProjectGeneration, filepath.Join(outputDir, "app"))
	}

	text := string(content)
	text = gradleNamespacePattern.ReplaceAllStringFunc(text, func(string) string {
		return fmt.Sprintf(`namespace "%s"`, config.PackageName)
	})
	text = gradleApplicationIDPattern.ReplaceAllStringFunc(text, func(string) string {
		return fmt.Sprintf(`applicationId "%s"`, config.PackageName)
	})
	text = gradleVersionCodePattern.ReplaceAllStringFunc(text, func(string) string {
		return "versionCode " + strconv.Itoa(config.VersionCode)
	})
	text = gradleVersionNamePattern.ReplaceAllStringFunc(text, func(string) string {
		return fmt.Sprintf(`versionName "%s"`, config.VersionName)
	})

	if err := os.WriteFile(gradlePath, []byte(text), 0o600); err != nil { //nolint:gosec // outputDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: build.gradleの更新に失敗しました: %w", ErrProjectGeneration, err)
	}

	return nil
}

// minimalTransparentPNG は1x1ピクセルの透明PNG（デフォルトアイコン用）。
var minimalTransparentPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 pixel
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, // RGBA, etc
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, // IDAT chunk
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, // IEND chunk
	0x42, 0x60, 0x82,
}

// mipmapDensities はアイコンを配置するmipmapディレクトリの解像度一覧。
var mipmapDensities = []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

// generateResources はAndroidリソースファイル（strings.xml、デフォルトアイコン）を生成する。
func (g *ProjectGenerator) generateResources(outputDir string, config ProjectConfig) error {
	resDir := filepath.Join(outputDir, "app", "src", "main", "res")

	valuesDir := filepath.Join(resDir, "values")
	if err := os.MkdirAll(valuesDir, 0o750); err != nil {
		return fmt.Errorf("%w: リソースの生成に失敗しました: %w", ErrProjectGeneration, err)
	}

	stringsXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">%s</string>
</resources>
`, config.AppName)

	if err := os.WriteFile(filepath.Join(valuesDir, "strings.xml"), []byte(stringsXML), 0o600); err != nil {
		return fmt.Errorf("%w: リソースの生成に失敗しました: %w", ErrProjectGeneration, err)
	}

	for _, density := range mipmapDensities {
		mipmapDir := filepath.Join(resDir, "mipmap-"+density)
		if err := os.MkdirAll(mipmapDir, 0o750); err != nil {
			return fmt.Errorf("%w: リソースの生成に失敗しました: %w", ErrProjectGeneration, err)
		}

		if err := os.WriteFile(filepath.Join(mipmapDir, "ic_launcher.png"), minimalTransparentPNG, 0o600); err != nil {
			return fmt.Errorf("%w: リソースの生成に失敗しました: %w", ErrProjectGeneration, err)
		}
	}

	return nil
}
