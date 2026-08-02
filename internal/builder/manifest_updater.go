package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	manifestPackageAttrPattern = regexp.MustCompile(`\s*package="[^"]*"`)
	activityTagPattern         = regexp.MustCompile(`<activity[^>]*(?:>|/>)`)
	serviceTagPattern          = regexp.MustCompile(`<service[^>]*(?:>|/>)`)
	receiverTagPattern         = regexp.MustCompile(`<receiver[^>]*(?:>|/>)`)
	screenOrientationPattern   = regexp.MustCompile(`android:screenOrientation="[^"]*"`)
)

// activityNameForkClassPattern はAndroidManifest.xmlのandroid:name属性値が
// forkActivityClassName（fork版クラス、パッケージ名書き換えのみで素通し
// 出力される）を指す箇所にマッチする。実テンプレートで確認されているのは
// パッケージ省略形のみだが、以下の3形態すべてを許容する。
//   - パッケージ省略形: android:name="KirikiriSDL2Activity"（実テンプレートの表記）
//   - 先頭ドット省略形: android:name=".KirikiriSDL2Activity"（本パッケージの
//     テストで使っている表記。実テンプレートでは未確認）
//   - 完全修飾: android:name="pw.uyjulian.krkrsdl2.KirikiriSDL2Activity"
//     （実テンプレートでは未確認。書き換え方針はrewriteActivityName参照）
//
// キャプチャグループ:
//  1. 先頭ドット（相対解決の省略形。無ければ空文字列）
//  2. fork版パッケージの完全修飾プレフィックス（無ければ空文字列）
var activityNameForkClassPattern = regexp.MustCompile(
	`android:name="(\.?)((?:pw\.uyjulian\.krkrsdl2\.)?)` + forkActivityClassName + `"`,
)

// rewriteActivityName はactivityNameForkClassPatternのマッチ全体(match)を、
// gameActivityClassNameを指すandroid:name属性へ書き換える。
//
// why not（完全修飾形でプレフィックスを保持しない理由）: javaSourceGenerator.Generateは
// 生成する2クラス（forkActivityClassName・gameActivityClassName）を常に
// packageName配下へ配置する。完全修飾形で元のfork版パッケージ接頭辞
// （pw.uyjulian.krkrsdl2.）をそのまま保持すると、packageNameが
// "pw.uyjulian.krkrsdl2"以外の場合に実在しないクラス
// （pw.uyjulian.krkrsdl2.KirikiriSDL2GameActivity）を指すことになり
// ActivityNotFoundExceptionになる。javaSourceGenerator.Generateの実際の配置先
// （packageName）とManifestの参照先を一致させるため、完全修飾形の
// プレフィックスは保持せずpackageNameへ置き換える。相対解決形
// （先頭ドット省略形・パッケージ省略形）はAndroidのコンポーネント名解決
// 規則上すでにnamespace（build.gradleのnamespace = packageName。
// buildGradleUpdater参照）を通じてpackageNameに解決されるため、
// プレフィックスをそのまま保持してよい。
func rewriteActivityName(match string, packageName string) string {
	sub := activityNameForkClassPattern.FindStringSubmatch(match)
	dotPrefix, fqcnPrefix := sub[1], sub[2]

	if fqcnPrefix != "" {
		return fmt.Sprintf(`android:name="%s.%s"`, packageName, gameActivityClassName)
	}

	return `android:name="` + dotPrefix + gameActivityClassName + `"`
}

var applicationTagPattern = regexp.MustCompile(`<application[^>]*>`)

// ScreenOrientationSensorLandscape は起動activityに固定する画面向きの値。
//
// why not: 横向き固定（ゲーム画面が4:3のため横向きの方が大きく表示される）
// とし、plain landscapeではなくsensorLandscapeを選ぶ。寝転んでプレイする際に
// 上下逆さまに持ち替えても追従してほしい（180度反転）というユーザー要望を、
// 横向き固定を保ったまま満たすため。
const ScreenOrientationSensorLandscape = "sensorLandscape"

// manifestUpdater はAndroidManifest.xmlを更新する。
type manifestUpdater struct {
	projectDir string
}

// newManifestUpdater はmanifestUpdaterを初期化する。
func newManifestUpdater(projectDir string) *manifestUpdater {
	return &manifestUpdater{projectDir: projectDir}
}

// Update はAndroidManifest.xmlを更新する
// （package属性の削除、android:exported="true"の付与、
// android:extractNativeLibs="true"の付与、activityへの
// android:screenOrientation="sensorLandscape"の付与、起動activityの
// android:nameのgameActivityClassNameへの書き換え）。
func (u *manifestUpdater) Update(packageName string) error {
	manifestPath := filepath.Join(u.projectDir, "app", "src", "main", "AndroidManifest.xml")

	content, err := os.ReadFile(manifestPath) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: AndroidManifest.xmlが見つかりません: %s", ErrTemplatePreparer, manifestPath)
	}

	text := string(content)
	text = manifestPackageAttrPattern.ReplaceAllString(text, "")

	// 起動activityはforkActivityClassName（パッケージ名書き換えのみで
	// 素通し出力されるfork版クラス）ではなく、mnemonic独自機能
	// （アセットコピー等）を実装するgameActivityClassNameを起動させる
	// 必要がある。activityNameForkClassPattern・rewriteActivityName参照。
	text = activityNameForkClassPattern.ReplaceAllStringFunc(text, func(match string) string {
		return rewriteActivityName(match, packageName)
	})

	// applicationタグにextractNativeLibs="true"を追加する。これにより
	// ネイティブライブラリがAPKから展開され、dlopen（krkrsdl2プラグインの
	// 動的読み込み）でアクセス可能になる。
	text = applicationTagPattern.ReplaceAllStringFunc(text, addExtractNativeLibsIfMissing)

	text = activityTagPattern.ReplaceAllStringFunc(text, func(tag string) string {
		return setScreenOrientation(addExportedIfMissing(tag))
	})
	text = serviceTagPattern.ReplaceAllStringFunc(text, addExportedIfMissing)
	text = receiverTagPattern.ReplaceAllStringFunc(text, addExportedIfMissing)

	if err := os.WriteFile(manifestPath, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// setScreenOrientation はactivityタグにandroid:screenOrientation属性を
// ScreenOrientationSensorLandscapeで設定する。既に属性が存在する場合は
// 値を置き換える（冪等）。
func setScreenOrientation(tag string) string {
	if screenOrientationPattern.MatchString(tag) {
		return screenOrientationPattern.ReplaceAllString(tag, fmt.Sprintf(`android:screenOrientation="%s"`, ScreenOrientationSensorLandscape))
	}

	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + fmt.Sprintf(` android:screenOrientation="%s"/>`, ScreenOrientationSensorLandscape)
	}

	return tag[:len(tag)-1] + fmt.Sprintf(` android:screenOrientation="%s">`, ScreenOrientationSensorLandscape)
}

// addExtractNativeLibsIfMissing はapplicationタグにandroid:extractNativeLibs
// 属性が無い場合"true"で追加する。
func addExtractNativeLibsIfMissing(tag string) string {
	if strings.Contains(tag, "android:extractNativeLibs") || !strings.HasSuffix(tag, ">") {
		return tag
	}

	return tag[:len(tag)-1] + ` android:extractNativeLibs="true">`
}

// addExportedIfMissing はタグにandroid:exported属性が無い場合"true"で追加する。
func addExportedIfMissing(tag string) string {
	if strings.Contains(tag, "android:exported") {
		return tag
	}

	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + ` android:exported="true"/>`
	}

	return tag[:len(tag)-1] + ` android:exported="true">`
}
