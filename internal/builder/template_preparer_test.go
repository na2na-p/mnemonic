package builder_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// minimalManifestXML はTemplatePreparerのフルパイプライン(Prepare)を通す
// テストで使う最小限の有効なAndroidManifest.xml。
const minimalManifestXML = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application></application>
</manifest>
`

// addMinimalManifest はprojectDirにminimalManifestXMLを書き込む。
// Prepare()はupdateManifestステップでAndroidManifest.xmlの存在を要求するため、
// 個別ステップ（.so抽出等）だけを検証したいテストでも完走できるようにする。
func addMinimalManifest(t *testing.T, projectDir string) {
	t.Helper()

	manifestDir := filepath.Join(projectDir, "app", "src", "main")
	require.NoError(t, os.MkdirAll(manifestDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(minimalManifestXML), 0o600))
}

// addMinimalBuildGradle はprojectDirに最小限のbuild.gradleを書き込む。
func addMinimalBuildGradle(t *testing.T, projectDir string) {
	t.Helper()

	appDir := filepath.Join(projectDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
}

// testSDL2Cache はSDL2 Javaソース一式を持つ、有効なSDL2SourceCacheを返す。
//
// why: TemplatePreparer.Prepareはfetch_sdl2_sources()を無条件に呼ぶため、
// 実ネットワークに触れずにテストを完走させるには、事前に有効なキャッシュを
// 用意してSDL2SourceFetcher.Fetchがキャッシュ復元経路(Cache.RestoreTo)を
// 通るようにする必要がある（実ネットワークへのアクセス禁止という制約への
// 対応。builder.SDL2SourceCache.Save/IsValidの正規経路を使う）。
func testSDL2Cache(t *testing.T) *builder.SDL2SourceCache {
	t.Helper()

	sourcesDir := t.TempDir()
	appDir := filepath.Join(sourcesDir, "org", "libsdl", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o750))

	for _, name := range builder.SDL2RequiredFiles {
		require.NoError(t, os.WriteFile(filepath.Join(appDir, name), []byte("dummy content"), 0o600))
	}

	cache := builder.NewSDL2SourceCache(t.TempDir())
	require.NoError(t, cache.Save(sourcesDir))

	return cache
}

func writeZipFile(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // テスト用の固定パス
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

func TestTemplatePreparer_Prepare(t *testing.T) {
	t.Parallel()

	t.Run("正常系: すべての処理が成功するケース", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"lib/arm64-v8a/libmain.so":   []byte("fake so content"),
			"lib/armeabi-v7a/libmain.so": []byte("fake so content"),
		})

		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte(`
