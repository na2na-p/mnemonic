package builder

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// センチネルエラー群。
var (
	// ErrTemplatePreparer はテンプレート準備に関する基本エラー。
	ErrTemplatePreparer = errors.New("テンプレートの準備に失敗しました")
	// ErrJniLibsNotFound はJNIライブラリが見つからない場合のエラー。
	ErrJniLibsNotFound = errors.New("JNIライブラリが見つかりません")
	// ErrSDL2SourceFetch はSDL2 Javaソースの取得に失敗した場合のエラー。
	ErrSDL2SourceFetch = errors.New("SDL2 Javaソースの取得に失敗しました")
)

// TemplatePreparer はAndroidプロジェクトテンプレートを準備する。
//
// 以下の処理を行う:
//  1. krkrsdl2_universal.apkから.soファイルを抽出してjniLibsに配置
//  2. SDL2 Javaソースをダウンロードして配置
//  3. krkrsdl2プラグイン(.so)をjniLibsに配置
//  4. fork版KirikiriSDL2Activity.javaをパッケージ名書き換えのみで素通し配置し、
//     mnemonic独自機能（assetsコピー等）を実装するKirikiriSDL2GameActivity
//     サブクラスを新規生成
//  5. app/build.gradleを更新（targetSdkVersion=34、namespace追加）
//  6. AndroidManifest.xmlを更新（android:exported="true"追加、起動activityの
//     android:nameをKirikiriSDL2GameActivityへ書き換え）
//  7. res/values/strings.xmlを作成（app_name設定）
type TemplatePreparer struct {
	projectDir string
	sdl2Cache  *SDL2SourceCache

	jniLibsExtractor    *jniLibsExtractor
	pluginPlacer        *pluginPlacer
	javaSourceGenerator *javaSourceGenerator
	buildGradleUpdater  *buildGradleUpdater
}

// NewTemplatePreparer はTemplatePreparerを初期化する。
// sdl2CacheがnilでもSDL2 Javaソースの取得自体は行われる（キャッシュ未使用で
// 毎回ダウンロードする）。
func NewTemplatePreparer(projectDir string, sdl2Cache *SDL2SourceCache) *TemplatePreparer {
	return &TemplatePreparer{
		projectDir:          projectDir,
		sdl2Cache:           sdl2Cache,
		jniLibsExtractor:    newJNILibsExtractor(projectDir),
		pluginPlacer:        newPluginPlacer(projectDir),
		javaSourceGenerator: newJavaSourceGenerator(projectDir),
		buildGradleUpdater:  newBuildGradleUpdater(projectDir),
	}
}

// Prepare はテンプレートを準備する。
// assetsDir/iconPathが空文字列の場合、対応する処理はスキップされる
// （iconPathはファイルが存在しない場合もスキップされ、代わりにデフォルト
// アイコンを生成する）。pluginsInfoがnilの場合、プラグインのjniLibs配置は
// スキップされる（呼び出し元が事前にPluginFetcherで取得したものを渡す
// 想定。why not: テストが実ネットワークに触れないようにするため、
// pluginsInfo=nilの場合に自前でダウンロードを試みるフォールバックは
// 持たない。呼び出し元（internal/pipeline）が明示的にfetchPlugins()の
// 結果を渡す設計に一本化した）。
func (p *TemplatePreparer) Prepare(packageName, appName, assetsDir, iconPath string, pluginsInfo *PluginsInfo) error {
	if err := p.jniLibsExtractor.Extract(); err != nil {
		return err
	}

	if err := p.fetchSDL2Sources(); err != nil {
		return err
	}

	if err := p.pluginPlacer.Place(pluginsInfo); err != nil {
		return err
	}

	if err := p.javaSourceGenerator.Generate(packageName); err != nil {
		return err
	}

	if err := p.buildGradleUpdater.Update(packageName); err != nil {
		return err
	}

	if err := p.updateManifest(packageName); err != nil {
		return err
	}

	if err := p.updateStringsXML(appName); err != nil {
		return err
	}

	if assetsDir != "" {
		if err := p.copyAssets(assetsDir); err != nil {
			return err
		}
	}

	if iconPath != "" {
		if _, err := os.Stat(iconPath); err == nil {
			return p.updateIcon(iconPath)
		}
	}

	return p.createDefaultIcon()
}

