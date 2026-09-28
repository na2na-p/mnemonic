package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/cache"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/fsutil"
)

// why not: t.Setenvはt.Parallel()を呼んだテストでは使えない
// （並列実行中の他テストに環境変数の変更が影響しうるため）ので、
// このテストはt.Parallel()を呼ばない。
func TestNewSDL2SourceCache(t *testing.T) {
	tests := []struct {
		name      string
		home      string
		wantCache bool
	}{
		{
			name:      "正常系: キャッシュディレクトリ配下のキャッシュを返す",
			home:      t.TempDir(),
			wantCache: true,
		},
		{
			name: "異常系: キャッシュディレクトリを解決できない場合は原因を警告してnilを返す",
			home: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", tt.home)
			t.Setenv("XDG_CACHE_HOME", "")

			logger := &recordingLogger{}

			dir, err := cache.Dir()
			if !tt.wantCache {
				require.Error(t, err)
				assert.Nil(t, newSDL2SourceCache(logger))
				assert.Equal(t, []string{
					"キャッシュディレクトリを解決できないため、SDL2ソースのキャッシュを使わずに続行します: " + err.Error(),
				}, logger.messages("WARNING"))

				return
			}

			require.NoError(t, err)
			got := newSDL2SourceCache(logger)
			require.NotNil(t, got)
			assert.Equal(t, filepath.Join(dir, "sdl2_sources"), got.CachePath())
			assert.Empty(t, logger.messages("WARNING"))
		})
	}
}

// TestBuildPipeline_FinalizeConvertedTree_MidiFailurePrecedesScriptRewrite は
// CONVERTフェーズの後処理において、MIDI変換がスクリプト調整より先に実行される
// という順序不変条件を固定する。
//
// why: ScriptAdjusterは.mid/.midi参照を無条件に.oggへ書き換える。MIDI変換の
// 失敗がスクリプト調整より後に判明する順序だと、実体の無い.oggを指す参照へ
// 書き換えられたツリーが出来上がる。コメントだけでは順序を入れ替えても誰も
// 気付けないためテストで固定する。
func TestBuildPipeline_FinalizeConvertedTree_MidiFailurePrecedesScriptRewrite(t *testing.T) {
	t.Parallel()

	const scriptSource = `@bgm storage="bgm/opening.mid"` + "\n"

	dir := t.TempDir()
	bgmDir := filepath.Join(dir, "bgm")
	require.NoError(t, os.MkdirAll(bgmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(bgmDir, "opening.mid"), []byte("MThd"), 0o600))

	scriptPath := filepath.Join(dir, "first.ks")
	require.NoError(t, os.WriteFile(scriptPath, []byte(scriptSource), 0o600))

	p := newTestPipeline(t)
	runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
		"fluidsynth": {err: errors.New("not found")},
	}}
	midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

	err := p.finalizeConvertedTree(dir, converter.ConversionSummary{}, midiConverter)

	require.ErrorIs(t, err, ErrMidiConversionUnavailable)

	// MIDI変換より前にスクリプトが書き換えられていないこと。書き換えられて
	// いれば、実体の無いbgm/opening.oggを指す参照が残ったAPKが出来上がる。
	content, readErr := os.ReadFile(scriptPath) //nolint:gosec // テストが直前に書き込んだ一時ファイルの読み戻し
	require.NoError(t, readErr)
	assert.Equal(t, scriptSource, string(content))
	assert.NotContains(t, string(content), ".ogg")
}

