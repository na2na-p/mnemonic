// Package info はゲームディレクトリの構成解析（エンジン検出・ファイル統計・
// エンコーディング検出）を提供する。
package info

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saintfish/chardet"
)

// FileStats はファイル統計を表す値。
type FileStats struct {
	Count          int
	Extensions     []string
	TotalSizeBytes int64
}

// GameInfo はゲーム情報を表す値。
//
// DetectedEncodingが空文字列の場合、Python版のNone（検出できなかった場合）に
// 相当する。
type GameInfo struct {
	Engine           string
	Scripts          FileStats
	Images           FileStats
	Audio            FileStats
	Video            FileStats
	DetectedEncoding string
}

var (
	scriptExtensions = []string{".ks", ".tjs"}
	imageExtensions  = []string{".png", ".jpg", ".jpeg", ".bmp", ".gif"}
	audioExtensions  = []string{".ogg", ".wav", ".mp3", ".flac", ".mid", ".midi"}
	videoExtensions  = []string{".mp4", ".avi", ".wmv", ".mkv"}
)

// DetectEngine はエンジンを検出する（"kirikiri" / "rpgmaker" / "unknown"）。
//
// pathの直下（非再帰）のエントリのみを走査する（Python版のpath.iterdir()に
// 相当）。
func DetectEngine(path string) string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "unknown"
	}

	for _, entry := range entries {
		if strings.EqualFold(filepath.Ext(entry.Name()), ".xp3") {
			return "kirikiri"
		}
	}

	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(filepath.Ext(entry.Name())), ".rgss") {
			return "rpgmaker"
		}
	}

	return "unknown"
}

// CollectFileStats はpath配下（再帰）のextensions一致ファイルの統計を収集する。
func CollectFileStats(path string, extensions []string) FileStats {
	extLower := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		extLower[strings.ToLower(ext)] = struct{}{}
	}

	var (
		count          int
		totalSizeBytes int64
	)
	foundExtensions := make(map[string]struct{})

	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Python版と同様、走査不能なエントリはスキップする
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := extLower[ext]; !ok {
			return nil
		}

		fileInfo, infoErr := d.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // 同上
		}

		count++
		totalSizeBytes += fileInfo.Size()
		foundExtensions[ext] = struct{}{}

		return nil
	})

	extensionsOut := make([]string, 0, len(foundExtensions))
	for ext := range foundExtensions {
		extensionsOut = append(extensionsOut, ext)
	}
	sort.Strings(extensionsOut)

	return FileStats{Count: count, Extensions: extensionsOut, TotalSizeBytes: totalSizeBytes}
}

// AnalyzeGame はゲームディレクトリを解析する。
func AnalyzeGame(path string) GameInfo {
	return GameInfo{
		Engine:           DetectEngine(path),
		Scripts:          CollectFileStats(path, scriptExtensions),
		Images:           CollectFileStats(path, imageExtensions),
		Audio:            CollectFileStats(path, audioExtensions),
		Video:            CollectFileStats(path, videoExtensions),
		DetectedEncoding: detectEncoding(path, scriptExtensions),
	}
}

// detectEncoding はスクリプトファイルのエンコーディングを検出する。
// 検出できない場合は空文字列を返す。
//
// why not: 複数のスクリプトファイルが混在するディレクトリでは、最初に見つかった
// ファイルの検出結果を採用し以降は走査しない（下のfilepath.WalkDir内のearly
// return参照）。Python版はpath.rglob("*")の列挙順（OSのディレクトリエントリ順
// —— 多くの場合ファイルシステム依存で作成順や無秩序になる）に依存するため、
// 同じディレクトリでも実行ごと・環境ごとに異なるファイルが「最初の1件」に
// なりうり、結果として検出されるエンコーディングが非決定的になる欠陥がある
// （実機で確認: 同一fixtureに対しPython "utf-8" / Go "ascii" のように結果が
// 割れた）。Go版はfilepath.WalkDirがディレクトリエントリを常にファイル名の
// 辞書順で返すことを利用し、意図的にPython版の非決定性を踏襲せず
// 「辞書順で最初に見つかったファイルが勝つ」という決定的な規則に統一する
// （TestAnalyzeGame_EncodingDetectionIsDeterministicByLexicalOrderでピン留め）。
func detectEncoding(path string, extensions []string) string {
	extLower := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		extLower[strings.ToLower(ext)] = struct{}{}
	}

	detector := chardet.NewTextDetector()
	encoding := ""

	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || encoding != "" {
			return nil //nolint:nilerr // 走査不能なエントリはスキップし、検出済みなら早期終了する
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := extLower[strings.ToLower(filepath.Ext(p))]; !ok {
			return nil
		}

		data, readErr := os.ReadFile(p) //nolint:gosec // 解析対象ディレクトリ配下の走査済みファイルを読む用途のため妥当
		if readErr != nil || len(data) == 0 {
			return nil
		}

		if charset, ok := detectCharset(detector, data); ok {
			encoding = charset
		}

		return nil
	})

	return encoding
}

// detectCharset はdataの文字コードを推定し、Python版chardetの語彙に正規化して返す。
//
// why not: github.com/saintfish/chardetには専用のASCII判定器がなく、純ASCII
// バイト列に対しても単バイト系のフォールバック候補（低信頼度）を返すことがある。
// Python版が使うchardetは全バイトが0x7F以下の場合に必ず"ascii"を返すため、
// internal/parser/detector.goのdetectCharsetと同じ理由でここでも同じ判定を
// 先に行う。
func detectCharset(detector *chardet.Detector, data []byte) (string, bool) {
	if isASCII(data) {
		return "ascii", true
	}

	result, err := detector.DetectBest(data)
	if err != nil || result == nil || result.Charset == "" {
		return "", false
	}

	return strings.ToLower(result.Charset), true
}

// isASCII はdataが7ビットASCII（0x00〜0x7F）のみで構成されているかを判定する。
// ESC(0x1B)はISO-2022-JP等7bitエンコーディングの制御文字であるため除外する
// （internal/parser/detector.goのisASCIIと同じ理由）。
func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 || b == 0x1b {
			return false
		}
	}

	return true
}
