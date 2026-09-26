package pipeline

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// MIDI変換に関するセンチネルエラー群。
var (
	// ErrMidiConversionUnavailable はMIDIアセットを含むゲームに対し、変換に
	// 必要なFluidSynthまたはサウンドフォントが利用できない場合のエラー。
	ErrMidiConversionUnavailable = errors.New("MIDI変換に必要な環境が利用できません")
	// ErrMidiConversionFailed は1つ以上のMIDIファイルの変換に失敗した場合のエラー。
	ErrMidiConversionFailed = errors.New("MIDIファイルの変換に失敗しました")
)

// midiRequirementGuide はMIDI変換の前提条件を満たせなかった場合に、
// 利用者が復旧するための手順を示す案内文を組み立てる。
//
// why not: macOSの案内をHomebrewのインストールコマンドだけで終わらせない。
// `brew install fluid-synth`はfluidsynthコマンドを提供するがサウンドフォントは
// 同梱せず、既定の探索先はいずれもLinuxの絶対パスであるため、コマンドだけを
// 入れた利用者は同じエラーに再突入して手詰まりになる。サウンドフォントの
// 別途入手と--soundfontでの指定まで案内する必要がある。
//
// why not: 定数ではなく関数にしているのは、既定の探索先パスを文面へ直書き
// すると converter 側の定義とずれても誰も気付けないため。SSOTである
// converterのパッケージ変数から都度組み立てる。
func midiRequirementGuide() string {
	return "ゲーム内にMIDIアセット(.mid/.midi)が含まれています。" +
		"krkrsdl2はMIDIを再生できず、スクリプト内の参照は必ず.oggへ書き換えられるため、" +
		"MIDI変換にはFluidSynthとサウンドフォントの両方が必須です。" +
		"インストール例: Debian/Ubuntu系は `apt-get install fluidsynth fluid-soundfont-gm`。" +
		"macOSは `brew install fluid-synth` でコマンドを導入したうえで、" +
		"サウンドフォント(.sf2/.sf3、例: FluidR3_GM)を別途入手し、" +
		"`--soundfont <パス>` で指定してください（既定の探索先である " +
		converter.MuseScoreSoundfontPath + " または " + converter.FluidR3SoundfontPath +
		" に配置しても構いません）。" +
		"なお--skip-videoのようなスキップ指定はMIDIには適用できません" +
		"（変換を省略するとBGMが一切鳴らないAPKが出来上がるため）。"
}

// newMidiConverter は設定値からMIDI変換器を構築する。
// Config.SoundfontPathが空文字列の場合、NewMidiConverterが
// converter.GetDefaultSoundfontPathによる既定の解決を行う。
func (b *BuildPipeline) newMidiConverter() *converter.MidiConverter {
	timeout := time.Duration(b.config.FFmpegTimeoutSeconds) * time.Second

	return converter.NewMidiConverter(b.config.SoundfontPath, 0, "", 0, timeout, nil)
}

// convertMidiFilesUsing はdirectory配下のMIDIファイルをOGG Vorbis形式に変換する。
//
// krkrsdl2はMIDI再生未対応のため、MIDIファイルをOGG Vorbisに変換する。
// スクリプト書き換え（ScriptAdjuster）で参照が.oggに変更されるため、
// 出力ファイル名は.mid/.midiを.oggに置換した形式にする
// （例: bgm/sinone.mid → bgm/sinone.ogg）。変換成功後、元のMIDIファイルは
// 削除する。
func convertMidiFilesUsing(directory string, midiConverter *converter.MidiConverter, logger Logger) error {
	midiFiles, err := findMidiFiles(directory)
	if err != nil {
		return err
	}

	// why not: 可用性の検査をMIDIの実在確認より先に行うと、MIDIを持たない
	// ゲームのビルドまでFluidSynthのインストールを強制することになる
	// （internal/doctorがFluidSynthをRequired=falseとしているのと同じ理由）。
	// 検査は必ずMIDIが実在する場合に限る。
	if len(midiFiles) == 0 {
		return nil
	}

	if err := ensureMidiConversionAvailable(midiConverter); err != nil {
		return err
	}

	return convertMidiFileList(midiFiles, midiConverter, logger)
}