// TestBuildPipeline_FinalizeConvertedTree_RemoveStaleVideoSourceFilesPrecedesNormalize は
// CONVERTフェーズの後処理において、動画の旧拡張子ファイル削除
// (removeStaleVideoSourceFiles)がファイル名正規化(normalizeCriticalFilenames、
// finalizeConvertedTreeの最後のステップ)より先に実行されるという順序不変
// 条件を固定する。MIDI変換とスクリプト調整の順序を固定する既存テスト
// （本ファイル上部）と同じくfinalizeConvertedTree自体を呼び出して検証する。
//
// why: normalizeCriticalFilenamesが先に走ると、大文字拡張子の旧ファイル
// (例: OP.WMV)が小文字にリネームされる。removeStaleVideoSourceFilesは
// summaryが記録した元のケース(.WMV)で旧ファイルパスを再構築するため、
// 既にリネーム済みだと削除に失敗し(best-effortで握りつぶされ)、旧ファイルが
// 永続的に残ってしまう。
func TestBuildPipeline_FinalizeConvertedTree_RemoveStaleVideoSourceFilesPrecedesNormalize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skipIfCaseInsensitiveFS(t, dir)

	staleFile := filepath.Join(dir, "OP.WMV")
	convertedFile := filepath.Join(dir, "OP.mpg")
	require.NoError(t, os.WriteFile(staleFile, []byte("raw copy from copyTree"), 0o600))
	require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

	// why not: copyPolyfillFiles(finalizeConvertedTreeの一部)は
	// system/font.ttfが無い場合に実ネットワークへフォントダウンロードを試みる
	// (copyFontFile参照)。あらかじめsystem/font.ttfを用意しその既存ファイル
	// ガードを通すことで、finalizeConvertedTreeを実ネットワークに触れず最後
	// まで実行できるようにする（polyfill_test.goのalwaysFailRoundTripper/
	// offlineFontFetcherが実ネットワークを避けているのと同じ方針）。
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "system"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "system", "font.ttf"), []byte("stub font"), 0o600))

	summary := converter.ConversionSummary{Results: []converter.ConversionResult{
		{
			SourcePath: filepath.Join(dir, "extract", "OP.WMV"),
			DestPath:   convertedFile,
			Status:     converter.StatusSuccess,
		},
	}}

	p := newTestPipeline(t)
	midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, fakeCommandRunner{})

	require.NoError(t, p.finalizeConvertedTree(dir, summary, midiConverter))

	// removeStaleVideoSourceFilesがnormalizeCriticalFilenamesより後に走ると、
	// 大文字のOP.WMVは既にnormalizeCriticalFilenamesによって小文字の
	// "op.wmv"へリネームされている。summaryが記録した元のケース(.WMV)で
	// 削除を試みても"OP.WMV"はもう存在せず削除に失敗し(best-effortで
	// 握りつぶされ)、リネーム後の"op.wmv"が残ってしまう。両方の表記を
	// 確認することで、順序が入れ替わった場合にこのテストが実際に落ちる
	// ことを保証する。
	assert.NoFileExists(t, staleFile)
	assert.NoFileExists(t, filepath.Join(dir, "op.wmv"))
	assert.FileExists(t, filepath.Join(dir, "op.mpg"))
}

// TestBuildPipeline_NewMidiConverter はConfig.SoundfontPathがMIDI変換器へ
// 引き渡されることを検証する（--soundfontフラグの経路の終端）。
func TestBuildPipeline_NewMidiConverter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		soundfontPath string
		want          string
	}{
		{
			name:          "正常系: 明示指定したサウンドフォントがそのまま使われる",
			soundfontPath: "/tmp/custom/MyFont.sf2",
			want:          "/tmp/custom/MyFont.sf2",
		},
		{
			name:          "正常系: 未指定なら既定の探索結果が使われる",
			soundfontPath: "",
			want:          converter.GetDefaultSoundfontPath(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := NewConfig("game.exe", "game.apk")
			config.SoundfontPath = tt.soundfontPath
			p := NewBuildPipeline(config)

			assert.Equal(t, tt.want, p.newMidiConverter().SoundfontPath())
		})
	}
}

