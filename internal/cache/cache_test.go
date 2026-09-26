package cache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/cache"
)

func TestDir(t *testing.T) {
	t.Parallel()

	dir, err := cache.Dir()

	require.NoError(t, err)
	assert.Contains(t, dir, "mnemonic")
}

// unsetenvForTest はkeyを未設定にし、テスト終了後に元の状態（存在した場合は元の値、
// 存在しなかった場合は未設定）へ復元する。
// t.Setenv("")は空文字列を設定するだけで「未設定」にはならないため、この用途には使えない。
func unsetenvForTest(t *testing.T, key string) {
	t.Helper()

	original, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// 環境変数を書き換えるサブテストがt.Setenvを使うため、本テスト自体は
// t.Parallel()にできない（t.Setenvは並行祖先を持つテストから呼べない）。
//
//nolint:tparallel // 理由は上記コメントの通り
func TestDirForOS(t *testing.T) {
	t.Run("正常系: Linuxでデフォルトキャッシュディレクトリ", func(t *testing.T) {
		unsetenvForTest(t, "XDG_CACHE_HOME")

		dir, err := cache.DirForOS("linux")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join(".cache", "mnemonic"))
	})

	t.Run("正常系: LinuxでXDG_CACHE_HOME設定時", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/custom/cache")

		dir, err := cache.DirForOS("linux")

		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/custom/cache", "mnemonic"), dir)
	})

	t.Run("正常系: macOSのキャッシュディレクトリ", func(t *testing.T) {
		t.Parallel()

		dir, err := cache.DirForOS("darwin")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join("Library", "Caches", "mnemonic"))
	})

	t.Run("正常系: Windowsのキャッシュディレクトリ（LOCALAPPDATA設定時）", func(t *testing.T) {
		t.Setenv("LOCALAPPDATA", `C:\Users\Test\AppData\Local`)

		dir, err := cache.DirForOS("windows")

		require.NoError(t, err)
		assert.Contains(t, dir, "mnemonic")
		assert.Contains(t, dir, "cache")
	})

	t.Run("正常系: Windowsのキャッシュディレクトリ（LOCALAPPDATA未設定時）", func(t *testing.T) {
		unsetenvForTest(t, "LOCALAPPDATA")

		dir, err := cache.DirForOS("windows")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join("AppData", "Local", "mnemonic", "cache"))
	})

	t.Run("正常系: 未知のOSはLinux相当のフォールバック", func(t *testing.T) {
		t.Parallel()

		dir, err := cache.DirForOS("plan9")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join(".cache", "mnemonic"))
	})
}

func TestTemplateCachePath(t *testing.T) {
	t.Parallel()

	t.Run("正常系: テンプレートキャッシュパスにバージョンが含まれる", func(t *testing.T) {
		t.Parallel()

		path, err := cache.TemplateCachePath("1.0.0")

		require.NoError(t, err)
		assert.Contains(t, path, "templates")
		assert.Contains(t, path, "1.0.0")
	})

	t.Run("正常系: 異なるバージョンで異なるパスを返す", func(t *testing.T) {
		t.Parallel()

		v1, err := cache.TemplateCachePath("1.0.0")
		require.NoError(t, err)
		v2, err := cache.TemplateCachePath("2.0.0")
		require.NoError(t, err)

		assert.NotEqual(t, v1, v2)
		assert.Contains(t, v1, "1.0.0")
		assert.Contains(t, v2, "2.0.0")
	})
}

func TestClearCacheDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 署名鍵を除く全キャッシュをクリアする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o600))
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "sdl2_sources"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sdl2_sources", "x"), []byte("sdl2"), 0o600))
		keystoreFile := filepath.Join(dir, "keystore", "debug.keystore")
		require.NoError(t, os.MkdirAll(filepath.Dir(keystoreFile), 0o750))
		require.NoError(t, os.WriteFile(keystoreFile, []byte("keystore"), 0o600))

		err := cache.ClearCacheDir(dir, false)

		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(dir, "test.txt"))
		assert.NoDirExists(t, filepath.Join(dir, "templates"))
		assert.NoDirExists(t, filepath.Join(dir, "sdl2_sources"))
		content, readErr := os.ReadFile(keystoreFile)
		require.NoError(t, readErr)
		assert.Equal(t, "keystore", string(content))
		assert.DirExists(t, dir)
	})

	t.Run("正常系: 署名鍵が無い場合は他のエントリをすべて削除しディレクトリを残す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o600))
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)

		err := cache.ClearCacheDir(dir, false)

		require.NoError(t, err)
		assert.DirExists(t, dir)
		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})

	t.Run("正常系: テンプレートのみクリアする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(cacheFile, []byte("test"), 0o600))
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)
		sdl2SourceFile := filepath.Join(dir, "sdl2_sources", "x")
		require.NoError(t, os.MkdirAll(filepath.Dir(sdl2SourceFile), 0o750))
		require.NoError(t, os.WriteFile(sdl2SourceFile, []byte("sdl2"), 0o600))
		keystoreFile := filepath.Join(dir, "keystore", "debug.keystore")
		require.NoError(t, os.MkdirAll(filepath.Dir(keystoreFile), 0o750))
		require.NoError(t, os.WriteFile(keystoreFile, []byte("keystore"), 0o600))

		err := cache.ClearCacheDir(dir, true)

		require.NoError(t, err)
		_, statErr := os.Stat(cacheFile)
		require.NoError(t, statErr)
		_, statErr = os.Stat(filepath.Join(dir, "templates"))
		assert.True(t, os.IsNotExist(statErr))
		_, statErr = os.Stat(sdl2SourceFile)
		require.NoError(t, statErr)
		assert.FileExists(t, keystoreFile)
	})

	t.Run("正常系: 存在しないディレクトリのクリアはエラーにならない", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(t.TempDir(), "nonexistent")

		err := cache.ClearCacheDir(nonexistent, false)

		assert.NoError(t, err)
	})
}

// writeTemplateMetadata はcacheDir/templates/version配下にビルドと同じ形式の
// メタデータを書き込み、そのバージョンディレクトリを返す。
func writeTemplateMetadata(t *testing.T, cacheDir, version, downloadedAt, expiresAt string) string {
	t.Helper()

	versionDir := filepath.Join(cacheDir, "templates", version)
	require.NoError(t, os.MkdirAll(versionDir, 0o750))
	data, err := json.Marshal(cache.TemplateMetadata{
		Version:      version,
		DownloadedAt: downloadedAt,
		ExpiresAt:    expiresAt,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, cache.TemplateMetadataFilename), data, 0o600))

	return versionDir
}

func formatMetadataTime(tm time.Time) string {
	return tm.UTC().Format(cache.TemplateMetadataTimeLayout)
}

func TestInfoForDir(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name          string
		setup         func(t *testing.T, cacheDir string)
		wantVersion   *string
		wantExpiresIn *int
		wantNoSize    bool
	}{
		{
			name:       "正常系: 空のキャッシュディレクトリではテンプレート情報が無い",
			setup:      func(*testing.T, string) {},
			wantNoSize: true,
		},
		{
			name: "正常系: テンプレート以外のファイルのみの場合はテンプレート情報が無い",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "test.txt"), []byte("test content"), 0o600))
			},
		},
		{
			name: "正常系: メタデータのexpires_atから残り有効日数を求める",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now.Add(-24*time.Hour)), formatMetadataTime(now.Add(3*24*time.Hour)))
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(3),
		},
		{
			name: "正常系: 有効期間が7日を超えるメタデータでもexpires_atに従う",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now), formatMetadataTime(now.Add(30*24*time.Hour)))
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(30),
		},
		{
			name: "正常系: expires_atが過去の場合は残り有効日数0",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now.Add(-10*24*time.Hour)), formatMetadataTime(now.Add(-3*24*time.Hour)))
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(0),
		},
		{
			name: "正常系: 更新日時ではなくdownloaded_atが最も新しいバージョンを報告する",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				olderDir := writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now.Add(-5*24*time.Hour)), formatMetadataTime(now.Add(2*24*time.Hour)))
				newerDir := writeTemplateMetadata(t, cacheDir, "2.0.0",
					formatMetadataTime(now.Add(-1*24*time.Hour)), formatMetadataTime(now.Add(6*24*time.Hour)))
				require.NoError(t, os.Chtimes(olderDir, now, now))
				require.NoError(t, os.Chtimes(newerDir, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)))
			},
			wantVersion:   new("2.0.0"),
			wantExpiresIn: new(6),
		},
		{
			name: "正常系: メタデータの無いバージョンディレクトリのみの場合はテンプレート情報が無い",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				versionDir := filepath.Join(cacheDir, "templates", "1.0.0")
				require.NoError(t, os.MkdirAll(versionDir, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(versionDir, "template.txt"), []byte("template"), 0o600))
			},
		},
		{
			name: "正常系: メタデータの無いバージョンディレクトリは更新日時が新しくても無視する",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				withMetadata := writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now.Add(-1*24*time.Hour)), formatMetadataTime(now.Add(4*24*time.Hour)))
				withoutMetadata := filepath.Join(cacheDir, "templates", "2.0.0")
				require.NoError(t, os.MkdirAll(withoutMetadata, 0o750))
				require.NoError(t, os.Chtimes(withMetadata, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)))
				require.NoError(t, os.Chtimes(withoutMetadata, now, now))
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(4),
		},
		{
			name: "正常系: downloaded_atを解釈できないバージョンは無視する",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "1.0.0",
					formatMetadataTime(now.Add(-2*24*time.Hour)), formatMetadataTime(now.Add(5*24*time.Hour)))
				writeTemplateMetadata(t, cacheDir, "2.0.0", "not-a-time", formatMetadataTime(now.Add(7*24*time.Hour)))
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(5),
		},
		{
			name: "正常系: downloaded_atを解釈できないバージョンしか無い場合はテンプレート情報が無い",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "2.0.0", "not-a-time", formatMetadataTime(now.Add(7*24*time.Hour)))
			},
		},
		{
			name: "正常系: expires_atを解釈できない場合はビルドと同じく期限切れとみなし0日",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				writeTemplateMetadata(t, cacheDir, "1.0.0", formatMetadataTime(now), "")
			},
			wantVersion:   new("1.0.0"),
			wantExpiresIn: new(0),
		},
		{
			name: "正常系: templates直下の通常ファイルはバージョンとして扱わない",
			setup: func(t *testing.T, cacheDir string) {
				t.Helper()
				templatesDir := filepath.Join(cacheDir, "templates")
				require.NoError(t, os.MkdirAll(templatesDir, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "stray.txt"), []byte("x"), 0o600))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			tt.setup(t, dir)

			result, err := cache.InfoForDir(dir)

			require.NoError(t, err)
			assert.Equal(t, dir, result.Directory)
			if tt.wantNoSize {
				assert.Zero(t, result.SizeBytes)
			} else {
				assert.Positive(t, result.SizeBytes)
			}
			assert.Equal(t, tt.wantVersion, result.TemplateVersion)
			assert.Equal(t, tt.wantExpiresIn, result.TemplateExpiresInDays)
		})
	}
}

