package builder_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// writeZip はfilesの内容でtmpDir/nameのZIPファイルを作成し、パスを返す。
func writeZip(t *testing.T, dir, name string, files map[string]string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	f, err := os.Create(path) //nolint:gosec // テスト用の固定パス
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for entryName, content := range files {
		w, err := zw.Create(entryName)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	return path
}

func validTemplateFiles() map[string]string {
	return map[string]string{
		"app/build.gradle": `android {
    namespace "com.krkrsdl2.template"
    defaultConfig {
        applicationId "com.krkrsdl2.template"
        versionCode 1
        versionName "1.0"
    }
}`,
		"app/src/main/AndroidManifest.xml": `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.krkrsdl2.template">
    <application
        android:label="Template App">
    </application>
</manifest>`,
		"settings.gradle": "include ':app'",
		"build.gradle":    "buildscript { }",
		"app/src/main/java/com/krkrsdl2/template/MainActivity.java": "",
	}
}

func TestProjectGenerator_ValidateTemplate(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 有効なテンプレートでtrueを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeZip(t, dir, "template.zip", validTemplateFiles())

		g := builder.NewProjectGenerator(path)

		ok, err := g.ValidateTemplate()

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("異常系: 必要なファイルが欠けている場合にErrInvalidTemplate", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeZip(t, dir, "incomplete.zip", map[string]string{"README.md": "incomplete"})

		g := builder.NewProjectGenerator(path)

		_, err := g.ValidateTemplate()

		assert.ErrorIs(t, err, builder.ErrInvalidTemplate)
	})

	t.Run("正常系/異常系: 必須ファイルの存在確認", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name          string
			files         []string
			expectedValid bool
		}{
			{
				name: "正常系: 全ての必須ファイルが存在",
				files: []string{
					"app/build.gradle", "app/src/main/AndroidManifest.xml",
					"settings.gradle", "build.gradle",
				},
				expectedValid: true,
			},
			{
				name:          "異常系: AndroidManifest.xmlが欠落",
				files:         []string{"app/build.gradle", "settings.gradle"},
				expectedValid: false,
			},
			{
				name:          "異常系: build.gradleが欠落",
				files:         []string{"app/src/main/AndroidManifest.xml"},
				expectedValid: false,
			},
			{
				name:          "異常系: 空のテンプレート",
				files:         nil,
				expectedValid: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				content := map[string]string{}
				for _, f := range tc.files {
					content[f] = "content of " + f
				}
				path := writeZip(t, dir, "template.zip", content)

				g := builder.NewProjectGenerator(path)
				ok, err := g.ValidateTemplate()

				if tc.expectedValid {
					require.NoError(t, err)
					assert.True(t, ok)
				} else {
					assert.ErrorIs(t, err, builder.ErrInvalidTemplate)
				}
			})
		}
	})
}

func TestProjectGenerator_Generate(t *testing.T) {
	t.Parallel()

	validConfig := builder.ProjectConfig{
		PackageName: "com.example.mygame",
		AppName:     "My Game",
		VersionCode: 10,
		VersionName: "2.0.0",
	}

	t.Run("正常系: 指定ディレクトリにプロジェクトが生成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templatePath := writeZip(t, dir, "template.zip", validTemplateFiles())
		outputDir := filepath.Join(dir, "output")
		require.NoError(t, os.Mkdir(outputDir, 0o750))

		g := builder.NewProjectGenerator(templatePath)
		result, err := g.Generate(outputDir, validConfig)

		require.NoError(t, err)
		assert.Equal(t, outputDir, result)
		assert.FileExists(t, filepath.Join(outputDir, "app", "build.gradle"))
		assert.FileExists(t, filepath.Join(outputDir, "app", "src", "main", "AndroidManifest.xml"))
		assert.FileExists(t, filepath.Join(outputDir, "settings.gradle"))
		assert.FileExists(t, filepath.Join(outputDir, "build.gradle"))
	})

	t.Run("正常系: package_nameとapp_nameが正しく設定される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templatePath := writeZip(t, dir, "template.zip", validTemplateFiles())
		outputDir := filepath.Join(dir, "output")
		require.NoError(t, os.Mkdir(outputDir, 0o750))

		g := builder.NewProjectGenerator(templatePath)
		_, err := g.Generate(outputDir, validConfig)
		require.NoError(t, err)

		manifestContent, err := os.ReadFile(filepath.Join(outputDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(manifestContent), `package="com.example.mygame"`)
		assert.Contains(t, string(manifestContent), `android:label="My Game"`)
		assert.NotContains(t, string(manifestContent), "com.krkrsdl2.template")
		assert.NotContains(t, string(manifestContent), "Template App")

		gradleContent, err := os.ReadFile(filepath.Join(outputDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(gradleContent), `applicationId "com.example.mygame"`)
		assert.Contains(t, string(gradleContent), `namespace "com.example.mygame"`)
		assert.Contains(t, string(gradleContent), "versionCode 10")
		assert.Contains(t, string(gradleContent), `versionName "2.0.0"`)
		assert.NotContains(t, string(gradleContent), "com.krkrsdl2.template")
		assert.NotContains(t, string(gradleContent), `versionName "1.0"`)
	})

	t.Run("正常系: 様々な設定でプロジェクトを生成できる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			packageName string
			appName     string
			versionCode int
			versionName string
		}{
			{name: "正常系: 標準的な設定", packageName: "com.krkr.visualnovel", appName: "Visual Novel", versionCode: 1, versionName: "1.0.0"},
			{name: "正常系: 長いパッケージ名", packageName: "jp.example.game.adventure", appName: "Adventure Game", versionCode: 50, versionName: "3.2.1"},
			{name: "正常系: 大きいバージョン番号", packageName: "com.test", appName: "Test", versionCode: 999, versionName: "99.99.99"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				templatePath := writeZip(t, dir, "template.zip", validTemplateFiles())
				outputDir := filepath.Join(dir, "output")
				require.NoError(t, os.Mkdir(outputDir, 0o750))

				config := builder.ProjectConfig{
					PackageName: tc.packageName,
					AppName:     tc.appName,
					VersionCode: tc.versionCode,
					VersionName: tc.versionName,
				}

				g := builder.NewProjectGenerator(templatePath)
				result, err := g.Generate(outputDir, config)

				require.NoError(t, err)
				assert.Equal(t, outputDir, result)

				manifestContent, err := os.ReadFile(filepath.Join(outputDir, "app", "src", "main", "AndroidManifest.xml"))
				require.NoError(t, err)
				assert.Contains(t, string(manifestContent), `package="`+tc.packageName+`"`)
				assert.Contains(t, string(manifestContent), `android:label="`+tc.appName+`"`)
			})
		}
	})
}