func TestConversionFailureError(t *testing.T) {
	t.Parallel()

	extractDir := filepath.Join(t.TempDir(), "extract")
	under := func(rel string) string { return filepath.Join(extractDir, rel) }

	const videoHint = "\n動画を変換しない場合は --skip-video を指定してください"

	// cappedResults は変換元パス順で先頭からn件の失敗結果を、並びを崩して返す。
	cappedResults := func(n int) []converter.ConversionResult {
		results := make([]converter.ConversionResult, 0, n)
		for i := range n {
			results = append(results, converter.ConversionResult{
				SourcePath: under(fmt.Sprintf("image/bg%02d.tlg", n-1-i)),
				Status:     converter.StatusFailed,
				Message:    "TLG形式ではありません",
			})
		}

		return results
	}
	cappedLines := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "\n  - %s: TLG形式ではありません", filepath.Join("image", fmt.Sprintf("bg%02d.tlg", i)))
		}

		return b.String()
	}

	tests := []struct {
		name    string
		results []converter.ConversionResult
		wantErr string
	}{
		{
			name: "正常系: 失敗が無ければnilを返す",
			results: []converter.ConversionResult{
				{SourcePath: under("a.ks"), Status: converter.StatusSuccess},
				{SourcePath: under("b.tlg"), Status: converter.StatusSkipped, Message: "変換不要"},
			},
		},
		{
			name: "異常系: 1件の失敗をextractDirからの相対パスで1行に報告する",
			results: []converter.ConversionResult{
				{SourcePath: under("image/bg.tlg"), Status: converter.StatusFailed, Message: "TLG形式ではありません"},
			},
			wantErr: "アセットの変換に失敗しました（1 件）\n" +
				"  - " + filepath.Join("image", "bg.tlg") + ": TLG形式ではありません",
		},
		{
			name: "異常系: 失敗した結果だけを変換元パス順に1件1行で報告する",
			results: []converter.ConversionResult{
				{SourcePath: under("script/c.ks"), Status: converter.StatusFailed, Message: "文字コードを判定できません"},
				{SourcePath: under("first.ks"), Status: converter.StatusSuccess},
				{SourcePath: under("image/bg.tlg"), Status: converter.StatusFailed, Message: "TLG形式ではありません"},
				{SourcePath: under("image/ev.tlg"), Status: converter.StatusSkipped, Message: "変換不要"},
				{SourcePath: under("image/aa.bmp"), Status: converter.StatusFailed, Message: "BMPを読めません"},
			},
			wantErr: "アセットの変換に失敗しました（3 件）\n" +
				"  - " + filepath.Join("image", "aa.bmp") + ": BMPを読めません\n" +
				"  - " + filepath.Join("image", "bg.tlg") + ": TLG形式ではありません\n" +
				"  - " + filepath.Join("script", "c.ks") + ": 文字コードを判定できません",
		},
		{
			name: "異常系: 成功とスキップ以外の状態はConversionManagerの集計と同じく失敗として報告する",
			results: []converter.ConversionResult{
				{SourcePath: under("first.ks"), Status: "", Message: "状態不明"},
			},
			wantErr: "アセットの変換に失敗しました（1 件）\n  - first.ks: 状態不明",
		},
		{
			name:    "異常系: 20件を超える失敗は変換元パス順の先頭20件だけを列挙し残りを件数で示す",
			results: cappedResults(25),
			wantErr: "アセットの変換に失敗しました（25 件）" + cappedLines(20) + "\n  …ほか 5 件",
		},
		{
			name:    "異常系: ちょうど20件の失敗は省略せずに全件列挙する",
			results: cappedResults(20),
			wantErr: "アセットの変換に失敗しました（20 件）" + cappedLines(20),
		},
		{
			name: "異常系: 動画の変換に失敗した場合は--skip-videoの案内を1回だけ末尾に付ける",
			results: []converter.ConversionResult{
				{SourcePath: under("video/op.wmv"), Status: converter.StatusFailed, Message: "動画変換に失敗しました"},
				{SourcePath: under("video/ed.AVI"), Status: converter.StatusFailed, Message: "動画変換に失敗しました"},
			},
			wantErr: "アセットの変換に失敗しました（2 件）\n" +
				"  - " + filepath.Join("video", "ed.AVI") + ": 動画変換に失敗しました\n" +
				"  - " + filepath.Join("video", "op.wmv") + ": 動画変換に失敗しました" +
				videoHint,
		},
		{
			name: "異常系: 列挙から省略された失敗が動画でも--skip-videoを案内する",
			results: append(cappedResults(20), converter.ConversionResult{
				SourcePath: under("video/op.mpg"), Status: converter.StatusFailed, Message: "動画変換に失敗しました",
			}),
			wantErr: "アセットの変換に失敗しました（21 件）" + cappedLines(20) + "\n  …ほか 1 件" + videoHint,
		},
		{
			name: "異常系: extractDirからの相対パスにできない変換元はそのままのパスで報告する",
			results: []converter.ConversionResult{
				{SourcePath: "image/bg.tlg", Status: converter.StatusFailed, Message: "TLG形式ではありません"},
			},
			wantErr: "アセットの変換に失敗しました（1 件）\n  - image/bg.tlg: TLG形式ではありません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := conversionFailureError(extractDir, converter.ConversionSummary{Results: tt.results})

			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, ErrAssetConversionFailed)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestLogPreferredSourceSkips(t *testing.T) {
	t.Parallel()

	extractDir := filepath.Join(t.TempDir(), "extract")
	under := func(rel string) string { return filepath.Join(extractDir, rel) }
	loser := func(rel, winnerRel string) converter.ConversionResult {
		dest := filepath.Join(t.TempDir(), "convert", strings.TrimSuffix(rel, filepath.Ext(rel))+filepath.Ext(winnerRel))

		return converter.ConversionResult{
			SourcePath: under(rel),
			DestPath:   dest,
			Status:     converter.StatusSkipped,
			Message:    fmt.Sprintf("同名の %s を優先したため変換しません", filepath.FromSlash(winnerRel)),
		}
	}
	losers := func(n int) []converter.ConversionResult {
		results := make([]converter.ConversionResult, 0, n)
		for i := range n {
			results = append(results, loser(fmt.Sprintf("video/op%02d.wmv", n-1-i), fmt.Sprintf("video/op%02d.mpg", n-1-i)))
		}

		return results
	}
	loserLines := func(n int) []string {
		lines := make([]string, 0, n)
		for i := range n {
			lines = append(lines, fmt.Sprintf("%s: 同名の %s を優先したため変換しません",
				filepath.Join("video", fmt.Sprintf("op%02d.wmv", i)), filepath.Join("video", fmt.Sprintf("op%02d.mpg", i))))
		}

		return lines
	}

	tests := []struct {
		name    string
		results []converter.ConversionResult
		want    []string
	}{
		{
			name: "正常系: 出力先が重複して別の変換元を優先したスキップだけを1件1行でINFOに出す",
			results: []converter.ConversionResult{
				loser("video/op.wmv", "video/op.mpg"),
				{SourcePath: under("video/op.mpg"), DestPath: filepath.Join(t.TempDir(), "op.mpg"), Status: converter.StatusSuccess},
				{
					SourcePath: under("scenario/first.ks"), DestPath: filepath.Join(t.TempDir(), "first.ks"),
					Status: converter.StatusSkipped, Message: "既にターゲットエンコーディングです",
				},
				{SourcePath: under("scenario/plain.tjs"), Status: converter.StatusSkipped, Message: "調整が不要なファイルです"},
				{SourcePath: under("image/bg.tlg"), Status: converter.StatusFailed, Message: "TLG形式ではありません"},
			},
			want: []string{
				filepath.Join("video", "op.wmv") + ": 同名の " + filepath.Join("video", "op.mpg") + " を優先したため変換しません",
			},
		},
		{
			name: "正常系: 該当するスキップが無ければ何も出さない",
			results: []converter.ConversionResult{
				{
					SourcePath: under("scenario/first.ks"), DestPath: filepath.Join(t.TempDir(), "first.ks"),
					Status: converter.StatusSkipped, Message: "既にターゲットエンコーディングです",
				},
			},
		},
		{
			name:    "正常系: 20件を超える場合は変換元パス順の先頭20件だけを出し残りを件数で示す",
			results: losers(22),
			want:    append(loserLines(20), "…ほか 2 件"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := &recordingLogger{}

			logPreferredSourceSkips(logger, extractDir, converter.ConversionSummary{Results: tt.results})

			assert.Equal(t, tt.want, logger.messages("INFO"))
			assert.Empty(t, logger.messages("WARNING"))
		})
	}
}

// newTemplatePreparerはcache.Dir()でSDL2ソースキャッシュの場所を決め、解決に失敗すると
// その旨も警告する。cache.Dir()はHOMEなどの環境変数から組み立てるため、各ケースで
// t.Setenvにより環境変数を固定する。t.Setenvは並行する祖先を持つテストから呼べない
// ため、本テストもサブテストもt.Parallel()にできない。
func TestBuildPipeline_NewTemplatePreparer(t *testing.T) {
	tests := []struct {
		name             string
		unresolvableHome bool
		message          string
	}{
		{
			name:    "正常系: TemplatePreparerの警告をパイプラインのLoggerへWARNINGとして流す",
			message: "x",
		},
		{
			name:             "正常系: ホームディレクトリを解決できなくてもキャッシュを使えない旨を警告したうえで警告を流す",
			unresolvableHome: true,
			message:          "x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unresolvableHome {
				t.Setenv("HOME", "")
				t.Setenv("USERPROFILE", "")
			} else {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("USERPROFILE", t.TempDir())
				t.Setenv("XDG_CACHE_HOME", t.TempDir())
				t.Setenv("LOCALAPPDATA", t.TempDir())
			}

			b := newTestPipeline(t)
			logger := &recordingLogger{}
			b.SetLogger(logger)

			b.newTemplatePreparer(t.TempDir()).Warn(tc.message)

			warnings := logger.messages("WARNING")
			if !tc.unresolvableHome {
				assert.Equal(t, []string{tc.message}, warnings)

				return
			}

			require.Len(t, warnings, 2)
			assert.Contains(t, warnings[0], "キャッシュディレクトリを解決できないため、SDL2ソースのキャッシュを使わずに続行します")
			assert.Equal(t, tc.message, warnings[1])
		})
	}
}

