package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/japanese"
)

// センチネルエラー群。
var (
	// ErrGameDirNotFound は指定されたゲームディレクトリが存在しない場合のエラー。
	ErrGameDirNotFound = errors.New("ディレクトリが存在しません")
	// ErrGameDirNotADirectory は指定されたパスがディレクトリではない場合のエラー。
	ErrGameDirNotADirectory = errors.New("ディレクトリではありません")
	// ErrEmptyGameDir はゲームディレクトリが空の場合のエラー。
	ErrEmptyGameDir = errors.New("ディレクトリが空です")
)

// EngineType は検出可能なエンジン種別を表す。
//
// サポートされるゲームエンジンの種類を表す。
type EngineType string

// EngineTypeの各値。
const (
	EngineKirikiri2     EngineType = "kirikiri2"
	EngineKirikiri2KAG3 EngineType = "kirikiri2_kag3"
	EngineUnknown       EngineType = "unknown"
)

// GameStructure はゲーム構成情報を表す。
//
// ゲームディレクトリの解析結果を保持する不変値。
type GameStructure struct {
	// Engine は検出されたゲームエンジンの種別。
	Engine EngineType
	// Title はゲームタイトル（Config.tjsから取得、取得できない場合は空文字列）。
	Title string
	// Scripts は検出されたスクリプトファイルの相対パス一覧。
	Scripts []string
	// ScriptEncoding はスクリプトファイルのエンコーディング（検出できない場合は空文字列）。
	ScriptEncoding string
	// Images は検出された画像ファイルの相対パス一覧。
	Images []string
	// Audio は検出された音声ファイルの相対パス一覧。
	Audio []string
	// Video は検出された動画ファイルの相対パス一覧。
	Video []string
	// Plugins は検出されたプラグインファイルの相対パス一覧。
	Plugins []string
}

var (
	scriptExtensions = map[string]bool{".ks": true, ".tjs": true}
	imageExtensions  = map[string]bool{".tlg": true, ".bmp": true, ".jpg": true, ".jpeg": true, ".png": true}
	audioExtensions  = map[string]bool{".ogg": true, ".wav": true, ".mp3": true}
	videoExtensions  = map[string]bool{".mpg": true, ".mpeg": true, ".wmv": true, ".avi": true}
	pluginExtensions = map[string]bool{".dll": true}
)

var titleRegexp = regexp.MustCompile(`;System\.title\s*=\s*"([^"]+)"`)

// GameDetector はゲーム構成を検出する。
//
// 指定されたゲームディレクトリを解析し、
// 使用されているエンジンの種類やリソースファイルを検出する。
type GameDetector struct {
	gameDir   string
	structure *GameStructure
}

// NewGameDetector はgameDirを対象に初期化する。
//
// gameDirが存在しない場合はErrGameDirNotFound、
// ディレクトリでない場合はErrGameDirNotADirectoryを返す。
func NewGameDetector(gameDir string) (*GameDetector, error) {
	info, err := os.Stat(gameDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrGameDirNotFound, gameDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrGameDirNotADirectory, gameDir)
	}

	return &GameDetector{gameDir: gameDir}, nil
}

// Detect はゲーム構成を検出して返す。
//
// ゲームディレクトリを走査し、エンジン種別の判定と
// 各種リソースファイルの検出を行う。
// ゲームディレクトリが空の場合はErrEmptyGameDirを返す。
func (d *GameDetector) Detect() (GameStructure, error) {
	allFiles, err := d.collectFiles()
	if err != nil {
		return GameStructure{}, err
	}

	if len(allFiles) == 0 {
		return GameStructure{}, fmt.Errorf("%w: %s", ErrEmptyGameDir, d.gameDir)
	}

	scripts := filterByExtensions(allFiles, scriptExtensions)
	images := filterByExtensions(allFiles, imageExtensions)
	audio := filterByExtensions(allFiles, audioExtensions)
	video := filterByExtensions(allFiles, videoExtensions)
	plugins := filterByExtensions(allFiles, pluginExtensions)

	engine := detectEngine(allFiles, scripts)
	title := d.detectTitle()
	scriptEncoding := d.detectScriptEncoding(scripts)

	structure := GameStructure{
		Engine:         engine,
		Title:          title,
		Scripts:        scripts,
		ScriptEncoding: scriptEncoding,
		Images:         images,
		Audio:          audio,
		Video:          video,
		Plugins:        plugins,
	}
	d.structure = &structure

	return structure, nil
}

// GetSummary は検出結果のサマリー文字列を返す。
//
// CLI表示用に検出結果を人間が読みやすい形式で整形する。
// まだDetectが呼ばれていない場合は内部でDetectを実行する。
func (d *GameDetector) GetSummary() (string, error) {
	if d.structure == nil {
		if _, err := d.Detect(); err != nil {
			return "", err
		}
	}

	s := d.structure

	encodingInfo := ""
	if s.ScriptEncoding != "" {
		encodingInfo = fmt.Sprintf(" (detected: %s)", s.ScriptEncoding)
	}

	videoCount := len(s.Video)
	videoSuffix := "s"
	if videoCount == 1 {
		videoSuffix = ""
	}

	lines := []string{
		fmt.Sprintf("Engine: %s", engineDisplayName(s.Engine)),
		fmt.Sprintf("Scripts: %d files%s", len(s.Scripts), encodingInfo),
		fmt.Sprintf("Images: %d files", len(s.Images)),
		fmt.Sprintf("Audio: %d files", len(s.Audio)),
		fmt.Sprintf("Video: %d file%s", videoCount, videoSuffix),
		fmt.Sprintf("Plugins: %d files", len(s.Plugins)),
	}

	return strings.Join(lines, "\n"), nil
}