// fetchSDL2Sources はSDL2のJavaソースファイル（SDLActivity.java等）を取得して
// 配置する。キャッシュが有効な場合はキャッシュから復元する。
func (p *TemplatePreparer) fetchSDL2Sources() error {
	javaDir := filepath.Join(p.projectDir, "app", "src", "main", "java")
	if err := os.MkdirAll(javaDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	fetcher := NewSDL2SourceFetcher(0, p.sdl2Cache)
	if err := fetcher.Fetch(javaDir); err != nil {
		return fmt.Errorf("%w: %w: %w", ErrTemplatePreparer, ErrSDL2SourceFetch, err)
	}

	return nil
}

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
// why not（完全修飾形でプレフィックスを保持しない理由）: updateJavaSourceは
// 生成する2クラス（forkActivityClassName・gameActivityClassName）を常に
// packageName配下へ配置する。完全修飾形で元のfork版パッケージ接頭辞
// （pw.uyjulian.krkrsdl2.）をそのまま保持すると、packageNameが
// "pw.uyjulian.krkrsdl2"以外の場合に実在しないクラス
// （pw.uyjulian.krkrsdl2.KirikiriSDL2GameActivity）を指すことになり
// ActivityNotFoundExceptionになる。updateJavaSourceの実際の配置先
// （packageName）とManifestの参照先を一致させるため、完全修飾形の
// プレフィックスは保持せずpackageNameへ置き換える。相対解決形
// （先頭ドット省略形・パッケージ省略形）はAndroidのコンポーネント名解決
// 規則上すでにnamespace（build.gradleのnamespace = packageName。
// updateBuildGradle参照）を通じてpackageNameに解決されるため、
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

// updateManifest はAndroidManifest.xmlを更新する
// （package属性の削除、android:exported="true"の付与、
// android:extractNativeLibs="true"の付与、activityへの
// android:screenOrientation="sensorLandscape"の付与、起動activityの
// android:nameのgameActivityClassNameへの書き換え）。
func (p *TemplatePreparer) updateManifest(packageName string) error {
	manifestPath := filepath.Join(p.projectDir, "app", "src", "main", "AndroidManifest.xml")

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

// xmlAttrEscaper はXML属性値向けのエスケープ処理を行う。
//
// why not: 標準ライブラリのhtml.EscapeStringは"を&#34;、'を&#39;という
// 異なる数値文字参照でエスケープする。"を&quot;、'を&#x27;にエスケープする
// 独自の変換表を持つreplacerを自前で用意する。
var xmlAttrEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)

var stringsXMLAppNamePattern = regexp.MustCompile(`(<string name="app_name">)[^<]*(</string>)`)

// updateStringsXML はres/values/strings.xmlを作成/更新する。
func (p *TemplatePreparer) updateStringsXML(appName string) error {
	valuesDir := filepath.Join(p.projectDir, "app", "src", "main", "res", "values")
	if err := os.MkdirAll(valuesDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	stringsXML := filepath.Join(valuesDir, "strings.xml")
	escapedAppName := xmlAttrEscaper.Replace(appName)

	if content, err := os.ReadFile(stringsXML); err == nil { //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
		text := stringsXMLAppNamePattern.ReplaceAllStringFunc(string(content), func(string) string {
			return "<string name=\"app_name\">" + escapedAppName + "</string>"
		})
		if err := os.WriteFile(stringsXML, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		return nil
	}

	content := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">%s</string>
</resources>
`, escapedAppName)

	if err := os.WriteFile(stringsXML, []byte(content), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// copyAssets はゲームファイルをapp/src/main/assets/dataにコピーする（既存ファイルはマージ）。
func (p *TemplatePreparer) copyAssets(assetsDir string) error {
	destDir := filepath.Join(p.projectDir, "app", "src", "main", "assets", "data")
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	err := filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return err
		}

		return copyFile(path, destPath)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// iconMipmapDensities はアイコンを配置するmipmapディレクトリの解像度一覧。
var iconMipmapDensities = []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

// updateIcon はアプリアイコンを各解像度のmipmapディレクトリへコピーする。
func (p *TemplatePreparer) updateIcon(iconPath string) error {
	resDir := filepath.Join(p.projectDir, "app", "src", "main", "res")

	for _, density := range iconMipmapDensities {
		mipmapDir := filepath.Join(resDir, "mipmap-"+density)
		if err := os.MkdirAll(mipmapDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		destPath := filepath.Join(mipmapDir, "ic_launcher.png")
		if err := copyFile(iconPath, destPath); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}
	}

	return nil
}

// defaultIconDensitySizes はデフォルトアイコン生成時の密度ごとのサイズ(px)。
var defaultIconDensitySizes = map[string]int{
	"mdpi":    48,
	"hdpi":    72,
	"xhdpi":   96,
	"xxhdpi":  144,
	"xxxhdpi": 192,
}

// defaultIconColor はデフォルトアイコンの色（吉里吉里のテーマカラーに近い青紫）。
var defaultIconColor = color.RGBA{R: 100, G: 80, B: 160, A: 255}

// createDefaultIcon はデフォルトアイコンを生成する。
//
// アイコンが提供されない場合のフォールバックとして、単色の正方形アイコンを
// 各解像度で生成する。
func (p *TemplatePreparer) createDefaultIcon() error {
	resDir := filepath.Join(p.projectDir, "app", "src", "main", "res")

	for _, density := range iconMipmapDensities {
		mipmapDir := filepath.Join(resDir, "mipmap-"+density)
		if err := os.MkdirAll(mipmapDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		destPath := filepath.Join(mipmapDir, "ic_launcher.png")
		if err := writeSolidColorPNG(destPath, defaultIconDensitySizes[density], defaultIconColor); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}
	}

	return nil
}

// writeSolidColorPNG はsize x sizeの単色PNG画像をpathへ書き出す。
func writeSolidColorPNG(path string, size int, c color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)

	f, err := os.Create(path) //nolint:gosec // ビルド成果物の出力用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return png.Encode(f, img)
}