func TestLogConversionNotes(t *testing.T) {
	t.Parallel()

	sourceDir := filepath.Join(t.TempDir(), "extract")

	tests := []struct {
		name    string
		results []converter.ConversionResult
		want    []string
	}{
		{
			name: "正常系: Messageを持つ成功した結果はsourceDirからの相対パスとともに1行記録する",
			results: []converter.ConversionResult{
				{SourcePath: filepath.Join(sourceDir, "data", "name.csv"), Status: converter.StatusSuccess, Message: "推定結果なし、shift_jis として復号"},
			},
			want: []string{filepath.Join("data", "name.csv") + ": 推定結果なし、shift_jis として復号"},
		},
		{
			name: "正常系: Messageの無い成功とSKIPPEDと失敗は記録しない",
			results: []converter.ConversionResult{
				{SourcePath: filepath.Join(sourceDir, "first.ks"), Status: converter.StatusSuccess},
				{SourcePath: filepath.Join(sourceDir, "readme.txt"), Status: converter.StatusSkipped, Message: "既にターゲットエンコーディングです"},
				{SourcePath: filepath.Join(sourceDir, "bg.tlg"), Status: converter.StatusFailed, Message: "TLG形式ではありません"},
			},
		},
		{
			name: "正常系: 並列ワーカーの完了順によらず変換元パス順に記録する",
			results: []converter.ConversionResult{
				{SourcePath: filepath.Join(sourceDir, "b.csv"), Status: converter.StatusSuccess, Message: "b"},
				{SourcePath: filepath.Join(sourceDir, "a.csv"), Status: converter.StatusSuccess, Message: "a"},
			},
			want: []string{"a.csv: a", "b.csv: b"},
		},
		{
			name: "正常系: sourceDirからの相対パスにできない変換元はそのまま示す",
			results: []converter.ConversionResult{
				{SourcePath: filepath.Join("relative", "name.csv"), Status: converter.StatusSuccess, Message: "m"},
			},
			want: []string{filepath.Join("relative", "name.csv") + ": m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := &recordingLogger{}

			logConversionNotes(logger, sourceDir, tt.results)

			assert.Equal(t, tt.want, logger.messages("VERBOSE"))
			assert.Empty(t, logger.messages("INFO"))
		})
	}
}

