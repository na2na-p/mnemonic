package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// BuildPipeline はビルドパイプラインオーケストレーター。
//
// Parser -> Converter -> Builder -> Signer の各コンポーネントを連携させ、
// Windows EXE/XP3からAndroid APKを生成するパイプラインを管理する。
//
// 使用例:
//
//	config := pipeline.NewConfig("game.exe", "game.apk")
//	p := pipeline.NewBuildPipeline(config)
//	if errs := p.Validate(); len(errs) == 0 {
//		result := p.Run(nil)
//	}
type BuildPipeline struct {
	config Config

	tempDirs []string

	// executePhase は個別フェーズの実行を担う関数。既定値はb.runPhase
	// （実際のフェーズ処理）。Run()が担うオーケストレーション（進捗コールバック
	// 呼び出し・phasesCompleted集計・統計収集・一時ディレクトリのクリーンアップ）
	// を個々のフェーズ実装の詳細から切り離してテストするため、構造体フィールドと
	// して差し替え可能にする（internal/converter.ConversionManager.SleepFuncと
	// 同じ設計方針）。
	executePhase func(Phase, buildArtifacts) (buildArtifacts, error)

	// keystorePath/keystoreValid/keystoreGenerateはデバッグ用キーストアの
	// 永続化（internal/pipeline/keystore.go）をkeytool実行から切り離して
	// テストするための差し替え可能フィールド（executePhaseと同じ設計方針）。
	keystorePath     func() (string, error)
	keystoreValid    func(path string) bool
	keystoreGenerate func(path string) error
}

// buildArtifacts はフェーズ間で引き渡すビルド成果物。値として次のフェーズへ渡す。
//
// why not: パイプラインの可変フィールドで受け渡すと、フェーズ間のデータの流れが
// 暗黙の状態変更に隠れ、Runの呼び出しをまたいで状態が残り、テストはフィールドを
// 直接書き換える必要がある。値で渡すことで流れが引数と戻り値に現れ、各フェーズを
// 引数だけでテストできる。
type buildArtifacts struct {
	extractDir    string
	convertDir    string
	projectDir    string
	unsignedAPK   string
	gameStructure *parser.GameStructure
}

// NewBuildPipeline はconfigをもとにBuildPipelineを初期化する。
func NewBuildPipeline(config Config) *BuildPipeline {
	b := &BuildPipeline{config: config}
	b.executePhase = b.runPhase
	b.keystorePath = resolveDebugKeystorePath
	b.keystoreValid = validateDebugKeystoreFile
	b.keystoreGenerate = generateDebugKeystoreFile

	return b
}

// Config は現在のパイプライン設定を返す。
func (b *BuildPipeline) Config() Config {
	return b.config
}

// Validate は設定を検証し、エラーメッセージのリストを返す。
//
// 入力ファイルの存在確認、出力パスの妥当性、オプション値の整合性などを検証する。
func (b *BuildPipeline) Validate() []string {
	var errs []string

	if _, err := os.Stat(b.config.InputPath); err != nil {
		errs = append(errs, fmt.Sprintf("入力ファイルが見つかりません: %s", b.config.InputPath))

		return errs
	}

	suffix := strings.ToLower(filepath.Ext(b.config.InputPath))
	if suffix != ".exe" && suffix != ".xp3" {
		errs = append(errs, fmt.Sprintf("サポートされていないファイル形式です: %s", suffix))
	}

	if b.config.KeystorePath != "" {
		if _, err := os.Stat(b.config.KeystorePath); err != nil {
			errs = append(errs, fmt.Sprintf("キーストアファイルが見つかりません: %s", b.config.KeystorePath))
		}
	}

	return errs
}

// Run はパイプラインを実行する。
//
// 各フェーズ（解析、抽出、変換、ビルド、署名）を順次実行し、
// 最終的にAPKファイルを生成する。progressCallbackがnilでない場合、
// 各フェーズの開始・完了時に進捗を通知する。
func (b *BuildPipeline) Run(progressCallback ProgressCallback) Result {
	start := time.Now()
	defer b.cleanupTempDirs()

	if errs := b.Validate(); len(errs) > 0 {
		return Result{Success: false, OutputPath: nil, ErrorMessage: errs[0]}
	}

	var phasesCompleted []Phase

	statistics := map[string]any{}

	var artifacts buildArtifacts

	for _, phase := range AllPhases() {
		phaseStart := time.Now()

		if progressCallback != nil {
			progressCallback(Progress{
				Phase: phase, Current: 0, Total: 1,
				Message: fmt.Sprintf("%sフェーズを開始...", phase),
			})
		}

		next, err := b.executePhase(phase, artifacts)
		if err != nil {
			return Result{
				Success:         false,
				OutputPath:      nil,
				ErrorMessage:    err.Error(),
				PhasesCompleted: phasesCompleted,
				Statistics:      statistics,
			}
		}
		artifacts = next

		phasesCompleted = append(phasesCompleted, phase)
		statistics[string(phase)+"_time_seconds"] = round2(time.Since(phaseStart).Seconds())

		if progressCallback != nil {
			progressCallback(Progress{
				Phase: phase, Current: 1, Total: 1,
				Message: fmt.Sprintf("%sフェーズが完了", phase),
			})
		}
	}

	statistics["total_time_seconds"] = round2(time.Since(start).Seconds())
	outputPath := b.config.OutputPath

	return Result{
		Success:         true,
		OutputPath:      &outputPath,
		PhasesCompleted: phasesCompleted,
		Statistics:      statistics,
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// cleanupTempDirs は生成した一時ディレクトリをすべて削除する。
func (b *BuildPipeline) cleanupTempDirs() {
	for _, dir := range b.tempDirs {
		_ = os.RemoveAll(dir)
	}
	b.tempDirs = nil
}

// newTempDir はprefixで一時ディレクトリを作成し、Run終了時のcleanupTempDirsで
// 削除されるよう登録する。
func (b *BuildPipeline) newTempDir(prefix string) (string, error) {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", fmt.Errorf("一時ディレクトリの作成に失敗しました: %w", err)
	}
	b.tempDirs = append(b.tempDirs, dir)

	return dir, nil
}

// runPhase は個別フェーズを実行する（executePhaseの既定実装）。
func (b *BuildPipeline) runPhase(phase Phase, a buildArtifacts) (buildArtifacts, error) {
	switch phase {
	case PhaseAnalyze:
		return b.executeAnalyze(a)
	case PhaseExtract:
		return b.executeExtract(a)
	case PhaseConvert:
		return b.executeConvert(a)
	case PhaseBuild:
		return b.executeBuild(a)
	case PhaseSign:
		return b.executeSign(a)
	default:
		return a, fmt.Errorf("未知のフェーズです: %s", phase)
	}
}
