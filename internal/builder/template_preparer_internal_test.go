package builder

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildSDL2CacheFixture は指定したcacheDir配下にSDL2 Javaソース一式を持つ
// 有効なSDL2SourceCacheを構築する。
//
// why: fetchSDL2Sources/Prepareは実ネットワークに触れずテストを完走させる
// ためにキャッシュ経由の復元経路を使う必要がある（テンプレート準備の
// 単体テスト共通の制約。template_preparer_test.goのtestSDL2Cacheと同じ
// 方針だが、このファイルはpackage builder（white-box）のため独立して定義する）。
func buildSDL2CacheFixture(t *testing.T) *SDL2SourceCache {
	t.Helper()

	sourcesDir := t.TempDir()
	appDir := filepath.Join(sourcesDir, "org", "libsdl", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o750))

	for _, name := range SDL2RequiredFiles {
		require.NoError(t, os.WriteFile(filepath.Join(appDir, name), []byte("dummy content"), 0o600))
	}

	cache := NewSDL2SourceCache(t.TempDir())
	require.NoError(t, cache.Save(sourcesDir))

	return cache
}

func TestTemplatePreparer_CreateDefaultIcon(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 全ての解像度でデフォルトアイコンが生成される", func(t *testing.T) {
		t.Parallel()

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.createDefaultIcon())

		resDir := filepath.Join(projectDir, "app", "src", "main", "res")
		for _, density := range iconMipmapDensities {
			iconFile := filepath.Join(resDir, "mipmap-"+density, "ic_launcher.png")
			assert.FileExists(t, iconFile, "mipmap-%s にアイコンがありません", density)
		}
	})

	t.Run("正常系: 各解像度で正しいサイズのアイコンが生成される", func(t *testing.T) {
		t.Parallel()

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.createDefaultIcon())

		resDir := filepath.Join(projectDir, "app", "src", "main", "res")
		for density, expectedSize := range defaultIconDensitySizes {
			iconFile := filepath.Join(resDir, "mipmap-"+density, "ic_launcher.png")

			f, err := os.Open(iconFile) //nolint:gosec // テストで自身が生成した一時ファイルを読む用途のため妥当
			require.NoError(t, err)

			img, decodeErr := png.Decode(f)
			require.NoError(t, decodeErr)
			_ = f.Close()

			bounds := img.Bounds()
			assert.Equal(t, expectedSize, bounds.Dx(), "density=%s", density)
			assert.Equal(t, expectedSize, bounds.Dy(), "density=%s", density)
		}
	})

	t.Run("正常系: 有効なPNGファイルが生成される", func(t *testing.T) {
		t.Parallel()

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.createDefaultIcon())

		iconFile := filepath.Join(projectDir, "app", "src", "main", "res", "mipmap-mdpi", "ic_launcher.png")
		f, err := os.Open(iconFile) //nolint:gosec // テストで自身が生成した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		defer func() { _ = f.Close() }()

		_, decodeErr := png.Decode(f)
		require.NoError(t, decodeErr)
	})
}

func TestTemplatePreparer_FetchSDL2Sources(t *testing.T) {
	t.Parallel()

	t.Run("正常系: SDL2 Javaソースが作成される", func(t *testing.T) {
		t.Parallel()

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, buildSDL2CacheFixture(t))

		require.NoError(t, p.fetchSDL2Sources())

		sdlAppDir := filepath.Join(projectDir, "app", "src", "main", "java", "org", "libsdl", "app")
		assert.DirExists(t, sdlAppDir)
		assert.FileExists(t, filepath.Join(sdlAppDir, "SDLActivity.java"))
	})

	t.Run("正常系: 指定したキャッシュから復元される", func(t *testing.T) {
		t.Parallel()

		cache := buildSDL2CacheFixture(t)
		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, cache)

		require.NoError(t, p.fetchSDL2Sources())

		// キャッシュ経由で復元されたことを検証する（実ネットワークに
		// 触れていれば、このキャッシュには存在しないダミー内容
		// "dummy content"には一致しないはずである）。
		sdlActivity := filepath.Join(projectDir, "app", "src", "main", "java", "org", "libsdl", "app", "SDLActivity.java")
		content, err := os.ReadFile(sdlActivity) //nolint:gosec // テストで自身が生成した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, "dummy content", string(content))
	})

	t.Run("異常系: キャッシュ復元に失敗した場合ErrTemplatePreparerとErrSDL2SourceFetchの両方を満たす", func(t *testing.T) {
		t.Parallel()

		// マーカー/バージョンファイルは有効だがorgディレクトリを欠いた
		// 壊れたキャッシュを用意し、実ネットワークに触れずに
		// Cache.RestoreToを確実に失敗させる（レビュー指摘: 一般センチネル
		// ErrTemplatePreparerと具体センチネルErrSDL2SourceFetchの両方を
		// errors.Isで検証する）。
		cacheDir := t.TempDir()
		cache := NewSDL2SourceCache(cacheDir)
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), SDL2CacheMarkerFile),
			[]byte(time.Now().Format(time.RFC3339Nano)),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), SDL2CacheVersionFile),
			[]byte(SDL2CacheCurrentVersion),
			0o600,
		))
		// "org" ディレクトリは意図的に作成しない

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, cache)

		err := p.fetchSDL2Sources()

		require.Error(t, err)
		require.ErrorIs(t, err, ErrTemplatePreparer)
		require.ErrorIs(t, err, ErrSDL2SourceFetch)
	})
}

