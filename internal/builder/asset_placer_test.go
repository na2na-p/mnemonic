package builder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// newAndroidProjectFixture はapp/src/main/assetsとapp/build.gradleを持つ
// Androidプロジェクト構造を作成する。
func newAndroidProjectFixture(t *testing.T) string {
	t.Helper()

	projectPath := filepath.Join(t.TempDir(), "android_project")
	assetsDir := filepath.Join(projectPath, "app", "src", "main", "assets")
	require.NoError(t, os.MkdirAll(assetsDir, 0o750))

	buildGradle := filepath.Join(projectPath, "app", "build.gradle")
	content := `android {
    namespace "com.example.game"
    defaultConfig {
        applicationId "com.example.game"
    }
}`
	require.NoError(t, os.WriteFile(buildGradle, []byte(content), 0o600))

	return projectPath
}

// newSourceAssetsFixture はテスト用のソースアセットディレクトリを作成する。
func newSourceAssetsFixture(t *testing.T) string {
	t.Helper()

	sourceDir := filepath.Join(t.TempDir(), "source_assets")
	require.NoError(t, os.MkdirAll(sourceDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "data.xp3"), []byte("xp3 archive content"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bgm.ogg"), []byte("ogg audio content"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "startup.tjs"), []byte("// startup script"), 0o600))

	subdir := filepath.Join(sourceDir, "images")
	require.NoError(t, os.MkdirAll(subdir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "title.png"), []byte("png image content"), 0o600))

	return sourceDir
}

func TestAssetPlacer_PlaceAssets(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 全てのアセットがapp/src/main/assets/に配置される", func(t *testing.T) {
		t.Parallel()

		projectPath := newAndroidProjectFixture(t)
		sourceDir := newSourceAssetsFixture(t)

		p := builder.NewAssetPlacer(projectPath, nil)

		result, err := p.PlaceAssets(sourceDir, nil)
		require.NoError(t, err)

		assetsDir := filepath.Join(projectPath, "app", "src", "main", "assets")
		assert.FileExists(t, filepath.Join(assetsDir, "data.xp3"))
		assert.FileExists(t, filepath.Join(assetsDir, "bgm.ogg"))
		assert.FileExists(t, filepath.Join(assetsDir, "startup.tjs"))
		assert.FileExists(t, filepath.Join(assetsDir, "images", "title.png"))
		assert.Equal(t, 4, result.TotalFiles)
	})

	t.Run("正常系: 配置結果（ファイル数・サイズ・配置ファイル一覧）が正しく返される", func(t *testing.T) {
		t.Parallel()

		projectPath := newAndroidProjectFixture(t)
		sourceDir := newSourceAssetsFixture(t)

		p := builder.NewAssetPlacer(projectPath, nil)

		result, err := p.PlaceAssets(sourceDir, nil)
		require.NoError(t, err)

		assert.Equal(t, 4, result.TotalFiles)

		expectedSize := int64(len("xp3 archive content") + len("ogg audio content") + len("// startup script") + len("png image content"))
		assert.Equal(t, expectedSize, result.TotalSize)

		assert.Contains(t, result.PlacedFiles, "data.xp3")
		assert.Contains(t, result.PlacedFiles, "bgm.ogg")
		assert.Contains(t, result.PlacedFiles, "startup.tjs")
	})

	t.Run("正常系: 除外パターンに一致するファイルは配置されない", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name            string
			excludePatterns []string
			expectedExclude []string
		}{
			{name: "正常系: bakファイルを除外", excludePatterns: []string{"*.bak"}, expectedExclude: []string{"backup.bak"}},
			{name: "正常系: システムファイルを除外", excludePatterns: []string{"thumbs.db", ".DS_Store"}, expectedExclude: []string{"thumbs.db", ".DS_Store"}},
			{name: "正常系: 複数のパターンを除外", excludePatterns: []string{"*.tmp", "*.bak"}, expectedExclude: []string{"temp.tmp", "backup.bak"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectPath := newAndroidProjectFixture(t)
				sourceDir := filepath.Join(t.TempDir(), "source_with_excludes")
				require.NoError(t, os.MkdirAll(sourceDir, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "data.xp3"), []byte("data"), 0o600))
				for _, excluded := range tc.expectedExclude {
					require.NoError(t, os.WriteFile(filepath.Join(sourceDir, excluded), []byte("excluded"), 0o600))
				}

				p := builder.NewAssetPlacer(projectPath, nil)

				result, err := p.PlaceAssets(sourceDir, tc.excludePatterns)
				require.NoError(t, err)

				assetsDir := filepath.Join(projectPath, "app", "src", "main", "assets")
				assert.FileExists(t, filepath.Join(assetsDir, "data.xp3"))
				for _, excluded := range tc.expectedExclude {
					assert.NoFileExists(t, filepath.Join(assetsDir, excluded))
				}
				assert.Equal(t, 1, result.TotalFiles)
			})
		}
	})
}

