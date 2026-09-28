package parser

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/adler32"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// WriteXP3Archive 用のセンチネルエラー群。
var (
	// ErrXP3UnencodableName はファイル名をXP3の索引に書けない場合のエラー。
	ErrXP3UnencodableName = errors.New("XP3のファイル名に使えない文字を含みます")
	// ErrXP3NonRegularFile は通常ファイル以外（シンボリックリンクなど）を含む場合のエラー。
	ErrXP3NonRegularFile = errors.New("通常ファイル以外はXP3に格納できません")
	// ErrXP3NoFiles は格納するファイルが1つもない場合のエラー。
	ErrXP3NoFiles = errors.New("XP3に格納するファイルがありません")
	// ErrXP3SourceNotDir は格納元がディレクトリではない場合のエラー。
	ErrXP3SourceNotDir = errors.New("XP3の格納元がディレクトリではありません")
	// ErrXP3DestinationInSource は出力先が格納元ディレクトリの中のファイルである
	// 場合のエラー。検出できる範囲はWriteXP3Archiveのドキュメントを参照。
	ErrXP3DestinationInSource = errors.New("XP3の出力先が格納元ディレクトリの中にあります")
	// ErrXP3NameCollision はkrkrzが同じ名前として扱うファイルが複数ある場合のエラー。
	ErrXP3NameCollision = errors.New("XP3内で同じ名前になるファイルがあります")
)

// xp3HeaderSize はマーク11バイトと索引位置uint64を合わせた長さ。
const xp3HeaderSize = 11 + 8

type xp3WriteEntry struct {
	name     string
	path     string
	utf16    []uint16
	offset   uint64
	size     uint64
	checksum uint32
}

// WriteXP3Archive はsrcDir配下の通常ファイルをすべて格納したXP3アーカイブをdstに書く。
//
// エントリ名はsrcDirからの相対パスを "/" 区切りにしたもので、名前のバイト順に
// 書く。データは無圧縮の1セグメント、索引は無圧縮の1ブロックとして末尾に置く。
// 入力の検査に失敗した場合はdstを作らず、書き込み途中で失敗した場合はdstを消す。
//
// 既に存在するdstがsrcDir内のファイルそのもの（シンボリックリンクやハードリンク
// 経由を含む）であれば、何も書く前に弾く。シンボリックリンク経由はsrcDirの走査前の
// パス判定で見つかるのでErrXP3DestinationInSourceを返す。ハードリンクは走査中の
// 照合で見つかるため、それより先に別の入力エラー（ErrXP3UnencodableNameなど）が
// 見つかればそのエラーを返す。まだ無いdstは、存在する祖先までのシンボリック
// リンクを解決したパスがsrcDirの中にある場合にErrXP3DestinationInSourceを返す。
// リンク先がまだ無いシンボリックリンクのdstや、大文字小文字を
// 区別しないファイルシステムで表記だけが違うdstはこの判定をすり抜け、srcDirの中に
// 1度だけ書かれうる。書かれたファイルは次の呼び出しでは存在するdstとして弾かれる。
//
// why not: 索引をzlibで圧縮しない。圧縮後のバイト列はzlibの実装と圧縮レベルで
// 変わり、出力をバイト単位で固定できなくなる。
func WriteXP3Archive(dst, srcDir string) error {
	entries, err := collectXP3Entries(dst, srcDir)
	if err != nil {
		return err
	}

	out, err := os.Create(dst) //nolint:gosec // 呼び出し側が指定した出力先に書く用途のため妥当
	if err != nil {
		return fmt.Errorf("XP3ファイルを作成できません: %w", err)
	}
	if err := writeXP3(out, entries); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("XP3ファイルを閉じられません: %w", err)
	}

	return nil
}

