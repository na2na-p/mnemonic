package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

const progressBarWidth = 40

var phaseEmoji = map[pipeline.Phase]string{
	pipeline.PhaseAnalyze: "🔍",
	pipeline.PhaseExtract: "📦",
	pipeline.PhaseConvert: "🔄",
	pipeline.PhaseBuild:   "🔨",
	pipeline.PhaseSign:    "🔏",
}

var phaseName = map[pipeline.Phase]string{
	pipeline.PhaseAnalyze: "Analyzing game structure",
	pipeline.PhaseExtract: "Extracting assets",
	pipeline.PhaseConvert: "Converting assets",
	pipeline.PhaseBuild:   "Building APK",
	pipeline.PhaseSign:    "Signing APK",
}

// ConsoleProgressDisplay はビルドパイプラインの各フェーズの進捗をコンソールに表示する。
type ConsoleProgressDisplay struct {
	useColor bool
	useEmoji bool
	out      io.Writer

	phase   pipeline.Phase
	total   int
	current int
}

// NewConsoleProgressDisplay はos.Stdoutへ出力するConsoleProgressDisplayを生成する。
func NewConsoleProgressDisplay(useColor, useEmoji bool) *ConsoleProgressDisplay {
	return NewConsoleProgressDisplayWithWriter(useColor, useEmoji, os.Stdout)
}

// NewConsoleProgressDisplayWithWriter は出力先を指定してConsoleProgressDisplayを生成する。
//
// テストでt.Parallel()を安全に使うためio.Writerを注入可能にした
// （logger.NewWithWritersと同じ設計判断）。
func NewConsoleProgressDisplayWithWriter(useColor, useEmoji bool, out io.Writer) *ConsoleProgressDisplay {
	return &ConsoleProgressDisplay{useColor: useColor, useEmoji: useEmoji, out: out}
}

// UseColor はカラー出力設定を返す。
//
// 実際の色付けロジックには使われておらず、フィールドとして保持されるのみ
// である（将来のANSIカラー対応向けの予約フィールド）。
func (d *ConsoleProgressDisplay) UseColor() bool {
	return d.useColor
}

// UseEmoji は絵文字表示設定を返す。
func (d *ConsoleProgressDisplay) UseEmoji() bool {
	return d.useEmoji
}

// Phase は直近にStartされたフェーズを返す。
func (d *ConsoleProgressDisplay) Phase() pipeline.Phase {
	return d.phase
}

// Total は直近にStartされたフェーズの総数を返す。
func (d *ConsoleProgressDisplay) Total() int {
	return d.total
}

// Current は直近のUpdateで渡された進捗値を返す。
func (d *ConsoleProgressDisplay) Current() int {
	return d.current
}

// Start はフェーズ開始を表示する。
func (d *ConsoleProgressDisplay) Start(phase pipeline.Phase, total int) {
	d.phase = phase
	d.total = total
	d.current = 0

	emoji := ""
	if d.useEmoji {
		emoji = phaseEmoji[phase]
	}
	name, ok := phaseName[phase]
	if !ok {
		name = string(phase)
	}
	prefix := ""
	if emoji != "" {
		prefix = emoji + " "
	}
	fmt.Fprintf(d.out, "%s%s...\n", prefix, name) //nolint:errcheck // 進捗表示の書き込み失敗は実用上ハンドリング不要
}

// Update は進捗を更新する。
func (d *ConsoleProgressDisplay) Update(current int, message string) {
	d.current = current
	if d.total <= 0 {
		return
	}

	// 呼び出し側がtotalを超える値や負値を渡しても panic せず表示上は 0-100% に
	// 丸め込む（不正な入力値でクラッシュしないための防御的な実装）。
	clamped := current
	if clamped < 0 {
		clamped = 0
	}
	if clamped > d.total {
		clamped = d.total
	}

	percent := clamped * 100 / d.total
	filled := progressBarWidth * clamped / d.total
	bar := strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)

	msgPart := ""
	if message != "" {
		msgPart = " " + message
	}
	fmt.Fprintf(d.out, "\r   [%s] %d%%%s", bar, percent, msgPart) //nolint:errcheck // 同上
}

// Finish はフェーズ終了を表示する。
func (d *ConsoleProgressDisplay) Finish(success bool, message string) {
	fullBar := strings.Repeat("█", progressBarWidth)

	if success {
		mark := "done"
		if d.useEmoji {
			mark = "✓"
		}
		fmt.Fprintf(d.out, "\r   [%s] 100%% %s\n", fullBar, mark) //nolint:errcheck // 同上

		return
	}

	mark := "failed"
	if d.useEmoji {
		mark = "✗"
	}
	msgPart := ""
	if message != "" {
		msgPart = ": " + message
	}
	fmt.Fprintf(d.out, "\r   [%s] %s%s\n", fullBar, mark, msgPart) //nolint:errcheck // 同上
}
