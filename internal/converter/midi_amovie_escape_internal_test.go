package converter

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireFunctionalFFprobe はffprobeが実際に動作するかを確認する。
// internal/pipeline/keystore_internal_test.goのrequireFunctionalKeytoolと
// 同じゲート方式（コマンドが存在してもPATH上でスタブのみの環境があるため、
// 単なるexec.LookPathでは判定できない。呼び出せない場合はテストをスキップする）。
func requireFunctionalFFprobe(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, "ffprobe", "-version").Run(); err != nil {
		t.Skipf("ffprobeが動作しない環境のためスキップします: %v", err)
	}
}

// writeTestWAV は440Hzの正弦波(振幅0.5、閾値-50dBを十分に上回る)audibleSeconds秒
// の後にデジタル無音(全サンプル0)をsilentSeconds秒続けたモノラル16bit PCM WAVを
// pathへ書き込む。detectTrailingSilenceStartが末尾無音を検出できることを、
// 外部ツール(ffmpeg)なしで確認するための最小自前WAV生成。
func writeTestWAV(t *testing.T, path string, audibleSeconds, silentSeconds float64) {
	t.Helper()

	const sampleRate = 8000
	audibleSamples := int(audibleSeconds * sampleRate)
	silentSamples := int(silentSeconds * sampleRate)
	totalSamples := audibleSamples + silentSamples

	var pcm bytes.Buffer
	for i := 0; i < audibleSamples; i++ {
		v := int16(0.5 * 32767 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		require.NoError(t, binary.Write(&pcm, binary.LittleEndian, v))
	}
	for i := 0; i < silentSamples; i++ {
		require.NoError(t, binary.Write(&pcm, binary.LittleEndian, int16(0)))
	}

	dataSize := uint32(totalSamples * 2) //nolint:gosec // テスト用の小さいサンプル数のため桁あふれしない
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint32(36)+dataSize))
	wav.WriteString("WAVE")
	wav.WriteString("fmt ")
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint32(16))) // fmtチャンクサイズ
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint16(1)))  // PCM
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint16(1)))  // モノラル
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint32(sampleRate)))
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint32(sampleRate*2))) // バイトレート
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint16(2)))            // ブロックアライン
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, uint16(16)))           // ビット深度
	wav.WriteString("data")
	require.NoError(t, binary.Write(&wav, binary.LittleEndian, dataSize))
	wav.Write(pcm.Bytes())

	require.NoError(t, os.WriteFile(path, wav.Bytes(), 0o600))
}

// TestMidiConverter_detectTrailingSilenceStart_SpecialCharacterPaths は
// 実ffprobe(モックなし)を使い、amovieフィルタへ渡すパスにシングルクォート・
// コロン・バックスラッシュを含む場合でも末尾無音を検出できることを検証する。
//
// why: レビュー指摘により判明した不具合の再発防止。escapeLavfiPathForAmovieの
// 単体テストは文字列変換の形だけを見ており、実際にamovieフィルタが
// そのエスケープ結果を正しく解釈できるかまでは検証していなかった
// （旧実装のシェル風エスケープ(閉じクォート+バックスラッシュ+クォート+開きクォート)
// はamovieでは機能せず、パスからクォートが消えてファイルを開けなかった）。
func TestMidiConverter_detectTrailingSilenceStart_SpecialCharacterPaths(t *testing.T) {
	t.Parallel()
	requireFunctionalFFprobe(t)

	cases := map[string]struct {
		filename string
	}{
		"正常系: シングルクォートを含むパス": {"it's.wav"},
		"正常系: コロンを含むパス":      {"weird:path.wav"},
		"正常系: バックスラッシュを含むパス": {"back\\slash.wav"},
		"正常系: 特殊文字を全て含むパス":   {"all_special':back\\slash.wav"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			wavPath := filepath.Join(dir, tc.filename)
			writeTestWAV(t, wavPath, 0.5, 1.0)

			c := NewMidiConverter(dir, 0, "", 0, 10*time.Second, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			start, found := c.detectTrailingSilenceStart(ctx, wavPath)

			require.True(t, found, "特殊文字を含むパスでも末尾無音を検出できるべき")
			assert.InDelta(t, 0.5, start, 0.1, "検出された無音開始位置は可聴区間の直後(約0.5秒)であるべき")
		})
	}
}