android {
    compileSdkVersion 30
    defaultConfig {
        applicationId "pw.uyjulian.krkrsdl2"
        minSdkVersion 16
        targetSdkVersion 30
    }
}
`), 0o600))

		manifestDir := filepath.Join(appDir, "src", "main")
		require.NoError(t, os.MkdirAll(manifestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		jniLibsDir := filepath.Join(projectDir, "app", "src", "main", "jniLibs")
		assert.FileExists(t, filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		assert.FileExists(t, javaFile)

		stringsXML := filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml")
		content, err := os.ReadFile(stringsXML)
		require.NoError(t, err)
		assert.Contains(t, string(content), "My Game")

		// デフォルトアイコンが生成されていることを確認（icon_path未指定のため）
		resDir := filepath.Join(projectDir, "app", "src", "main", "res")
		assert.FileExists(t, filepath.Join(resDir, "mipmap-mdpi", "ic_launcher.png"))
	})

	t.Run("異常系: APKファイルが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrJniLibsNotFound)
		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "ベースAPKが見つかりません")
	})
}

func TestTemplatePreparer_ExtractJNILibs(t *testing.T) {
	t.Parallel()

	t.Run("正常系: APKから.soファイルを抽出", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"lib/arm64-v8a/libmain.so":   []byte("arm64 so content"),
			"lib/armeabi-v7a/libmain.so": []byte("armeabi so content"),
			"lib/x86/libmain.so":         []byte("x86 so content"),
			"lib/x86_64/libmain.so":      []byte("x86_64 so content"),
		})
		addMinimalBuildGradle(t, projectDir)
		addMinimalManifest(t, projectDir)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		jniLibsDir := filepath.Join(projectDir, "app", "src", "main", "jniLibs")
		assert.FileExists(t, filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "armeabi-v7a", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "x86", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "x86_64", "libmain.so"))

		content, err := os.ReadFile(filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))
		require.NoError(t, err)
		assert.Equal(t, "arm64 so content", string(content))
	})

	t.Run("異常系: APK内に.soファイルがない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"AndroidManifest.xml": []byte("manifest content"),
			"classes.dex":         []byte("dex content"),
		})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrJniLibsNotFound)
		assert.ErrorContains(t, err, ".soファイルが見つかりません")
	})

	t.Run("異常系: APKファイルが不正な場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(projectDir, "krkrsdl2_universal.apk"), []byte("invalid zip content"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "無効なAPKファイルです")
	})

	t.Run("正常系: サポートされるABIのみ抽出される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name              string
			abi               string
			expectedExtracted bool
		}{
			{name: "正常系: arm64-v8a はサポート対象", abi: "arm64-v8a", expectedExtracted: true},
			{name: "正常系: armeabi-v7a はサポート対象", abi: "armeabi-v7a", expectedExtracted: true},
			{name: "正常系: x86 はサポート対象", abi: "x86", expectedExtracted: true},
			{name: "正常系: x86_64 はサポート対象", abi: "x86_64", expectedExtracted: true},
			{name: "正常系: mips はサポート対象外", abi: "mips", expectedExtracted: false},
			{name: "正常系: armeabi はサポート対象外", abi: "armeabi", expectedExtracted: false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := filepath.Join(t.TempDir(), "project")
				require.NoError(t, os.MkdirAll(projectDir, 0o750))

				writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
					"lib/" + tc.abi + "/libtest.so": []byte("test so content"),
					"lib/arm64-v8a/libmain.so":      []byte("arm64 so content"),
				})
				addMinimalBuildGradle(t, projectDir)
				addMinimalManifest(t, projectDir)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

				soFile := filepath.Join(projectDir, "app", "src", "main", "jniLibs", tc.abi, "libtest.so")
				if tc.expectedExtracted {
					assert.FileExists(t, soFile, "ABI: %s", tc.abi)
				} else {
					assert.NoFileExists(t, soFile, "ABI: %s", tc.abi)
				}
			})
		}
	})
}

func newProjectWithAPK(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o750))
	writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
		"lib/arm64-v8a/libmain.so": []byte("so content"),
	})

	return projectDir
}

func TestTemplatePreparer_UpdateJavaSource(t *testing.T) {
	t.Parallel()

	t.Run("正常系: パッケージ名が正しく反映される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			packageName string
		}{
			{name: "正常系: 標準的なパッケージ名", packageName: "com.example.game"},
			{name: "正常系: 日本ドメインのパッケージ名", packageName: "jp.example.mygame"},
			{name: "正常系: 数字を含むパッケージ名", packageName: "org.test.app123"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := newFullyPreparableProject(t)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare(tc.packageName, "My Game", "", "", nil))

				packagePath := filepath.Join(strings.Split(tc.packageName, ".")...)
				javaFile := filepath.Join(projectDir, "app", "src", "main", "java", packagePath, "KirikiriSDL2Activity.java")
				assert.FileExists(t, javaFile)

				content, err := os.ReadFile(javaFile)
				require.NoError(t, err)
				assert.Contains(t, string(content), "package "+tc.packageName+";")
			})
		}
	})

	t.Run("正常系: 古いJavaディレクトリが削除される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		oldJavaDir := filepath.Join(projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2")
		require.NoError(t, os.MkdirAll(oldJavaDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(oldJavaDir, "KirikiriSDL2Activity.java"), []byte("old content"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		assert.NoDirExists(t, oldJavaDir)
	})

	t.Run("正常系: getArgumentsにholdalpha=yesが含まれる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		assert.Contains(t, string(content), `"-holdalpha=yes"`)
	})

	t.Run("正常系: getArgumentsにSIMD無効化フラグが含まれる（C実装を使用）", func(t *testing.T) {
		t.Parallel()

		// ARMデバイスではSIMDeエミュレーションに問題があるため、純粋なC実装の
		// ブレンド関数を使用することでアルファブレンディングの互換性を確保する
		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		assert.Contains(t, text, `"-cpummx=no"`)
		assert.Contains(t, text, `"-cpusse=no"`)
		assert.Contains(t, text, `"-cpusse2=no"`)
	})

	t.Run("正常系: showSelectListメソッドが含まれる（krkrsdl2ネイティブがJNI経由で呼び出す）", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		assert.Contains(t, text, "public static int showSelectList(final String title, final String[] items)")
	})
}

func TestTemplatePreparer_UpdateBuildGradle(t *testing.T) {
	t.Parallel()

	newBuildGradleProject := func(t *testing.T, content string) string {
		t.Helper()

		projectDir := filepath.Join(t.TempDir(), "project")
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte(content), 0o600))
		addMinimalManifest(t, projectDir)

		return projectDir
	}

	t.Run("正常系: namespaceが追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `namespace "com.example.game"`)
	})

	t.Run("正常系: compileSdkVersionが34に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "compileSdkVersion 34")
	})

	t.Run("正常系: targetSdkVersionが34に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        targetSdkVersion 30\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "targetSdkVersion 34")
	})

	t.Run("正常系: minSdkVersionが21に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        minSdkVersion 16\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "minSdkVersion 21")
	})

	t.Run("正常系: CMake設定が削除される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, `
