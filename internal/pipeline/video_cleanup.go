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
// 複製した変換前拡張子の生ファイルを削除する。出力先が重複し同名の別素材を
// 優先してスキップされた動画(DestPathに優先した素材の出力先を持つStatusSkipped)
// についても、同じく複製された生ファイルを削除する。
//
// why: スキップされた動画の生ファイルも、変換に成功した動画の旧拡張子ファイルと
// 同じく参照されない死蔵アセットになる。adjustScriptsは引用符で囲まれた文字列
// リテラル内の動画参照を.mpgへ書き換えるため、そうした元の拡張子での参照は優先した
// 素材の出力先へ向く。引用符の無い参照(例: storage=op.wmv)は書き換えられず、
// 変換に成功した動画の旧拡張子ファイルを削除する場合と同じく、削除後は実体の無い
// ファイルを指したままになる。
//
// why: VideoConverter.GetOutputExtensionは常に".mpg"を返すため、.wmv/.avi/
// .mpeg入力はConversionManagerによって新しい拡張子のファイルとして書き出され、
// copyTreeが複製した旧拡張子ファイルはconvertDir内に残ったままになる。
// adjustScriptsは(--skip-videoでない限り)引用符で囲まれた文字列リテラル内の
// 動画参照を.mpgへ書き換えるため、旧ファイルを残しても書き換えた参照は
// 解決できる（書き換えた参照は変換後ファイルを指すため、削除の成否はその解決可否に
// 影響しない。引用符の無い参照は書き換えの対象外であり、この前提に含まれない）。削除失敗を
// エラーにしないのは、
// convertMidiFileListWith内のos.Remove(result.SourcePath)と同じ理由: 変換自体は成功し
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
		if !replacedByDest(result) {
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

// replacedByDest はresultの変換元に代わってDestPathの実体がAPKへ入るかどうかを返す。
//
// why not: StatusSkippedを一律に対象とはしない。削除するパスは変換元パスではなく
// DestPathの拡張子を変換元の拡張子へ置き換えたものであり、DestPathが空だと
// カレントディレクトリ相対の".wmv"のようなパスを削除してしまう。ConvertDirectoryは
// Converterの無いファイルをタスクにしないため、DestPathが空のスキップ結果は
// 現状生じない。これを作る経路が将来増えても無関係なファイルを消さないよう除外する。
// VideoConverter.ConvertはStatusSkippedを返さないため、DestPathを持つ動画の
// スキップは出力先の重複で別素材を優先した場合に限られる。
func replacedByDest(result converter.ConversionResult) bool {
	switch result.Status {
	case converter.StatusSuccess:
		return true
	case converter.StatusSkipped:
		return result.DestPath != ""
	default:
		return false
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
