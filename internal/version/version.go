// Package version はビルド時に埋め込まれるバージョン情報を提供する。
package version

// value はビルド時に -ldflags "-X ...version.value=x.y.z" で上書きされる。
var value = "0.1.0-dev"

// String は現在のバージョン文字列を返す。
func String() string {
	return value
}
