//go:build darwin || linux

package fsutil

import "golang.org/x/sys/unix"

// FreeSpace はpathを含むファイルシステムで、特権を持たないプロセスが使える
// 空き容量をバイト数で返す。
//
// why not: Bfree（ファイルシステム全体の空きブロック）ではなくBavailを使う。
// statfs(2)の定義ではBavailはスーパーユーザー以外が使える空きブロックであり、
// Bfreeはそれより大きくなりうる。
func FreeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}

	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec // Bsizeの型はGOOS/GOARCHで異なり、Linuxの多くのアーキテクチャでは符号付き（s390xとdarwinはuint32）だが、ブロックサイズは常に正の値
}