// findMidiFiles はdirectory配下の.mid/.midiファイルを再帰的に列挙する。
func findMidiFiles(directory string) ([]string, error) {
	var midiFiles []string

	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".mid" || ext == ".midi" {
			midiFiles = append(midiFiles, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("MIDIファイルの走査に失敗しました: %w", err)
	}

	return midiFiles, nil
}

// ensureMidiConversionAvailable はMIDI変換の前提条件（FluidSynthの実行可能性と
// サウンドフォントの実在）を検査する。
//
// why not: 前提条件を満たさない場合に変換自体を黙ってスキップする方法もある。
// しかしスクリプトの.mid→.ogg書き換えは(MIDI変換の成否によらず)無条件に走る
// ため、変換をスキップすると存在しない.oggを指す参照が残りBGMが無音のAPKが
// 完成してしまう（実機で確認済み）。前提条件の欠如は「やることが無い」
// ではなくビルドの失敗として扱う。
//
// why not: サウンドフォントの実在確認をここで行うのは、converter.
// GetDefaultSoundfontPathがFluidR3のパスへ実在確認なしにフォールバックし、
// MidiConverter.Convertが不在をファイル単位の失敗としてしか報告
// しないため。全ファイルを試して初めて原因が判明するより、着手前に一度だけ
// 検査して単一のエラーへまとめる方が原因を特定しやすい。
func ensureMidiConversionAvailable(midiConverter *converter.MidiConverter) error {
	if !midiConverter.IsFluidsynthAvailable() {
		return fmt.Errorf(
			"%w: fluidsynthコマンドを実行できません。%s",
			ErrMidiConversionUnavailable, midiRequirementGuide(),
		)
	}

	soundfontPath := midiConverter.SoundfontPath()
	if _, err := os.Stat(soundfontPath); err != nil {
		return fmt.Errorf(
			"%w: サウンドフォントが見つかりません: %s。%s",
			ErrMidiConversionUnavailable, soundfontPath, midiRequirementGuide(),
		)
	}

	return nil
}

// maxMidiWorkers はMIDI変換で同時に走らせるワーカー数の上限。
//
// why not: CPU数だけで並列化しない。fluidsynthはワーカーごとにサウンド
// フォント全体を常駐させるため、ワーカー数 × サウンドフォント分のメモリを
// 使う。converter.CalculateWorkers(nil)はメモリを見ないので、
// CPU数どおりに並列化すると小型機でメモリが枯渇する。2は並列の
// 利点を残しつつ常駐メモリをサウンドフォント2つ分に抑える上限。
const maxMidiWorkers = 2

// why not: 下限の1を省かない。0以下をそのままNewConversionManagerへ渡すと
// 「自動計算」と解釈されCPU数どおりのワーカー数に戻ってしまうため。
func midiWorkerCount(cpuCount int) int {
	return max(1, min(cpuCount, maxMidiWorkers))
}

// convertMidiFileList はmidiFilesをOGGへ変換し、失敗を集約して返す。
//
// why not: 最初の失敗で打ち切らず全ファイルを試すのは、利用者が一度の実行で
// 失敗した全ファイルを把握できるようにするため。ただし1件でも失敗した場合は
// エラーを返し、変換されなかったMIDIを指す.ogg参照がAPKへ混入するのを防ぐ。
func convertMidiFileList(midiFiles []string, midiConverter *converter.MidiConverter, logger Logger) error {
	return convertMidiFileListWith(midiFiles, midiConverter, nil, logger)
}

// convertMidiFileListWith はsleepがnilでなければConversionManagerのリトライ
// 待機に使う。
//
// why not: 待機関数をパッケージ変数で差し替えられるようにしない。t.Parallel()
// で並行に走るテストが同じ変数を書き換えるとデータ競合になるため、呼び出し
// ごとの引数として受け取る。
func convertMidiFileListWith(
	midiFiles []string,
	midiConverter *converter.MidiConverter,
	sleep func(time.Duration),
	logger Logger,
) error {
	tasks := make([]converter.FileTask, 0, len(midiFiles))
	for _, midiFile := range midiFiles {
		tasks = append(tasks, converter.FileTask{Source: midiFile, Dest: withSuffix(midiFile, ".ogg")})
	}

	manager := converter.NewConversionManager(
		[]converter.Converter{midiConverter}, nil, midiWorkerCount(converter.CalculateWorkers(nil)), nil,
	)
	if sleep != nil {
		manager.SleepFunc = sleep
	}

	results := manager.ConvertFiles(tasks).Results
	// why not: ConvertFilesの結果は並列ワーカーの完了順に並び実行ごとに
	// 変わるため、そのまま報告すると同じ失敗でもエラー文の並びが揺れる。
	// 変換元パス順に並べ替えて報告を決定的にする。
	slices.SortFunc(results, func(a, b converter.ConversionResult) int {
		return cmp.Compare(a.SourcePath, b.SourcePath)
	})

	var failures []string

	for _, result := range results {
		if result.Status != converter.StatusSuccess {
			failures = append(failures, fmt.Sprintf("%s: %s", result.SourcePath, result.Message))

			continue
		}

		// why not: 削除失敗はビルドエラーに昇格させず警告に留める。変換自体は
		// 成功しており.oggの実体が揃っているため、スクリプトの.ogg参照は解決でき
		// 無音にならない（存在しないファイルを指す参照が残る不具合のクラスには
		// 該当しない）。残留した.midは再生されない死蔵アセットとしてAPKへ同梱
		// されるだけ（サイズ増のみ）であり、これでビルド全体を落とす方が
		// 損害が大きい。
		if err := os.Remove(result.SourcePath); err != nil {
			logger.Warning(fmt.Sprintf("変換済みMIDIファイルを削除できませんでした（APKに残ります）: %v", err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%w: %s", ErrMidiConversionFailed, strings.Join(failures, " / "))
	}

	return nil
}
