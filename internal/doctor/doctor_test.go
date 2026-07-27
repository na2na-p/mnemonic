package doctor_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// 必須(Required=true)ではなく任意の依存として登録されていることを検証する。
//
// why: FluidSynthはMidiConverter（T-211）が呼び出す外部ツールだが、feat版
// doctor.pyのDependencyInfo(name="FluidSynth", required=False)に合わせ、
// MIDIアセットを含まないゲームのビルドをFluidSynth未インストール環境でも
// ブロックしないようにするための意図的な設計（必須依存にすると全ゲームで
// FluidSynthのインストールを強制することになり、Python版の意図と乖離する）。
func TestDependencies_ContainsOptionalTools(t *testing.T) {
	t.Parallel()

	dep := findDependency(t, "FluidSynth")

	assert.Equal(t, "fluidsynth", dep.Command)
	assert.Equal(t, "--version", dep.VersionFlag)
	assert.False(t, dep.Required)
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
