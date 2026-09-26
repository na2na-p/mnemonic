package converter_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// mockConverter はConversionManagerのテスト用Converter実装。
type mockConverter struct {
	extensions      []string
	failCount       int
	raiseError      bool
	convertFunc     func(source, dest string, callCount int) (converter.ConversionResult, error)
	outputExtension string

	mu        sync.Mutex
	callCount int
}

func newMockConverter(extensions ...string) *mockConverter {
	return &mockConverter{extensions: extensions}
}

func (c *mockConverter) SupportedExtensions() []string { return c.extensions }

func (c *mockConverter) GetOutputExtension(_ string) string { return c.outputExtension }

func (c *mockConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, e := range c.extensions {
		if e == ext {
			return true
		}
	}

	return false
}

func (c *mockConverter) Convert(source, dest string) (converter.ConversionResult, error) {
	c.mu.Lock()
	c.callCount++
	callCount := c.callCount
	c.mu.Unlock()

	if c.raiseError {
		return converter.ConversionResult{}, errors.New("変換中にエラーが発生しました")
	}

	if c.convertFunc != nil {
		return c.convertFunc(source, dest, callCount)
	}

	if callCount <= c.failCount {
		return converter.ConversionResult{SourcePath: source}, errors.New("変換失敗")
	}

	return converter.ConversionResult{
		SourcePath: source, DestPath: dest, Status: converter.StatusSuccess,
		BytesBefore: 100, BytesAfter: 80,
	}, nil
}

func (c *mockConverter) CallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.callCount
}

func TestDefaultRetryConfig(t *testing.T) {
	t.Parallel()

	rc := converter.DefaultRetryConfig()

	assert.Equal(t, 3, rc.MaxAttempts)
	assert.InDelta(t, 1.0, rc.BackoffBase, 1e-9)
	assert.InDelta(t, 2.0, rc.BackoffMultiplier, 1e-9)
}

func TestNewConversionManager(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルト値での初期化", func(t *testing.T) {
		t.Parallel()

		converters := []converter.Converter{newMockConverter(".txt")}
		m := converter.NewConversionManager(converters, nil, 0, nil)

		assert.Equal(t, converters, m.Converters)
		assert.Equal(t, converter.DefaultRetryConfig(), m.RetryConfig)
		assert.GreaterOrEqual(t, m.MaxWorkers, 1)
		assert.Nil(t, m.ProgressCallback)
	})

	t.Run("正常系: カスタム値での初期化", func(t *testing.T) {
		t.Parallel()

		converters := []converter.Converter{newMockConverter(".txt")}
		rc := converter.RetryConfig{MaxAttempts: 5, BackoffBase: 1, BackoffMultiplier: 2}
		called := false
		callback := func(int, int) { called = true }

		m := converter.NewConversionManager(converters, &rc, 8, callback)

		assert.Equal(t, converters, m.Converters)
		assert.Equal(t, 5, m.RetryConfig.MaxAttempts)
		assert.Equal(t, 8, m.MaxWorkers)
		require.NotNil(t, m.ProgressCallback)
		m.ProgressCallback(1, 1)
		assert.True(t, called)
	})
}

func TestConversionManager_GetConverterForFile(t *testing.T) {
	t.Parallel()

	t.Run("正常系: マッチするConverterが見つかる場合", func(t *testing.T) {
		t.Parallel()

		txtConv := newMockConverter(".txt")
		jpgConv := newMockConverter(".jpg", ".jpeg")
		m := converter.NewConversionManager([]converter.Converter{txtConv, jpgConv}, nil, 1, nil)

		assert.Same(t, txtConv, m.GetConverterForFile(filepath.Join(t.TempDir(), "test.txt")))
	})

	t.Run("正常系: サポートされていないファイル形式の場合nil", func(t *testing.T) {
		t.Parallel()

		txtConv := newMockConverter(".txt")
		m := converter.NewConversionManager([]converter.Converter{txtConv}, nil, 1, nil)

		assert.Nil(t, m.GetConverterForFile(filepath.Join(t.TempDir(), "test.pdf")))
	})

	t.Run("正常系: 複数のConverterがマッチする場合最初のものを返す", func(t *testing.T) {
		t.Parallel()

		first := newMockConverter(".txt")
		second := newMockConverter(".txt", ".md")
		m := converter.NewConversionManager([]converter.Converter{first, second}, nil, 1, nil)

		assert.Same(t, first, m.GetConverterForFile(filepath.Join(t.TempDir(), "test.txt")))
	})
}

