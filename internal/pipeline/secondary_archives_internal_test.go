package pipeline

import (
	"encoding/binary"
	"hash/adler32"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/parser"
	"github.com/na2na-p/mnemonic/internal/resources"
)

// writeTestXP3 はfiles（"/"区切りの相対パス→内容）を格納したXP3アーカイブをpathに書く。
//
// why not: 複数エントリのアーカイブを手書きのバイト列で組まない。XP3の形式は
// parser.WriteXP3Archiveのテストがバイト単位で固定しており、ここで確かめたいのは
// パイプラインがアーカイブをどう扱うかである。
func writeTestXP3(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	src := t.TempDir()
	writeTestFiles(t, src, files)
	require.NoError(t, parser.WriteXP3Archive(path, src))
}

// writeTestFiles はfiles（"/"区切りの相対パス→内容）をdir配下に書く。
func writeTestFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()

	for name, data := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
}

// extractTestXP3 はpathのXP3アーカイブを読み取り器で一時ディレクトリへ展開し、
// エントリ名の一覧と展開先を返す。
func extractTestXP3(t *testing.T, path string) ([]string, string) {
	t.Helper()

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, archive.ExtractAll(dir))

	return archive.ListFiles(), dir
}

// relativeFiles はdir配下の通常ファイルを"/"区切りの相対パスで名前順に返す。
func relativeFiles(t *testing.T, dir string) []string {
	t.Helper()

	var files []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))

		return nil
	}))
	slices.Sort(files)

	return files
}

// runThroughConvert はinputをANALYZE・EXTRACT・CONVERTの順に通す。
func runThroughConvert(t *testing.T, p *BuildPipeline) (buildArtifacts, error) {
	t.Helper()

	analyzed, err := p.executeAnalyze(buildArtifacts{})
	require.NoError(t, err)
	extracted, err := p.executeExtract(analyzed)
	require.NoError(t, err)

	return p.executeConvert(extracted)
}

// newSecondaryTestPipeline はdirのinputを入力とし、空き容量の確認を常に通す
// パイプラインを返す。
func newSecondaryTestPipeline(t *testing.T, dir, input string) (*BuildPipeline, *recordingLogger) {
	t.Helper()

	p := NewBuildPipeline(NewConfig(filepath.Join(dir, input), filepath.Join(dir, "output.apk")))
	t.Cleanup(p.cleanupTempDirs)
	p.freeSpace = func(string) (uint64, error) { return math.MaxUint64, nil }
	logger := &recordingLogger{}
	p.SetLogger(logger)

	return p, logger
}

// stubFont はcopyFontFileが既定のFontFetcher経由で実キャッシュや実ネットワークに
// 触れないよう、起動アーカイブに置くsystem/font.ttf。
var stubFont = []byte("stub font")

const aboutHandler = "\tfunction onHelpAboutMenuItemClick(sender)\n" +
	"\t{\n" +
	"\t\tvar win = new global.KAGWindow(false, aboutWidth, aboutHeight);\n" +
	"\t\twin.setPos(left + ((width - win.width)>>1), top + ((height - win.height)>>1));\n" +
	"\t\twin.process('about.ks' ,,, true);\n" +
	"\t\twin.showModal();\n" +
	"\t\tinvalidate win;\n" +
	"\t}\n"