func TestBuildPipeline_ExecuteConvert_LogsConversionNotes(t *testing.T) {
	t.Parallel()

	extractDir := t.TempDir()
	files := map[string]string{
		"first.ks":        "*start\n吾輩は猫である。名前はまだ無い。\n",
		"system/font.ttf": "stub font",
		// Shift_JISの"猫"。chardetは候補を1つも返さない。
		"data/name.csv": "\x94\x4c",
	}
	for name, content := range files {
		path := filepath.Join(extractDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	p := newTestPipeline(t)
	t.Cleanup(p.cleanupTempDirs)
	logger := &recordingLogger{}
	p.SetLogger(logger)

	_, err := p.executeConvert(buildArtifacts{extractDir: extractDir})

	require.NoError(t, err)
	assert.Contains(t, logger.messages("VERBOSE"), filepath.Join("data", "name.csv")+": 推定結果なし、shift_jis として復号")
}

// TestBuildPipeline_ExecuteAnalyze_XP3 は、infoのflagsのビット31（krkrz
// base/XP3Archive.h のTVP_XP3_FILE_PROTECTED）が立ったXP3も、ANALYZEフェーズで
// 拒否せずに受け付けることを検証する。
func TestBuildPipeline_ExecuteAnalyze_XP3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		infoFlags uint32
	}{
		{"正常系: 保護フラグの無いXP3を受け付ける", 0},
		{"正常系: 保護フラグ（ビット31）が立ったXP3も受け付ける", 0x80000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, "data.xp3")
			require.NoError(t, os.WriteFile(input, storedXP3BytesWithInfoFlags("startup.tjs", tt.infoFlags, []byte("// startup")), 0o600))

			p := NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))
			t.Cleanup(p.cleanupTempDirs)

			_, err := p.executeAnalyze(buildArtifacts{})

			require.NoError(t, err)
		})
	}
}

