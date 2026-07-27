package pipeline

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/resources"
)

// skipIfCaseInsensitiveFS はdirが大文字小文字を区別しないファイルシステム
// （例: macOSのAPFS既定設定）上にある場合、このテストをスキップする。
//
// why not: os.Rename("Foo.tmp", "foo.tmp")は大文字小文字を区別しない
// ファイルシステム上では単なる同一ファイルの表記変更になり、Python版
// （pathlib経由でも同じOS依存の挙動）を含め、リネーム後も旧表記の
// パスでos.Statが成功し続ける。これはリネーム処理自体の不具合ではなく
// ファイルシステムの性質であり、Python版もこの環境では同じ挙動になる
// （レビュー指摘: 本番コードを変更すべき問題ではない）。CI/DevContainerの
// 実行環境（Linux、大文字小文字を区別する）では本テストは実際に
// リネームの旧名不在を検証する。判定はconstではなく実際のt.TempDir()を
// probeして行う（tmpディレクトリのマウント設定次第でホストのデフォルト
// と異なる場合があるため）。
func skipIfCaseInsensitiveFS(t *testing.T, dir string) {
	t.Helper()

	probe := filepath.Join(dir, "Foo.tmp")
	require.NoError(t, os.WriteFile(probe, []byte("probe"), 0o600))

	if _, err := os.Stat(filepath.Join(dir, "foo.tmp")); err == nil {
		t.Skip("大文字小文字を区別しないファイルシステムのためスキップ")
	}
}

// TestBuildPipeline_NormalizeCriticalFilenames はPython版
// TestBuildPipelineNormalizeCriticalFilenamesの移植。
func TestBuildPipeline_NormalizeCriticalFilenames(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 大文字ファイル名を小文字に変換", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		skipIfCaseInsensitiveFS(t, dir)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Config.TJS"), []byte("config"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.TXT"), []byte("readme"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "config.tjs"))
		assert.FileExists(t, filepath.Join(dir, "readme.txt"))
		assert.NoFileExists(t, filepath.Join(dir, "Data.XP3"))
		assert.NoFileExists(t, filepath.Join(dir, "Config.TJS"))
		assert.NoFileExists(t, filepath.Join(dir, "README.TXT"))
	})

	t.Run("正常系: 小文字ファイル名はそのまま", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.xp3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.tjs"), []byte("config"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "config.tjs"))
	})

	t.Run("正常系: ネストしたディレクトリ内のファイルも正規化", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		skipIfCaseInsensitiveFS(t, dir)
		nestedDir := filepath.Join(dir, "system")
		deepDir := filepath.Join(nestedDir, "plugins")
		require.NoError(t, os.MkdirAll(deepDir, 0o750))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "MainWindow.TJS"), []byte("mainwindow"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(deepDir, "Plugin.DLL"), []byte("plugin"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(nestedDir, "mainwindow.tjs"))
		assert.FileExists(t, filepath.Join(deepDir, "plugin.dll"))
		assert.NoFileExists(t, filepath.Join(dir, "Data.XP3"))
		assert.NoFileExists(t, filepath.Join(nestedDir, "MainWindow.TJS"))
		assert.NoFileExists(t, filepath.Join(deepDir, "Plugin.DLL"))
	})

	t.Run("正常系: 正規化後の名前が既に存在する場合はスキップする", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.xp3"), []byte("lower"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("upper"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "Data.XP3"))
	})
}

// TestBuildPipeline_AdjustScripts はPython版
// TestBuildPipelineAdjustScriptsの移植。ScriptAdjuster自体は別途
// script_test.goで検証済みのため、ここではpipeline側の再帰探索と
// 呼び出しが行われることを実際の内容変化で確認する。
func TestBuildPipeline_AdjustScripts(t *testing.T) {
	t.Parallel()

	t.Run("正常系: startup.tjsにポリフィル初期化ディレクティブを追加する", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		startupFile := filepath.Join(dir, "startup.tjs")
		require.NoError(t, os.WriteFile(startupFile, []byte("// original content"), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(startupFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), "// krkrsdl2 polyfill initialization")
	})

	t.Run("正常系: 大文字小文字のバリエーションを検出する", func(t *testing.T) {
		t.Parallel()

		for _, variant := range []string{"Startup.tjs", "STARTUP.TJS", "StartUp.tjs"} {
			t.Run(variant, func(t *testing.T) {
				t.Parallel()

				p := newTestPipeline(t)
				dir := t.TempDir()
				startupFile := filepath.Join(dir, variant)
				require.NoError(t, os.WriteFile(startupFile, []byte("// original content"), 0o600))

				require.NoError(t, p.adjustScripts(dir))

				content, err := os.ReadFile(startupFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
				require.NoError(t, err)
				assert.Contains(t, string(content), "// krkrsdl2 polyfill initialization")
			})
		}
	})

	t.Run("正常系: 全ての.ksファイルを再帰的に処理する", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		scenarioDir := filepath.Join(dir, "scenario")
		require.NoError(t, os.MkdirAll(scenarioDir, 0o750))
		firstKS := filepath.Join(scenarioDir, "first.ks")
		require.NoError(t, os.WriteFile(firstKS, []byte(`[loadplugin module="wuvorbis.dll"]`), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(firstKS) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), `libwuvorbis.so`)
	})
}