func TestBuildPipeline_ExecuteConvert_ShipsSecondaryArchives(t *testing.T) {
	t.Parallel()

	const scenario = "吾輩は猫である。名前はまだ無い。\n"
	sjisScenario, err := japanese.ShiftJIS.NewEncoder().String(scenario)
	require.NoError(t, err)
	voice := []byte("OggS\x00\x02voice")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ-stub-16-bytes-without-archive"), 0o600))
	writeTestXP3(t, filepath.Join(dir, "data.xp3"), map[string][]byte{
		"startup.tjs":        []byte("// data startup\n"),
		"system/font.ttf":    stubFont,
		"scenario/about.ks":  []byte("[title name=\"バージョン情報\"]\\\nVer.1.05\n[s]"),
		"scenario/first.ks":  []byte("*start\n"),
		"scenario/second.ks": []byte("*second\n"),
	})
	writeTestXP3(t, filepath.Join(dir, "patch.xp3"), map[string][]byte{
		"first.ks":       []byte(sjisScenario),
		"startup.tjs":    []byte("// patch startup\n"),
		"MainWindow.tjs": []byte(aboutHandler),
		"about.ks":       []byte("[title name=\"パッチ\"]\\\nPatch\n[s]"),
		"plugin/foo.dll": []byte("MZ"),
	})
	writeTestXP3(t, filepath.Join(dir, "Voice.XP3"), map[string][]byte{"v001.ogg": voice})
	writeTestXP3(t, filepath.Join(dir, "onlyplugin.xp3"), map[string][]byte{"Plugin/foo.dll": []byte("MZ")})

	p, logger := newSecondaryTestPipeline(t, dir, "game.exe")

	got, err := runThroughConvert(t, p)
	require.NoError(t, err)

	t.Run("正常系: 変換後に格納するファイルがあるアーカイブだけを小文字の名前で変換ツリーの直下に置く", func(t *testing.T) {
		t.Parallel()

		assert.FileExists(t, filepath.Join(got.convertDir, "patch.xp3"))
		assert.FileExists(t, filepath.Join(got.convertDir, "voice.xp3"))
		assert.NoFileExists(t, filepath.Join(got.convertDir, "onlyplugin.xp3"))
		assert.True(t, got.shipsSecondaryArchives)
	})

	t.Run("正常系: 見つかったアーカイブは変換すると知らせ、同梱したものだけを同梱すると知らせる", func(t *testing.T) {
		t.Parallel()

		infos := logger.messages("INFO")
		assert.Contains(t, infos, "data.xp3と同じフォルダで見つかった次のXP3アーカイブを変換します: Voice.XP3, onlyplugin.xp3, patch.xp3")
		assert.Contains(t, infos, "次のXP3アーカイブをAPKに同梱します: voice.xp3, patch.xp3")
		assert.Contains(t, logger.messages("WARNING"), "onlyplugin.xp3は変換後に格納するファイルが無いため、APKに同梱しません")
	})

	t.Run("正常系: 副アーカイブのスクリプトはBOM付きUTF-8に変換しstartup向けの読み込みを加えず、プラグインを除く", func(t *testing.T) {
		t.Parallel()

		names, extracted := extractTestXP3(t, filepath.Join(got.convertDir, "patch.xp3"))
		assert.ElementsMatch(t, []string{"MainWindow.tjs", "about.ks", "first.ks", "startup.tjs"}, names)

		first, err := os.ReadFile(filepath.Join(extracted, "first.ks")) //nolint:gosec // テストで展開した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, append([]byte{0xef, 0xbb, 0xbf}, scenario...), first)

		startup, err := os.ReadFile(filepath.Join(extracted, "startup.tjs")) //nolint:gosec // テストで展開した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.NotContains(t, string(startup), "polyfillinitialize")
		assert.Contains(t, string(startup), "// patch startup")
	})

	t.Run("正常系: 副アーカイブのスクリプト調整には起動アーカイブのabout.ksを使う", func(t *testing.T) {
		t.Parallel()

		_, extracted := extractTestXP3(t, filepath.Join(got.convertDir, "patch.xp3"))
		mainWindow, err := os.ReadFile(filepath.Join(extracted, "MainWindow.tjs")) //nolint:gosec // テストで展開した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(mainWindow), `System.inform("Ver.1.05", "バージョン情報");`)
	})

	t.Run("正常系: 変換対象でない素材はそのまま格納する", func(t *testing.T) {
		t.Parallel()

		names, extracted := extractTestXP3(t, filepath.Join(got.convertDir, "voice.xp3"))
		assert.Equal(t, []string{"v001.ogg"}, names)
		data, err := os.ReadFile(filepath.Join(extracted, "v001.ogg")) //nolint:gosec // テストで展開した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, voice, data)
	})

	t.Run("正常系: 副アーカイブの中身は起動アーカイブのツリーに混ざらず、起動アーカイブにだけstartup向けの読み込みを加える", func(t *testing.T) {
		t.Parallel()

		assert.NoFileExists(t, filepath.Join(got.convertDir, "first.ks"))
		assert.NoFileExists(t, filepath.Join(got.convertDir, "v001.ogg"))
		startup, err := os.ReadFile(filepath.Join(got.convertDir, "startup.tjs"))
		require.NoError(t, err)
		assert.Contains(t, string(startup), "polyfillinitialize")
		assert.Contains(t, string(startup), "// data startup")
	})

	t.Run("正常系: 同梱するときはsystem/exepathoverride.tjsを書く", func(t *testing.T) {
		t.Parallel()

		assert.FileExists(t, filepath.Join(got.convertDir, "system", "exepathoverride.tjs"))
	})

	t.Run("正常系: 詰め直したXP3は複製を残さず変換ツリーへ移す", func(t *testing.T) {
		t.Parallel()

		var stageDirs []string
		for _, d := range p.tempDirs {
			if strings.HasPrefix(filepath.Base(d), "mnemonic_secondary_xp3_") {
				stageDirs = append(stageDirs, d)
			}
		}
		require.Len(t, stageDirs, 1)
		entries, err := os.ReadDir(stageDirs[0])
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("正常系: 副アーカイブの展開ツリーと変換ツリーは詰め直した後に消す", func(t *testing.T) {
		t.Parallel()

		var trees []string
		for _, d := range p.tempDirs {
			base := filepath.Base(d)
			if strings.HasPrefix(base, "mnemonic_secondary_extract_") || strings.HasPrefix(base, "mnemonic_secondary_convert_") {
				trees = append(trees, d)
			}
		}
		assert.Len(t, trees, 6)
		for _, d := range trees {
			assert.NoDirExists(t, d)
		}
	})
}

// TestBuildPipeline_ExecuteConvert_ExePathOverride は、CONVERTフェーズが副アーカイブを
// 1つ以上同梱したときだけsystem/exepathoverride.tjsを書くことを固定する。
func TestBuildPipeline_ExecuteConvert_ExePathOverride(t *testing.T) {
	t.Parallel()

	want, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + resources.ExePathOverrideFile)
	require.NoError(t, err)

	tests := []struct {
		name        string
		siblings    map[string]map[string][]byte
		wantPresent bool
	}{
		{name: "正常系: 副アーカイブが無ければ書かない", wantPresent: false},
		{
			name:        "正常系: 副アーカイブを同梱すれば埋め込みの内容を書く",
			siblings:    map[string]map[string][]byte{"bgm.xp3": {"bgm01.ogg": []byte("OggS\x00")}},
			wantPresent: true,
		},
		{
			name:        "正常系: 副アーカイブがすべて変換後に空になり同梱しなければ書かない",
			siblings:    map[string]map[string][]byte{"plugin.xp3": {"plugin/a.dll": []byte("MZ")}},
			wantPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTestXP3(t, filepath.Join(dir, "data.xp3"), map[string][]byte{
				"startup.tjs":     []byte("// startup\n"),
				"system/font.ttf": stubFont,
			})
			for name, files := range tt.siblings {
				writeTestXP3(t, filepath.Join(dir, name), files)
			}
			p, logger := newSecondaryTestPipeline(t, dir, "data.xp3")

			a, err := runThroughConvert(t, p)
			require.NoError(t, err)

			assert.Equal(t, tt.wantPresent, a.shipsSecondaryArchives)
			shippedInfo := slices.ContainsFunc(logger.messages("INFO"), func(m string) bool {
				return strings.HasPrefix(m, "次のXP3アーカイブをAPKに同梱します: ")
			})
			assert.Equal(t, tt.wantPresent, shippedInfo)
			overridePath := filepath.Join(a.convertDir, "system", "exepathoverride.tjs")
			if !tt.wantPresent {
				assert.NoFileExists(t, overridePath)

				return
			}
			got, readErr := os.ReadFile(overridePath) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
			require.NoError(t, readErr)
			assert.Equal(t, want, got)
		})
	}
}