func TestProjectGenerator_Generate_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 無効なテンプレートでErrInvalidTemplate", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templatePath := writeZip(t, dir, "invalid.zip", map[string]string{"README.md": "not a valid template"})
		outputDir := filepath.Join(dir, "output")
		require.NoError(t, os.Mkdir(outputDir, 0o750))

		config := builder.ProjectConfig{PackageName: "com.example.game", AppName: "My Game", VersionCode: 1, VersionName: "1.0.0"}

		g := builder.NewProjectGenerator(templatePath)
		_, err := g.Generate(outputDir, config)

		assert.ErrorIs(t, err, builder.ErrInvalidTemplate)
	})

	t.Run("異常系: 出力ディレクトリが存在しない場合", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templatePath := writeZip(t, dir, "template.zip", validTemplateFiles())
		outputDir := filepath.Join(dir, "nonexistent", "output")

		config := builder.ProjectConfig{PackageName: "com.example.game", AppName: "My Game", VersionCode: 1, VersionName: "1.0.0"}

		g := builder.NewProjectGenerator(templatePath)
		_, err := g.Generate(outputDir, config)

		assert.ErrorIs(t, err, builder.ErrProjectGeneration)
	})

	t.Run("異常系: 不正なpackage_nameでErrProjectGeneration", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			packageName string
		}{
			{name: "異常系: 空のパッケージ名", packageName: ""},
			{name: "異常系: ドットなしのパッケージ名", packageName: "invalid"},
			{name: "異常系: 数字で始まるパッケージ名", packageName: "com.123invalid"},
			{name: "異常系: 予約語を含むパッケージ名", packageName: "com.example.class"},
			{name: "異常系: 連続するドットを含むパッケージ名", packageName: "com..double.dot"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				templatePath := writeZip(t, dir, "template.zip", validTemplateFiles())
				outputDir := filepath.Join(dir, "output")
				require.NoError(t, os.Mkdir(outputDir, 0o750))

				config := builder.ProjectConfig{PackageName: tc.packageName, AppName: "My Game", VersionCode: 1, VersionName: "1.0.0"}

				g := builder.NewProjectGenerator(templatePath)
				_, err := g.Generate(outputDir, config)

				assert.ErrorIs(t, err, builder.ErrProjectGeneration)
			})
		}
	})

	t.Run("異常系: テンプレートファイルが存在しない場合", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templatePath := filepath.Join(dir, "nonexistent.zip")
		outputDir := filepath.Join(dir, "output")
		require.NoError(t, os.Mkdir(outputDir, 0o750))

		config := builder.ProjectConfig{PackageName: "com.example.game", AppName: "My Game", VersionCode: 1, VersionName: "1.0.0"}

		g := builder.NewProjectGenerator(templatePath)
		_, err := g.Generate(outputDir, config)

		assert.ErrorIs(t, err, builder.ErrProjectGeneration)
	})
}

func TestProjectGenerator_ExtractTemplate_ZipSlip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := validTemplateFiles()
	files["../../evil.txt"] = "escaped!"
	templatePath := writeZip(t, dir, "malicious.zip", files)
	outputDir := filepath.Join(dir, "output")
	require.NoError(t, os.Mkdir(outputDir, 0o750))

	config := builder.ProjectConfig{PackageName: "com.example.game", AppName: "My Game", VersionCode: 1, VersionName: "1.0.0"}

	g := builder.NewProjectGenerator(templatePath)
	_, err := g.Generate(outputDir, config)

	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "evil.txt"))
}