android {
    compileSdkVersion 30
    externalNativeBuild {
        cmake {
            path "CMakeLists.txt"
        }
    }
    defaultConfig {
        externalNativeBuild {
            ndk {
                abiFilters 'arm64-v8a', 'armeabi-v7a'
            }
        }
    }
}
`)
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		text := string(content)
		assert.NotContains(t, strings.ToLower(text), "cmake")
		assert.NotContains(t, text, "externalNativeBuild")
	})

	t.Run("正常系: applicationIdが更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        applicationId \"pw.uyjulian.krkrsdl2\"\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `applicationId "com.example.game"`)
	})

	t.Run("異常系: build.gradleが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := newProjectWithAPK(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "build.gradleが見つかりません")
	})
}

func TestTemplatePreparer_UpdateManifest(t *testing.T) {
	t.Parallel()

	newManifestProject := func(t *testing.T, content string) string {
		t.Helper()

		projectDir := filepath.Join(t.TempDir(), "project")
		manifestDir := filepath.Join(projectDir, "app", "src", "main")
		require.NoError(t, os.MkdirAll(manifestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(content), 0o600))
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))

		return projectDir
	}

	t.Run("正常系: package属性が削除される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), `package="`)
	})

	t.Run("正常系: android:exported=trueがactivityに追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:exported="true"`)
	})

	t.Run("正常系: android:exported=trueがserviceに追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <service android:name=".MyService">
        </service>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:exported="true"`)
	})

	t.Run("正常系: 既にexportedがある場合は重複追加しない", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity" android:exported="false">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Equal(t, 1, countOccurrences(string(content), `android:exported="`))
	})

	t.Run("異常系: AndroidManifest.xmlが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := newProjectWithAPK(t)
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "AndroidManifest.xmlが見つかりません")
	})
}