// TestBuildPipeline_ExecuteConvert_WithoutSecondaryArchives は、副アーカイブが無い入力の
// 変換ツリーが起動アーカイブの中身とpolyfillだけからなることを固定する。
func TestBuildPipeline_ExecuteConvert_WithoutSecondaryArchives(t *testing.T) {
	t.Parallel()

	archive := map[string][]byte{
		"startup.tjs":       []byte("// startup\n"),
		"system/font.ttf":   stubFont,
		"scenario/first.ks": []byte("*start\n"),
		"bgm/BGM01.ogg":     []byte("OggS\x00"),
	}
	want := []string{"bgm/bgm01.ogg", "scenario/first.ks", "startup.tjs", "system/font.ttf"}
	for _, name := range resources.SystemPolyfillFiles {
		want = append(want, "system/"+strings.ToLower(name))
	}
	slices.Sort(want)

	tests := []struct {
		name  string
		input string
		files func(t *testing.T, dir string)
	}{
		{
			name:  "正常系: data.xp3だけの入力",
			input: "data.xp3",
			files: func(t *testing.T, dir string) {
				t.Helper()
				writeTestXP3(t, filepath.Join(dir, "data.xp3"), archive)
			},
		},
		{
			name:  "正常系: XP3を埋め込んだEXEだけの入力",
			input: "game.exe",
			files: func(t *testing.T, dir string) {
				t.Helper()
				xp3 := filepath.Join(t.TempDir(), "embedded.xp3")
				writeTestXP3(t, xp3, archive)
				data, err := os.ReadFile(xp3) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "game.exe"), append([]byte("MZ-stub-16-bytes"), data...), 0o600))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			tt.files(t, dir)
			p, _ := newSecondaryTestPipeline(t, dir, tt.input)

			got, err := runThroughConvert(t, p)
			require.NoError(t, err)

			// why not: 名前をそのまま比べない。大文字小文字を区別しないファイルシステムでは
			// normalizeCriticalFilenamesが小文字の名前を既存とみなして改名を飛ばす。
			var files []string
			for _, name := range relativeFiles(t, got.convertDir) {
				files = append(files, strings.ToLower(name))
			}
			slices.Sort(files)
			assert.Equal(t, want, files)
			assert.False(t, got.shipsSecondaryArchives)
		})
	}
}

