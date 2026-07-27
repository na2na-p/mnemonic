package doctor_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/doctor"
)

// findDependency はdoctor.Dependencies（本番の依存ツール一覧）からnameに一致する
// エントリを検索する。テストを本番のDependencyInfoと突き合わせて検証するために使う。
func findDependency(t *testing.T, name string) doctor.DependencyInfo {
	t.Helper()

	for _, dep := range doctor.Dependencies {
		if dep.Name == name {
			return dep
		}
	}

	t.Fatalf("dependency %q not found in doctor.Dependencies", name)

	return doctor.DependencyInfo{}
}

func TestDependencies_Count(t *testing.T) {
	t.Parallel()

	assert.Len(t, doctor.Dependencies, 5)
}

func TestDependencies_ContainsRequiredTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		caseName        string // t.Run表示用の日本語テストケース名
		name            string // doctor.Dependencies内の検索キー（DependencyInfo.Name）
		expectedCommand string
		expectedRequire bool
	}{
		{
			caseName: "正常系: Java_JDKが必須依存として登録されている", name: "Java JDK",
			expectedCommand: "java", expectedRequire: true,
		},
		{
			caseName: "正常系: Android_SDKが必須依存として登録されている", name: "Android SDK",
			expectedCommand: "sdkmanager", expectedRequire: true,
		},
		{
			caseName: "正常系: Android_NDKが必須依存として登録されている", name: "Android NDK",
			expectedCommand: "ndk-build", expectedRequire: true,
		},
		{caseName: "正常系: FFmpegが必須依存として登録されている", name: "FFmpeg", expectedCommand: "ffmpeg", expectedRequire: true},
	}

	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			t.Parallel()

			var matched *doctor.DependencyInfo
			for i := range doctor.Dependencies {
				if doctor.Dependencies[i].Name == tt.name {
					matched = &doctor.Dependencies[i]
				}
			}

			require.NotNil(t, matched)
			assert.Equal(t, tt.expectedCommand, matched.Command)
			assert.Equal(t, tt.expectedRequire, matched.Required)
		})
	}
}

// TestDependencies_ContainsOptionalTools はMIDI変換に使うFluidSynthが
// 条件付き依存（Required=false かつ 条件を説明するNote付き）として登録されて
// いることを検証する。
//
// why: FluidSynthはMIDIアセットを含むゲームのビルドでは必須だが、含まない
// ゲームでは不要である。doctor全体をブロックしないためRequired=falseのまま
// 据え置き、代わりにNoteで「MIDIを含むゲームでは必須」という条件を利用者へ
// 伝える。Required=trueにするとMIDIを持たないゲームのビルドまで
// FluidSynthのインストールを強制することになる。
func TestDependencies_ContainsOptionalTools(t *testing.T) {
	t.Parallel()

	dep := findDependency(t, "FluidSynth")

	assert.Equal(t, "fluidsynth", dep.Command)
	assert.Equal(t, "--version", dep.VersionFlag)
	assert.False(t, dep.Required)
	assert.Contains(t, dep.Note, "MIDI")
}

// TestCheckDependency_NoteIsSurfacedWhenMissing は条件付き依存が見つからない
// 場合、その条件（Note）が利用者向けメッセージへ現れることを検証する。
//
// why: doctorが「オプション」とだけ表示すると、MIDIを含むゲームでビルドが
// 失敗する理由を利用者が事前に知る手段が無くなる（T-220の無音APK問題）。
func TestCheckDependency_NoteIsSurfacedWhenMissing(t *testing.T) {
	t.Parallel()

	// 存在しないコマンドなので、reasonは必ず「見つかりません」側になる。
	const missingCommand = "nonexistent_command_xyz123"

	baseReason := "コマンド '" + missingCommand + "' が見つかりません"

	tests := []struct {
		name        string
		note        string
		wantMessage string
	}{
		{
			name:        "正常系: 見つからない場合はNoteがメッセージへ追記される",
			note:        "MIDIを含むゲームのビルドには必須です",
			wantMessage: baseReason + "。MIDIを含むゲームのビルドには必須です",
		},
		{
			name:        "正常系: Noteが空ならメッセージは理由のみで追記されない",
			note:        "",
			wantMessage: baseReason,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := doctor.DependencyInfo{
				Name: "Test", Command: missingCommand, VersionFlag: "--version", Required: false, Note: tt.note,
			}

			result := doctor.CheckDependency(info)

			require.False(t, result.Found)
			// Equalで固定する。Containsだけだと、Noteの有無に関わらず一定の
			// 文言を後置するような実装でも通ってしまう。
			assert.Equal(t, tt.wantMessage, result.Message)
		})
	}
}

// TestCheckDependency_PostCheck はコマンドが見つかっても追加検査に失敗した場合、
// 「見つからない」扱いとして理由が表示されることを検証する。
//
// why: fluidsynthはサウンドフォントを同梱しないため「コマンドはあるが
// サウンドフォントが無い」状態が起こりやすく、これをOKと表示すると
// MIDIを含むゲームのビルドが失敗する理由を利用者が事前に知る手段が無くなる。
func TestCheckDependency_PostCheck(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not found in PATH")
	}

	tests := []struct {
		name        string
		postCheck   func() (bool, string)
		wantFound   bool
		wantMessage string
	}{
		{
			name:        "正常系: 追加検査が成功すればFoundのまま",
			postCheck:   func() (bool, string) { return true, "" },
			wantFound:   true,
			wantMessage: "",
		},
		{
			name:        "異常系: 追加検査が失敗すれば理由付きでNGになる",
			postCheck:   func() (bool, string) { return false, "サウンドフォントが見つかりません: /path/to.sf2" },
			wantFound:   false,
			wantMessage: "サウンドフォントが見つかりません: /path/to.sf2",
		},
		{
			name:        "正常系: 追加検査が未設定ならFoundのまま",
			postCheck:   nil,
			wantFound:   true,
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := doctor.DependencyInfo{
				Name: "Test", Command: "go", VersionFlag: "version", Required: false, PostCheck: tt.postCheck,
			}

			result := doctor.CheckDependency(info)

			assert.Equal(t, tt.wantFound, result.Found)
			assert.Equal(t, tt.wantMessage, result.Message)
			// 追加検査の成否に関わらず、取得済みのバージョンは保持する。
			assert.NotEmpty(t, result.Version)
		})
	}
}

