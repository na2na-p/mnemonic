package converter

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Converter は個々のアセット変換処理を表すインターフェース。
//
// why: Goの慣習に従い、利用側であるConversionManagerパッケージ（本ファイル）で
// 定義する。EncodingConverter/ScriptAdjuster/ImageConverter/VideoConverterは
// 同一パッケージ内で構造的にこのインターフェースを満たす。
//
// Convertがerrorを返すのは、呼び出し元(ConversionManager)まで失敗が伝播する
// ケース（ImageConverterのvalidateSourceやTLG未実装エラー等）をGoの慣用的な
// エラー戻り値として表現するため。EncodingConverter/ScriptAdjuster/
// VideoConverterは全ての既知の失敗を自身でConversionResultへ変換している
// ためerr=nilを返す（詳細は各Convertメソッドのdocコメントを参照）。
type Converter interface {
	CanConvert(filePath string) bool
	Convert(source, dest string) (ConversionResult, error)
	SupportedExtensions() []string

	// GetOutputExtension はsourcePathに対する変換後ファイルの拡張子
	// （ドット付き小文字、例: ".png"）を返す。拡張子を変更しない場合は
	// 空文字列を返す。
	GetOutputExtension(sourcePath string) string
}

// RetryConfig はリトライ動作を制御する設定。
// 指数バックオフ: backoffBase * (backoffMultiplier ** (attempt-1)) 秒待機する。
type RetryConfig struct {
	MaxAttempts       int
	BackoffBase       float64
	BackoffMultiplier float64
}

// DefaultRetryConfig はリトライ設定の既定値を返す。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 3, BackoffBase: 1.0, BackoffMultiplier: 2.0}
}

// ConversionTask は単一ファイルの変換タスクを表す。
type ConversionTask struct {
	Source     string
	Dest       string
	Converter  Converter
	RetryCount int
}

// FileTask は変換元・変換先パスの組。ConvertFilesへの入力単位。
type FileTask struct {
	Source string
	Dest   string
}

// ConversionSummary は複数ファイル変換結果の集計。
type ConversionSummary struct {
	Total   int
	Success int
	Failed  int
	Skipped int
	Results []ConversionResult
}

// ProgressCallback は進捗報告用コールバック(完了数, 総数)。
type ProgressCallback func(completed, total int)

// MemoryPerWorkerMB は1ワーカーあたりのメモリ使用量想定値(MB)。
const MemoryPerWorkerMB = 500

// ConversionManager は複数ファイルの並列変換を管理する。
type ConversionManager struct {
	Converters       []Converter
	RetryConfig      RetryConfig
	MaxWorkers       int
	ProgressCallback ProgressCallback

	// SleepFunc はリトライ待機に使う関数。既定はtime.Sleep。
	//
	// why: テストでリトライ待機時間を検証・高速化するため注入可能にする。
	SleepFunc func(time.Duration)
}

// NewConversionManager はConversionManagerを初期化する。
// retryConfigがnilの場合はDefaultRetryConfig()を使用する。
// maxWorkersが0以下の場合はCalculateWorkers(nil)で自動計算する。
func NewConversionManager(
	converters []Converter,
	retryConfig *RetryConfig,
	maxWorkers int,
	progressCallback ProgressCallback,
) *ConversionManager {
	rc := DefaultRetryConfig()
	if retryConfig != nil {
		rc = *retryConfig
	}

	workers := maxWorkers
	if workers <= 0 {
		workers = CalculateWorkers(nil)
	}

	return &ConversionManager{
		Converters:       converters,
		RetryConfig:      rc,
		MaxWorkers:       workers,
		ProgressCallback: progressCallback,
		SleepFunc:        time.Sleep,
	}
}

// GetConverterForFile はfilePathに対応するConverterを検索し、最初にマッチした
// ものを返す。存在しない場合はnilを返す。
func (m *ConversionManager) GetConverterForFile(filePath string) Converter {
	for _, c := range m.Converters {
		if c.CanConvert(filePath) {
			return c
		}
	}

	return nil
}

// ConvertFiles は複数ファイルを並列変換し、サマリーを返す。
//
// why not: Goではerrgroup.WithContextは最初のエラーで打ち切る挙動になるが、
// convertWithRetry内で既に全ての失敗を捕捉しConversionResultへ変換している
// ため「エラーで打ち切る」概念がそもそも無い。そのため、MaxWorkers個の
// goroutineがタスクチャネルを消費し、結果を全て収集し切るまで待つ
// 単純なワーカープールを採用した（境界付き並列度・全結果収集という性質を
// 素直に表現できるため）。
func (m *ConversionManager) ConvertFiles(files []FileTask) ConversionSummary {
	summary := ConversionSummary{Total: len(files)}

	workers := m.MaxWorkers
	if workers < 1 {
		workers = 1
	}

	tasksCh := make(chan FileTask)
	resultsCh := make(chan ConversionResult, len(files))

	var (
		wg             sync.WaitGroup
		mu             sync.Mutex
		completedCount int
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()

			for task := range tasksCh {
				result := m.convertWithRetry(task.Source, task.Dest)

				// why: ロック解放後にコールバックを呼ぶと、複数のgoroutineが
				// completedCountの読み取り値をまたいで並行にコールバックを
				// 呼び出し得るため、コールバック側の状態（外部スライスへの追記等）
				// に対してデータレースや順序の非単調性を引き起こす。
				// そのためロック区間内で呼び出し、直列化する。
				mu.Lock()
				completedCount++
				if m.ProgressCallback != nil {
					m.ProgressCallback(completedCount, summary.Total)
				}
				mu.Unlock()

				resultsCh <- result
			}
		}()
	}

	go func() {
		for _, f := range files {
			tasksCh <- f
		}
		close(tasksCh)
	}()

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for result := range resultsCh {
		summary.Results = append(summary.Results, result)

		switch result.Status {
		case StatusSuccess:
			summary.Success++
		case StatusSkipped:
			summary.Skipped++
		case StatusFailed:
			summary.Failed++
		default:
			summary.Failed++
		}
	}

	return summary
}