func newFullyPreparableProject(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	appDir := filepath.Join(projectDir, "app")
	manifestDir := filepath.Join(appDir, "src", "main")
	require.NoError(t, os.MkdirAll(manifestDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application></application>
</manifest>
`), 0o600))
	writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

	return projectDir
}

func TestTemplatePreparer_UpdateStringsXML(t *testing.T) {
	t.Parallel()

	t.Run("正常系: strings.xmlが作成される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		stringsXML := filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml")
		content, err := os.ReadFile(stringsXML)
		require.NoError(t, err)
		assert.Contains(t, string(content), `<string name="app_name">My Game</string>`)
	})

	t.Run("正常系: 既存のstrings.xmlが更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		valuesDir := filepath.Join(projectDir, "app", "src", "main", "res", "values")
		require.NoError(t, os.MkdirAll(valuesDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "strings.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">Old Name</string>
    <string name="other">Other Value</string>
</resources>
`), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "New Game Name", "", "", nil))

		content, err := os.ReadFile(filepath.Join(valuesDir, "strings.xml"))
		require.NoError(t, err)
		text := string(content)
		assert.Contains(t, text, `<string name="app_name">New Game Name</string>`)
		assert.NotContains(t, text, "Old Name")
		assert.Contains(t, text, "Other Value")
	})

	t.Run("正常系: XML特殊文字がエスケープされる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name           string
			appName        string
			expectedEscape string
		}{
			{name: "正常系: アンパサンドのエスケープ", appName: "Game & Play", expectedEscape: "Game &amp; Play"},
			{name: "正常系: 山括弧のエスケープ", appName: "Game <Test>", expectedEscape: "Game &lt;Test&gt;"},
			{name: "正常系: ダブルクオートのエスケープ", appName: `Game "Quote"`, expectedEscape: "Game &quot;Quote&quot;"},
			{name: "正常系: シングルクオートのエスケープ", appName: "Game 'Single'", expectedEscape: "Game &#x27;Single&#x27;"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := newFullyPreparableProject(t)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare("com.example.game", tc.appName, "", "", nil))

				content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml"))
				require.NoError(t, err)
				assert.Contains(t, string(content), tc.expectedEscape)
			})
		}
	})
}

func TestTemplatePreparer_UpdateIcon(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 各解像度にアイコンがコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		iconPath := filepath.Join(t.TempDir(), "icon.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake png content"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", iconPath, nil))

		densities := []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}
		resDir := filepath.Join(projectDir, "app", "src", "main", "res")

		for _, density := range densities {
			iconFile := filepath.Join(resDir, "mipmap-"+density, "ic_launcher.png")
			require.FileExists(t, iconFile, "mipmap-%s にアイコンがありません", density)

			content, err := os.ReadFile(iconFile)
			require.NoError(t, err)
			assert.Equal(t, "fake png content", string(content))
		}
	})
}

func TestTemplatePreparer_CopyAssets(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ゲームファイルがコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		assetsSrc := filepath.Join(t.TempDir(), "game_files")
		require.NoError(t, os.MkdirAll(assetsSrc, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "data.xp3"), []byte("xp3 content"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "config.tjs"), []byte("config"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", assetsSrc, "", nil))

		assetsDest := filepath.Join(projectDir, "app", "src", "main", "assets", "data")
		assert.FileExists(t, filepath.Join(assetsDest, "data.xp3"))
		assert.FileExists(t, filepath.Join(assetsDest, "config.tjs"))
	})

	t.Run("正常系: サブディレクトリもコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		assetsSrc := filepath.Join(t.TempDir(), "game_files")
		require.NoError(t, os.MkdirAll(filepath.Join(assetsSrc, "scenario"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "scenario", "first.ks"), []byte("scenario"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", assetsSrc, "", nil))

		assetsDest := filepath.Join(projectDir, "app", "src", "main", "assets", "data")
		assert.FileExists(t, filepath.Join(assetsDest, "scenario", "first.ks"))
	})
}