// collectFiles はゲームディレクトリ内の全ファイルを相対パス（"/"区切り）で収集する。
func (d *GameDetector) collectFiles() ([]string, error) {
	var files []string

	err := filepath.WalkDir(d.gameDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == ".gitkeep" {
			return nil
		}

		rel, err := filepath.Rel(d.gameDir, path)
		if err != nil {
			return fmt.Errorf("相対パスの解決に失敗しました: %w", err)
		}
		files = append(files, filepath.ToSlash(rel))

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ゲームディレクトリの走査に失敗しました: %w", err)
	}

	sort.Strings(files)

	return files, nil
}

func filterByExtensions(files []string, extensions map[string]bool) []string {
	result := make([]string, 0)
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if extensions[ext] {
			result = append(result, f)
		}
	}

	return result
}

func detectEngine(allFiles, scripts []string) EngineType {
	fileNames := make(map[string]bool, len(allFiles))
	for _, f := range allFiles {
		fileNames[strings.ToLower(filepath.Base(f))] = true
	}

	isKirikiri2 := fileNames["data.xp3"] || fileNames["game.exe"]
	if !isKirikiri2 {
		return EngineUnknown
	}

	for _, s := range scripts {
		if strings.HasSuffix(s, ".ks") {
			return EngineKirikiri2KAG3
		}
	}

	return EngineKirikiri2
}

// detectScriptEncoding はスクリプトファイルの文字コードを推定する。
//
// 検出できない場合は空文字列を返す。
func (d *GameDetector) detectScriptEncoding(scripts []string) string {
	if len(scripts) == 0 {
		return ""
	}

	detector := chardet.NewTextDetector()

	for _, scriptPath := range scripts {
		fullPath := filepath.Join(d.gameDir, filepath.FromSlash(scriptPath))

		rawData, err := os.ReadFile(fullPath) //nolint:gosec // ゲームディレクトリ配下の走査済みファイルを読む用途のため妥当
		if err != nil || len(rawData) == 0 {
			continue
		}

		if charset, ok := detectCharset(detector, rawData); ok {
			return charset
		}
	}

	return ""
}

// detectCharset はrawDataの文字コードを推定し、Python版chardetの語彙に
// 正規化して返す。
//
// why not: github.com/saintfish/chardet には専用のASCII判定器がなく、
// 純ASCIIバイト列に対しても単バイト系のフォールバック候補
// （例: "ISO-8859-1"、低信頼度）を返す。一方Python版が使うchardetは
// 全バイトが0x7F以下の場合に専用の高速パスで必ず"ascii"を返す
// （ゲーム構成検出結果に含まれる文字コード名はGetSummaryの表示や
// PR4の再エンコード判定に使われるため、この語彙差はユーザー可視の
// 挙動差になる）。そのため、まずGo側でも同じ判定を先に行い、
// 純ASCIIなら"ascii"を返してchardetの推定より優先する。
func detectCharset(detector *chardet.Detector, rawData []byte) (string, bool) {
	if isASCII(rawData) {
		return "ascii", true
	}

	result, err := detector.DetectBest(rawData)
	if err != nil || result == nil || result.Charset == "" {
		return "", false
	}

	return strings.ToLower(result.Charset), true
}

// isASCII はdataが7ビットASCII（0x00〜0x7F）のみで構成されているかを判定する。
func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return false
		}
	}

	return true
}

// detectTitle はConfig.tjsからゲームタイトルを取得する。
//
// 取得できない場合は空文字列を返す。
func (d *GameDetector) detectTitle() string {
	configPaths := []string{
		filepath.Join(d.gameDir, "system", "Config.tjs"),
		filepath.Join(d.gameDir, "Config.tjs"),
	}

	for _, configPath := range configPaths {
		raw, err := os.ReadFile(configPath) //nolint:gosec // ゲームディレクトリ配下の固定サブパスを読む用途のため妥当
		if err != nil {
			continue
		}

		if title, ok := extractTitle(raw); ok {
			return title
		}
	}

	return ""
}

// extractTitle はUTF-8、次いでcp932(Shift_JIS)としてデコードを試み、
// System.titleの値を抽出する。
//
// why not: Python版はcp932を先に試し、UnicodeDecodeErrorが発生した場合のみ
// utf-8へフォールバックする。しかしcp932の"strict"デコードはコードページの
// 未定義コードポイントでのみ失敗し、その判定にはcp932専用の完全な変換表が
// 要る（golang.org/x/text/encoding/japaneseのデコーダはレンジベースで
// 寛容にデコードしてしまい、未定義コードポイントでもエラーを返さない）。
// 一方、Shift_JIS系の2バイト文字列が有効なUTF-8として解釈できることは
// 実質的にありえないため、まずutf8.Validで判定してUTF-8を優先させれば、
// 「cp932で失敗したらutf-8を使う」というPython版の実効的な結果
// （cp932エンコードのファイルはcp932として、utf-8エンコードのファイルは
// utf-8として読める）と同じ着地点になる。
func extractTitle(raw []byte) (string, bool) {
	if utf8.Valid(raw) {
		if m := titleRegexp.FindStringSubmatch(string(raw)); m != nil {
			return m[1], true
		}
	}

	if decoded, err := japanese.ShiftJIS.NewDecoder().String(string(raw)); err == nil {
		if m := titleRegexp.FindStringSubmatch(decoded); m != nil {
			return m[1], true
		}
	}

	return "", false
}

func engineDisplayName(engine EngineType) string {
	switch engine {
	case EngineKirikiri2:
		return "Kirikiri 2"
	case EngineKirikiri2KAG3:
		return "Kirikiri 2 (KAG3)"
	case EngineUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}