// TestBuildPipeline_RemovePluginDirectory はPython版
// TestBuildPipelineRemovePluginDirectoryの移植。
func TestBuildPipeline_RemovePluginDirectory(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 小文字のpluginディレクトリが削除される", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		pluginDir := filepath.Join(dir, "plugin")
		require.NoError(t, os.MkdirAll(pluginDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "test.dll"), make([]byte, 10), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.NoDirExists(t, pluginDir)
	})

	for _, dirName := range []string{"Plugin", "PLUGIN", "Plugins", "plugins", "PLUGINS"} {
		t.Run("正常系: 大文字小文字のバリエーション"+dirName+"が削除される", func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)
			dir := t.TempDir()
			pluginDir := filepath.Join(dir, dirName)
			require.NoError(t, os.MkdirAll(pluginDir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "wuvorbis.dll"), make([]byte, 10), 0o600))

			require.NoError(t, p.removePluginDirectory(dir))

			assert.NoDirExists(t, pluginDir)
		})
	}

	t.Run("正常系: pluginディレクトリがない場合は何もしない", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		otherDir := filepath.Join(dir, "other")
		require.NoError(t, os.MkdirAll(otherDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(otherDir, "test.txt"), []byte("test"), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.DirExists(t, otherDir)
		assert.FileExists(t, filepath.Join(otherDir, "test.txt"))
	})

	t.Run("正常系: ネストされたDLLファイルも含めて削除される", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		pluginDir := filepath.Join(dir, "plugin")
		subDir := filepath.Join(pluginDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "wuvorbis.dll"), make([]byte, 10), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "other.dll"), make([]byte, 10), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.NoDirExists(t, pluginDir)
	})
}

// TestBuildPipeline_CopyPolyfillFiles はPython版
// TestBuildPipelineCopyPolyfillFilesの移植。
func TestBuildPipeline_CopyPolyfillFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: systemディレクトリが作成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		require.NoError(t, copyPolyfillFilesUsing(dir, offlineFontFetcher(t)))

		assert.DirExists(t, filepath.Join(dir, "system"))
	})

	t.Run("正常系: 全ポリフィルファイルがコピーされる（SaveDataPath_patch.tjsを除く）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		require.NoError(t, copyPolyfillFilesUsing(dir, offlineFontFetcher(t)))

		systemDir := filepath.Join(dir, "system")
		for _, name := range resources.SystemPolyfillFiles {
			want, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
			require.NoError(t, err)

			got, readErr := os.ReadFile(filepath.Join(systemDir, name)) //nolint:gosec // 埋め込みリソースをコピーしたテスト用ファイルを読む用途のため妥当
			require.NoError(t, readErr)
			assert.Equal(t, want, got)
		}

		assert.NoFileExists(t, filepath.Join(systemDir, "SaveDataPath_patch.tjs"))
	})

	t.Run("正常系: フォント取得に失敗してもビルドは継続する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		fetcher := builder.NewFontFetcher(t.TempDir(), &http.Client{Transport: alwaysFailRoundTripper{}})

		err := copyPolyfillFilesUsing(dir, fetcher)

		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(dir, "system", "font.ttf"))
	})
}

// TestCopyFontFile はcopyFontFile単体の挙動を検証する。
func TestCopyFontFile(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュ済みフォントをsystem/font.ttfとしてコピーする", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()

		require.NoError(t, copyFontFile(systemDir, offlineFontFetcher(t)))

		got, err := os.ReadFile(filepath.Join(systemDir, "font.ttf")) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("fake koruri ttf content"), got)
	})

	t.Run("正常系: 既にfont.ttfが存在する場合は上書きしない", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()
		fontDest := filepath.Join(systemDir, "font.ttf")
		require.NoError(t, os.WriteFile(fontDest, []byte("existing"), 0o600))

		require.NoError(t, copyFontFile(systemDir, offlineFontFetcher(t)))

		got, err := os.ReadFile(fontDest) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("existing"), got)
	})

	t.Run("正常系: フォント取得に失敗した場合はエラーを返さずスキップする", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()
		fetcher := builder.NewFontFetcher(t.TempDir(), &http.Client{Transport: alwaysFailRoundTripper{}})

		err := copyFontFile(systemDir, fetcher)

		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(systemDir, "font.ttf"))
	})
}

// offlineFontFetcher はキャッシュ済みフォントを持つFontFetcherを返す
// （実ネットワークに触れずGetFont()が成功する）。
func offlineFontFetcher(t *testing.T) *builder.FontFetcher {
	t.Helper()

	cacheDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, builder.KoruriTTFFilename),
		[]byte("fake koruri ttf content"),
		0o600,
	))

	return builder.NewFontFetcher(cacheDir, &http.Client{Transport: alwaysFailRoundTripper{}})
}

// alwaysFailRoundTripper は常にエラーを返すhttp.RoundTripper。
// テストが誤って実ネットワークへ到達しないことを保証するために使う。
type alwaysFailRoundTripper struct{}

