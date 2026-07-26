package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrGameDirNotFoundForAssets はAssetScannerの対象ゲームディレクトリが
// 存在しない場合のセンチネルエラー。
var ErrGameDirNotFoundForAssets = errors.New("ゲームディレクトリが見つかりません")

// AssetType はアセットの種別を表す。
//
// ゲームアセットの種類を分類する。スクリプト、画像、音声、動画、その他に分類される。
type AssetType string

// AssetTypeの各値。
const (
	AssetScript AssetType = "script"
	AssetImage  AssetType = "image"
	AssetAudio  AssetType = "audio"
	AssetVideo  AssetType = "video"
	AssetOther  AssetType = "other"
)

// allAssetTypes はAssetManifest.GetSummaryでの列挙順を定義する
// （Pythonの `for asset_type in AssetType` に相当する宣言順）。
var allAssetTypes = []AssetType{AssetScript, AssetImage, AssetAudio, AssetVideo, AssetOther}

// ConversionAction は変換アクションを表す。
//
// アセットファイルに対して実行する変換処理の種類を定義する。
type ConversionAction string

// ConversionActionの各値。
const (
	ConvertEncodeUTF8 ConversionAction = "encode_utf8"
	ConvertWebP       ConversionAction = "convert_webp"
	ConvertOgg        ConversionAction = "convert_ogg"
	ConvertMP4        ConversionAction = "convert_mp4"
	ConvertCopy       ConversionAction = "copy"
	ConvertSkip       ConversionAction = "skip"
)

type extensionRule struct {
	assetType AssetType
	action    ConversionAction
	target    string // 空文字列は変換なし（Pythonの None に相当）
}

// extensionRules は拡張子ごとの種別・変換アクション・変換後フォーマットの対応表。
var extensionRules = map[string]extensionRule{
	".ks":   {AssetScript, ConvertEncodeUTF8, ""},
	".tjs":  {AssetScript, ConvertEncodeUTF8, ""},
	".tlg":  {AssetImage, ConvertWebP, ".webp"},
	".bmp":  {AssetImage, ConvertWebP, ".webp"},
	".jpg":  {AssetImage, ConvertWebP, ".webp"},
	".jpeg": {AssetImage, ConvertWebP, ".webp"},
	".png":  {AssetImage, ConvertWebP, ".webp"},
	".wav":  {AssetAudio, ConvertOgg, ".ogg"},
	".ogg":  {AssetAudio, ConvertCopy, ""},
	".mp3":  {AssetAudio, ConvertCopy, ""},
	".mpg":  {AssetVideo, ConvertMP4, ".mp4"},
	".mpeg": {AssetVideo, ConvertMP4, ".mp4"},
	".wmv":  {AssetVideo, ConvertMP4, ".mp4"},
	".avi":  {AssetVideo, ConvertMP4, ".mp4"},
}

// converterNameToAction はconversion_rules設定のconverter名からConversionActionへの対応表。
var converterNameToAction = map[string]ConversionAction{
	"encode_utf8":  ConvertEncodeUTF8,
	"convert_webp": ConvertWebP,
	"convert_ogg":  ConvertOgg,
	"convert_mp4":  ConvertMP4,
	"copy":         ConvertCopy,
	"skip":         ConvertSkip,
}

// AssetFile はアセットファイル情報を表す。
//
// 単一のアセットファイルに関するメタデータを保持する不変値。
type AssetFile struct {
	// Path はアセットファイルのパス（ゲームディレクトリからの相対パス）。
	Path string
	// AssetType はアセットの種別。
	AssetType AssetType
	// Action は実行する変換アクション。
	Action ConversionAction
	// SourceFormat は元ファイルのフォーマット（拡張子）。
	SourceFormat string
	// TargetFormat は変換後のフォーマット（変換しない場合は空文字列）。
	TargetFormat string
}

// AssetManifest はアセット一覧（マニフェスト）を表す。
//
// ゲームディレクトリ内の全アセットファイル情報を管理する。
type AssetManifest struct {
	// GameDir はゲームディレクトリのパス。
	GameDir string
	// Files はアセットファイル情報のリスト。
	Files []AssetFile
}

// FilterByType は指定種別のファイルのみ取得する。
func (m AssetManifest) FilterByType(assetType AssetType) []AssetFile {
	result := make([]AssetFile, 0)
	for _, f := range m.Files {
		if f.AssetType == assetType {
			result = append(result, f)
		}
	}

	return result
}

// FilterByAction は指定アクションのファイルのみ取得する。
func (m AssetManifest) FilterByAction(action ConversionAction) []AssetFile {
	result := make([]AssetFile, 0)
	for _, f := range m.Files {
		if f.Action == action {
			result = append(result, f)
		}
	}

	return result
}

