package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// startupArchive はビルドで展開するXP3アーカイブの選択結果。
type startupArchive struct {
	// path は展開するアーカイブのパス。embeddedがtrueならXP3を埋め込んだEXEのパス。
	path string
	// adjacent はEXEの入力に対し、同じフォルダのdata.xp3をpathに選んだことを表す。
	adjacent bool
	// embedded はpathがXP3を埋め込んだEXEであることを表す。
	embedded bool
	// embeddedUnused はdata.xp3を選んだEXEの入力が、使わないXP3を埋め込んでいることを表す。
	embeddedUnused bool
	// windowsLoads は、Windows版がpathの代わりに読み込む同じフォルダのものの表示名。
	// Windows版もpathを読み込む場合は空。
	windowsLoads string
	// ignored はpathと同じフォルダにあって読まない.xp3ファイルの名前（名前順）。
	// 同じフォルダの.xp3ファイルを同梱しない入力（名前がdata.xp3でないXP3）だけで使う。
	ignored []string
	// secondaries はpathと同じフォルダにあって変換してAPKへ同梱する.xp3ファイルの名前（名前順）。
	secondaries []string
	// unbundled はpathと同じフォルダにあってWindows版のゲームが読み込みうるが、
	// APKに含めないものの表示名（findUnbundledの順）。
	unbundled []string
}

// windowsProbe はWindows版がEXEのフォルダで確かめる名前と、それがフォルダで
// あるべきかどうか。
type windowsProbe struct {
	name string
	dir  bool
}

// Windows版がEXEのフォルダで確かめるもの。EXE自身の埋め込みXP3は
// probeDataExeの後、probeDataDirの前に確かめる。
var (
	probeContentData = windowsProbe{name: "content-data", dir: true}
	probeDataXP3     = windowsProbe{name: "data.xp3"}
	probeDataExe     = windowsProbe{name: "data.exe"}
	probeDataDir     = windowsProbe{name: "data", dir: true}
)

