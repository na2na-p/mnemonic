// Package resources はビルド成果物へ同梱する静的リソースファイルを提供する。
//
// バイナリへの同梱にはgo:embedを使うため、実行時にパッケージ配置場所へ依存しない。
package resources

import "embed"

// SystemPolyfillFS はkrkrsdl2/kag3由来のpolyfill TJSファイル一式を保持する。
//
// 不足クラスのpolyfill・スタブを提供する。SystemPolyfillFilesが実際に
// ゲームデータへコピーする対象を定義する。
//
//go:embed system_polyfill/*.tjs
var SystemPolyfillFS embed.FS

// SystemPolyfillFiles はビルド時にゲームデータのsystem/へコピーするpolyfill
// ファイル名の一覧。
//
// why not: SystemPolyfillFSには6ファイル（KAGParser.tjs、MenuItem_stub.tjs、
// MIDISoundBuffer_stub.tjs、PolyfillInitialize.tjs、SaveDataPath_patch.tjs、
// VideoOverlay_stub.tjs）を同梱するが、Python版の_copy_polyfill_filesは
// SaveDataPath_patch.tjsをコピー対象リストに含めない
// （どこからも参照されない未使用リソースであり、意図的に除外されていると
// 判断した）。Go版もこの一覧を忠実に踏襲し、5ファイルのみをコピー対象とする。
var SystemPolyfillFiles = []string{
	"PolyfillInitialize.tjs",
	"MenuItem_stub.tjs",
	"KAGParser.tjs",
	"MIDISoundBuffer_stub.tjs",
	"VideoOverlay_stub.tjs",
}