func TestBuildPipeline_ExecuteExtract_SecondaryArchiveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		data         map[string][]byte
		siblings     map[string][]byte
		wantErr      error
		wantMessages []string
	}{
		{
			name:     "異常系: 読み込めない副アーカイブはフォルダから移して再実行するよう案内する",
			data:     map[string][]byte{"startup.tjs": []byte("//")},
			siblings: map[string][]byte{"bgm.xp3": []byte("not an archive")},
			wantErr:  parser.ErrInvalidXP3,
			wantMessages: []string{
				ErrSecondaryArchiveUnreadable.Error() + ": bgm.xp3",
				"bgm.xp3を{dir}から別の場所へ移して再実行してください",
			},
		},
		{
			name:     "異常系: 起動アーカイブの直下に小文字にすると同じ名前になるファイルがあればエラーを返す",
			data:     map[string][]byte{"startup.tjs": []byte("//"), "Patch.XP3": []byte("inner")},
			siblings: map[string][]byte{"patch.xp3": nil},
			wantErr:  ErrSecondaryArchiveRootConflict,
			wantMessages: []string{
				"起動アーカイブの直下のPatch.XP3と、同梱するpatch.xp3がAPKで同じ名前になります",
				"patch.xp3を{dir}から別の場所へ移して再実行してください",
			},
		},
		{
			name:     "異常系: 起動アーカイブの直下に同じ名前のフォルダがあればエラーを返す",
			data:     map[string][]byte{"startup.tjs": []byte("//"), "bgm.xp3/a.ogg": []byte("OggS")},
			siblings: map[string][]byte{"bgm.xp3": nil},
			wantErr:  ErrSecondaryArchiveRootConflict,
			wantMessages: []string{
				"起動アーカイブの直下のbgm.xp3と、同梱するbgm.xp3がAPKで同じ名前になります",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTestXP3(t, filepath.Join(dir, "data.xp3"), tt.data)
			for name, data := range tt.siblings {
				if data == nil {
					writeTestXP3(t, filepath.Join(dir, name), map[string][]byte{"a.ks": []byte("*a\n")})

					continue
				}
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
			}
			p, _ := newSecondaryTestPipeline(t, dir, "data.xp3")

			analyzed, err := p.executeAnalyze(buildArtifacts{})
			require.NoError(t, err)
			_, err = p.executeExtract(analyzed)

			require.ErrorIs(t, err, tt.wantErr)
			for _, m := range tt.wantMessages {
				assert.Contains(t, err.Error(), strings.ReplaceAll(m, "{dir}", dir))
			}
		})
	}
}