func (alwaysFailRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("テストでは実ネットワークへのアクセスを許可しない")
}

// fakeCommandRunner はconverter.CommandRunnerの単純なテスト用実装。
// コマンド名(nameの末尾)ごとに固定の応答を返す。
type fakeCommandRunner struct {
	// responses はコマンド名(例: "fluidsynth")をキーとする応答設定。
	responses map[string]fakeCommandResponse
}

type fakeCommandResponse struct {
	output []byte
	err    error
}

func (r fakeCommandRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	resp, ok := r.responses[name]
	if !ok {
		return nil, errors.New("予期しないコマンド: " + name)
	}

	return resp.output, resp.err
}

// TestBuildPipeline_ConvertMidiFiles はconvertMidiFilesUsingの仕様を検証する
// （converter.MidiConverterのCommandRunner注入口を使い、実プロセスの
// fluidsynth/ffmpegに触れずに検証する）。
//
// MIDIが実在するのに変換できない場合はビルドを失敗させ、MIDIを含まない
// ゲームはFluidSynth無しでも成功する、という2点が本テストの主眼。
func TestBuildPipeline_ConvertMidiFiles(t *testing.T) {
	t.Parallel()

	t.Run("異常系: MIDIが存在しfluidsynthが利用不可ならセンチネルエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFile := filepath.Join(dir, "bgm.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {err: errors.New("not found")},
		}}
		midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionUnavailable)
		assert.Contains(t, err.Error(), "fluidsynth")
		assert.Contains(t, err.Error(), "apt-get install fluidsynth fluid-soundfont-gm")
		assert.Contains(t, err.Error(), "brew install fluid-synth")
		assert.Contains(t, err.Error(), "--skip-video")
		assert.FileExists(t, midiFile)
		assert.NoFileExists(t, filepath.Join(dir, "bgm.ogg"))
	})

	t.Run("異常系: MIDIが存在しサウンドフォントが実在しないならセンチネルエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFile := filepath.Join(dir, "bgm.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		missingSoundfont := filepath.Join(dir, "absent.sf2")

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {},
		}}
		midiConverter := converter.NewMidiConverter(missingSoundfont, 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionUnavailable)
		assert.Contains(t, err.Error(), missingSoundfont)
		assert.FileExists(t, midiFile)
	})

	t.Run("異常系: 個別ファイルの変換失敗はビルドエラーとして伝播する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFile := filepath.Join(dir, "bgm.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		// fluidsynth --version（可用性確認）は成功させ、ffmpegだけを失敗させる
		// ことで、MIDI単体の変換失敗が握りつぶされないことを検証する。
		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {err: errors.New("ffmpeg exited with 1")},
		}}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionFailed)
		assert.Contains(t, err.Error(), "bgm.mid")
		assert.FileExists(t, midiFile)
	})

	t.Run("正常系: 変換成功時は.oggへ変換し元のMIDIファイルを削除する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		bgmDir := filepath.Join(dir, "bgm")
		require.NoError(t, os.MkdirAll(bgmDir, 0o750))
		midiFile := filepath.Join(bgmDir, "sinone.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {},
		}}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter))

		assert.NoFileExists(t, midiFile)
	})

	t.Run("正常系: MIDIが無ければfluidsynth未インストールでも成功する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("readme"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {err: errors.New("not found")},
		}}
		midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter))
	})
}

// fakeIconExtractor はparser.IconExtractorのテスト用実装。
type fakeIconExtractor struct {
	path string
	err  error
}

func (f fakeIconExtractor) Extract(string, string) (string, error) {
	return f.path, f.err
}

// TestBuildPipeline_FindGameIcon_FallsBackToExeExtraction はPython版
// test_extracts_icon_from_exe_when_no_icon_fileの移植。
func TestBuildPipeline_FindGameIcon_FallsBackToExeExtraction(t *testing.T) {
	t.Parallel()

	t.Run("正常系: アイコンファイルがない場合はEXEからアイコン抽出を試みる", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir

		extractedIcon := filepath.Join(extractDir, "extracted_icon.png")
		require.NoError(t, os.WriteFile(extractedIcon, []byte("\x89PNG\r\n\x1a\n"), 0o600))

		result := p.findGameIconUsing(fakeIconExtractor{path: extractedIcon})

		assert.Equal(t, extractedIcon, result)
	})

	t.Run("正常系: EXE抽出も失敗した場合は空文字列を返す", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir

		result := p.findGameIconUsing(fakeIconExtractor{err: errors.New("抽出失敗")})

		assert.Empty(t, result)
	})

	t.Run("正常系: 既存アイコンファイルが見つかった場合はEXE抽出を試みない", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir
		pngPath := filepath.Join(extractDir, "icon.png")
		require.NoError(t, os.WriteFile(pngPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))

		// このエクストラクタが呼ばれた場合は必ずエラーになるため、
		// 呼ばれていないことを間接的に検証する。
		result := p.findGameIconUsing(fakeIconExtractor{err: errors.New("呼ばれてはいけない")})

		assert.Equal(t, pngPath, result)
	})
}