func collectXP3Entries(dst, srcDir string) ([]xp3WriteEntry, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return nil, fmt.Errorf("XP3の格納元を参照できません: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrXP3SourceNotDir, srcDir)
	}
	if err := checkDestinationOutsideSource(dst, srcDir); err != nil {
		return nil, err
	}
	dstInfo, dstStatErr := os.Stat(dst)

	var entries []xp3WriteEntry
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", ErrXP3NonRegularFile, path)
		}
		if dstStatErr == nil {
			fileInfo, err := d.Info()
			if err != nil {
				return fmt.Errorf("格納するファイルを参照できません: %w", err)
			}
			if os.SameFile(fileInfo, dstInfo) {
				return fmt.Errorf("%w: %s は %s と同じファイルです", ErrXP3DestinationInSource, dst, path)
			}
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("相対パスを求められません: %w", err)
		}
		name := filepath.ToSlash(rel)
		encoded, err := encodeXP3Name(name)
		if err != nil {
			return err
		}
		entries = append(entries, xp3WriteEntry{name: name, path: path, utf16: encoded})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("XP3の格納元を走査できません: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrXP3NoFiles, srcDir)
	}

	slices.SortFunc(entries, func(a, b xp3WriteEntry) int {
		return cmp.Compare(a.name, b.name)
	})
	if err := checkXP3NameCollision(entries); err != nil {
		return nil, err
	}

	return entries, nil
}

// checkDestinationOutsideSource は、存在する祖先までのシンボリックリンクを解決した
// dstのパスがsrcDirの中にある場合にエラーを返す。
//
// why not: リンク先がまだ無いシンボリックリンクをos.Readlinkでたどったり、
// ファイルシステムが大文字小文字を区別するかを調べたりはしない。前者はリンクの
// 連鎖を、後者はボリュームごとの設定を正しく扱う必要があり、すり抜けても書かれる
// のは有限の1ファイルで、既存のdstはos.SameFileで確実に弾けるため。
//
// why not: パスの比較だけで済ませない。srcDir内のファイルへのハードリンクは
// srcDirの外のパスでも同じファイルなので、os.Createで切り詰めたうえで書き込み中の
// 自分自身を読むことになる。collectXP3Entriesがos.SameFileでも照合する。
func checkDestinationOutsideSource(dst, srcDir string) error {
	realSrc, err := resolveExistingPrefix(srcDir)
	if err != nil {
		return err
	}
	realDst, err := resolveExistingPrefix(dst)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realSrc, realDst)
	if err != nil {
		return nil //nolint:nilerr // 相対パスにできない（別ボリューム）ならsrcDirの中ではない
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s（格納元 %s）", ErrXP3DestinationInSource, dst, srcDir)
	}

	return nil
}

// resolveExistingPrefix はpathを絶対パスにし、存在する最も深い祖先までの
// シンボリックリンクを解決したパスを返す。まだ無いdstも解決済みのsrcDirと
// 比べられるようにするため。
func resolveExistingPrefix(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("絶対パスを求められません: %w", err)
	}
	dir, rest := abs, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

func checkXP3NameCollision(entries []xp3WriteEntry) error {
	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		key := normalizeXP3Name(e.name)
		if other, ok := seen[key]; ok {
			return fmt.Errorf("%w: %s と %s", ErrXP3NameCollision, other, e.name)
		}
		seen[key] = e.name
	}

	return nil
}

// encodeXP3Name は名前を索引に書くUTF-16符号単位列にする。
//
// why not: utf16.Encodeに任せない。BMP外の文字はサロゲートペアになるが、XP3は
// マーク末尾の0x01で文字コードを「BMP 16bit Unicode」と宣言しており（krkrz
// base/XP3Archive.cpp のマーク定義の注記）、krkrzも名前をTVPStringFromBMPUnicodeで
// 読む。不正なUTF-8はU+FFFDに置き換わり、元の名前が失われる。
func encodeXP3Name(name string) ([]uint16, error) {
	encoded := make([]uint16, 0, len(name))
	for i, r := range name {
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(name[i:]); size <= 1 {
				return nil, fmt.Errorf("%w: 不正なUTF-8です: %q", ErrXP3UnencodableName, name)
			}
		}
		if r > 0xFFFF {
			return nil, fmt.Errorf("%w: BMP外の文字 %U を含みます: %s", ErrXP3UnencodableName, r, name)
		}
		encoded = append(encoded, uint16(r)) //nolint:gosec // 直前でBMP外（0xFFFF超）を弾いている
	}
	if len(encoded) > math.MaxUint16 {
		return nil, fmt.Errorf("%w: 名前が%d符号単位を超えます: %d", ErrXP3UnencodableName, math.MaxUint16, len(encoded))
	}

	return encoded, nil
}