// TestResolveStartupArchive_SecondaryNameCollision は、小文字にすると同じ名前になる
// .xp3ファイルがあればANALYZEの時点でエラーにすることを固定する。
func TestResolveStartupArchive_SecondaryNameCollision(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skipIfCaseInsensitiveFS(t, dir)
	archive := storedXP3Bytes("startup.tjs", []byte("//"))
	for _, name := range []string{"data.xp3", "Patch.xp3", "patch.XP3"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), archive, 0o600))
	}

	_, err := resolveStartupArchive(filepath.Join(dir, "data.xp3"))

	require.ErrorIs(t, err, ErrSecondaryArchiveNameCollision)
	assert.Contains(t, err.Error(), "Patch.xp3, patch.XP3")
}

// TestBuildPipeline_FinalizeSecondaryTree は副アーカイブの後処理が、起動アーカイブと
// 同じく古い動画ファイルを消し、MIDI変換の失敗でスクリプトを書き換える前に止まり、
// プラグインディレクトリを消し、polyfillを置かないことを固定する。
func TestBuildPipeline_FinalizeSecondaryTree(t *testing.T) {
	t.Parallel()

	const script = `@bgm storage="bgm/opening.mid"` + "\n"

	tests := []struct {
		name       string
		files      map[string][]byte
		fluidsynth fakeCommandResponse
		// staleVideo はsummaryで変換済みとするop.mpgの元の動画を置くかどうか。
		staleVideo  bool
		wantErr     error
		wantFiles   []string
		wantScript  string
		wantNoFiles []string
	}{
		{
			name:        "正常系: 古い動画ファイルとプラグインディレクトリを消し、polyfillは置かずスクリプトを調整する",
			files:       map[string][]byte{"first.ks": []byte(`[movie storage="op.wmv"]`), "plugin/a.dll": []byte("MZ"), "op.mpg": []byte("mpeg")},
			staleVideo:  true,
			wantFiles:   []string{"first.ks", "op.mpg"},
			wantScript:  "\ufeff" + `[movie storage="op.mpg"]`,
			wantNoFiles: []string{"op.wmv", "plugin", "system"},
		},
		{
			name:        "異常系: MIDIを変換できなければスクリプトを書き換える前に止める",
			files:       map[string][]byte{"first.ks": []byte(script), "bgm/opening.mid": []byte("MThd")},
			fluidsynth:  fakeCommandResponse{err: os.ErrNotExist},
			wantErr:     ErrMidiConversionUnavailable,
			wantFiles:   []string{"bgm/opening.mid", "first.ks"},
			wantScript:  script,
			wantNoFiles: []string{"system"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTestFiles(t, dir, tt.files)
			var summary converter.ConversionSummary
			if tt.staleVideo {
				stale := filepath.Join(dir, "op.wmv")
				require.NoError(t, os.WriteFile(stale, []byte("wmv"), 0o600))
				summary.Results = []converter.ConversionResult{
					{SourcePath: stale, DestPath: filepath.Join(dir, "op.mpg"), Status: converter.StatusSuccess},
				}
			}
			runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{"fluidsynth": tt.fluidsynth}}
			midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)
			p := newTestPipeline(t)

			err := p.finalizeSecondaryTree(dir, summary, midiConverter, converter.AboutDialog{})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFiles, relativeFiles(t, dir))
			content, readErr := os.ReadFile(filepath.Join(dir, "first.ks")) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantScript, string(content))
			for _, name := range tt.wantNoFiles {
				assert.NoFileExists(t, filepath.Join(dir, name))
				assert.NoDirExists(t, filepath.Join(dir, name))
			}
		})
	}
}

