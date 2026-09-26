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

	"github.com/na2na-p/mnemonic/internal/charset"
)

// FileStats はファイル統計を表す値。
type FileStats struct {
	Count          int
	Extensions     []string
	TotalSizeBytes int64
}

// GameInfo はゲーム情報を表す値。
//
// DetectedEncodingが空文字列の場合、検出できなかったことを表す。
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
	// .tlgはKirikiriネイティブの画像形式で、ビルド時の変換対象そのものであるため、
	// 一覧から外すとアセット量の集計が実態より少なく見える。
	imageExtensions = []string{".png", ".jpg", ".jpeg", ".bmp", ".gif", ".tlg"}
	audioExtensions = []string{".ogg", ".wav", ".mp3", ".flac", ".mid", ".midi"}
	videoExtensions = []string{".mp4", ".avi", ".wmv", ".mkv"}
)

// DetectEngine はエンジンを検出する（"kirikiri" / "rpgmaker" / "unknown"）。
//
// pathの直下（非再帰）のエントリのみを走査する。
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
			return nil //nolint:nilerr // 走査不能なエントリはスキップする
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
// return参照）。OSのディレクトリエントリ順は多くの場合ファイルシステム依存で
// 作成順や無秩序になるため、それに依存すると同じディレクトリでも実行ごと・
// 環境ごとに異なるファイルが「最初の1件」になりうり、結果として検出される
// エンコーディングが非決定的になってしまう。filepath.WalkDirがディレクトリ
// エントリを常にファイル名の辞書順で返すことを利用し、「辞書順で最初に
// 見つかったファイルが勝つ」という決定的な規則に統一する
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

		if name, ok := charset.Detect(detector, data); ok {
			encoding = name
		}

		return nil
	})

	return encoding
}
