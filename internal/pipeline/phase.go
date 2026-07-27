// Package pipeline はビルドパイプラインの段階を表す共有型を提供する。
//
// PR2時点ではlogger（進捗表示）が参照するPhase型のみを切り出す。
// パイプライン本体（各フェーズの実行制御）はPR7で移植する。
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
