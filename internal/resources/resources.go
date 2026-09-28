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
// why not: SystemPolyfillFSには8ファイル（ExePathOverride.tjs、KAGParser.tjs、
// MenuItem_stub.tjs、MenuOpener.tjs、MIDISoundBuffer_stub.tjs、
// PolyfillInitialize.tjs、SaveDataPath_patch.tjs、VideoOverlay_stub.tjs）を
// 同梱する。このうち2ファイルはこの一覧から除外する。SaveDataPath_patch.tjsは
// どこからも参照されない未使用リソースである。ExePathOverride.tjsは
// System.exePathをゲーム全体で書き換えるため、必要なビルドだけが個別に
// 書き込む（ExePathOverrideFile参照）。MenuOpener.tjsはmnemonic独自の
// ポリフィルであり、コピー対象に含める。
var SystemPolyfillFiles = []string{
	"PolyfillInitialize.tjs",
	"MenuItem_stub.tjs",
	"MenuOpener.tjs",
	"KAGParser.tjs",
	"MIDISoundBuffer_stub.tjs",
	"VideoOverlay_stub.tjs",
}

// ExePathOverrideFile はSystem.exePathを書き換えるpolyfillの埋め込みファイル名。
const ExePathOverrideFile = "ExePathOverride.tjs"

// ExePathOverrideStorage はExePathOverrideFileをsystem/へ書き込むときの名前。
// PolyfillInitialize.tjsはこの名前で存在を確かめて実行する。
const ExePathOverrideStorage = "exepathoverride.tjs"
