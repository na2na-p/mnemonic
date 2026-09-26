package pipeline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// normalizeCriticalFilenames はdirectory配下の全ファイル名を正規化
// （小文字化）する。
//
// Windowsはファイル名の大文字小文字を区別しないが、Androidは区別する。
// 変換フェーズの最後（他の変換処理が元のケースでファイルを作成した後）に
// 実行する必要がある。
func (b *BuildPipeline) normalizeCriticalFilenames(directory string) error {
	var allPaths []string

	err := filepath.WalkDir(directory, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == directory {
			return nil
		}
		allPaths = append(allPaths, path)

		return nil
	})
	if err != nil {
		return fmt.Errorf("ファイル名正規化のための走査に失敗しました: %w", err)
	}

	// 深い階層から処理する（パス階層の深い順にソートする）。
	sort.Slice(allPaths, func(i, j int) bool {
		return len(strings.Split(allPaths[i], string(filepath.Separator))) >
			len(strings.Split(allPaths[j], string(filepath.Separator)))
	})

	for _, path := range allPaths {
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			continue
		}

		name := filepath.Base(path)
		lower := strings.ToLower(name)
		if name == lower {
			continue
		}

		newPath := filepath.Join(filepath.Dir(path), lower)
		if fileExists(newPath) {
			continue
		}

		if err := os.Rename(path, newPath); err != nil {
			return fmt.Errorf("ファイル名の正規化に失敗しました: %s: %w", path, err)
		}
	}

	return nil
}
