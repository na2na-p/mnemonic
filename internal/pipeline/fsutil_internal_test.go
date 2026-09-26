package pipeline

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		content       []byte
		setup         func(t *testing.T, dir string, content []byte) (src, dst string)
		checkMode     bool
		wantStatError bool
	}{
		{
			name:    "同一パスへのコピーは内容を保持する",
			content: []byte("original content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				path := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(path, content, 0o644))

				return path, path
			},
		},
		{
			name:    "ハードリンク経由のコピーは内容を保持する",
			content: []byte("hard-linked content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "linked.txt")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				if err := os.Link(src, dst); err != nil {
					t.Skipf("このファイルシステムはハードリンクをサポートしません: %v", err)
				}

				return src, dst
			},
		},
		{
			// O_CREATE|O_TRUNCは既存ファイルのモードを変更しない(POSIX)ため、
			// このケースはモードを検証しない。モードの検証はdst新規作成のケースで行う。
			name:    "既存の別ファイルへのコピーは内容が上書きされる",
			content: []byte("distinct content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "dst.txt")
				require.NoError(t, os.WriteFile(src, content, 0o600))
				require.NoError(t, os.WriteFile(dst, []byte("stale content that is much longer than the new one"), 0o600))

				return src, dst
			},
		},
		{
			name:      "dstが存在しない場合は新規作成しモードを保持する",
			content:   []byte("new file content"),
			checkMode: true,
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "nested", "dst.txt")
				require.NoError(t, os.WriteFile(src, content, 0o644))
				require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))

				return src, dst
			},
		},
		{
			name:          "dstの親が通常ファイルの場合はstatエラーを伝播する",
			wantStatError: true,
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				notADir := filepath.Join(dir, "not-a-dir")
				require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))

				return src, filepath.Join(notADir, "dst.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			src, dst := tt.setup(t, dir, tt.content)

			err := copyFile(src, dst)

			if tt.wantStatError {
				var pathErr *fs.PathError
				require.ErrorAs(t, err, &pathErr)
				assert.Equal(t, "stat", pathErr.Op)

				return
			}

			require.NoError(t, err)
			got, readErr := os.ReadFile(dst)
			require.NoError(t, readErr)
			assert.Equal(t, tt.content, got)

			if tt.checkMode {
				srcInfo, statErr := os.Stat(src)
				require.NoError(t, statErr)
				dstInfo, statErr := os.Stat(dst)
				require.NoError(t, statErr)
				assert.Equal(t, srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
			}
		})
	}
}

type templateZipEntry struct {
	name    string
	content string
}

func writeTemplateZip(t *testing.T, path string, entries []templateZipEntry) {
	t.Helper()

	archive, err := os.Create(path) //nolint:gosec // t.TempDir配下のテスト専用パス
	require.NoError(t, err)

	zw := zip.NewWriter(archive)
	for _, entry := range entries {
		writer, createErr := zw.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		require.NoError(t, createErr)
		_, writeErr := writer.Write([]byte(entry.content))
		require.NoError(t, writeErr)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, archive.Close())
}

func replaceZipEntryContent(t *testing.T, path, oldContent, newContent string) {
	t.Helper()
	require.Len(t, newContent, len(oldContent))

	content, err := os.ReadFile(path) //nolint:gosec // t.TempDir配下のテスト専用パス
	require.NoError(t, err)
	require.Equal(t, 1, bytes.Count(content, []byte(oldContent)))

	content = bytes.Replace(content, []byte(oldContent), []byte(newContent), 1)
	require.NoError(t, os.WriteFile(path, content, 0o600))
}

func replaceZipCompressionMethod(t *testing.T, path string, method uint16) {
	t.Helper()

	content, err := os.ReadFile(path) //nolint:gosec // t.TempDir配下のテスト専用パス
	require.NoError(t, err)

	localHeader := bytes.Index(content, []byte{'P', 'K', 3, 4})
	require.NotEqual(t, -1, localHeader)
	binary.LittleEndian.PutUint16(content[localHeader+8:localHeader+10], method)

	centralHeader := bytes.Index(content, []byte{'P', 'K', 1, 2})
	require.NotEqual(t, -1, centralHeader)
	binary.LittleEndian.PutUint16(content[centralHeader+10:centralHeader+12], method)

	require.NoError(t, os.WriteFile(path, content, 0o600))
}

