package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// TemplatePreparer関連の定数。
const (
	// TargetSDKVersion は推奨ターゲットSDKバージョン。
	TargetSDKVersion = 34
	// CompileSDKVersion は推奨コンパイルSDKバージョン。
	CompileSDKVersion = 34
	// MinSDKVersion は推奨最小SDKバージョン。
	MinSDKVersion = 21
)

var (
	buildGradleNamespaceCheckPattern = regexp.MustCompile(`namespace`)
	androidBlockStartPattern         = regexp.MustCompile(`android\s*\{`)
	compileSdkVersionPattern         = regexp.MustCompile(`compileSdkVersion\s+\d+`)
	minSdkVersionPattern             = regexp.MustCompile(`minSdkVersion\s+\d+`)
	targetSdkVersionPattern          = regexp.MustCompile(`targetSdkVersion\s+\d+`)
	applicationIDCheckPattern        = regexp.MustCompile(`applicationId`)
	applicationIDValuePattern        = regexp.MustCompile(`applicationId\s+"[^"]+"`)
	cmakeExternalNativeBuildPattern  = regexp.MustCompile(`(?s)\s*externalNativeBuild\s*\{[^}]*cmake\s*\{[^}]*\}[^}]*\}`)
	ndkExternalNativeBuildPattern    = regexp.MustCompile(`(?s)\s*externalNativeBuild\s*\{[^}]*ndk\s*\{[^}]*\}[^}]*\}`)
	standaloneNdkAbiFiltersPattern   = regexp.MustCompile(`(?s)\s*ndk\s*\{[^}]*abiFilters[^}]*\}`)
)

// buildGradleUpdater はapp/build.gradleを更新する。
type buildGradleUpdater struct {
	projectDir string
}

// newBuildGradleUpdater はbuildGradleUpdaterを初期化する。
func newBuildGradleUpdater(projectDir string) *buildGradleUpdater {
	return &buildGradleUpdater{projectDir: projectDir}
}

// Update はapp/build.gradleを更新する
// （namespace追加、SDKバージョン更新、CMake設定の削除）。
func (u *buildGradleUpdater) Update(packageName string) error {
	buildGradle := filepath.Join(u.projectDir, "app", "build.gradle")

	content, err := os.ReadFile(buildGradle) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: build.gradleが見つかりません: %s", ErrTemplatePreparer, buildGradle)
	}

	text := string(content)

	if !buildGradleNamespaceCheckPattern.MatchString(text) {
		text = androidBlockStartPattern.ReplaceAllStringFunc(text, func(match string) string {
			return match + fmt.Sprintf("\n    namespace \"%s\"", packageName)
		})
	}

	text = compileSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("compileSdkVersion %d", CompileSDKVersion))
	text = minSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("minSdkVersion %d", MinSDKVersion))
	text = targetSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("targetSdkVersion %d", TargetSDKVersion))

	if applicationIDCheckPattern.MatchString(text) {
		text = applicationIDValuePattern.ReplaceAllStringFunc(text, func(string) string {
			return fmt.Sprintf(`applicationId "%s"`, packageName)
		})
	}

	text = cmakeExternalNativeBuildPattern.ReplaceAllString(text, "")
	text = ndkExternalNativeBuildPattern.ReplaceAllString(text, "")
	text = standaloneNdkAbiFiltersPattern.ReplaceAllString(text, "")

	if err := os.WriteFile(buildGradle, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}
