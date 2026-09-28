package pipeline

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/parser"
)

// 副アーカイブ（起動アーカイブと同じフォルダにあってAPKへ同梱する.xp3ファイル）の
// エラー。
var (
	// ErrSecondaryArchiveNameCollision は小文字にすると同じ名前になる副アーカイブが
	// 複数ある場合のエラー。
	ErrSecondaryArchiveNameCollision = errors.New("同梱するXP3アーカイブの名前が重なります")
	// ErrSecondaryArchiveUnreadable は副アーカイブをXP3として読み込めない場合のエラー。
	ErrSecondaryArchiveUnreadable = errors.New("同梱するXP3アーカイブを読み込めません")
	// ErrSecondaryArchiveRootConflict は起動アーカイブの直下に、副アーカイブを置く
	// 名前と大文字小文字を除いて同じ名前のファイルかフォルダがある場合のエラー。
	ErrSecondaryArchiveRootConflict = errors.New("同梱するXP3アーカイブと同じ名前のものが起動アーカイブにあります")
)

// unbundledFiles はKAG3のsystem/Initialize.tjsがあれば読み込む、System.exePath
// （Windows版ではEXEのフォルダ）直下のファイル。
var unbundledFiles = []string{"Override2.tjs", "AfterInit2.tjs"}

// unbundledDir はKAG3のsystem/Initialize.tjsがStorages.addAutoPathで加える、
// System.exePath直下のフォルダ。
const unbundledDir = "video"

// secondaryArchive はAPKへ同梱する副アーカイブ。
type secondaryArchive struct {
	// source は入力のフォルダでのファイル名。
	source string
	// name はAPKに置くファイル名（sourceを小文字にしたもの）。
	name string
	// dir はsourceがあるフォルダ。
	dir     string
	archive *parser.XP3Archive
}

// stagedArchive は詰め直した副アーカイブ。
type stagedArchive struct {
	// name はAPKに置くファイル名。
	name string
	path string
}

// checkSecondaryNameCollision はnamesに小文字にすると同じ名前になるものがあれば、
// 重なるものの組をすべて挙げたErrSecondaryArchiveNameCollisionを返す。dirは
// namesがあるフォルダで、案内に使う。
func checkSecondaryNameCollision(dir string, names []string) error {
	groups := map[string][]string{}
	for _, name := range names {
		key := strings.ToLower(name)
		groups[key] = append(groups[key], name)
	}

	var collisions []string
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		if len(groups[key]) > 1 {
			collisions = append(collisions, strings.Join(groups[key], ", "))
		}
	}
	if len(collisions) == 0 {
		return nil
	}

	return fmt.Errorf("%w: 同梱するXP3アーカイブの名前は小文字にするため、次のものがAPKで同じ名前になります: %s。"+
		"使わない方を%sから別の場所へ移して再実行してください",
		ErrSecondaryArchiveNameCollision, strings.Join(collisions, " / "), dir)
}

// findUnbundled はdirの中で、Windows版のゲームが読み込みうるがAPKに含めないものの
// 表示名をentriesの順に返す。対象はサブフォルダ直下の.xp3ファイル、unbundledFiles、
// unbundledDirのフォルダで、名前は大文字小文字を区別せずに比べる。
//
// why not: サブフォルダより深い階層は調べない。入力のフォルダにはゲーム以外の
// ファイルも置かれうる（ダウンロードフォルダなど）ため、再帰的な走査は量に上限が無い。
func findUnbundled(dir string, entries []os.DirEntry) []string {
	var found []string
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if !info.IsDir() {
			if info.Mode().IsRegular() && slices.ContainsFunc(unbundledFiles, func(f string) bool { return strings.EqualFold(f, name) }) {
				found = append(found, name)
			}

			continue
		}

		if strings.EqualFold(name, unbundledDir) {
			found = append(found, name+"フォルダ")
		}
		subEntries, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if !strings.EqualFold(filepath.Ext(sub.Name()), ".xp3") {
				continue
			}
			if subInfo, err := os.Stat(filepath.Join(path, sub.Name())); err == nil && subInfo.Mode().IsRegular() {
				found = append(found, filepath.Join(name, sub.Name()))
			}
		}
	}

	return found
}

