package main

import "fmt"

// formatSize はバイト数を人間が読みやすい形式に変換する。
func formatSize(sizeBytes int64) string {
	const unit = 1024

	switch {
	case sizeBytes < unit:
		return fmt.Sprintf("%d B", sizeBytes)
	case sizeBytes < unit*unit:
		return fmt.Sprintf("%.1f KB", float64(sizeBytes)/unit)
	case sizeBytes < unit*unit*unit:
		return fmt.Sprintf("%.1f MB", float64(sizeBytes)/(unit*unit))
	default:
		return fmt.Sprintf("%.1f GB", float64(sizeBytes)/(unit*unit*unit))
	}
}