// convertWithRetry はリトライ付きで単一ファイルを変換する。
func (m *ConversionManager) convertWithRetry(source, dest string) ConversionResult {
	conv := m.GetConverterForFile(source)
	if conv == nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusSkipped,
			Message:    "対応するConverterが見つかりません",
		}
	}

	var (
		lastResult    ConversionResult
		lastErr       error
		hasLastResult bool
	)

	for attempt := 0; attempt < m.RetryConfig.MaxAttempts; attempt++ {
		result, err := conv.Convert(source, dest)

		switch {
		case err != nil:
			lastErr = err
			lastResult = ConversionResult{SourcePath: source, Status: StatusFailed, Message: err.Error()}
			hasLastResult = true
		case result.Status == StatusSuccess:
			return result
		default:
			lastResult = result
			hasLastResult = true
			lastErr = nil
		}

		if attempt+1 < m.RetryConfig.MaxAttempts {
			backoff := m.RetryConfig.BackoffBase * math.Pow(m.RetryConfig.BackoffMultiplier, float64(attempt))
			m.sleep(backoff)
		}
	}

	if lastErr != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("最大リトライ回数超過: %s", lastErr),
		}
	}

	if hasLastResult {
		return lastResult
	}

	return ConversionResult{SourcePath: source, Status: StatusFailed, Message: "変換に失敗しました"}
}

func (m *ConversionManager) sleep(seconds float64) {
	fn := m.SleepFunc
	if fn == nil {
		fn = time.Sleep
	}
	fn(time.Duration(seconds * float64(time.Second)))
}

// ConvertDirectory はsourceDir配下の対応ファイルをdestDirへ変換し、サマリーを
// 返す。recursive=trueの場合はサブディレクトリも再帰的に処理する。
func (m *ConversionManager) ConvertDirectory(sourceDir, destDir string, recursive bool) (ConversionSummary, error) {
	files, err := m.collectDirectoryFiles(sourceDir, destDir, recursive)
	if err != nil {
		return ConversionSummary{}, err
	}

	return m.ConvertFiles(files), nil
}

func (m *ConversionManager) collectDirectoryFiles(sourceDir, destDir string, recursive bool) ([]FileTask, error) {
	if recursive {
		return m.collectDirectoryFilesRecursive(sourceDir, destDir)
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("ディレクトリの走査に失敗しました: %w", err)
	}

	files := make([]FileTask, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(sourceDir, entry.Name())
		conv := m.GetConverterForFile(path)
		if conv == nil {
			continue
		}

		dest := filepath.Join(destDir, entry.Name())
		if outputExt := conv.GetOutputExtension(path); outputExt != "" {
			dest = withExtension(dest, outputExt)
		}

		files = append(files, FileTask{Source: path, Dest: dest})
	}

	return files, nil
}

func (m *ConversionManager) collectDirectoryFilesRecursive(sourceDir, destDir string) ([]FileTask, error) {
	var files []FileTask

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		conv := m.GetConverterForFile(path)
		if conv == nil {
			return nil
		}

		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return relErr
		}

		dest := filepath.Join(destDir, rel)
		if outputExt := conv.GetOutputExtension(path); outputExt != "" {
			dest = withExtension(dest, outputExt)
		}

		files = append(files, FileTask{Source: path, Dest: dest})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ディレクトリの走査に失敗しました: %w", err)
	}

	return files, nil
}

// withExtension はpathの拡張子をextへ置き換えたパスを返す（PathlibのPath.
// with_suffixに相当）。
func withExtension(path, ext string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext
}

// CalculateWorkers は最適なワーカー数を計算する。1ワーカーあたり
// MemoryPerWorkerMBのメモリを想定する。availableMemoryMBがnilの場合は
// 自動検出を試みる。
//
// why not: Go Library SSOTにはクロスプラットフォームのメモリ量検出ライブラリが
// 無いため、availableMemoryMBが明示されない場合の自動検出はCPUコア数のみで
// 計算する経路になる。
func CalculateWorkers(availableMemoryMB *int) int {
	return calculateWorkersFor(availableMemoryMB, runtime.NumCPU())
}

func calculateWorkersFor(availableMemoryMB *int, cpuCount int) int {
	if cpuCount < 1 {
		cpuCount = 1
	}

	if availableMemoryMB != nil {
		workers := *availableMemoryMB / MemoryPerWorkerMB

		return max(1, min(workers, cpuCount))
	}

	return max(1, cpuCount)
}
