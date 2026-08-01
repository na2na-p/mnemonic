package builder

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrAssetPlacement はアセット配置に関する基本エラー。
var ErrAssetPlacement = errors.New("アセットの配置に失敗しました")

// AssetConfig はアセット配置の設定を表す不変値。
type AssetConfig struct {
	NoCompressExtensions []string
	ExcludePatterns      []string
}

// AssetPlacementResult はアセット配置結果を表す不変値。
type AssetPlacementResult struct {
	TotalFiles  int
	TotalSize   int64
	PlacedFiles []string
}

// AssetPlacer は変換済みアセットをAndroidプロジェクトに配置する。
type AssetPlacer struct {
	projectPath     string
	excludePatterns []string
}

// NewAssetPlacer はAssetPlacerを初期化する。
func NewAssetPlacer(projectPath string, excludePatterns []string) *AssetPlacer {
	return &AssetPlacer{projectPath: projectPath, excludePatterns: excludePatterns}
}

func (p *AssetPlacer) assetsDir() string {
	return filepath.Join(p.projectPath, "app", "src", "main", "assets")
}

// matchesAnyPattern はnameがpatternsのいずれかにfnmatch相当で一致するかを判定する。
//
// why not: この関数はfilepath.Matchでbasename（ディレクトリ区切りを含まない
// ファイル名）のみを照合する用途に限定される。filepath.Matchは"*"がパス
// 区切りを越えてマッチしないglob実装だが、ここではbasenameのみを渡すため
// パス区切りは登場せず、ドット付き拡張子・固定ファイル名という単純な
// パターンの照合には十分である（internal/parser/fnmatch.goのような完全な
// fnmatch.translate相当の再実装は、この用途では過剰なため採用しない）。
func matchesAnyPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, err := filepath.Match(pattern, name); err == nil && ok {
			return true
		}
	}

	return false
}

// PlaceAssets はsourceDir配下のファイルをAndroidプロジェクトのassetsディレクトリへ配置する。
// excludePatternsがnilの場合はインスタンスに設定された除外パターンを使用する。
func (p *AssetPlacer) PlaceAssets(sourceDir string, excludePatterns []string) (AssetPlacementResult, error) {
	if _, err := os.Stat(sourceDir); err != nil {
		return AssetPlacementResult{}, fmt.Errorf("%w: source directory does not exist: %s", ErrAssetPlacement, sourceDir)
	}

	if _, err := os.Stat(p.projectPath); err != nil {
		return AssetPlacementResult{}, fmt.Errorf("%w: project path does not exist: %s", ErrAssetPlacement, p.projectPath)
	}

	assetsDir := p.assetsDir()
	if _, err := os.Stat(assetsDir); err != nil {
		return AssetPlacementResult{}, fmt.Errorf("%w: assets directory does not exist: %s", ErrAssetPlacement, assetsDir)
	}

	patterns := p.excludePatterns
	if excludePatterns != nil {
		patterns = excludePatterns
	}

	var (
		placedFiles []string
		totalSize   int64
	)

	walkErr := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if matchesAnyPattern(d.Name(), patterns) {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		destFile := filepath.Join(assetsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destFile), 0o750); err != nil {
			return err
		}
		if err := copyFile(path, destFile); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		placedFiles = append(placedFiles, relPath)
		totalSize += info.Size()

		return nil
	})
	if walkErr != nil {
		return AssetPlacementResult{}, fmt.Errorf("%w: %w", ErrAssetPlacement, walkErr)
	}

	return AssetPlacementResult{
		TotalFiles:  len(placedFiles),
		TotalSize:   totalSize,
		PlacedFiles: placedFiles,
	}, nil
}

// findBuildGradle はapp/build.gradleまたはapp/build.gradle.ktsのパスを探す。
// 存在しない場合はok=falseを返す。
func (p *AssetPlacer) findBuildGradle() (string, bool) {
	gradlePath := filepath.Join(p.projectPath, "app", "build.gradle")
	if _, err := os.Stat(gradlePath); err == nil {
		return gradlePath, true
	}

	gradleKtsPath := filepath.Join(p.projectPath, "app", "build.gradle.kts")
	if _, err := os.Stat(gradleKtsPath); err == nil {
		return gradleKtsPath, true
	}

	return "", false
}

var (
	aaptOptionsPattern    = regexp.MustCompile(`aaptOptions\s*\{`)
	noCompressLinePattern = regexp.MustCompile(`(?m)(\s*)noCompress.*`)
	androidBlockPattern   = regexp.MustCompile(`(?sm)(android\s*\{)(.*?)(^\})`)
)

