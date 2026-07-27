// Package pipeline はビルドパイプラインの段階を表す共有型と、
// パイプライン本体（BuildPipeline: 各フェーズの実行制御）を提供する。
//
// PR2でlogger（進捗表示）が参照するPhase型のみを切り出し、PR7
// （本ファイル）でパイプライン本体を移植した。
package pipeline

// Phase はビルドパイプラインの各段階を表す。
//
// パイプラインは以下の順序で実行される:
//  1. PhaseAnalyze: ゲーム構造解析
//  2. PhaseExtract: アセット抽出
//  3. PhaseConvert: アセット変換
//  4. PhaseBuild: APKビルド
//  5. PhaseSign: APK署名
type Phase string

// Phaseの各段階。
const (
	PhaseAnalyze Phase = "analyze"
	PhaseExtract Phase = "extract"
	PhaseConvert Phase = "convert"
	PhaseBuild   Phase = "build"
	PhaseSign    Phase = "sign"
)

// AllPhases はビルドパイプラインの全フェーズを実行順で返す。
func AllPhases() []Phase {
	return []Phase{PhaseAnalyze, PhaseExtract, PhaseConvert, PhaseBuild, PhaseSign}
}