// TestTemplatePreparer_CopyPluginsToJNILibs はcopyPluginsToJNILibsの挙動を
// 検証する。
func TestTemplatePreparer_CopyPluginsToJNILibs(t *testing.T) {
	t.Parallel()

	t.Run("正常系: pluginsInfoがnilの場合は何もしない", func(t *testing.T) {
		t.Parallel()

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.copyPluginsToJNILibs(nil))

		jniLibsDir := filepath.Join(projectDir, "app", "src", "main", "jniLibs")
		assert.NoDirExists(t, jniLibsDir)
	})

	t.Run("正常系: 各ABIのjniLibsディレクトリにプラグインが配置される", func(t *testing.T) {
		t.Parallel()

		pluginSrcDir := t.TempDir()
		extransPath := filepath.Join(pluginSrcDir, "libextrans.so")
		require.NoError(t, os.WriteFile(extransPath, []byte("extrans so content"), 0o600))

		pluginsInfo := &PluginsInfo{
			Plugins: map[string]PluginInfo{
				"extrans": {
					Name: "extrans",
					Paths: map[string]string{
						"arm64-v8a": extransPath,
					},
				},
			},
		}

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.copyPluginsToJNILibs(pluginsInfo))

		destPath := filepath.Join(projectDir, "app", "src", "main", "jniLibs", "arm64-v8a", "libextrans.so")
		content, err := os.ReadFile(destPath) //nolint:gosec // テストで自身が生成した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, "extrans so content", string(content))

		// SupportedABIsの全ディレクトリが作成される（プラグインが無いABIも含む）
		for _, abi := range SupportedABIs {
			assert.DirExists(t, filepath.Join(projectDir, "app", "src", "main", "jniLibs", abi))
		}
	})

	t.Run("正常系: プラグインファイルが見つからない場合はスキップする", func(t *testing.T) {
		t.Parallel()

		pluginsInfo := &PluginsInfo{
			Plugins: map[string]PluginInfo{
				"extrans": {
					Name: "extrans",
					Paths: map[string]string{
						"arm64-v8a": filepath.Join(t.TempDir(), "does-not-exist.so"),
					},
				},
			},
		}

		projectDir := t.TempDir()
		p := NewTemplatePreparer(projectDir, nil)

		require.NoError(t, p.copyPluginsToJNILibs(pluginsInfo))
	})
}

// miniForkJavaFixture はfork版KirikiriSDL2Activity.javaの構造を模した最小限の
// フィクスチャ。generateActivityJavaの変換ロジック（fork由来メソッドの保持、
// パッケージ書き換え、独自メンバの注入）を単体で検証するために使う。
const miniForkJavaFixture = `package pw.uyjulian.krkrsdl2;

import android.app.Activity;
import android.app.AlertDialog;
import org.libsdl.app.SDLActivity;

public class KirikiriSDL2Activity extends SDLActivity {
    public static int showSelectList(final String title, final String[] items) {
        return -1;
    }
}
`

// TestGenerateActivityJava はgenerateActivityJavaの変換ロジックを検証する。
func TestGenerateActivityJava(t *testing.T) {
	t.Parallel()

	t.Run("正常系: package宣言が対象パッケージへ書き換えられる", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		assert.Contains(t, got, "package com.example.game;")
		assert.NotContains(t, got, "package pw.uyjulian.krkrsdl2;")
	})

	t.Run("正常系: fork由来のメソッドが保持される", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		assert.Contains(t, got, "public static int showSelectList(final String title, final String[] items)")
		assert.Contains(t, got, "import android.app.AlertDialog;")
	})

	t.Run("正常系: mnemonic独自メンバが注入される", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		assert.Contains(t, got, "protected void onCreate(Bundle savedInstanceState)")
		assert.Contains(t, got, "protected String[] getArguments()")
		assert.Contains(t, got, "private void copyAssetsToInternal()")
		assert.Contains(t, got, `"-holdalpha=yes"`)
	})

	t.Run("正常系: mnemonic独自importが重複なく追加される", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		for _, imp := range mnemonicJavaImports {
			assert.Equal(t, 1, strings.Count(got, imp), "import %sが重複または欠落している", imp)
		}
	})

	t.Run("正常系: 括弧の対応が保たれ構文的に妥当", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		assert.Equal(t, strings.Count(got, "{"), strings.Count(got, "}"), "括弧の数が一致しません")
	})

	t.Run("正常系: クラスレベルのdocコメントが注入される", func(t *testing.T) {
		t.Parallel()

		got, err := generateActivityJava(miniForkJavaFixture, "com.example.game")

		require.NoError(t, err)
		assert.Contains(t, got, "KirikiriSDL2用のメインアクティビティ")
		assert.Equal(t, 1, strings.Count(got, "KirikiriSDL2用のメインアクティビティ"), "docコメントが重複している")
	})

	t.Run("異常系: package宣言が無いソースはエラーになる", func(t *testing.T) {
		t.Parallel()

		_, err := generateActivityJava("public class Foo {}\n", "com.example.game")

		require.Error(t, err)
		require.ErrorIs(t, err, ErrTemplatePreparer)
	})
}