func TestInfoForDir_NonexistentDirectory(t *testing.T) {
	t.Parallel()

	nonexistent := filepath.Join(t.TempDir(), "nonexistent")

	result, err := cache.InfoForDir(nonexistent)

	require.NoError(t, err)
	assert.Equal(t, nonexistent, result.Directory)
	assert.Equal(t, int64(0), result.SizeBytes)
	assert.Nil(t, result.TemplateVersion)
	assert.Nil(t, result.TemplateExpiresInDays)
}

func TestReadTemplateMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(t *testing.T, versionDir string)
		want   cache.TemplateMetadata
		wantOK bool
	}{
		{
			name: "正常系: メタデータを読み込める",
			setup: func(t *testing.T, versionDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(versionDir, 0o750))
				data := `{"version":"1.0.0","downloaded_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-08T00:00:00Z"}`
				require.NoError(t, os.WriteFile(filepath.Join(versionDir, "metadata.json"), []byte(data), 0o600))
			},
			want: cache.TemplateMetadata{
				Version:      "1.0.0",
				DownloadedAt: "2026-01-01T00:00:00Z",
				ExpiresAt:    "2026-01-08T00:00:00Z",
			},
			wantOK: true,
		},
		{
			name: "異常系: メタデータファイルが無い場合はok=false",
			setup: func(t *testing.T, versionDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(versionDir, 0o750))
			},
		},
		{
			name:  "異常系: バージョンディレクトリが無い場合はok=false",
			setup: func(*testing.T, string) {},
		},
		{
			name: "異常系: メタデータが壊れている場合はok=false",
			setup: func(t *testing.T, versionDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(versionDir, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(versionDir, "metadata.json"), []byte("{broken"), 0o600))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			versionDir := filepath.Join(t.TempDir(), "templates", "1.0.0")
			tt.setup(t, versionDir)

			got, ok := cache.ReadTemplateMetadata(versionDir)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInfo_CreationAndFieldAccess(t *testing.T) {
	t.Parallel()

	info := cache.Info{
		Directory:             "/tmp/cache",
		SizeBytes:             1024,
		TemplateVersion:       new("1.0.0"),
		TemplateExpiresInDays: new(7),
	}

	assert.Equal(t, "/tmp/cache", info.Directory)
	assert.Equal(t, int64(1024), info.SizeBytes)
	require.NotNil(t, info.TemplateVersion)
	assert.Equal(t, "1.0.0", *info.TemplateVersion)
	require.NotNil(t, info.TemplateExpiresInDays)
	assert.Equal(t, 7, *info.TemplateExpiresInDays)
}
