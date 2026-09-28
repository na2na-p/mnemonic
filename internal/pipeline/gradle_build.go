package pipeline

import (
	"errors"
	"time"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// gradleBuilder はBUILDフェーズが使うGradleビルドの振る舞い。
//
// why not: *builder.GradleBuilderを直接受け取らない。実物はgradlewの実行を
// 伴うため、失敗時に全出力をどこへ記録するかをGradle無しでテストできない。
type gradleBuilder interface {
	Build(buildType string) (builder.BuildResult, error)
}

// newExecGradleBuilder はprojectDirでgradlewを実行するgradleBuilderを返す
// （BuildPipeline.newGradleBuilderの既定実装）。
//
// why not: builder.NewGradleBuilderの戻り値をそのまま返さない。失敗時のnilの
// *builder.GradleBuilderをgradleBuilderへ入れると、nilでないインターフェースになる。
func newExecGradleBuilder(projectDir string, timeout time.Duration) (gradleBuilder, error) {
	gradle, err := builder.NewGradleBuilder(projectDir, timeout, nil)
	if err != nil {
		return nil, err
	}

	return gradle, nil
}

// runGradleBuild はreleaseビルドを実行し、生成されたAPKのパスを返す。
// ビルドが失敗した場合とAPKが見つからない場合は、Gradleの全出力をloggerの
// Debugへ記録する。
//
// why not: 全出力をエラー文へ入れない。エラー文は端末へそのまま表示されるため、
// 原因以外の数百行で端末が埋まる。Debugは端末へは詳細度が最大の時だけ出し、
// ログファイルへは常に書く。
func runGradleBuild(gradle gradleBuilder, logger Logger) (string, error) {
	result, err := gradle.Build("release")
	if err != nil {
		if buildErr, ok := errors.AsType[*builder.GradleBuildError](err); ok {
			logGradleOutput(logger, buildErr.Output)
		}

		return "", err
	}
	if !result.Success || result.APKPath == nil {
		logGradleOutput(logger, result.OutputLog)

		return "", ErrGradleAPKMissing
	}

	return *result.APKPath, nil
}

func logGradleOutput(logger Logger, output string) {
	logger.Debug("Gradleの出力:\n" + output)
}