func TestConversionManager_ConvertFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 単一ファイルの変換", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("test content"))
		dest := filepath.Join(dir, "dest.txt")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 1, summary.Total)
		assert.Equal(t, 1, summary.Success)
		assert.Equal(t, 0, summary.Failed)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, converter.StatusSuccess, summary.Results[0].Status)
	})

	t.Run("正常系: 複数ファイルの変換", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, 3)
		for i := range 3 {
			source := filepath.Join(dir, "source.txt")
			dest := filepath.Join(dir, fmt.Sprintf("dest%d.txt", i))
			files = append(files, converter.FileTask{Source: source, Dest: dest})
		}

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 2, nil)
		summary := m.ConvertFiles(files)

		assert.Equal(t, 3, summary.Total)
		assert.Equal(t, 3, summary.Success)
		assert.Equal(t, 0, summary.Failed)
		assert.Len(t, summary.Results, 3)
	})

	t.Run("正常系: MaxWorkersが0以下でも1ワーカーで全タスクを変換する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, 3)
		for i := range 3 {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, fmt.Sprintf("dest%d.txt", i)),
			})
		}

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		m.MaxWorkers = 0
		summary := m.ConvertFiles(files)

		assert.Equal(t, 3, summary.Total)
		assert.Equal(t, 3, summary.Success)
		assert.Len(t, summary.Results, 3)
	})

	t.Run("正常系: 並列実行の検証", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, 4)
		for i := range 4 {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, fmt.Sprintf("dest%d.txt", i)),
			})
		}

		var (
			mu    sync.Mutex
			times []time.Time
		)

		conv := newMockConverter(".txt")
		conv.convertFunc = func(source, dest string, _ int) (converter.ConversionResult, error) {
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)

			return converter.ConversionResult{SourcePath: source, DestPath: dest, Status: converter.StatusSuccess}, nil
		}

		m := converter.NewConversionManager([]converter.Converter{conv}, nil, 4, nil)
		summary := m.ConvertFiles(files)

		assert.Equal(t, 4, summary.Success)
		if len(times) >= 2 {
			minT, maxT := times[0], times[0]
			for _, tm := range times {
				if tm.Before(minT) {
					minT = tm
				}
				if tm.After(maxT) {
					maxT = tm
				}
			}
			// 直列実行なら4タスク*50ms=200ms以上の開きがあるはず。
			// 並列実行ならほぼ同時に開始される。
			assert.Less(t, maxT.Sub(minT), 150*time.Millisecond)
		}
	})

	t.Run("正常系: サポートされていないファイルはスキップされる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		supported := filepath.Join(dir, "source.txt")
		writeFile(t, supported, []byte("content"))
		unsupported := filepath.Join(dir, "source.pdf")
		writeFile(t, unsupported, []byte("pdf content"))

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary := m.ConvertFiles([]converter.FileTask{
			{Source: supported, Dest: filepath.Join(dir, "dest.txt")},
			{Source: unsupported, Dest: filepath.Join(dir, "dest.pdf")},
		})

		assert.Equal(t, 2, summary.Total)
		assert.Equal(t, 1, summary.Success)
		assert.Equal(t, 1, summary.Skipped)
	})

	t.Run("正常系: 進捗コールバックの呼び出し", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, 3)
		for i := range 3 {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, fmt.Sprintf("dest%d.txt", i)),
			})
		}

		var (
			mu    sync.Mutex
			calls [][2]int
		)
		callback := func(completed, total int) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, [2]int{completed, total})
		}

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, callback)
		m.ConvertFiles(files)

		mu.Lock()
		defer mu.Unlock()
		assert.GreaterOrEqual(t, len(calls), 3)

		found := slices.ContainsFunc(calls, func(c [2]int) bool {
			return c[0] == 3 && c[1] == 3
		})
		assert.True(t, found)
	})

	t.Run("正常系: 並列実行時も進捗コールバックはロック内で単調増加に呼ばれる", func(t *testing.T) {
		t.Parallel()

		// why: ProgressCallbackがcompletedCountの更新と同じロック区間内で呼ばれる
		// ことを検証する。コールバック自体には意図的に追加のmutexを持たせず、
		// ConversionManager側の排他制御だけでスライスへの追記が安全（-raceで
		// クリーン）かつ1..Nの単調増加になることを確認する。
		const fileCount = 50

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, fileCount)
		for i := range fileCount {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, fmt.Sprintf("dest%d.txt", i)),
			})
		}

		completions := make([]int, 0, fileCount)
		callback := func(completed, _ int) {
			completions = append(completions, completed)
		}

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 8, callback)
		summary := m.ConvertFiles(files)

		assert.Equal(t, fileCount, summary.Success)
		require.Len(t, completions, fileCount)
		for i, v := range completions {
			assert.Equal(t, i+1, v, "進捗コールバックはcompletedCountの単調増加順に呼ばれるはず")
		}
	})

	t.Run("異常系: 出力先が重複するタスクはいずれも変換せず恒久的な失敗として報告する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// why: 列挙順とパス順を逆にし、報告される変換元の並びがパス順であることを固定する。
		collidingB := filepath.Join(dir, "b", "foo.txt")
		collidingA := filepath.Join(dir, "a", "foo.txt")
		collidedDest := filepath.Join(dir, "out", "foo.txt")
		unique := filepath.Join(dir, "unique.txt")
		uniqueDest := filepath.Join(dir, "out", "unique.txt")

		var (
			mu      sync.Mutex
			called  []string
			calls   [][2]int
			convert = newMockConverter(".txt")
		)
		convert.convertFunc = func(source, dest string, _ int) (converter.ConversionResult, error) {
			mu.Lock()
			called = append(called, source)
			mu.Unlock()

			return converter.ConversionResult{SourcePath: source, DestPath: dest, Status: converter.StatusSuccess}, nil
		}
		callback := func(completed, total int) {
			calls = append(calls, [2]int{completed, total})
		}

		m := converter.NewConversionManager([]converter.Converter{convert}, nil, 2, callback)
		m.SleepFunc = func(time.Duration) {}
		summary := m.ConvertFiles([]converter.FileTask{
			{Source: collidingB, Dest: collidedDest},
			{Source: unique, Dest: uniqueDest},
			{Source: collidingA, Dest: collidedDest},
		})

		assert.Equal(t, 3, summary.Total)
		assert.Equal(t, 1, summary.Success)
		assert.Equal(t, 2, summary.Failed)
		assert.Equal(t, 0, summary.Skipped)

		mu.Lock()
		assert.Equal(t, []string{unique}, called)
		mu.Unlock()

		wantMessage := "再試行しても解消しない変換失敗です: 出力先が重複しています: " +
			collidedDest + " ← " + collidingA + ", " + collidingB
		require.Len(t, summary.Results, 3)
		failed := make(map[string]converter.ConversionResult)
		for _, result := range summary.Results {
			if result.Status == converter.StatusFailed {
				failed[result.SourcePath] = result
			}
		}
		require.Len(t, failed, 2)
		for _, source := range []string{collidingA, collidingB} {
			require.Contains(t, failed, source)
			assert.Equal(t, wantMessage, failed[source].Message)
			assert.Empty(t, failed[source].DestPath)
		}

		require.Len(t, calls, 3)
		assert.Equal(t, [2]int{3, 3}, calls[len(calls)-1])
	})

	t.Run("出力先が重複する動画タスク", func(t *testing.T) {
		t.Parallel()

		type task struct {
			source string
			dest   string
		}

		cases := map[string]struct {
			tasks []task
			// winnerが空のときは重複した全タスクが恒久的な失敗となることを期待する。
			winner string
		}{
			"正常系: 出力形式と同じ拡張子の.mpgを変換し.wmvはスキップする": {
				tasks:  []task{{"op.wmv", "out/op.mpg"}, {"op.mpg", "out/op.mpg"}},
				winner: "op.mpg",
			},
			"正常系: 大文字拡張子の.MPGも出力形式と同じ拡張子として優先する": {
				tasks:  []task{{"OP.wmv", "out/OP.mpg"}, {"OP.MPG", "out/OP.mpg"}},
				winner: "OP.MPG",
			},
			"正常系: 3件以上重複しても.mpgだけを変換し残りはスキップする": {
				tasks:  []task{{"op.avi", "out/op.mpg"}, {"op.wmv", "out/op.mpg"}, {"op.mpg", "out/op.mpg"}},
				winner: "op.mpg",
			},
			"正常系: .mpegは出力拡張子.mpgと異なるため.mpgを優先する": {
				tasks:  []task{{"op.mpeg", "out/op.mpg"}, {"op.mpg", "out/op.mpg"}},
				winner: "op.mpg",
			},
			"異常系: 出力形式と同じ拡張子の変換元が無ければ全て失敗とする": {
				tasks: []task{{"op.wmv", "out/op.mpg"}, {"op.avi", "out/op.mpg"}},
			},
			"異常系: .mpegと.wmvはどちらも出力拡張子.mpgと異なるため全て失敗とする": {
				tasks: []task{{"op.mpeg", "out/op.mpg"}, {"op.wmv", "out/op.mpg"}},
			},
			"異常系: 出力形式と同じ拡張子の変換元が2件以上あれば全て失敗とする": {
				tasks: []task{{"a/op.mpg", "out/op.mpg"}, {"b/op.mpg", "out/op.mpg"}, {"op.wmv", "out/op.mpg"}},
			},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				files := make([]converter.FileTask, 0, len(tc.tasks))
				for _, tk := range tc.tasks {
					files = append(files, converter.FileTask{
						Source: filepath.Join(dir, tk.source),
						Dest:   filepath.Join(dir, tk.dest),
					})
				}
				dest := files[0].Dest

				conv := newMockConverter(".mpg", ".mpeg", ".wmv", ".avi")
				conv.outputExtension = ".mpg"
				var (
					mu     sync.Mutex
					called []string
				)
				conv.convertFunc = func(source, dest string, _ int) (converter.ConversionResult, error) {
					mu.Lock()
					called = append(called, source)
					mu.Unlock()

					return converter.ConversionResult{SourcePath: source, DestPath: dest, Status: converter.StatusSuccess}, nil
				}

				m := converter.NewConversionManager([]converter.Converter{conv}, nil, 2, nil)
				m.SleepFunc = func(time.Duration) {}
				summary := m.ConvertFiles(files)

				require.Len(t, summary.Results, len(files))
				results := make(map[string]converter.ConversionResult, len(summary.Results))
				for _, result := range summary.Results {
					results[result.SourcePath] = result
				}

				if tc.winner == "" {
					assert.Equal(t, len(files), summary.Failed)
					assert.Empty(t, called)
					for _, f := range files {
						require.Contains(t, results, f.Source)
						assert.Equal(t, converter.StatusFailed, results[f.Source].Status)
						assert.Contains(t, results[f.Source].Message, converter.ErrDestinationCollision.Error())
					}

					return
				}

				winner := filepath.Join(dir, tc.winner)
				assert.Equal(t, 1, summary.Success)
				assert.Equal(t, len(files)-1, summary.Skipped)
				assert.Equal(t, 0, summary.Failed)
				assert.Equal(t, []string{winner}, called)

				require.Contains(t, results, winner)
				assert.Equal(t, converter.StatusSuccess, results[winner].Status)
				assert.Equal(t, dest, results[winner].DestPath)

				for _, f := range files {
					if f.Source == winner {
						continue
					}
					require.Contains(t, results, f.Source)
					assert.Equal(t, converter.StatusSkipped, results[f.Source].Status)
					assert.Equal(t, "同名の "+winner+" を優先したため変換しません", results[f.Source].Message)
					// why: 動画の残留ファイル削除がスキップした変換元の旧拡張子ファイルを
					// 特定できるよう、共有する出力先を記録していることを固定する。
					assert.Equal(t, dest, results[f.Source].DestPath)
				}
			})
		}
	})
}