func TestFsutil_ExtractTemplateZip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entries        []templateZipEntry
		missingArchive bool
		invalidArchive bool
		wantErr        bool
		wantFiles      map[string]string
		wantDirs       []string
		outsidePaths   func(root string) []string
		prepare        func(t *testing.T, archivePath, destDir string)
	}{
		{
			name: "正常系: 入れ子のディレクトリとファイルを展開する",
			entries: []templateZipEntry{
				{name: "app/"},
				{name: "app/src/main.txt", content: "main content"},
				{name: "assets/images/logo.txt", content: "logo content"},
				{name: "empty/"},
			},
			wantFiles: map[string]string{
				"app/src/main.txt":       "main content",
				"assets/images/logo.txt": "logo content",
			},
			wantDirs: []string{"app", "empty"},
		},
		{
			name:         "異常系: 一階層上への脱出を拒否する",
			entries:      []templateZipEntry{{name: "../evil.txt", content: "escaped"}},
			wantErr:      true,
			outsidePaths: func(root string) []string { return []string{filepath.Join(root, "evil.txt")} },
		},
		{
			name:    "異常系: 二階層上への脱出を拒否する",
			entries: []templateZipEntry{{name: "../../evil.txt", content: "escaped"}},
			wantErr: true,
			outsidePaths: func(root string) []string {
				return []string{filepath.Join(root, "evil.txt"), filepath.Join(root, "..", "evil.txt")}
			},
		},
		{
			name:         "異常系: 子ディレクトリ経由の脱出を拒否する",
			entries:      []templateZipEntry{{name: "a/../../evil.txt", content: "escaped"}},
			wantErr:      true,
			outsidePaths: func(root string) []string { return []string{filepath.Join(root, "evil.txt")} },
		},
		{
			name:    "正常系: 絶対パスを展開先配下へ無害化する",
			entries: []templateZipEntry{{name: "/etc/evil.txt", content: "neutralized"}},
			wantFiles: map[string]string{
				"etc/evil.txt": "neutralized",
			},
			outsidePaths: func(string) []string { return []string{"/etc/evil.txt"} },
		},
		{
			name:         "異常系: バックスラッシュによる脱出を拒否する",
			entries:      []templateZipEntry{{name: `..\evil.txt`, content: "escaped"}},
			wantErr:      true,
			outsidePaths: func(root string) []string { return []string{filepath.Join(root, "evil.txt")} },
		},
		{
			name:         "異常系: 前方一致する兄弟ディレクトリへの脱出を拒否する",
			entries:      []templateZipEntry{{name: "../out-evil/x", content: "escaped"}},
			wantErr:      true,
			outsidePaths: func(root string) []string { return []string{filepath.Join(root, "out-evil", "x")} },
		},
		{
			name:           "異常系: 存在しないアーカイブを拒否する",
			missingArchive: true,
			wantErr:        true,
		},
		{
			name:           "異常系: ZIPではないファイルを拒否する",
			invalidArchive: true,
			wantErr:        true,
		},
		{
			name:    "異常系: 展開先ディレクトリを作成できない場合に失敗する",
			entries: []templateZipEntry{{name: "nested/file.txt", content: "content"}},
			wantErr: true,
			prepare: func(t *testing.T, _ string, destDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(destDir, []byte("not a directory"), 0o600))
			},
		},
		{
			name:    "異常系: 未対応の圧縮方式を拒否する",
			entries: []templateZipEntry{{name: "file.txt", content: "content"}},
			wantErr: true,
			prepare: func(t *testing.T, archivePath, _ string) {
				t.Helper()
				replaceZipCompressionMethod(t, archivePath, 99)
			},
		},
		{
			name:    "異常系: 展開先が既存ディレクトリの場合に失敗する",
			entries: []templateZipEntry{{name: "file.txt", content: "content"}},
			wantErr: true,
			prepare: func(t *testing.T, _ string, destDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(destDir, "file.txt"), 0o750))
			},
		},
		{
			name:    "異常系: チェックサムが不正なエントリを拒否する",
			entries: []templateZipEntry{{name: "file.txt", content: "checksum-source"}},
			wantErr: true,
			prepare: func(t *testing.T, archivePath, _ string) {
				t.Helper()
				replaceZipEntryContent(t, archivePath, "checksum-source", "checksum-broken")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			archivePath := filepath.Join(root, "template.zip")
			destDir := filepath.Join(root, "out")

			switch {
			case tt.invalidArchive:
				require.NoError(t, os.WriteFile(archivePath, []byte("not a zip archive"), 0o600))
			case !tt.missingArchive:
				writeTemplateZip(t, archivePath, tt.entries)
			}
			if tt.prepare != nil {
				tt.prepare(t, archivePath, destDir)
			}

			err := extractTemplateZip(archivePath, destDir)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			for relativePath, wantContent := range tt.wantFiles {
				content, readErr := os.ReadFile(filepath.Join(destDir, relativePath)) //nolint:gosec // destDir配下のテスト生成物
				require.NoError(t, readErr)
				assert.Equal(t, wantContent, string(content))
			}
			for _, relativePath := range tt.wantDirs {
				assert.DirExists(t, filepath.Join(destDir, relativePath))
			}
			if tt.outsidePaths != nil {
				for _, outsidePath := range tt.outsidePaths(root) {
					assert.NoFileExists(t, outsidePath)
				}
			}
		})
	}
}