// GetSummary は種別ごとのファイル数を取得する。
//
// ファイル数が0の種別はPython版と同様にキーを含めない。
func (m AssetManifest) GetSummary() map[AssetType]int {
	summary := make(map[AssetType]int)
	for _, assetType := range allAssetTypes {
		count := 0
		for _, f := range m.Files {
			if f.AssetType == assetType {
				count++
			}
		}
		if count > 0 {
			summary[assetType] = count
		}
	}

	return summary
}

// ConversionRule はAssetScannerの設定に含まれる変換ルールオーバーライド1件を表す。
type ConversionRule struct {
	Pattern   string
	Converter string
}

// ScannerConfig はAssetScannerのスキャン設定を表す。
type ScannerConfig struct {
	// Exclude は除外するファイルパターン（fnmatch形式）のリスト。
	Exclude []string
	// ConversionRules は変換ルールのオーバーライド設定のリスト。
	ConversionRules []ConversionRule
}

// AssetScanner はアセットをスキャンしてマニフェストを生成する。
//
// ゲームディレクトリ内のアセットファイルを走査し、
// 各ファイルの種別と変換アクションを判定してマニフェストを生成する。
type AssetScanner struct {
	gameDir string
	config  ScannerConfig
}

// NewAssetScanner はgameDirとconfigを指定して初期化する。
//
// gameDirが存在しない場合はErrGameDirNotFoundForAssetsを返す。
// configがnilの場合はゼロ値のScannerConfig（除外・上書きなし）として扱う。
func NewAssetScanner(gameDir string, config *ScannerConfig) (*AssetScanner, error) {
	if _, err := os.Stat(gameDir); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrGameDirNotFoundForAssets, gameDir)
	}

	cfg := ScannerConfig{}
	if config != nil {
		cfg = *config
	}

	return &AssetScanner{gameDir: gameDir, config: cfg}, nil
}

// Scan はアセットをスキャンしてマニフェストを返す。
//
// ゲームディレクトリ内のすべてのアセットファイルを走査し、
// それぞれに対して種別と変換アクションを判定する。
func (s *AssetScanner) Scan() (AssetManifest, error) {
	files := make([]AssetFile, 0)

	err := filepath.WalkDir(s.gameDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// 隠しファイルは除外する。
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		relPath, err := filepath.Rel(s.gameDir, path)
		if err != nil {
			return fmt.Errorf("相対パスの解決に失敗しました: %w", err)
		}
		relSlash := filepath.ToSlash(relPath)

		if s.shouldExclude(relSlash) {
			return nil
		}

		files = append(files, s.classifyFile(relSlash))

		return nil
	})
	if err != nil {
		return AssetManifest{}, fmt.Errorf("ゲームディレクトリの走査に失敗しました: %w", err)
	}

	return AssetManifest{GameDir: s.gameDir, Files: files}, nil
}

func (s *AssetScanner) shouldExclude(relSlash string) bool {
	base := pathBase(relSlash)
	for _, pattern := range s.config.Exclude {
		if matchGlob(relSlash, pattern) || matchGlob(base, pattern) {
			return true
		}
	}

	return false
}

func (s *AssetScanner) conversionRuleOverride(relSlash string) (ConversionAction, bool) {
	for _, rule := range s.config.ConversionRules {
		if matchGlob(relSlash, rule.Pattern) {
			action, ok := converterNameToAction[rule.Converter]
			if !ok {
				continue
			}

			return action, true
		}
	}

	return "", false
}

func (s *AssetScanner) classifyFile(relSlash string) AssetFile {
	ext := strings.ToLower(filepath.Ext(relSlash))

	rule, ok := extensionRules[ext]
	assetType := AssetOther
	action := ConvertCopy
	target := ""
	if ok {
		assetType = rule.assetType
		action = rule.action
		target = rule.target
	}

	if overrideAction, found := s.conversionRuleOverride(relSlash); found {
		action = overrideAction
		if action == ConvertSkip {
			target = ""
		}
	}

	return AssetFile{
		Path:         relSlash,
		AssetType:    assetType,
		Action:       action,
		SourceFormat: ext,
		TargetFormat: target,
	}
}

// pathBase はrelSlash（"/"区切り）の最後の要素を返す。
//
// why not: relSlashはfilepath.ToSlashで正規化済みのため、OS依存のfilepath.Base
// ではなく単純な文字列分割で十分であり、Windows上でも一貫した結果になる。
func pathBase(relSlash string) string {
	idx := strings.LastIndex(relSlash, "/")
	if idx == -1 {
		return relSlash
	}

	return relSlash[idx+1:]
}