// TestBuildPipeline_ExecuteConvert_SecondaryArchiveFailures は、副アーカイブの展開や
// 変換に失敗すればCONVERTフェーズを止め、起動アーカイブの後処理へ進まないことを固定する。
func TestBuildPipeline_ExecuteConvert_SecondaryArchiveFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		writeBad     func(t *testing.T, path string)
		wantErr      []error
		wantNotErr   []error
		wantPrefix   string
		wantMessages []string
		wantAbsent   []string
	}{
		{
			name: "異常系: 副アーカイブ内のアセットの変換に失敗すればアーカイブ名と相対パスを添えて止める",
			writeBad: func(t *testing.T, path string) {
				t.Helper()
				writeTestXP3(t, path, map[string][]byte{
					"first.ks":       []byte("*start\n"),
					"image/bg01.tlg": []byte("not a tlg image"),
				})
			},
			wantErr: []error{ErrAssetConversionFailed},
			wantMessages: []string{
				"bad.xp3: ",
				"\n  - " + filepath.Join("image", "bg01.tlg") + ": 再試行しても解消しない変換失敗です: TLG形式ではありません",
			},
		},
		{
			name: "異常系: 副アーカイブを展開できなければフォルダから移して再実行するよう案内する",
			writeBad: func(t *testing.T, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(path, storedXP3Bytes("../evil.ks", []byte("*a\n")), 0o600))
			},
			wantErr: []error{ErrSecondaryArchiveUnreadable, parser.ErrInvalidXP3},
			// 案内にアーカイブ名が含まれるので、「bad.xp3: 」を重ねて前に付けない。
			wantPrefix: ErrSecondaryArchiveUnreadable.Error() + ": bad.xp3: ",
			wantMessages: []string{
				"bad.xp3を{dir}から別の場所へ移して再実行してください",
			},
		},
		{
			name: "異常系: ファイルとディレクトリを兼ねるエントリ名があればフォルダから移して再実行するよう案内する",
			writeBad: func(t *testing.T, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(path, storedXP3EntriesBytes([]storedEntry{
					{name: "a", data: []byte("file")},
					{name: "a/b", data: []byte("child")},
				}), 0o600))
			},
			wantErr:    []error{ErrSecondaryArchiveUnreadable, parser.ErrEntryPathConflict},
			wantNotErr: []error{parser.ErrInvalidXP3},
			wantPrefix: ErrSecondaryArchiveUnreadable.Error() + ": bad.xp3: ",
			wantMessages: []string{
				`"a" と "a/b"`,
				"bad.xp3を{dir}から別の場所へ移して再実行してください",
			},
		},
		{
			name: "異常系: アーカイブの形式によらない展開の失敗はアーカイブ名だけを添え、移す案内をしない",
			writeBad: func(t *testing.T, path string) {
				t.Helper()
				// 1要素の名前長の上限が255バイトのファイルシステムでは、それを超える名前の
				// 展開がENAMETOOLONGで失敗する。
				require.NoError(t, os.WriteFile(path, storedXP3EntriesBytes([]storedEntry{
					{name: strings.Repeat("a", 300) + ".ks", data: []byte("*a\n")},
				}), 0o600))
			},
			wantErr:    []error{syscall.ENAMETOOLONG},
			wantNotErr: []error{ErrSecondaryArchiveUnreadable, parser.ErrInvalidXP3, parser.ErrEntryPathConflict},
			wantPrefix: "bad.xp3: ",
			wantAbsent: []string{"別の場所へ移して再実行してください", ErrSecondaryArchiveUnreadable.Error()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTestXP3(t, filepath.Join(dir, "data.xp3"), map[string][]byte{
				"startup.tjs":     []byte("// startup\n"),
				"system/font.ttf": stubFont,
			})
			tt.writeBad(t, filepath.Join(dir, "bad.xp3"))
			p, _ := newSecondaryTestPipeline(t, dir, "data.xp3")

			a, err := runThroughConvert(t, p)

			require.Error(t, err)
			for _, want := range tt.wantErr {
				require.ErrorIs(t, err, want)
			}
			for _, notWant := range tt.wantNotErr {
				require.NotErrorIs(t, err, notWant)
			}
			assert.True(t, strings.HasPrefix(err.Error(), tt.wantPrefix), err.Error())
			for _, m := range tt.wantMessages {
				assert.Contains(t, err.Error(), strings.ReplaceAll(m, "{dir}", dir))
			}
			for _, m := range tt.wantAbsent {
				assert.NotContains(t, err.Error(), m)
			}
			// polyfillはcopyPolyfillFilesが書くため、これが無いことで起動アーカイブの
			// finalizeConvertedTreeへ進んでいないことを確かめる。system/は起動アーカイブの
			// font.ttfで既にあるので、ディレクトリの有無では確かめられない。
			for _, name := range []string{"PolyfillInitialize.tjs", "polyfillinitialize.tjs"} {
				assert.NoFileExists(t, filepath.Join(a.convertDir, "system", name))
			}
			rootXP3, globErr := filepath.Glob(filepath.Join(a.convertDir, "*.xp3"))
			require.NoError(t, globErr)
			assert.Empty(t, rootXP3)
		})
	}
}

