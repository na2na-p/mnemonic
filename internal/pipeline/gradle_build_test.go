package pipeline

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

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

// TestBuildPipeline_Run_GradleFailureErr はGradleの失敗がResult.Errのエラー連鎖に
// センチネルを残したまま届き、ErrorMessageがその文言と一致することを検証する。
func TestBuildPipeline_Run_GradleFailureErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   builder.BuildResult
		buildErr error
		wantErr  error
	}{
		{
			name:     "異常系: Gradleビルドの失敗はErrGradleBuildFailedをResult.Errに残す",
			buildErr: &builder.GradleBuildError{ExitCode: 1, Output: "* What went wrong:\nboom\n\n* Try:\n"},
			wantErr:  builder.ErrGradleBuildFailed,
		},
		{
			name:    "異常系: APKが見つからない失敗はErrGradleAPKMissingをResult.Errに残す",
			result:  builder.BuildResult{Success: true, OutputLog: "BUILD SUCCESSFUL"},
			wantErr: ErrGradleAPKMissing,
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
			require.ErrorIs(t, result.Err, tt.wantErr)
			assert.Equal(t, result.Err.Error(), result.ErrorMessage)
		})
	}
}

// TestBuildPipeline_Run_GradleFailureThroughBuildPhase は実際のrunPhaseと
// executeBuildを通しても、Gradleビルダーの生成とビルドの失敗がResult.Errの
// エラー連鎖にセンチネルを残したまま届くことを検証する。
func TestBuildPipeline_Run_GradleFailureThroughBuildPhase(t *testing.T) {
	t.Parallel()

	errNewBuilder := errors.New("Gradleビルダーの生成に失敗")
	apkPath := "/project/app/build/outputs/apk/release/app.apk"

	tests := []struct {
		name       string
		factoryErr error
		result     builder.BuildResult
		buildErr   error
		wantErr    error
		wantAPK    string
	}{
		{
			name:       "異常系: Gradleビルダーの生成の失敗はそのエラーをResult.Errに残す",
			factoryErr: errNewBuilder,
			wantErr:    errNewBuilder,
		},
		{
			name:     "異常系: Gradleビルドの失敗はErrGradleBuildFailedをResult.Errに残す",
			buildErr: &builder.GradleBuildError{ExitCode: 1, Output: "* What went wrong:\nboom\n\n* Try:\n"},
			wantErr:  builder.ErrGradleBuildFailed,
		},
		{
			name:    "異常系: APKが見つからない失敗はErrGradleAPKMissingをResult.Errに残す",
			result:  builder.BuildResult{Success: true, OutputLog: "BUILD SUCCESSFUL"},
			wantErr: ErrGradleAPKMissing,
		},
		{
			name:    "正常系: 生成されたAPKのパスを後続のフェーズへ渡す",
			result:  builder.BuildResult{Success: true, APKPath: &apkPath},
			wantAPK: apkPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			gradle := NewMockgradleBuilder(ctrl)
			if tt.factoryErr == nil {
				gradle.EXPECT().Build("release").Return(tt.result, tt.buildErr)
			}

			projectDir := t.TempDir()
			convertDir := t.TempDir()

			p := newValidPipelineForOrchestration(t)
			p.config.GradleTimeoutSeconds = 42
			p.prepareProject = func(a buildArtifacts, _, _ string) (string, error) {
				assert.Equal(t, convertDir, a.convertDir)

				return projectDir, nil
			}
			p.newGradleBuilder = func(dir string, timeout time.Duration) (gradleBuilder, error) {
				assert.Equal(t, projectDir, dir)
				assert.Equal(t, 42*time.Second, timeout)
				if tt.factoryErr != nil {
					return nil, tt.factoryErr
				}

				return gradle, nil
			}

			var signed buildArtifacts
			p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
				switch phase {
				case PhaseBuild:
					return p.runPhase(phase, a)
				case PhaseSign:
					signed = a
				default:
					a.convertDir = convertDir
				}

				return a, nil
			}

			result := p.Run(nil)

			if tt.wantErr == nil {
				require.NoError(t, result.Err)
				assert.True(t, result.Success)
				assert.Equal(t, tt.wantAPK, signed.unsignedAPK)
				assert.Equal(t, projectDir, signed.projectDir)

				return
			}

			assert.False(t, result.Success)
			require.ErrorIs(t, result.Err, tt.wantErr)
			assert.Equal(t, result.Err.Error(), result.ErrorMessage)
		})
	}
}

// TestBuildPipeline_ExecuteBuild_ReturnsPrepareProjectError はプロジェクトの
// 準備に失敗した場合、Gradleビルダーを生成せずにそのエラーを返すことを検証する。
func TestBuildPipeline_ExecuteBuild_ReturnsPrepareProjectError(t *testing.T) {
	t.Parallel()

	errPrepare := errors.New("プロジェクトの準備に失敗")

	p := newTestPipeline(t)
	p.prepareProject = func(buildArtifacts, string, string) (string, error) {
		return "", errPrepare
	}
	p.newGradleBuilder = func(string, time.Duration) (gradleBuilder, error) {
		t.Fatal("プロジェクトの準備に失敗した後でGradleビルダーを生成した")

		return nil, nil
	}

	_, err := p.runPhase(PhaseBuild, buildArtifacts{convertDir: t.TempDir()})

	require.ErrorIs(t, err, errPrepare)
}

func TestNewExecGradleBuilder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		projectDir func(t *testing.T) string
		wantErr    bool
	}{
		{
			name:       "正常系: プロジェクトのgradle.propertiesへ書き込めればビルダーを返す",
			projectDir: func(t *testing.T) string { t.Helper(); return t.TempDir() },
		},
		{
			name: "異常系: gradle.propertiesへ書き込めなければnilのビルダーとエラーを返す",
			projectDir: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "missing")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.projectDir(t)

			got, err := newExecGradleBuilder(dir, time.Second)

			if tt.wantErr {
				require.Error(t, err)
				// assert.Nilは型付きnilポインタを包んだインターフェースもnilと判定するため、
				// nilインターフェースとの一致をassert.Equalで確かめる。
				assert.Equal(t, gradleBuilder(nil), got)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.FileExists(t, filepath.Join(dir, "gradle.properties"))
		})
	}
}

// TestBuildPipeline_ExecuteBuild_KeepsProjectDirOnPrepareFailure はテンプレートの
// 準備に失敗しても、作成済みのプロジェクトディレクトリを成果物に残すことを検証する。
// テンプレートキャッシュの場所はHOMEなどの環境変数から決まるため、t.Setenvで
// 一時ディレクトリへ向ける。t.Setenvはt.Parallel()と併用できない。
func TestBuildPipeline_ExecuteBuild_KeepsProjectDirOnPrepareFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	p := newTestPipeline(t)
	p.config.TemplateOffline = true
	p.newGradleBuilder = func(string, time.Duration) (gradleBuilder, error) {
		t.Fatal("テンプレートの準備に失敗した後でGradleビルダーを生成した")

		return nil, nil
	}
	t.Cleanup(p.cleanupTempDirs)

	got, err := p.executeBuild(buildArtifacts{convertDir: t.TempDir()})

	require.ErrorIs(t, err, ErrTemplateUnavailable)
	require.NotEmpty(t, got.projectDir)
	assert.DirExists(t, got.projectDir)
}
