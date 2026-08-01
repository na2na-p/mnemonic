package builder

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
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
	manifestUpdater     *manifestUpdater
	stringsXMLUpdater   *stringsXMLUpdater
	assetCopier         *assetCopier
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
		manifestUpdater:     newManifestUpdater(projectDir),
		stringsXMLUpdater:   newStringsXMLUpdater(projectDir),
		assetCopier:         newAssetCopier(projectDir),
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

	if err := p.manifestUpdater.Update(packageName); err != nil {
		return err
	}

	if err := p.stringsXMLUpdater.Update(appName); err != nil {
		return err
	}

	if assetsDir != "" {
		if err := p.assetCopier.Copy(assetsDir); err != nil {
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