// ConfigureBuildGradle はbuild.gradleにアセット設定（noCompress拡張子）を追加する。
func (p *AssetPlacer) ConfigureBuildGradle(assetConfig AssetConfig) error {
	gradlePath, ok := p.findBuildGradle()
	if !ok {
		return fmt.Errorf("%w: build.gradle or build.gradle.kts not found in: %s", ErrAssetPlacement, filepath.Join(p.projectPath, "app"))
	}

	content, err := os.ReadFile(gradlePath) //nolint:gosec // projectPath配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAssetPlacement, err)
	}

	text := string(content)
	isKotlin := filepath.Ext(gradlePath) == ".kts"

	if len(assetConfig.NoCompressExtensions) > 0 {
		text = addNoCompressConfig(text, assetConfig.NoCompressExtensions, isKotlin)
	}

	if err := os.WriteFile(gradlePath, []byte(text), 0o600); err != nil { //nolint:gosec // projectPath配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrAssetPlacement, err)
	}

	return nil
}

// buildNoCompressLine はnoCompress設定の1行を構築する。
func buildNoCompressLine(extensions []string, isKotlin bool) string {
	if isKotlin {
		quoted := ""
		for i, ext := range extensions {
			if i > 0 {
				quoted += ", "
			}
			quoted += fmt.Sprintf(`"%s"`, ext)
		}

		return fmt.Sprintf("        noCompress += listOf(%s)", quoted)
	}

	quoted := ""
	for i, ext := range extensions {
		if i > 0 {
			quoted += ", "
		}
		quoted += fmt.Sprintf(`'%s'`, ext)
	}

	return fmt.Sprintf("        noCompress %s", quoted)
}

func addNoCompressConfig(content string, extensions []string, isKotlin bool) string {
	if aaptOptionsPattern.MatchString(content) {
		return updateExistingAaptOptions(content, extensions, isKotlin)
	}

	return addNewAaptOptions(content, extensions, isKotlin)
}

func updateExistingAaptOptions(content string, extensions []string, isKotlin bool) string {
	noCompressLine := buildNoCompressLine(extensions, isKotlin)

	if noCompressLinePattern.MatchString(content) {
		// why not: noCompressLinePattern の group1（先頭の空白。直前行との改行を含む）を
		// 捨てて丸ごと置換すると、noCompressが既存aaptOptionsの2行目以降にある場合
		// （例: cruncherEnabled falseの次の行）に直前行との改行が失われ、
		// 1行に連結されてGradleの構文エラーになる。group1を保持しつつ
		// noCompress部分のみを置換することで、直前行の改行を失わない。
		return noCompressLinePattern.ReplaceAllStringFunc(content, func(match string) string {
			groups := noCompressLinePattern.FindStringSubmatch(match)
			if groups == nil {
				return match
			}

			return groups[1] + strings.TrimSpace(noCompressLine)
		})
	}

	return aaptOptionsPattern.ReplaceAllStringFunc(content, func(match string) string {
		return match + "\n" + noCompressLine
	})
}

// addNewAaptOptions はandroidブロックの末尾にaaptOptionsブロックを新規追加する。
//
// why not: ReplaceAllStringFuncでマッチした範囲だけをその場で置換する設計に
// より、android{}ブロックの前後にある他のトップレベルブロック
// （dependencies{}等）やコメントを保持する。マッチしたandroid{...}ブロックの
// 3グループのみでcontent全体を再構築して置き換える実装は、androidブロックの
// 前後に他のトップレベルブロックやコメントがある実際のbuild.gradleでは
// マッチ範囲外のテキストが丸ごと消えるデータロスにつながるため採用しない。
func addNewAaptOptions(content string, extensions []string, isKotlin bool) string {
	noCompressLine := buildNoCompressLine(extensions, isKotlin)
	aaptBlock := "    aaptOptions {\n" + noCompressLine + "\n    }"

	return androidBlockPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := androidBlockPattern.FindStringSubmatch(match)
		if groups == nil {
			return match
		}

		return groups[1] + groups[2] + aaptBlock + "\n" + groups[3]
	})
}

// ValidatePlacement はアセット配置が正しく行われたかを検証する。
func (p *AssetPlacer) ValidatePlacement() (bool, error) {
	assetsDir := p.assetsDir()
	if _, err := os.Stat(assetsDir); err != nil {
		return false, nil
	}

	hasFiles := false
	walkErr := filepath.WalkDir(assetsDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			hasFiles = true
		}

		return nil
	})
	if walkErr != nil {
		return false, fmt.Errorf("%w: %w", ErrAssetPlacement, walkErr)
	}

	return hasFiles, nil
}