// storedEntry はstoredXP3EntriesBytesに格納するエントリ。
type storedEntry struct {
	name string
	data []byte
}

// storedXP3EntriesBytes はentriesを無圧縮で並べた順に格納し、索引も無圧縮で書いた
// XP3アーカイブのバイト列を返す。
//
// why not: parser.WriteXP3Archiveで作らない。ディレクトリから格納するため、
// ファイル"a"と"a/b"のように同じ名前がファイルとディレクトリを兼ねるアーカイブは作れない。
func storedXP3EntriesBytes(entries []storedEntry) []byte {
	const headerSize = 19 // マジック(11) + 索引オフセット(8)

	le := binary.LittleEndian
	chunk := func(name string, body []byte) []byte {
		return append(le.AppendUint64([]byte(name), uint64(len(body))), body...)
	}

	var data, table []byte
	for _, e := range entries {
		offset := uint64(headerSize + len(data))
		data = append(data, e.data...)
		nameUTF16 := utf16.Encode([]rune(e.name))

		var info []byte
		info = le.AppendUint32(info, 0)
		info = le.AppendUint64(info, uint64(len(e.data)))
		info = le.AppendUint64(info, uint64(len(e.data)))
		info = le.AppendUint16(info, uint16(len(nameUTF16))) //nolint:gosec // テストで渡す名前は短い既知の値
		for _, u := range nameUTF16 {
			info = le.AppendUint16(info, u)
		}

		var segm []byte
		segm = le.AppendUint32(segm, 0)
		segm = le.AppendUint64(segm, offset)
		segm = le.AppendUint64(segm, uint64(len(e.data)))
		segm = le.AppendUint64(segm, uint64(len(e.data)))

		var body []byte
		body = append(body, chunk("info", info)...)
		body = append(body, chunk("segm", segm)...)
		body = append(body, chunk("adlr", le.AppendUint32(nil, adler32.Checksum(e.data)))...)
		table = append(table, chunk("File", body)...)
	}

	archive := append([]byte{}, parser.XP3Magic...)
	archive = le.AppendUint64(archive, uint64(headerSize+len(data)))
	archive = append(archive, data...)
	archive = append(archive, 0x00)
	archive = le.AppendUint64(archive, uint64(len(table)))

	return append(archive, table...)
}