func writeXP3(out io.WriteSeeker, entries []xp3WriteEntry) error {
	header := make([]byte, xp3HeaderSize)
	copy(header, XP3Magic)
	if _, err := out.Write(header); err != nil {
		return fmt.Errorf("XP3ヘッダーを書けません: %w", err)
	}

	position := uint64(xp3HeaderSize)
	for i := range entries {
		size, checksum, err := copyXP3Data(out, entries[i].path)
		if err != nil {
			return err
		}
		entries[i].offset = position
		entries[i].size = size
		entries[i].checksum = checksum
		position += size
	}
	indexOffset := position

	table := buildXP3Table(entries)
	index := make([]byte, 0, 1+8+len(table))
	index = append(index, indexEncodeRaw)
	index = binary.LittleEndian.AppendUint64(index, uint64(len(table)))
	index = append(index, table...)
	if _, err := out.Write(index); err != nil {
		return fmt.Errorf("XP3の索引を書けません: %w", err)
	}

	if _, err := out.Seek(int64(len(XP3Magic)), io.SeekStart); err != nil {
		return fmt.Errorf("XP3の索引位置へ移動できません: %w", err)
	}
	if _, err := out.Write(binary.LittleEndian.AppendUint64(nil, indexOffset)); err != nil {
		return fmt.Errorf("XP3の索引位置を書けません: %w", err)
	}

	return nil
}

func copyXP3Data(out io.Writer, path string) (uint64, uint32, error) {
	in, err := os.Open(path) //nolint:gosec // 格納元ディレクトリの走査で得たパスを読む用途のため妥当
	if err != nil {
		return 0, 0, fmt.Errorf("格納するファイルを開けません: %w", err)
	}
	defer func() { _ = in.Close() }()

	hash := adler32.New()
	n, err := io.Copy(io.MultiWriter(out, hash), in)
	if err != nil {
		return 0, 0, fmt.Errorf("%s をXP3に書けません: %w", path, err)
	}

	return uint64(n), hash.Sum32(), nil //nolint:gosec // io.Copyが返すバイト数は負にならない
}

func buildXP3Table(entries []xp3WriteEntry) []byte {
	var table bytes.Buffer
	for _, e := range entries {
		info := make([]byte, 0, 22+2*len(e.utf16))
		info = binary.LittleEndian.AppendUint32(info, 0)
		info = binary.LittleEndian.AppendUint64(info, e.size)
		info = binary.LittleEndian.AppendUint64(info, e.size)
		info = binary.LittleEndian.AppendUint16(info, uint16(len(e.utf16))) //nolint:gosec // encodeXP3Nameが65535符号単位以下に制限済み
		for _, u := range e.utf16 {
			info = binary.LittleEndian.AppendUint16(info, u)
		}

		segm := make([]byte, 0, 28)
		segm = binary.LittleEndian.AppendUint32(segm, 0)
		segm = binary.LittleEndian.AppendUint64(segm, e.offset)
		segm = binary.LittleEndian.AppendUint64(segm, e.size)
		segm = binary.LittleEndian.AppendUint64(segm, e.size)

		adlr := binary.LittleEndian.AppendUint32(nil, e.checksum)

		var file []byte
		file = appendXP3Chunk(file, "info", info)
		file = appendXP3Chunk(file, "segm", segm)
		file = appendXP3Chunk(file, "adlr", adlr)
		table.Write(appendXP3Chunk(nil, "File", file))
	}

	return table.Bytes()
}

func appendXP3Chunk(dst []byte, name string, body []byte) []byte {
	dst = append(dst, name...)
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(body)))
	return append(dst, body...)
}

// normalizeXP3Name はkrkrzのtTVPArchive::NormalizeInArchiveStorageName
// （base/StorageIntf.cpp）と同じ規則で名前を正規化する。
//
// why not: strings.ToLowerを使わない。krkrzが小文字にするのはASCIIのA-Zだけで、
// それ以外の大文字を畳むと、krkrzでは区別できる名前まで衝突として弾いてしまう。
func normalizeXP3Name(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case 'A' <= c && c <= 'Z':
			c += 'a' - 'A'
		case c == '\\':
			c = '/'
		}
		if c == '/' && (b.Len() == 0 || strings.HasSuffix(b.String(), "/")) {
			continue
		}
		b.WriteByte(c)
	}

	return b.String()
}