func TestBuildPipeline_ExecuteExtract_ChecksFreeSpace(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte{'s'}, 100)
	archive := storedXP3Bytes("startup.tjs", payload)
	// krkrzと同じく、EXE内のXP3はオフセット16以降の16バイト境界からしか探されない。
	exeStub := []byte("MZ-stub-16-bytes")
	exe := append(append([]byte{}, exeStub...), archive...)
	planned := uint64(len(payload))
	secondArchive := storedXP3Bytes("second.tjs", payload)
	twoArchivesExe := append(append(append([]byte{}, exeStub...), archive...), secondArchive...)
	errStatfs := errors.New("statfs failed")

	fixedFree := func(free uint64) func(string) (uint64, error) {
		return func(string) (uint64, error) { return free, nil }
	}

	tests := []struct {
		name        string
		inputName   string
		input       []byte
		freeSpace   func(string) (uint64, error)
		wantErr     error
		wantMessage string
		wantFree    uint64
		wantWarning string
		wantFiles   []string
	}{
		{
			name:      "正常系: 空き容量が展開結果3つ分ちょうどならXP3を展開する",
			inputName: "data.xp3",
			input:     archive,
			freeSpace: fixedFree(3 * planned),
			wantFiles: []string{"startup.tjs"},
		},
		{
			name:        "異常系: 空き容量が展開結果3つ分に1バイト足りなければ何も展開せずにエラーを返す",
			inputName:   "data.xp3",
			input:       archive,
			freeSpace:   fixedFree(3*planned - 1),
			wantErr:     ErrInsufficientDiskSpace,
			wantMessage: "展開に必要な容量 300 B が一時ディレクトリ ",
			wantFree:    3*planned - 1,
			wantFiles:   []string{},
		},
		{
			name:      "正常系: EXEも書き出し済みの埋め込みXP3を数えず展開結果3つ分の空き容量で展開する",
			inputName: "game.exe",
			input:     exe,
			freeSpace: fixedFree(3 * planned),
			wantFiles: []string{"startup.tjs"},
		},
		{
			name:        "異常系: EXEで空き容量が展開結果3つ分に1バイト足りなければ何も展開せずにエラーを返す",
			inputName:   "game.exe",
			input:       exe,
			freeSpace:   fixedFree(3*planned - 1),
			wantErr:     ErrInsufficientDiskSpace,
			wantMessage: "展開に必要な容量 300 B が一時ディレクトリ ",
			wantFree:    3*planned - 1,
			wantFiles:   []string{},
		},
		{
			name:      "正常系: EXEの最初のXP3の後ろに続くXP3は展開せず、EXE終端までを1つのXP3として書き出す",
			inputName: "game.exe",
			input:     twoArchivesExe,
			freeSpace: fixedFree(3 * planned),
			wantFiles: []string{"startup.tjs"},
		},
		{
			name:      "正常系: 空き容量が非常に大きくても比較できる",
			inputName: "data.xp3",
			input:     archive,
			freeSpace: fixedFree(math.MaxUint64),
			wantFiles: []string{"startup.tjs"},
		},
		{
			name:        "正常系: 空き容量を取得できなければ警告して展開を続ける",
			inputName:   "data.xp3",
			input:       archive,
			freeSpace:   func(string) (uint64, error) { return 0, errStatfs },
			wantWarning: "一時ディレクトリの空き容量を取得できないため、展開前の容量確認を省略します: statfs failed",
			wantFiles:   []string{"startup.tjs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, tt.inputName)
			require.NoError(t, os.WriteFile(input, tt.input, 0o600))

			p := NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))
			t.Cleanup(p.cleanupTempDirs)
			logger := &recordingLogger{}
			p.SetLogger(logger)
			var queried []string
			p.freeSpace = func(path string) (uint64, error) {
				queried = append(queried, path)
				return tt.freeSpace(path)
			}

			analyzed, err := p.executeAnalyze(buildArtifacts{})
			require.NoError(t, err)

			got, err := p.executeExtract(analyzed)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantMessage)
				assert.Contains(t, err.Error(), "一時ディレクトリ "+filepath.Dir(got.extractDir)+" の空き容量 "+fsutil.FormatSize(int64(tt.wantFree))+" を超えています")
				assert.Contains(t, err.Error(), "空き容量のあるディレクトリを環境変数TMPDIR（WindowsではTMP）に指定して再実行してください")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, []string{got.extractDir}, queried)
			if tt.wantWarning != "" {
				assert.Equal(t, []string{tt.wantWarning}, logger.messages("WARNING"))
			} else {
				assert.Empty(t, logger.messages("WARNING"))
			}
			entries, err := os.ReadDir(got.extractDir)
			require.NoError(t, err)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			assert.ElementsMatch(t, tt.wantFiles, names)
		})
	}
}

