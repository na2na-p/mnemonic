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
// why not: SystemPolyfillFSには7ファイル（KAGParser.tjs、MenuItem_stub.tjs、
// MenuOpener.tjs、MIDISoundBuffer_stub.tjs、PolyfillInitialize.tjs、
// SaveDataPath_patch.tjs、VideoOverlay_stub.tjs）を同梱する。このうち
// SaveDataPath_patch.tjsだけはコピー対象から除外する（どこからも参照されない
// 未使用リソースであり、Python版の_copy_polyfill_filesが対象とする5ファイル
// にも含まれていないため、意図的に除外されていると判断した）。MenuOpener.tjs
// はPython版に存在しないmnemonic独自の新規ポリフィルであり、Python版との
// 対応関係とは独立にコピー対象へ含める。
var SystemPolyfillFiles = []string{
	"PolyfillInitialize.tjs",
	"MenuItem_stub.tjs",
	"MenuOpener.tjs",
	"KAGParser.tjs",
	"MIDISoundBuffer_stub.tjs",
	"VideoOverlay_stub.tjs",
}