// openSecondaryArchives はdirのnamesを副アーカイブとして読み込む。
// 読み込めないものがあればErrSecondaryArchiveUnreadableを返す。
func openSecondaryArchives(dir string, names []string) ([]secondaryArchive, error) {
	archives := make([]secondaryArchive, 0, len(names))
	for _, name := range names {
		archive, err := parser.NewXP3Archive(filepath.Join(dir, name))
		if err != nil {
			return nil, unreadableSecondaryError(dir, name, err)
		}
		archives = append(archives, secondaryArchive{source: name, name: strings.ToLower(name), dir: dir, archive: archive})
	}

	return archives, nil
}

// unreadableSecondaryError はdirの副アーカイブnameを読めなかった原因errに、
// そのファイルをフォルダから移して再実行する案内を添えたErrSecondaryArchiveUnreadableを返す。
func unreadableSecondaryError(dir, name string, err error) error {
	return fmt.Errorf("%w: %s: %w。同じフォルダのXP3アーカイブはすべてAPKへ同梱するため、"+
		"含めない場合は%sを%sから別の場所へ移して再実行してください",
		ErrSecondaryArchiveUnreadable, name, err, name, dir)
}

// checkSecondaryRootConflict は展開した起動アーカイブのextractDirの直下に、
// secondariesのいずれかを置く名前と大文字小文字を除いて同じ名前のものがあれば
// ErrSecondaryArchiveRootConflictを返す。dirは副アーカイブがあるフォルダで、案内に使う。
//
// why not: 大文字小文字を区別して比べない。normalizeCriticalFilenamesは起動アーカイブの
// ファイル名を後で小文字にするため、Patch.XP3もAPKではpatch.xp3になる。
func checkSecondaryRootConflict(extractDir, dir string, secondaries []secondaryArchive) error {
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return fmt.Errorf("展開したアーカイブを読み込めません: %w", err)
	}

	for _, s := range secondaries {
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), s.name) {
				return fmt.Errorf("%w: 起動アーカイブの直下の%sと、同梱する%sがAPKで同じ名前になります。"+
					"%sを%sから別の場所へ移して再実行してください",
					ErrSecondaryArchiveRootConflict, entry.Name(), s.source, s.source, dir)
			}
		}
	}

	return nil
}

// packSecondaryArchives はsecondariesを1つずつ展開・変換してXP3に詰め直し、
// 一時ディレクトリに置いたものを返す。変換後に格納するファイルが無いものは
// 警告して除く。「このソフトについて」の表示内容は変換済みの起動アーカイブ
// primaryDirのabout.ksから読む。
//
// why not: 詰め直しを起動アーカイブの後処理（finalizeConvertedTree）の後に回さない。
// 後処理はexepathoverride.tjsを書くかどうかを同梱するアーカイブの数で決め、
// 変換後に空になって同梱しないアーカイブは詰め直すまで分からない。
// 詰め直したXP3を別の一時ディレクトリに置き、後処理の後でprimaryDirへ移すのは
// 後処理の走査に見せないためだが、正しさには必要ない。後処理の走査は拡張子
// （.mid/.midi、.ks/.tjs）か名前（プラグインディレクトリ）で対象を選び、
// ファイル名の小文字化は既に小文字の名前を飛ばすため、.xp3は変わらない。
func (b *BuildPipeline) packSecondaryArchives(
	secondaries []secondaryArchive,
	primaryDir string,
	midiConverter *converter.MidiConverter,
) ([]stagedArchive, error) {
	if len(secondaries) == 0 {
		return nil, nil
	}

	about, err := loadAboutDialog(primaryDir)
	if err != nil {
		return nil, err
	}

	stageDir, err := b.newTempDir("mnemonic_secondary_xp3_")
	if err != nil {
		return nil, err
	}

	var staged []stagedArchive
	for _, s := range secondaries {
		path, shipped, err := b.packSecondaryArchive(s, stageDir, about, midiConverter)
		if errors.Is(err, ErrSecondaryArchiveUnreadable) {
			return nil, err
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.source, err)
		}
		if shipped {
			staged = append(staged, stagedArchive{name: s.name, path: path})
		}
	}

	return staged, nil
}

