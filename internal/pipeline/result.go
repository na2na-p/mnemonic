package pipeline

// Progress はパイプラインの実行進捗を表す値。
//
// 進捗コールバックを通じて、各フェーズの進捗状況を通知するために使用される。
type Progress struct {
	Phase   Phase
	Current int
	Total   int
	Message string
}

// ProgressCallback はパイプライン実行中の進捗通知を受け取るコールバック。
type ProgressCallback func(progress Progress)

// Result はパイプライン実行結果を表す値。
//
// 成功/失敗の状態、出力ファイルパス、統計情報などを含む。
// OutputPathはPythonの `Path | None` に相当し、失敗時のNoneを表現するため
// ポインタとする。Statisticsはフェーズごとの所要時間（float64、秒）と
// 処理件数（int）等、値の型が混在するためPython版のdict[str, Any]と同様
// map[string]anyとする。
type Result struct {
	Success         bool
	OutputPath      *string
	ErrorMessage    string
	PhasesCompleted []Phase
	Statistics      map[string]any
}
