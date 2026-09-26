package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// removeStaleVideoSourceFiles はVideoConverterが拡張子を変更して変換に成功した
// ファイルについて、copyTree(実行済みのexecuteConvert冒頭)がconvertDirへ
// 複製した変換前拡張子の生ファイルを削除する。
//
// why: VideoConverter.GetOutputExtensionは常に".mpg"を返すため、.wmv/.avi/
// .mpeg入力はConversionManagerによって新しい拡張子のファイルとして書き出され、
// copyTreeが複製した旧拡張子ファイルはconvertDir内に残ったままになる。
// adjustScriptsは(--skip-videoでない限り)動画参照を.mpgへ書き換えるため、
// 旧ファイルを残しても新しい参照は解決できる（変換後ファイルが既に存在する
// ため、削除の成否がスクリプト参照の解決可否には影響しない）。削除失敗を
// エラーにしないのは、
// convertMidiFileListのos.Remove(midiFile)と同じ理由: 変換自体は成功し
// スクリプト参照も解決できるため、残留は死蔵アセットとしてAPKサイズが
// 増えるだけで実害が無い。
//
// why not: --skip-video時はVideoConverterがconvertersに登録されないため
// (phases.go executeConvert参照)、summary.Resultsに動画由来の結果が
// 一切含まれず、このループは自然に何もしない。adjustScripts側も
// SkipVideo時はDefaultRulesWithoutVideoExtensionsを使い動画参照を書き換え
// ないため、「参照は.mpgのまま、ファイルは元の拡張子のまま」という整合が
// 保たれる（TestBuildPipeline_AdjustScripts_SkipVideo参照）。
//
// why not: 「削除してよいか」の判定に拡張子の文字列比較(EqualFold等)は
// 使わない。例えば入力が".MPG"(大文字綴り)の場合、GetOutputExtensionは
// 常に小文字".mpg"を返すため拡張子は文字列としては食い違う
// (EqualFold(".MPG",".mpg")はtrueだが==はfalse)。ここで単純な==へ緩めると
// ケースを区別しないファイルシステム(macOS既定)ではdestと同一実体を削除
// してしまう。逆にEqualFoldのまま「文字列が(大文字小文字を無視して)一致
// するなら同一実体とみなしてスキップ」という判定だけに頼ると、ケースを
// 区別するファイルシステム(Android向けビルドを行うLinux CI/DevContainer)
// では"OP.MPG"(copyTree由来の未変換の生ファイル)と"OP.mpg"(変換済み)が
// 実際には別ファイルであるにもかかわらず同一実体と誤認し、生ファイルを
// 消し損なう。パス文字列ではなくos.SameFileで実体そのものを比較すること
// で、ファイルシステムのケース区別有無によらず「本当に同じファイルか」を
// 判定する。
func removeStaleVideoSourceFiles(summary converter.ConversionSummary) {
	videoExts := converter.NewVideoConverter(0, nil).SupportedExtensions()

	for _, result := range summary.Results {
		if result.Status != converter.StatusSuccess {
			continue
		}

		sourceExt := filepath.Ext(result.SourcePath)
		if !slices.Contains(videoExts, strings.ToLower(sourceExt)) {
			continue
		}

		stalePath := withSuffix(result.DestPath, sourceExt)
		if sameUnderlyingFile(stalePath, result.DestPath) {
			continue
		}

		_ = os.Remove(stalePath)
	}
}

// sameUnderlyingFile はaとbが同一の実体ファイルを指すかどうかを返す。
// いずれかのos.Statが失敗する場合(片方が既に存在しない等)はfalseを返す。
func sameUnderlyingFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	if errA != nil {
		return false
	}

	infoB, errB := os.Stat(b)
	if errB != nil {
		return false
	}

	return os.SameFile(infoA, infoB)
}