// packSecondaryArchive はsを専用の一時ツリーへ展開・変換し、stageDir/s.nameへ
// XP3として詰め直す。変換後に格納するファイルが無ければ警告してfalseを返す。
// 展開ツリーと変換ツリーは戻る前に消す。
func (b *BuildPipeline) packSecondaryArchive(
	s secondaryArchive,
	stageDir string,
	about converter.AboutDialog,
	midiConverter *converter.MidiConverter,
) (string, bool, error) {
	extractDir, err := b.newTempDir("mnemonic_secondary_extract_")
	if err != nil {
		return "", false, err
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	convertDir, err := b.newTempDir("mnemonic_secondary_convert_")
	if err != nil {
		return "", false, err
	}
	defer func() { _ = os.RemoveAll(convertDir) }()

	b.log().Info(fmt.Sprintf("%sを変換します", s.source))

	// why not: 展開の失敗すべてにアーカイブを移す案内を付けない。ExtractAllは
	// 出力先の作成や書き込みの失敗をErrInvalidXP3で包まずに返し、その原因は
	// 容量不足や権限のような手元の環境でもありうる。そのときアーカイブを外すよう
	// 案内するのは誤りになる。
	if err := s.archive.ExtractAll(extractDir); err != nil {
		if errors.Is(err, parser.ErrInvalidXP3) {
			return "", false, unreadableSecondaryError(s.dir, s.source, err)
		}

		return "", false, fmt.Errorf("展開に失敗しました: %w", err)
	}
	if err := copyTree(extractDir, convertDir); err != nil {
		return "", false, err
	}

	summary, err := b.convertAssets(extractDir, convertDir)
	if err != nil {
		return "", false, err
	}

	if err := b.finalizeSecondaryTree(convertDir, summary, midiConverter, about); err != nil {
		return "", false, err
	}

	dst := filepath.Join(stageDir, s.name)
	err = parser.WriteXP3Archive(dst, convertDir)
	if errors.Is(err, parser.ErrXP3NoFiles) {
		b.log().Warning(fmt.Sprintf("%sは変換後に格納するファイルが無いため、APKに同梱しません", s.source))

		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("XP3アーカイブへの詰め直しに失敗しました: %w", err)
	}

	return dst, true, nil
}

// finalizeSecondaryTree は副アーカイブの変換済みのdirectoryへ、finalizeConvertedTreeと
// 同じ順で古い動画ファイルの削除、MIDI変換、プラグインディレクトリの削除、
// スクリプト調整を適用する。
//
// why not: polyfillのコピーとファイル名の小文字化はしない。polyfillは起動アーカイブの
// system/に置けば足り、アーカイブ内の名前はエンジンが読み込み時に小文字にする
// （krkrz base/StorageIntf.cpp のtTVPArchive::NormalizeInArchiveStorageName）。
//
// why not: startup.tjsへpolyfillの読み込みを加えない。エンジンが最初に実行する
// startup.tjsはプロジェクトの直下から探され（base/StorageIntf.cpp のTVPGetPlacedPath）、
// 副アーカイブはその後にスクリプトのStorages.addAutoPathでマウントされるため、
// 副アーカイブのstartup.tjsは起動時に実行されない。
func (b *BuildPipeline) finalizeSecondaryTree(
	directory string,
	summary converter.ConversionSummary,
	midiConverter *converter.MidiConverter,
	about converter.AboutDialog,
) error {
	removeStaleVideoSourceFiles(summary)

	if err := convertMidiFilesUsing(directory, midiConverter, b.log()); err != nil {
		return fmt.Errorf("MIDI変換に失敗しました: %w", err)
	}

	if err := b.removePluginDirectory(directory); err != nil {
		return err
	}

	return b.adjustScriptsWith(directory, about, false)
}

// placeStagedArchives はstagedを変換済みの起動アーカイブdirectoryの直下へ移す。
//
// why not: コピーせず名前を変えて移す。一時ディレクトリはどれもos.TempDir()の下にあり
// 同じファイルシステム上にあるため、移動は容量を増やさない。
func placeStagedArchives(directory string, staged []stagedArchive) error {
	for _, s := range staged {
		if err := os.Rename(s.path, filepath.Join(directory, s.name)); err != nil {
			return fmt.Errorf("XP3アーカイブの配置に失敗しました: %s: %w", s.name, err)
		}
	}

	return nil
}