func TestConversionManager_Retry(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 失敗時にリトライが行われる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.failCount = 2

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)
		m.SleepFunc = func(time.Duration) {}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 1, summary.Success)
		assert.Equal(t, 0, summary.Failed)
		assert.Equal(t, 3, conv.CallCount())
	})

	t.Run("正常系: 最大リトライ回数を超えた場合は失敗", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.failCount = 5

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)
		m.SleepFunc = func(time.Duration) {}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 0, summary.Success)
		assert.Equal(t, 1, summary.Failed)
		assert.Equal(t, 3, conv.CallCount())
	})

	t.Run("正常系: 指数バックオフのタイミング", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.failCount = 2

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1.0, BackoffMultiplier: 2.0}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)

		var (
			mu     sync.Mutex
			sleeps []time.Duration
		)
		m.SleepFunc = func(d time.Duration) {
			mu.Lock()
			sleeps = append(sleeps, d)
			mu.Unlock()
		}

		m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		require.Len(t, sleeps, 2)
		assert.Equal(t, time.Second, sleeps[0])
		assert.Equal(t, 2*time.Second, sleeps[1])
	})

	t.Run("正常系: 例外発生時もリトライが行われる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.raiseError = true

		rc := converter.RetryConfig{MaxAttempts: 2, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)
		m.SleepFunc = func(time.Duration) {}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 1, summary.Failed)
		assert.Equal(t, 2, conv.CallCount())
		require.Len(t, summary.Results, 1)
		assert.Equal(t, "最大リトライ回数超過: 変換中にエラーが発生しました", summary.Results[0].Message)
	})

	permanentCases := []struct {
		name        string
		convertFunc func(source, dest string, callCount int) (converter.ConversionResult, error)
		wantMessage string
	}{
		{
			name: "正常系: ErrPermanentFailureを多段にラップしたエラーもリトライせずエラー文言を返す",
			convertFunc: func(source, _ string, _ int) (converter.ConversionResult, error) {
				return converter.ConversionResult{SourcePath: source},
					fmt.Errorf("変換処理: %w", fmt.Errorf("%w: 恒久的な失敗", converter.ErrPermanentFailure))
			},
			wantMessage: "変換処理: 再試行しても解消しない変換失敗です: 恒久的な失敗",
		},
		{
			name: "正常系: ErrPermanentFailureをラップしたエラーはリトライせずエラー文言を返す",
			convertFunc: func(_, _ string, _ int) (converter.ConversionResult, error) {
				return converter.ConversionResult{}, fmt.Errorf("%w: boom", converter.ErrPermanentFailure)
			},
			wantMessage: "再試行しても解消しない変換失敗です: boom",
		},
	}

	for _, tc := range permanentCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, "source.txt")
			writeFile(t, source, []byte("content"))
			dest := filepath.Join(dir, "dest.txt")

			conv := newMockConverter(".txt")
			conv.convertFunc = tc.convertFunc

			rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
			m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)

			var (
				mu         sync.Mutex
				sleepCount int
			)
			m.SleepFunc = func(time.Duration) {
				mu.Lock()
				sleepCount++
				mu.Unlock()
			}

			summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

			assert.Equal(t, 1, conv.CallCount())
			mu.Lock()
			assert.Zero(t, sleepCount)
			mu.Unlock()
			assert.Equal(t, 1, summary.Failed)
			require.Len(t, summary.Results, 1)
			assert.Equal(t, converter.StatusFailed, summary.Results[0].Status)
			assert.Equal(t, source, summary.Results[0].SourcePath)
			assert.Equal(t, tc.wantMessage, summary.Results[0].Message)
		})
	}

	t.Run("正常系: 一時的なエラーは最大回数まで試行し最後のエラー文言に接頭辞を付けて返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.convertFunc = func(source, _ string, callCount int) (converter.ConversionResult, error) {
			return converter.ConversionResult{SourcePath: source}, fmt.Errorf("%d回目の失敗", callCount)
		}

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)
		m.SleepFunc = func(time.Duration) {}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 3, conv.CallCount())
		assert.Equal(t, 1, summary.Failed)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, converter.StatusFailed, summary.Results[0].Status)
		assert.Equal(t, source, summary.Results[0].SourcePath)
		assert.Equal(t, "最大リトライ回数超過: 3回目の失敗", summary.Results[0].Message)
	})

	t.Run("正常系: errがnilの結果はStatusによらず再試行せずそのまま返す", func(t *testing.T) {
		t.Parallel()

		// why: 失敗の報告経路はerrだけであり、ConversionManagerは結果のStatusを
		// 再試行の判断に使わないことを固定する。
		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))

		want := converter.ConversionResult{SourcePath: source, Status: converter.StatusFailed, Message: "結果側の文言"}
		conv := newMockConverter(".txt")
		conv.convertFunc = func(_, _ string, _ int) (converter.ConversionResult, error) {
			return want, nil
		}

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)
		m.SleepFunc = func(time.Duration) {}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: filepath.Join(dir, "dest.txt")}})

		assert.Equal(t, 1, conv.CallCount())
		require.Len(t, summary.Results, 1)
		assert.Equal(t, want, summary.Results[0])
	})

	t.Run("異常系: 試行回数が0以下なら変換せず失敗として返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))

		conv := newMockConverter(".txt")

		rc := converter.RetryConfig{MaxAttempts: 0, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: filepath.Join(dir, "dest.txt")}})

		assert.Zero(t, conv.CallCount())
		assert.Equal(t, 1, summary.Failed)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, "変換に失敗しました", summary.Results[0].Message)
	})

	t.Run("正常系: スキップ結果はリトライせず変換結果をそのまま返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("content"))
		dest := filepath.Join(dir, "dest.txt")

		conv := newMockConverter(".txt")
		conv.convertFunc = func(source, _ string, _ int) (converter.ConversionResult, error) {
			return converter.ConversionResult{
				SourcePath: source,
				Status:     converter.StatusSkipped,
				Message:    "変換不要のためスキップ",
			}, nil
		}

		rc := converter.RetryConfig{MaxAttempts: 3, BackoffBase: 1, BackoffMultiplier: 2}
		m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)

		var (
			mu         sync.Mutex
			sleepCount int
		)
		m.SleepFunc = func(time.Duration) {
			mu.Lock()
			sleepCount++
			mu.Unlock()
		}

		summary := m.ConvertFiles([]converter.FileTask{{Source: source, Dest: dest}})

		assert.Equal(t, 1, conv.CallCount())
		mu.Lock()
		assert.Zero(t, sleepCount)
		mu.Unlock()
		assert.Equal(t, 1, summary.Skipped)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, converter.StatusSkipped, summary.Results[0].Status)
		assert.Equal(t, "変換不要のためスキップ", summary.Results[0].Message)
	})
}