// TestDependencies_FluidSynthHasSoundfontPostCheck は本番のFluidSynthエントリに
// サウンドフォントの追加検査が結び付いていることを検証する。
//
// why not: t.Parallel()を呼ばない。converterのパッケージ変数（探索先パス）を
// 一時的に書き換えるため、同パッケージ内でCheckAllDependenciesを呼ぶ並列テスト
// と競合する。非並列テストは並列テストの再開前に完走するため、これで直列化できる。
func TestDependencies_FluidSynthHasSoundfontPostCheck(t *testing.T) {
	dep := findDependency(t, "FluidSynth")

	require.NotNil(t, dep.PostCheck)

	// 既定の探索先を実在しないパスへ差し替え、追加検査が失敗を報告することを確認する。
	origMuseScore := converter.MuseScoreSoundfontPath
	origFluidR3 := converter.FluidR3SoundfontPath

	t.Cleanup(func() {
		converter.MuseScoreSoundfontPath = origMuseScore
		converter.FluidR3SoundfontPath = origFluidR3
	})

	converter.MuseScoreSoundfontPath = filepath.Join(t.TempDir(), "absent.sf3")
	converter.FluidR3SoundfontPath = filepath.Join(t.TempDir(), "absent.sf2")

	ok, reason := dep.PostCheck()

	assert.False(t, ok)
	assert.Contains(t, reason, "サウンドフォントが見つかりません")
	assert.Contains(t, reason, "--soundfont")
}

func TestDependencies_AllHaveVersionFlag(t *testing.T) {
	t.Parallel()

	for _, dep := range doctor.Dependencies {
		assert.NotEmpty(t, dep.VersionFlag)
	}
}

func TestCheckDependency_ReturnsCheckResult(t *testing.T) {
	t.Parallel()

	info := doctor.DependencyInfo{Name: "Test", Command: "nonexistent_command_xyz123", VersionFlag: "--version", Required: true}

	result := doctor.CheckDependency(info)

	assert.Equal(t, "Test", result.Name)
}

func TestCheckDependency_FoundStatus(t *testing.T) {
	t.Parallel()

	// why: doctor.Dependenciesの実際のFFmpegエントリ（command="ffmpeg"）を
	// 参照する。テストが独自に"ffmpeg"のようなハードコードしたコマンド名で
	// 検証すると、本番のDependencyInfo.Command（"ffmpeg"）が実行環境で解決
	// できなくなった場合にテストだけが誤って通り続けてしまうため、必ず
	// 本番の定義から拾う。
	ffmpegDep := findDependency(t, "FFmpeg")

	tests := []struct {
		name         string
		command      string
		versionFlag  string
		expectFound  bool
		wantsMessage bool
	}{
		{name: "正常系: ffmpegが見つかる", command: ffmpegDep.Command, versionFlag: ffmpegDep.VersionFlag, expectFound: true},
		{
			name: "異常系: 存在しないコマンド", command: "nonexistent_command_xyz123", versionFlag: "--version",
			expectFound: false, wantsMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := doctor.DependencyInfo{Name: "Test", Command: tt.command, VersionFlag: tt.versionFlag, Required: true}

			result := doctor.CheckDependency(info)

			assert.Equal(t, tt.expectFound, result.Found)
			if tt.wantsMessage {
				assert.NotEmpty(t, result.Message)
			}
		})
	}
}

func TestCheckDependency_ExtractsVersion(t *testing.T) {
	t.Parallel()

	// why: goコマンド自体はこのテストバイナリをビルドした環境に存在する保証は
	// あるが、その環境がテスト実行時のPATH上にあるとは限らない（クロス
	// コンパイル済みバイナリの実行等）。ハードな前提にせず、見つからない場合は
	// スキップして環境依存でテストが壊れないようにする。
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not found in PATH")
	}

	info := doctor.DependencyInfo{Name: "Go", Command: "go", VersionFlag: "version", Required: true}

	result := doctor.CheckDependency(info)

	require.True(t, result.Found)
	assert.NotEmpty(t, result.Version)
}

func TestCheckAllDependencies_ReturnsResultForEveryDependency(t *testing.T) {
	t.Parallel()

	results := doctor.CheckAllDependencies()

	require.Len(t, results, len(doctor.Dependencies))

	resultNames := make(map[string]struct{}, len(results))
	for _, r := range results {
		resultNames[r.Name] = struct{}{}
	}

	for _, dep := range doctor.Dependencies {
		_, ok := resultNames[dep.Name]
		assert.True(t, ok, "missing result for %s", dep.Name)
	}
}

func TestExtractVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "正常系: セマンティックバージョン", output: "ffmpeg version 6.0.1", want: "6.0.1"},
		{name: "正常系: マイナーバージョンのみ", output: "version 1.5", want: "1.5"},
		{name: "正常系: メジャーバージョンのみ", output: "version 17", want: "17"},
		{name: "異常系: バージョン番号なし", output: "no version here", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, doctor.ExtractVersion(tt.output))
		})
	}
}