// resolveStartupArchive は入力inputPathから展開するXP3アーカイブを選ぶ。
//
// XP3の入力はそのまま使う。EXEの入力では、同じフォルダのdata.xp3を埋め込みXP3より
// 優先し、どちらも無ければエラーを返す。選んだアーカイブの代わりにWindows版が読み込む
// content-dataフォルダやdata.exeがあれば、その表示名をwindowsLoadsに入れる。
//
// why not: krkrsdl2（src/core/base/sdl2/SysInitImpl.cpp）の選択順に合わせない。
// 利用者が遊んでいるのはWindows版の吉里吉里で、krkr2とkrkrzのWin32版
// （base/win32/SysInitImpl.cpp のTVPBeforeSystemInit）はEXEのフォルダの
// content-dataフォルダ、data.xp3、data.exe、EXE自身に埋め込まれたアーカイブ、
// dataフォルダの順に確かめ、最初に見つかったものを読む。krkrsdl2の
// Win32向けの選択処理は#if 0で無効になっている。
//
// why not: content-dataフォルダ、data.exe、dataフォルダを展開しない。mnemonicは
// XP3アーカイブの展開しか扱わないため、見つけたことを知らせるだけにする。
//
// why not: data.xp3の中身がXP3かを確かめて埋め込みXP3に戻ることはしない。Win32版は
// data.xp3が存在してフォルダでないこと（FileExists）だけでそれを選び、中身を
// 確かめるのは埋め込みXP3の判定（TVPIsXP3Archive）だけである。
//
// why not: 名前を大文字小文字を区別して比べない。Win32版は"data.xp3"などの
// 文字列をパスに連結してWindowsのファイルシステムに問い合わせるため、
// Data.XP3のような表記でも見つかる。
//
// EXEの入力と名前がdata.xp3のXP3の入力では、同じフォルダの他の.xp3ファイルを
// secondariesに、Windows版のゲームが読み込みうるが同梱しないものをunbundledに入れる。
// secondariesに小文字にすると同じ名前になるものがあればエラーを返す。
// それ以外のXP3の入力では、同じフォルダの他の.xp3ファイルをignoredに入れる。
//
// why not: 名前がdata.xp3でないXP3の入力では同じフォルダの.xp3ファイルを同梱しない。
// EXEから取り出した埋め込みアーカイブのように、ゲームのフォルダの外で単独で
// 扱われているアーカイブでは、同じフォルダの.xp3ファイルが同じゲームのものとは限らない。
func resolveStartupArchive(inputPath string) (startupArchive, error) {
	dir := filepath.Dir(inputPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return startupArchive{}, fmt.Errorf("入力ファイルのフォルダを読み込めません: %w", err)
	}

	chosen := startupArchive{path: inputPath}
	if strings.EqualFold(filepath.Ext(inputPath), ".exe") {
		inputInfo, err := os.Stat(inputPath)
		if err != nil {
			return startupArchive{}, fmt.Errorf("入力ファイルの情報を取得できません: %w", err)
		}
		find := func(probes ...windowsProbe) string {
			return findWindowsProbe(dir, entries, inputInfo, probes)
		}

		embedded, err := hasEmbeddedXP3(inputPath)
		if err != nil {
			return startupArchive{}, err
		}
		switch dataXP3 := find(probeDataXP3); {
		case dataXP3 != "":
			chosen.path = filepath.Join(dir, dataXP3)
			chosen.adjacent = true
			chosen.embeddedUnused = embedded
			chosen.windowsLoads = find(probeContentData)
		case embedded:
			chosen.embedded = true
			chosen.windowsLoads = find(probeContentData, probeDataExe)
		default:
			if other := find(probeContentData, probeDataExe, probeDataDir); other != "" {
				return startupArchive{}, fmt.Errorf("EXEファイル内にXP3アーカイブが見つかりません: %s。"+
					"Windows版は同じフォルダの%sを読み込みますが、mnemonicは対応していません", inputPath, other)
			}

			return startupArchive{}, fmt.Errorf("EXEファイル内にXP3アーカイブが見つかりません: %s", inputPath)
		}
	}

	chosenInfo, err := os.Stat(chosen.path)
	if err != nil {
		return startupArchive{}, fmt.Errorf("アーカイブの情報を取得できません: %w", err)
	}
	var siblings []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".xp3") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.Mode().IsRegular() && !os.SameFile(info, chosenInfo) {
			siblings = append(siblings, name)
		}
	}

	if !strings.EqualFold(filepath.Ext(inputPath), ".exe") && !strings.EqualFold(filepath.Base(inputPath), probeDataXP3.name) {
		chosen.ignored = siblings

		return chosen, nil
	}
	if err := checkSecondaryNameCollision(dir, siblings); err != nil {
		return startupArchive{}, err
	}
	chosen.secondaries = siblings
	chosen.unbundled = findUnbundled(dir, entries)

	return chosen, nil
}

// findWindowsProbe はprobesを順に確かめ、dirの中で最初に見つかったものの表示名を
// 返す。フォルダであるべきものは「<名前>フォルダ」とし、入力のEXE自身は数えない。
// 見つからなければ空文字列を返す。
func findWindowsProbe(dir string, entries []os.DirEntry, input os.FileInfo, probes []windowsProbe) string {
	for _, probe := range probes {
		for _, entry := range entries {
			name := entry.Name()
			if !strings.EqualFold(name, probe.name) {
				continue
			}
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil || info.IsDir() != probe.dir || os.SameFile(info, input) {
				continue
			}
			if probe.dir {
				return name + "フォルダ"
			}

			return name
		}
	}

	return ""
}

// hasEmbeddedXP3 はexePathがXP3を埋め込んでいるかを返す。
func hasEmbeddedXP3(exePath string) (bool, error) {
	extractor, err := parser.NewEmbeddedXP3Extractor(exePath)
	if err != nil {
		return false, err
	}

	_, found, err := extractor.FindEmbeddedXP3()

	return found, err
}