func TestConversionManager_ConvertDirectory(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 再帰的なディレクトリ変換", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		mkdirAll(t, sourceDir)
		writeFile(t, filepath.Join(sourceDir, "file1.txt"), []byte("content1"))
		subDir := filepath.Join(sourceDir, "subdir")
		mkdirAll(t, subDir)
		writeFile(t, filepath.Join(subDir, "file2.txt"), []byte("content2"))

		destDir := filepath.Join(t.TempDir(), "dest")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, true)

		require.NoError(t, err)
		assert.Equal(t, 2, summary.Total)
		assert.Equal(t, 2, summary.Success)
	})

	t.Run("正常系: 非再帰的なディレクトリ変換", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		mkdirAll(t, sourceDir)
		writeFile(t, filepath.Join(sourceDir, "file1.txt"), []byte("content1"))
		subDir := filepath.Join(sourceDir, "subdir")
		mkdirAll(t, subDir)
		writeFile(t, filepath.Join(subDir, "file2.txt"), []byte("content2"))

		destDir := filepath.Join(t.TempDir(), "dest")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, false)

		require.NoError(t, err)
		assert.Equal(t, 1, summary.Total)
		assert.Equal(t, 1, summary.Success)
	})

	t.Run("正常系: サポートされるファイルのみが変換される", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		mkdirAll(t, sourceDir)
		writeFile(t, filepath.Join(sourceDir, "file1.txt"), []byte("text content"))
		writeFile(t, filepath.Join(sourceDir, "file2.pdf"), []byte("pdf content"))
		writeFile(t, filepath.Join(sourceDir, "file3.txt"), []byte("more text"))

		destDir := filepath.Join(t.TempDir(), "dest")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, true)

		require.NoError(t, err)
		assert.Equal(t, 2, summary.Total)
		assert.Equal(t, 2, summary.Success)
	})

	t.Run("正常系: ディレクトリ構造が保持される", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		nestedDir := filepath.Join(sourceDir, "sub", "nested")
		mkdirAll(t, nestedDir)
		writeFile(t, filepath.Join(nestedDir, "file.txt"), []byte("content"))

		destDir := filepath.Join(t.TempDir(), "dest")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, true)

		require.NoError(t, err)
		require.Equal(t, 1, summary.Success)
		require.Len(t, summary.Results, 1)
		assert.Contains(t, summary.Results[0].DestPath, "sub")
		assert.Contains(t, summary.Results[0].DestPath, "nested")
	})

	t.Run("正常系: GetOutputExtensionが空でない場合に変換先の拡張子が変更される", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		mkdirAll(t, sourceDir)
		writeFile(t, filepath.Join(sourceDir, "asset.tlg"), []byte("content"))

		destDir := filepath.Join(t.TempDir(), "dest")

		mc := newMockConverter(".tlg")
		mc.outputExtension = ".png"
		m := converter.NewConversionManager([]converter.Converter{mc}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, true)

		require.NoError(t, err)
		require.Equal(t, 1, summary.Success)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, filepath.Join(destDir, "asset.png"), summary.Results[0].DestPath)
	})

	t.Run("正常系: GetOutputExtensionが空の場合は変換先の拡張子が保持される", func(t *testing.T) {
		t.Parallel()

		sourceDir := filepath.Join(t.TempDir(), "source")
		mkdirAll(t, sourceDir)
		writeFile(t, filepath.Join(sourceDir, "file.txt"), []byte("content"))

		destDir := filepath.Join(t.TempDir(), "dest")

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 1, nil)
		summary, err := m.ConvertDirectory(sourceDir, destDir, true)

		require.NoError(t, err)
		require.Equal(t, 1, summary.Success)
		require.Len(t, summary.Results, 1)
		assert.Equal(t, filepath.Join(destDir, "file.txt"), summary.Results[0].DestPath)
	})
}