func TestAssetPlacer_PlaceAssets_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("異常系: ソースディレクトリが存在しない場合", func(t *testing.T) {
		t.Parallel()

		projectPath := newAndroidProjectFixture(t)
		nonexistentSource := filepath.Join(t.TempDir(), "nonexistent_source")

		p := builder.NewAssetPlacer(projectPath, nil)

		_, err := p.PlaceAssets(nonexistentSource, nil)

		require.ErrorIs(t, err, builder.ErrAssetPlacement)
		assert.ErrorContains(t, err, "does not exist")
	})

	t.Run("異常系: プロジェクトパスが無効な場合", func(t *testing.T) {
		t.Parallel()

		invalidProjectPath := filepath.Join(t.TempDir(), "nonexistent_project")
		sourceDir := newSourceAssetsFixture(t)

		p := builder.NewAssetPlacer(invalidProjectPath, nil)

		_, err := p.PlaceAssets(sourceDir, nil)

		require.ErrorIs(t, err, builder.ErrAssetPlacement)
		assert.ErrorContains(t, err, "does not exist")
	})

	t.Run("異常系: assetsディレクトリが存在しない場合", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectPath, 0o750))
		sourceDir := newSourceAssetsFixture(t)

		p := builder.NewAssetPlacer(projectPath, nil)

		_, err := p.PlaceAssets(sourceDir, nil)

		require.ErrorIs(t, err, builder.ErrAssetPlacement)
		assert.ErrorContains(t, err, "does not exist")
	})
}