func TestRequiredExtractSpace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		planned []int64
		want    int64
	}{
		{"正常系: 見積もりの3つ分", []int64{100}, 300},
		{"正常系: 複数アーカイブの見積もりは合計してから3つ分にする", []int64{60, 40}, 300},
		{"正常系: アーカイブが無ければ0", nil, 0},
		{"正常系: 3つ分がint64をちょうど超えない場合はそのまま返す", []int64{math.MaxInt64 / 3}, math.MaxInt64 / 3 * 3},
		{"正常系: 3つ分がint64を超える場合は最大値で飽和する", []int64{math.MaxInt64/3 + 1}, math.MaxInt64},
		{"正常系: 複数アーカイブの見積もりの合計がint64を超える場合は最大値で飽和する", []int64{math.MaxInt64, 2}, math.MaxInt64},
		{"正常系: 複数アーカイブの見積もりの合計が大きく超える場合も最大値で飽和する", []int64{math.MaxInt64 - 1, 1000}, math.MaxInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, requiredExtractSpace(tt.planned))
		})
	}
}

func TestBuildPipeline_ExecuteExtract_KeepsEmbeddedArchiveOutOfExtractDir(t *testing.T) {
	t.Parallel()

	archive := storedXP3Bytes("startup.tjs", []byte("// startup"))
	exe := append([]byte("MZ-stub-16-bytes"), archive...)

	tests := []struct {
		name             string
		inputName        string
		input            []byte
		wantEmbeddedName string
	}{
		{
			name:             "正常系: EXEの埋め込みXP3は展開先とは別の一時ディレクトリに<stem>_0.xp3として書き出す",
			inputName:        "game.exe",
			input:            exe,
			wantEmbeddedName: "game_0.xp3",
		},
		{
			name:             "正常系: 大文字の拡張子のEXEも埋め込みXP3を展開先に置かない",
			inputName:        "ゲーム.EXE",
			input:            exe,
			wantEmbeddedName: "ゲーム_0.xp3",
		},
		{
			name:      "正常系: XP3の入力では展開先以外の一時ディレクトリを作らない",
			inputName: "data.xp3",
			input:     archive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			input := filepath.Join(dir, tt.inputName)
			require.NoError(t, os.WriteFile(input, tt.input, 0o600))

			p := NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))
			p.freeSpace = func(string) (uint64, error) { return math.MaxUint64, nil }

			analyzed, err := p.executeAnalyze(buildArtifacts{})
			require.NoError(t, err)

			got, err := p.executeExtract(analyzed)
			require.NoError(t, err)

			entries, err := os.ReadDir(got.extractDir)
			require.NoError(t, err)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			assert.Equal(t, []string{"startup.tjs"}, names)

			otherDirs := slices.DeleteFunc(slices.Clone(p.tempDirs), func(d string) bool { return d == got.extractDir })
			var embeddedPaths []string
			if tt.wantEmbeddedName != "" {
				require.Len(t, otherDirs, 1)
				embeddedPath := filepath.Join(otherDirs[0], tt.wantEmbeddedName)
				assert.FileExists(t, embeddedPath)
				assert.Equal(t, filepath.Dir(got.extractDir), filepath.Dir(otherDirs[0]))
				embeddedPaths = append(embeddedPaths, embeddedPath)
			} else {
				assert.Empty(t, otherDirs)
			}

			p.cleanupTempDirs()

			assert.NoDirExists(t, got.extractDir)
			for _, path := range embeddedPaths {
				assert.NoFileExists(t, path)
				assert.NoDirExists(t, filepath.Dir(path))
			}
		})
	}
}

