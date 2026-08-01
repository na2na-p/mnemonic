package signer

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// findAndroidBuildTool はANDROID_HOME配下のbuild-toolsディレクトリから
// 最新バージョンのtoolNameを検索し、見つからない場合はシステムPATHから検索する。
// 見つからない場合は空文字列とfalseを返す。
//
// why not: zipalign探索とapksigner探索は全く同じ探索ロジック（ANDROID_HOME
// 検索→バージョン名の降順ソート→PATH検索）を必要とする。探索順序を変更する
// 際に片方だけ更新漏れするリスクを避けるため、1箇所に集約する。
//
// バージョンのソートは単純な文字列降順であり、セマンティックバージョニング
// としての比較ではない。
//
// why not: os.ReadDirが返すDirEntry.IsDir()はシンボリックリンク自体の種別
// （リンクはリンクとして報告されディレクトリとは報告されない）を見るため、
// バージョンディレクトリがシンボリックリンクの場合に除外されてしまう。
// シンボリックリンクを解決した先の種別を見たいため、os.Stat（symlinkを
// 解決する）で判定する。
func findAndroidBuildTool(toolName string) (string, bool) {
	if androidHome := os.Getenv("ANDROID_HOME"); androidHome != "" {
		buildToolsDir := filepath.Join(androidHome, "build-tools")

		if entries, err := os.ReadDir(buildToolsDir); err == nil {
			versions := make([]string, 0, len(entries))
			for _, e := range entries {
				info, statErr := os.Stat(filepath.Join(buildToolsDir, e.Name())) //nolint:gosec // ANDROID_HOME配下のディレクトリ一覧から得たパスの種別確認のみで、内容の読み書きは行わない
				if statErr == nil && info.IsDir() {
					versions = append(versions, e.Name())
				}
			}

			sort.Sort(sort.Reverse(sort.StringSlice(versions)))

			for _, v := range versions {
				candidate := filepath.Join(buildToolsDir, v, toolName)
				if _, statErr := os.Stat(candidate); statErr == nil { //nolint:gosec // ANDROID_HOME配下のディレクトリ一覧から得たパスの存在確認のみで、内容の読み書きは行わない
					return candidate, true
				}
			}
		}
	}

	if p, err := exec.LookPath(toolName); err == nil {
		return p, true
	}

	return "", false
}
