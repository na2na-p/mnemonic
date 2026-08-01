package builder

import (
	"errors"
	"fmt"
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

// TemplatePreparer はAndroidプロジェクトテンプレートの準備を、目的ごとの
// 協力者（JNIライブラリ抽出/プラグイン配置/Javaソース生成/build.gradle更新/
// AndroidManifest.xml更新/strings.xml更新/アセットコピー/アイコン準備）に
// 委譲して束ねるオーケストレータ。各協力者の実装や不変条件はそれぞれの型が
// 保持し、TemplatePreparer自身はPrepareでの呼び出し順序のみに責任を持つ。
//
// why not（SDL2ソース取得だけ専用の協力者型を持たない理由）: SDL2ソース取得の
// 実体は既にSDL2SourceFetcher/SDL2SourceCache（sdl2_sources.go）にあり、
// fetchSDL2Sourcesはjavaディレクトリの作成とその呼び出しのみを行う薄い委譲
// （独自の不変条件を持たない）であるため、他の協力者と同格の型を新設するほどの
// 実装がない。
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
	iconProvisioner     *iconProvisioner
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
		iconProvisioner:     newIconProvisioner(projectDir),
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
			return p.iconProvisioner.UpdateIcon(iconPath)
		}
	}

	return p.iconProvisioner.CreateDefault()
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