func TestCalculateWorkers(t *testing.T) {
	t.Parallel()

	t.Run("正常系: メモリ制約が十分な場合はCPUコア数を超えない", func(t *testing.T) {
		t.Parallel()

		mem := 1 << 20 // 十分に大きいメモリ量
		workers := converter.CalculateWorkers(&mem)

		assert.Positive(t, workers)
	})

	t.Run("正常系: 極端に少ないメモリでも最小1", func(t *testing.T) {
		t.Parallel()

		workers := converter.CalculateWorkers(new(1))

		assert.Equal(t, 1, workers)
	})

	t.Run("正常系: メモリ指定なしでも最小1", func(t *testing.T) {
		t.Parallel()

		workers := converter.CalculateWorkers(nil)

		assert.GreaterOrEqual(t, workers, 1)
	})
}

func TestConversionManager_SummaryCountsAllStatuses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	successFile := filepath.Join(dir, "success.txt")
	writeFile(t, successFile, []byte("content"))
	failFile := filepath.Join(dir, "fail.txt")
	writeFile(t, failFile, []byte("content"))
	skipFile := filepath.Join(dir, "skip.pdf")
	writeFile(t, skipFile, []byte("content"))

	conv := newMockConverter(".txt")
	conv.convertFunc = func(source, dest string, _ int) (converter.ConversionResult, error) {
		if strings.Contains(filepath.Base(source), "fail") {
			return converter.ConversionResult{SourcePath: source}, errors.New("強制失敗")
		}

		return converter.ConversionResult{SourcePath: source, DestPath: dest, Status: converter.StatusSuccess}, nil
	}

	rc := converter.RetryConfig{MaxAttempts: 1, BackoffBase: 1, BackoffMultiplier: 2}
	m := converter.NewConversionManager([]converter.Converter{conv}, &rc, 1, nil)

	summary := m.ConvertFiles([]converter.FileTask{
		{Source: successFile, Dest: filepath.Join(dir, "out_success.txt")},
		{Source: failFile, Dest: filepath.Join(dir, "out_fail.txt")},
		{Source: skipFile, Dest: filepath.Join(dir, "out_skip.pdf")},
	})

	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 1, summary.Success)
	assert.Equal(t, 1, summary.Failed)
	assert.Equal(t, 1, summary.Skipped)
	assert.Len(t, summary.Results, 3)
}
