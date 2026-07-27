package converter_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// mockConverter はConversionManagerのテスト用Converter実装。
// Python版tests/converter/test_manager.pyのMockConverterに相当する。
type mockConverter struct {
	extensions  []string
	failCount   int
	raiseError  bool
	convertFunc func(source, dest string, callCount int) (converter.ConversionResult, error)

	mu        sync.Mutex
	callCount int
}

func newMockConverter(extensions ...string) *mockConverter {
	return &mockConverter{extensions: extensions}
}

func (c *mockConverter) SupportedExtensions() []string { return c.extensions }

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
		return converter.ConversionResult{SourcePath: source, Status: converter.StatusFailed, Message: "変換失敗"}, nil
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

func TestConversionTask_Fields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	dest := filepath.Join(dir, "dest.txt")
	conv := newMockConverter(".txt")

	task := converter.ConversionTask{Source: source, Dest: dest, Converter: conv, RetryCount: 2}

	assert.Equal(t, source, task.Source)
	assert.Equal(t, dest, task.Dest)
	assert.Same(t, conv, task.Converter)
	assert.Equal(t, 2, task.RetryCount)
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
			dest := filepath.Join(dir, "dest.txt")
			files = append(files, converter.FileTask{Source: source, Dest: dest})
			_ = i
		}

		m := converter.NewConversionManager([]converter.Converter{newMockConverter(".txt")}, nil, 2, nil)
		summary := m.ConvertFiles(files)

		assert.Equal(t, 3, summary.Total)
		assert.Equal(t, 3, summary.Success)
		assert.Equal(t, 0, summary.Failed)
		assert.Len(t, summary.Results, 3)
	})

	t.Run("正常系: 並列実行の検証", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, 4)
		for i := range 4 {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, "dest.txt"),
			})
			_ = i
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
				Dest:   filepath.Join(dir, "dest.txt"),
			})
			_ = i
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

		found := false
		for _, c := range calls {
			if c[0] == 3 && c[1] == 3 {
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("正常系: 並列実行時も進捗コールバックはロック内で単調増加に呼ばれる", func(t *testing.T) {
		t.Parallel()

		// why: レビュー指摘の回帰防止。ProgressCallbackがcompletedCountの更新と
		// 同じロック区間内で呼ばれることを検証する。コールバック自体には
		// 意図的に追加のmutexを持たせず、ConversionManager側の排他制御だけで
		// スライスへの追記が安全（-raceでクリーン）かつ1..Nの単調増加になる
		// ことを確認する（Python版がwith lock:内でcallbackを呼ぶ挙動と同一）。
		const fileCount = 50

		dir := t.TempDir()
		files := make([]converter.FileTask, 0, fileCount)
		for range fileCount {
			files = append(files, converter.FileTask{
				Source: filepath.Join(dir, "source.txt"),
				Dest:   filepath.Join(dir, "dest.txt"),
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

		mem := 1
		workers := converter.CalculateWorkers(&mem)

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
			return converter.ConversionResult{SourcePath: source, Status: converter.StatusFailed, Message: "強制失敗"}, nil
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
