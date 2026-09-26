//go:build windows

package fsutil

import "golang.org/x/sys/windows"

// FreeSpace はpathを含むボリュームで、呼び出し元のユーザーが使える空き容量を
// バイト数で返す。
//
// why not: GetDiskFreeSpaceExのtotalNumberOfFreeBytes（ボリューム全体の空き）
// ではなくfreeBytesAvailableToCallerを使う。Win32 APIのドキュメントでは後者は
// 呼び出し元スレッドのユーザーが使える空きバイト数で、ユーザーごとの
// クォータがある場合は前者より小さくなりうる。
func FreeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	var available uint64
	if err := windows.GetDiskFreeSpaceEx(p, &available, nil, nil); err != nil {
		return 0, err
	}

	return available, nil
}