func TestAssetPlacer_ConfigureBuildGradle(t *testing.T) {
	t.Parallel()

	t.Run("正常系: aaptOptions.noCompressが正しく設定される（Groovy形式）", func(t *testing.T) {
		t.Parallel()

		projectPath := newAndroidProjectFixture(t)

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg", ".mp3"}}

		require.NoError(t, p.ConfigureBuildGradle(config))

		content, err := os.ReadFile(filepath.Join(projectPath, "app", "build.gradle"))
		require.NoError(t, err)

		assert.Contains(t, string(content), "aaptOptions")
		assert.Contains(t, string(content), "noCompress")
		assert.Contains(t, string(content), "'.ogg'")
		assert.Contains(t, string(content), "'.mp3'")
	})

	t.Run("正常系: androidブロック外のトップレベル定義が保持される", func(t *testing.T) {
		t.Parallel()

		// ReplaceAllStringFuncでマッチ範囲のみを置換するため、androidブロック外の
		// トップレベル定義（例: 末尾のdependencies{}）が保持されることを確認する
		// （addNewAaptOptionsのwhy not参照）。
		projectPath := filepath.Join(t.TempDir(), "surrounding_blocks_project")
		buildGradle := filepath.Join(projectPath, "app", "build.gradle")
		require.NoError(t, os.MkdirAll(filepath.Dir(buildGradle), 0o750))
		content := `// top-level comment
android {
    namespace "com.example.game"
}

dependencies {
    implementation "androidx.core:core-ktx:1.12.0"
}
`
		require.NoError(t, os.WriteFile(buildGradle, []byte(content), 0o600))

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg"}}

		require.NoError(t, p.ConfigureBuildGradle(config))

		result, err := os.ReadFile(buildGradle)
		require.NoError(t, err)
		text := string(result)

		assert.Contains(t, text, "// top-level comment")
		assert.Contains(t, text, "dependencies {")
		assert.Contains(t, text, `implementation "androidx.core:core-ktx:1.12.0"`)
		assert.Contains(t, text, "aaptOptions")
	})

	t.Run("正常系: aaptOptions.noCompressが正しく設定される（Kotlin形式）", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "kotlin_project")
		buildGradle := filepath.Join(projectPath, "app", "build.gradle.kts")
		require.NoError(t, os.MkdirAll(filepath.Dir(buildGradle), 0o750))
		content := `android {
    namespace = "com.example.game"
    defaultConfig {
        applicationId = "com.example.game"
    }
}`
		require.NoError(t, os.WriteFile(buildGradle, []byte(content), 0o600))

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg", ".mp3"}}

		require.NoError(t, p.ConfigureBuildGradle(config))

		result, err := os.ReadFile(buildGradle)
		require.NoError(t, err)

		assert.Contains(t, string(result), "aaptOptions")
		assert.Contains(t, string(result), "noCompress")
		assert.Contains(t, string(result), "listOf")
		assert.Contains(t, string(result), `".ogg"`)
		assert.Contains(t, string(result), `".mp3"`)
	})

	t.Run("正常系: 既存の設定を壊さない", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "existing_settings_project")
		buildGradle := filepath.Join(projectPath, "app", "build.gradle")
		require.NoError(t, os.MkdirAll(filepath.Dir(buildGradle), 0o750))
		content := `android {
    namespace "com.example.game"
    defaultConfig {
        applicationId "com.example.game"
        minSdk 21
        targetSdk 34
    }
    buildTypes {
        release {
            minifyEnabled true
        }
    }
}`
		require.NoError(t, os.WriteFile(buildGradle, []byte(content), 0o600))

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg"}}

		require.NoError(t, p.ConfigureBuildGradle(config))

		result, err := os.ReadFile(buildGradle)
		require.NoError(t, err)
		text := string(result)

		assert.Contains(t, text, `namespace "com.example.game"`)
		assert.Contains(t, text, "minSdk 21")
		assert.Contains(t, text, "targetSdk 34")
		assert.Contains(t, text, "buildTypes")
		assert.Contains(t, text, "minifyEnabled true")
		assert.Contains(t, text, "aaptOptions")
		assert.Contains(t, text, "noCompress")
	})

	t.Run("正常系: 様々なnoCompress拡張子で設定できる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name       string
			extensions []string
		}{
			{name: "正常系: 単一の拡張子", extensions: []string{".ogg"}},
			{name: "正常系: 複数の拡張子", extensions: []string{".ogg", ".mp3", ".wav"}},
			{name: "正常系: 多数の拡張子", extensions: []string{".ogg", ".mp3", ".wav", ".flac", ".opus", ".xp3"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectPath := newAndroidProjectFixture(t)

				p := builder.NewAssetPlacer(projectPath, nil)
				config := builder.AssetConfig{NoCompressExtensions: tc.extensions}

				require.NoError(t, p.ConfigureBuildGradle(config))

				content, err := os.ReadFile(filepath.Join(projectPath, "app", "build.gradle"))
				require.NoError(t, err)

				for _, ext := range tc.extensions {
					assert.Contains(t, string(content), "'"+ext+"'")
				}
			})
		}
	})

	t.Run("正常系: 既存noCompressが2行目以降にある場合も構文を壊さない", func(t *testing.T) {
		t.Parallel()

		// レビュー指摘: noCompressLinePatternのgroup1（直前行との改行を含む
		// 先頭空白）を捨てて置換すると、noCompressがaaptOptionsブロックの
		// 2行目以降にある場合に直前行と連結され構文エラーになる回帰テスト。
		projectPath := filepath.Join(t.TempDir(), "existing_no_compress_project")
		buildGradle := filepath.Join(projectPath, "app", "build.gradle")
		require.NoError(t, os.MkdirAll(filepath.Dir(buildGradle), 0o750))
		content := `android {
    namespace "com.example.game"
    aaptOptions {
        cruncherEnabled false
        noCompress '.wav'
    }
}`
		require.NoError(t, os.WriteFile(buildGradle, []byte(content), 0o600))

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg"}}

		require.NoError(t, p.ConfigureBuildGradle(config))

		result, err := os.ReadFile(buildGradle)
		require.NoError(t, err)
		text := string(result)

		assert.Contains(t, text, "cruncherEnabled false\n        noCompress '.ogg'")
		assert.NotContains(t, text, "cruncherEnabled falsenoCompress")
		assert.NotContains(t, text, "false        noCompress")
	})

	t.Run("異常系: build.gradleが存在しない場合", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "no_gradle_project")
		require.NoError(t, os.MkdirAll(filepath.Join(projectPath, "app"), 0o750))

		p := builder.NewAssetPlacer(projectPath, nil)
		config := builder.AssetConfig{NoCompressExtensions: []string{".ogg"}}

		err := p.ConfigureBuildGradle(config)

		require.ErrorIs(t, err, builder.ErrAssetPlacement)
		assert.ErrorContains(t, err, "not found")
	})
}

func TestAssetPlacer_ValidatePlacement(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 必要なファイルが全て配置されている場合にtrue", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "project_with_assets")
		assetsDir := filepath.Join(projectPath, "app", "src", "main", "assets")
		require.NoError(t, os.MkdirAll(assetsDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "data.xp3"), []byte("xp3 content"), 0o600))

		p := builder.NewAssetPlacer(projectPath, nil)

		ok, err := p.ValidatePlacement()

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("正常系: ファイルが欠けている場合にfalse", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "empty_project")
		assetsDir := filepath.Join(projectPath, "app", "src", "main", "assets")
		require.NoError(t, os.MkdirAll(assetsDir, 0o750))

		p := builder.NewAssetPlacer(projectPath, nil)

		ok, err := p.ValidatePlacement()

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("正常系: assetsディレクトリが存在しない場合にfalse", func(t *testing.T) {
		t.Parallel()

		projectPath := filepath.Join(t.TempDir(), "no_assets_dir_project")
		require.NoError(t, os.MkdirAll(projectPath, 0o750))

		p := builder.NewAssetPlacer(projectPath, nil)

		ok, err := p.ValidatePlacement()

		require.NoError(t, err)
		assert.False(t, ok)
	})
}
