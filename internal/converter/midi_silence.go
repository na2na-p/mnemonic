package converter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// trailingSilenceThresholdDB / trailingSilenceMinDuration / trailingSilenceKeepMargin
// はMIDI変換結果の末尾無音トリムに使う閾値・最小継続時間・トリム後に残す
// マージン(秒)。
//
// why not: FluidSynthは既定でリバーブ/コーラスが有効なため、MIDIファイル自体が
// 持つ末尾休符（実測で0〜3.3秒）に加え、最後のノートのリリースと残響減衰で
// さらに数秒分の可聴〜準無音区間をレンダリングする（実測: サンプルBGM 9曲で
// トレーリング無音3.2〜7.8秒）。-50dB/0.3秒という組み合わせは、この実測データで
// 「本当に鳴り終わった後の無音」と「曲中の短い休符」（実測データのsinone.midに
// 存在する0.49〜0.55秒の内部休符）を唯一正しく判別できた値。閾値を緩める
// （絶対値を下げる）と曲中の自然なディミヌエンドまで削れ、厳しくする
// （絶対値を上げる）とFluidSynthの残響テールを十分にトリムできない。
// stop_silenceに相当するkeepMarginは、無音区間を完全に0秒までカットすると
// ループ時に不自然なクリック音が乗るため、若干の無音を意図的に残す。
const (
	trailingSilenceThresholdDB = -50
	trailingSilenceMinDuration = 0.3
	trailingSilenceKeepMargin  = 0.3
)

// silenceProbeOutput はffprobe `-f lavfi -i "amovie=...,silencedetect=..."`
// `-show_entries frame_tags=lavfi.silence_start,lavfi.silence_end -of json`
// の出力を表す。
type silenceProbeOutput struct {
	Frames []silenceProbeFrame `json:"frames"`
}

type silenceProbeFrame struct {
	Tags map[string]string `json:"tags"`
}

type trailingSilenceDetector struct {
	runner  CommandRunner
	timeout time.Duration
}

func newTrailingSilenceDetector(runner CommandRunner, timeout time.Duration) trailingSilenceDetector {
	return trailingSilenceDetector{runner: runner, timeout: timeout}
}

// trimPoint はwavPathの末尾無音トリム位置(秒)を検出する。
// 末尾が無音で終わっていない場合や検出に失敗した場合は(0, false)を返し、
// 呼び出し側はトリムなしでffmpegを実行する（無音トリムはUX向上のための
// ベストエフォート処理であり、検出失敗を変換全体の失敗にはしない）。
func (d trailingSilenceDetector) trimPoint(wavPath string) (float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	start, found := d.silenceStart(ctx, wavPath)
	if !found {
		return 0, false
	}

	return start + trailingSilenceKeepMargin, true
}

// silenceStart はwavPathの末尾に付与された無音の開始秒を検出する。
//
// why not: ffmpegのsilencedetectフィルタはイベントをログ(stderr)へ出力するが、
// CommandRunner.Runは成功時に標準出力のみを返す（video.goのffprobe JSON
// パース処理へstderrの警告が混入しないようにするための既存契約であり、
// このMIDI変換専用の目的だけのために変更しない）。そこで
// ffprobe + lavfi(amovie+silencedetect) を使い、イベントを標準出力上の
// JSON(frame_tags)として取得する。「末尾が無音で終わっているか」の判定
// そのものはIOを伴わないtrailingSilenceStartFromFramesへ委譲し、この関数は
// ffprobeの実行とJSONパースのみを担う。
func (d trailingSilenceDetector) silenceStart(ctx context.Context, wavPath string) (float64, bool) {
	filterInput := fmt.Sprintf(
		"amovie=%s,silencedetect=noise=%ddB:d=%s",
		escapeLavfiPathForAmovie(wavPath),
		trailingSilenceThresholdDB,
		strconv.FormatFloat(trailingSilenceMinDuration, 'f', -1, 64),
	)

	out, err := d.runner.Run(ctx, "ffprobe",
		"-v", "error",
		"-f", "lavfi",
		"-i", filterInput,
		"-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end",
		"-of", "json",
	)
	if err != nil {
		return 0, false
	}

	var probe silenceProbeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, false
	}

	return trailingSilenceStartFromFrames(probe.Frames)
}

// trailingSilenceStartFromFrames はffprobeから得たframe_tags列(silenceProbeOutput.
// Frames)から、末尾の無音開始秒を判定する純粋関数。IOを行わないため
// ffprobeを起動せずに判定ロジック単体をテストできる。
//
// 末尾の無音区間だけを安全に検出するため、直近のタグが"silence_start"のまま
// 終わっている（＝その後"silence_end"で終了していない）場合だけを「末尾まで
// 無音が続いている」とみなす。曲中に短い休符が複数回あっても、最後に観測される
// タグが"silence_end"であれば末尾は無音でないと判定してトリムしない。
func trailingSilenceStartFromFrames(frames []silenceProbeFrame) (float64, bool) {
	var lastStart float64
	trailing := false

	for _, frame := range frames {
		if v, ok := frame.Tags["lavfi.silence_start"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				lastStart = f
				trailing = true
			}

			continue
		}
		if _, ok := frame.Tags["lavfi.silence_end"]; ok {
			trailing = false
		}
	}

	if !trailing {
		return 0, false
	}

	return lastStart, true
}

// escapeLavfiPathForAmovie はamovieフィルタの引数として渡すパスをエスケープする。
//
// why not: amovie=<path>はavfilterのフィルタグラフ記述内に埋め込まれるため、
// シェルのクォート規則(閉じクォート+バックスラッシュ+クォート+開きクォート方式)は
// 通用しない。実機のffprobe(exec直接呼び出し、シェル非経由)で検証した結果、
// amovieのフィルタグラフ記述は「フィルタオプション値自体のエスケープ」と
// 「フィルタグラフ記述全体のエスケープ」の2段階が重なったエスケープを要求し、
// バックスラッシュ1文字は4文字(`\\\\`)、シングルクォート1文字はバックスラッシュ
// 3つ+クォート(`\\\'`)、コロン1文字はバックスラッシュ2つ+コロン(`\\:`)に
// それぞれ変換しないとamovieがパスを正しく解釈できない（文字ごとに必要な
// バックスラッシュ本数が異なる。パスにこれらの文字が含まれるのは主にWindows版の
// 一時ディレクトリ(例: C:\Users\...\AppData\Local\Temp)のケース）。バックスラッシュ
// の変換を最初に行うのは、後続のクォート・コロン置換で挿入するバックスラッシュ
// 自体が再度多重エスケープされるのを防ぐため。
func escapeLavfiPathForAmovie(path string) string {
	s := path
	s = strings.ReplaceAll(s, `\`, `\\\\`)
	s = strings.ReplaceAll(s, `'`, `\\\'`)
	s = strings.ReplaceAll(s, `:`, `\\:`)

	return s
}
