package doctor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/doctor"
)

func TestDependencies_Count(t *testing.T) {
	t.Parallel()

	assert.Len(t, doctor.Dependencies, 5)
}

func TestDependencies_ContainsRequiredTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		expectedCommand string
		expectedRequire bool
	}{
		{name: "Python", expectedCommand: "python", expectedRequire: true},
		{name: "Java JDK", expectedCommand: "java", expectedRequire: true},
		{name: "Android SDK", expectedCommand: "sdkmanager", expectedRequire: true},
		{name: "Android NDK", expectedCommand: "ndk-build", expectedRequire: true},
		{name: "FFmpeg", expectedCommand: "ffmpeg", expectedRequire: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestDependencies_AllHaveVersionFlag(t *testing.T) {
	t.Parallel()

	for _, dep := range doctor.Dependencies {
		assert.NotEmpty(t, dep.VersionFlag)
	}
}

func TestCheckDependency_ReturnsCheckResult(t *testing.T) {
	t.Parallel()

	info := doctor.DependencyInfo{Name: "Python", Command: "python", VersionFlag: "--version", Required: true}

	result := doctor.CheckDependency(info)

	assert.Equal(t, "Python", result.Name)
}

func TestCheckDependency_FoundStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		command      string
		versionFlag  string
		expectFound  bool
		wantsMessage bool
	}{
		{name: "正常系: pythonが見つかる", command: "python3", versionFlag: "--version", expectFound: true},
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
		{name: "正常系: セマンティックバージョン", output: "Python 3.12.0", want: "3.12.0"},
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