func TestBuildPipeline_ExecuteAnalyze_ChoosesStartupArchive(t *testing.T) {
	t.Parallel()

	dataArchive := storedXP3Bytes("startup.tjs", []byte("// data"))
	embeddedExe := append([]byte("MZ-stub-16-bytes"), storedXP3Bytes("startup.tjs", []byte("// embedded"))...)
	plainExe := []byte("MZ-stub-16-bytes-without-archive")

	tests := []struct {
		name         string
		input        string
		files        map[string][]byte
		dirs         []string
		wantStartup  string
		wantInfo     []string
		wantWarnings []string
	}{
		{
			name:        "正常系: XP3を埋め込んでいないEXEは同じフォルダのdata.xp3を展開し、そのことを知らせる",
			input:       "game.exe",
			files:       map[string][]byte{"game.exe": plainExe, "data.xp3": dataArchive},
			wantStartup: "// data",
			wantInfo:    []string{"EXEと同じフォルダのdata.xp3を読み込みます: {dir}data.xp3"},
		},
		{
			name:        "正常系: EXEがXP3を埋め込んでいてもdata.xp3を展開し、埋め込みXP3を使わないことも知らせる",
			input:       "game.exe",
			files:       map[string][]byte{"game.exe": embeddedExe, "data.xp3": dataArchive},
			wantStartup: "// data",
			wantInfo:    []string{"EXEと同じフォルダのdata.xp3を読み込みます（EXEに埋め込まれたXP3アーカイブは使いません）: {dir}data.xp3"},
		},
		{
			name:         "正常系: Windows版がdata.xp3の代わりにcontent-dataフォルダを読み込む場合は警告する",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": plainExe, "data.xp3": dataArchive},
			dirs:         []string{"content-data"},
			wantStartup:  "// data",
			wantInfo:     []string{"EXEと同じフォルダのdata.xp3を読み込みます: {dir}data.xp3"},
			wantWarnings: []string{"Windows版は同じフォルダのcontent-dataフォルダを読み込みますが、APKにはdata.xp3の内容を含めます"},
		},
		{
			name:        "正常系: Windows版が埋め込みXP3の代わりにdata.exeを読み込む場合は警告し、続けて展開しない.xp3ファイルを警告する",
			input:       "game.exe",
			files:       map[string][]byte{"game.exe": embeddedExe, "data.exe": plainExe, "patch.xp3": dataArchive},
			wantStartup: "// embedded",
			wantWarnings: []string{
				"Windows版は同じフォルダのdata.exeを読み込みますが、APKにはgame.exeに埋め込まれたXP3アーカイブの内容を含めます",
				"game.exeと同じフォルダにある次のXP3アーカイブは読み込まれず、APKに含まれません: patch.xp3",
			},
		},
		{
			name:        "正常系: data.xp3が無ければEXEの埋め込みXP3を展開する",
			input:       "game.exe",
			files:       map[string][]byte{"game.exe": embeddedExe},
			wantStartup: "// embedded",
		},
		{
			name:  "正常系: 展開しない.xp3ファイルを1行の警告で知らせる",
			input: "game.exe",
			files: map[string][]byte{
				"game.exe":  plainExe,
				"data.xp3":  dataArchive,
				"voice.xp3": dataArchive,
				"patch.xp3": dataArchive,
			},
			wantStartup:  "// data",
			wantInfo:     []string{"EXEと同じフォルダのdata.xp3を読み込みます: {dir}data.xp3"},
			wantWarnings: []string{"data.xp3と同じフォルダにある次のXP3アーカイブは読み込まれず、APKに含まれません: patch.xp3, voice.xp3"},
		},
		{
			name:         "正常系: 埋め込みXP3を展開する場合も同じフォルダの.xp3ファイルを警告する",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "patch.xp3": dataArchive},
			wantStartup:  "// embedded",
			wantWarnings: []string{"game.exeと同じフォルダにある次のXP3アーカイブは読み込まれず、APKに含まれません: patch.xp3"},
		},
		{
			name:         "正常系: XP3の入力でも同じフォルダの他の.xp3ファイルを警告する",
			input:        "data.xp3",
			files:        map[string][]byte{"data.xp3": dataArchive, "bgm.xp3": dataArchive},
			wantStartup:  "// data",
			wantWarnings: []string{"data.xp3と同じフォルダにある次のXP3アーカイブは読み込まれず、APKに含まれません: bgm.xp3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, data := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
			}
			for _, name := range tt.dirs {
				require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o750))
			}

			p := NewBuildPipeline(NewConfig(filepath.Join(dir, tt.input), filepath.Join(dir, "output.apk")))
			t.Cleanup(p.cleanupTempDirs)
			p.freeSpace = func(string) (uint64, error) { return math.MaxUint64, nil }
			logger := &recordingLogger{}
			p.SetLogger(logger)

			analyzed, err := p.executeAnalyze(buildArtifacts{})
			require.NoError(t, err)
			got, err := p.executeExtract(analyzed)
			require.NoError(t, err)

			startup, err := os.ReadFile(filepath.Join(got.extractDir, "startup.tjs"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantStartup, string(startup))
			var wantInfo []string
			for _, m := range tt.wantInfo {
				wantInfo = append(wantInfo, strings.ReplaceAll(m, "{dir}", dir+string(filepath.Separator)))
			}
			assert.Equal(t, wantInfo, logger.messages("INFO"))
			assert.Equal(t, tt.wantWarnings, logger.messages("WARNING"))
		})
	}
}
