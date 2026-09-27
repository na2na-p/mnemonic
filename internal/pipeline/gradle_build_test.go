package pipeline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func TestRunGradleBuild(t *testing.T) {
	t.Parallel()

	apkPath := "/project/app/build/outputs/apk/release/app.apk"
	failureOutput := "Welcome to Gradle 7.5!\n* What went wrong:\nExecution failed for task ':app:mergeReleaseResources'.\n\n* Try:\n> Run with --info\n"

	tests := []struct {
		name          string
		result        builder.BuildResult
		buildErr      error
		wantAPK       string
		wantErr       error
		wantErrAbsent string
		wantDebug     []string
	}{
		{
			name:      "正常系: APKのパスを返し、Gradleの出力は記録しない",
			result:    builder.BuildResult{Success: true, APKPath: &apkPath, OutputLog: "BUILD SUCCESSFUL"},
			wantAPK:   apkPath,
			wantDebug: nil,
		},
		{
			name:          "異常系: ビルド失敗時はGradleの全出力をDEBUGへ記録し、エラーには原因だけを残す",
			buildErr:      &builder.GradleBuildError{ExitCode: 1, Output: failureOutput},
			wantErr:       builder.ErrGradleBuildFailed,
			wantErrAbsent: "Welcome to Gradle",
			wantDebug:     []string{"Gradleの出力:\n" + failureOutput},
		},
		{
			name:      "異常系: Gradleを実行できなかった場合は記録する出力が無い",
			buildErr:  builder.ErrGradleTimeout,
			wantErr:   builder.ErrGradleTimeout,
			wantDebug: nil,
		},
		{
			name:          "異常系: 成功終了でもAPKが無ければGradleの全出力をDEBUGへ記録し、エラーには含めない",
			result:        builder.BuildResult{Success: true, OutputLog: "BUILD SUCCESSFUL in 1m"},
			wantErr:       ErrGradleAPKMissing,
			wantErrAbsent: "BUILD SUCCESSFUL",
			wantDebug:     []string{"Gradleの出力:\nBUILD SUCCESSFUL in 1m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			gradle := NewMockgradleBuilder(ctrl)
			gradle.EXPECT().Build("release").Return(tt.result, tt.buildErr)
			logger := &recordingLogger{}

			got, err := runGradleBuild(gradle, logger)

			assert.Equal(t, tt.wantDebug, logger.messages("DEBUG"))
			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.wantAPK, got)

				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErrAbsent != "" {
				assert.NotContains(t, err.Error(), tt.wantErrAbsent)
			}
			assert.Empty(t, got)
		})
	}
}

// TestBuildPipeline_Run_GradleFailureMessage はGradleの失敗がResult.ErrorMessageの
// 先頭にセンチネルの文言を残したまま届くことを検証する。CLIはこの先頭の文言で
// Gradleの失敗を見分け、出力全文の在りかを案内する。
func TestBuildPipeline_Run_GradleFailureMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     builder.BuildResult
		buildErr   error
		wantPrefix string
	}{
		{
			name:       "異常系: Gradleビルドの失敗はErrGradleBuildFailedの文言で始まる",
			buildErr:   &builder.GradleBuildError{ExitCode: 1, Output: "* What went wrong:\nboom\n\n* Try:\n"},
			wantPrefix: builder.ErrGradleBuildFailed.Error(),
		},
		{
			name:       "異常系: APKが見つからない失敗はErrGradleAPKMissingの文言で始まる",
			result:     builder.BuildResult{Success: true, OutputLog: "BUILD SUCCESSFUL"},
			wantPrefix: ErrGradleAPKMissing.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			gradle := NewMockgradleBuilder(ctrl)
			gradle.EXPECT().Build("release").Return(tt.result, tt.buildErr)

			p := newValidPipelineForOrchestration(t)
			p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
				if phase != PhaseBuild {
					return a, nil
				}
				_, err := runGradleBuild(gradle, p.log())

				return a, err
			}

			result := p.Run(nil)

			assert.False(t, result.Success)
			assert.True(t, strings.HasPrefix(result.ErrorMessage, tt.wantPrefix), "ErrorMessage = %q", result.ErrorMessage)
		})
	}
}
