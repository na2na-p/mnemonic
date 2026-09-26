//go:build !darwin && !linux && !windows

package fsutil

import "errors"

// FreeSpace は空き容量の取得に対応していないOSでは常にerrors.ErrUnsupportedを返す。
func FreeSpace(string) (uint64, error) {
	return 0, errors.ErrUnsupported
}
