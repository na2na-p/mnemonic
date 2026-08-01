package converter

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/webp"
	"golang.org/x/image/bmp"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

// ErrTLGDecodeNotImplemented はTLG画像のデコードが未実装であることを示す
// センチネルエラー。
//
// why not: TLG6の本体デコード実装はスコープ外であり、ヘッダのマジックバイト
// 判定とヘッダ解析のみ実装する。TLG5は実装済みのため、このエラーはTLG6形式の
// ファイルに対してのみ返る。
var ErrTLGDecodeNotImplemented = errors.New("TLGデコードは未実装です")

// ErrUnsupportedImageFormat はstdlib/x-imageで対応していない画像拡張子を
// 指定した場合のエラー。
var ErrUnsupportedImageFormat = errors.New("サポートされていない画像形式です")

// ErrTLGInvalidFormat はTLG5/TLG6/SDSのいずれのマジックバイトにも一致しない
// データに対するエラー。
var ErrTLGInvalidFormat = errors.New("TLG形式ではありません")

var (
	tlg5Magic = tlg.TLG5Magic
	tlg6Magic = tlg.TLG6Magic
	sdsMagic  = []byte("TLG0.0\x00sds\x1a")
)

// TLGVersion はTLG画像のバージョンを表す。
type TLGVersion string

// TLGVersionの各値。
const (
	TLGVersionTLG5    TLGVersion = "TLG5"
	TLGVersionTLG6    TLGVersion = "TLG6"
	TLGVersionUnknown TLGVersion = "UNKNOWN"
)

// QualityPreset はWebP変換時の品質プリセット。
type QualityPreset int

// QualityPresetの各値。
const (
	QualityHigh   QualityPreset = 95
	QualityMedium QualityPreset = 85
	QualityLow    QualityPreset = 70
)

// OutputFormat は画像出力形式を表す。
//
// krkrsdl2がWebP未対応のため、PNG出力をデフォルトとする
// （feat/exe-icon-extraction 77634b1の設計意図）。
type OutputFormat string

// OutputFormatの各値。
const (
	OutputFormatWebP OutputFormat = "webp"
	OutputFormatPNG  OutputFormat = "png"
)

// TLGInfo はTLG画像のメタ情報を表す不変値。
type TLGInfo struct {
	Version  TLGVersion
	Width    int
	Height   int
	HasAlpha bool
}

// TLGImageDecoder はTLG形式の画像ファイルを読み込み、image.Imageへ変換する。
// TLG5およびTLG6形式に対応（TLG6は本体デコード未実装）。SDSコンテナ形式も
// サポートする。
type TLGImageDecoder struct {
	tlg5Decoder *tlg.TLG5Decoder
	tlg6Decoder *tlg.TLG6Decoder
}

// NewTLGImageDecoder はTLGImageDecoderを初期化する。
func NewTLGImageDecoder() *TLGImageDecoder {
	return &TLGImageDecoder{
		tlg5Decoder: tlg.NewTLG5Decoder(),
		tlg6Decoder: tlg.NewTLG6Decoder(),
	}
}

// IsTLGFile はfilePathがTLG5/TLG6/SDS形式かどうかをマジックバイトで判定する。
func (d *TLGImageDecoder) IsTLGFile(filePath string) bool {
	f, err := os.Open(filePath) //nolint:gosec // 呼び出し側が指定したアセットパスを読む用途のため妥当
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, len(tlg5Magic))
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}

	return bytes.Equal(header, tlg5Magic) || bytes.Equal(header, tlg6Magic) || bytes.Equal(header, sdsMagic)
}

// unwrapSDS はSDSコンテナから内部のTLGデータを抽出する。
//
// SDSコンテナ構造: マジック(11バイト) + チャンクサイズ(4バイト、リトル
// エンディアン) + 内部TLGデータ(TLG5またはTLG6)。
func unwrapSDS(data []byte) []byte {
	if !bytes.HasPrefix(data, sdsMagic) {
		return data
	}

	const sdsHeaderSize = 15 // マジック(11) + チャンクサイズ(4)
	if len(data) < sdsHeaderSize {
		return data
	}

	return data[sdsHeaderSize:]
}

// detectVersion はTLGデータのバージョンを判別する。
func detectVersion(data []byte) TLGVersion {
	switch {
	case bytes.HasPrefix(data, tlg5Magic):
		return TLGVersionTLG5
	case bytes.HasPrefix(data, tlg6Magic):
		return TLGVersionTLG6
	default:
		return TLGVersionUnknown
	}
}

// readTLGSource はfilePathを読み込み、SDSコンテナを解いた生データを返す。
// ファイルが存在しない場合はErrSourceNotFoundを返す。
func readTLGSource(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // 呼び出し側が指定したアセットパスを読む用途のため妥当
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSourceNotFound, filePath)
	}

	return unwrapSDS(data), nil
}

// GetInfo はTLG画像のメタ情報を取得する。
//
// why not: TLG6のヘッダ解析は実装済みのTLG6Decoder.ParseHeaderで完結するため、
// GetInfoはTLG5/TLG6のいずれでもErrTLGDecodeNotImplementedを返さない
// （decode()本体のみが未実装であるため）。
func (d *TLGImageDecoder) GetInfo(filePath string) (TLGInfo, error) {
	data, err := readTLGSource(filePath)
	if err != nil {
		return TLGInfo{}, err
	}

	switch detectVersion(data) {
	case TLGVersionTLG5:
		header, parseErr := d.tlg5Decoder.ParseHeader(data)
		if parseErr != nil {
			return TLGInfo{}, parseErr
		}

		return TLGInfo{Version: TLGVersionTLG5, Width: header.Width, Height: header.Height, HasAlpha: header.Colors == 4}, nil
	case TLGVersionTLG6:
		header, parseErr := d.tlg6Decoder.ParseHeader(data)
		if parseErr != nil {
			return TLGInfo{}, parseErr
		}

		return TLGInfo{Version: TLGVersionTLG6, Width: header.Width, Height: header.Height, HasAlpha: header.Colors == 4}, nil
	default:
		return TLGInfo{}, fmt.Errorf("%w: %s", ErrTLGInvalidFormat, filePath)
	}
}

// Decode はTLG画像をデコードしてimage.Imageを返す。
// TLG6形式の場合はErrTLGDecodeNotImplementedを返す（本体デコード未実装）。
func (d *TLGImageDecoder) Decode(filePath string) (image.Image, error) {
	data, err := readTLGSource(filePath)
	if err != nil {
		return nil, err
	}

	switch detectVersion(data) {
	case TLGVersionTLG5:
		img, decErr := d.tlg5Decoder.Decode(data)
		if decErr != nil {
			return nil, decErr
		}

		return img, nil
	case TLGVersionTLG6:
		// why not: tlg6Decoder.Decode()はマジックバイトが有効な限り常に
		// tlg.ErrTLG6NotImplemented（"TLG6デコードは未実装です"）を返す。
		// これをErrTLGDecodeNotImplementedへさらに%wでラップすると
		// 「TLGデコードは未実装です: TLG6デコードは未実装です」という
		// 冗長な二重メッセージになるため、下位エラーの文言は引き継がず
		// ErrTLGDecodeNotImplementedのみを返す。呼び出し自体は、将来
		// tlg6Decoder.Decodeが実装された際にここを更新し忘れないための
		// フックとして残す。
		_, _ = d.tlg6Decoder.Decode(data)

		return nil, ErrTLGDecodeNotImplemented
	default:
		return nil, fmt.Errorf("%w: %s", ErrTLGInvalidFormat, filePath)
	}
}

// DecodeToFile はTLG画像をデコードしてファイルに保存する。
// dest拡張子から出力形式を決定する（.png/.webpに対応）。
func (d *TLGImageDecoder) DecodeToFile(source, dest string) error {
	img, err := d.Decode(source)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	return encodeImageToFile(img, dest, int(QualityHigh))
}

// encodeImageToFile はimgをdestの拡張子に応じた形式でファイルへ書き出す。
//
// why not(losslessAlphaとのパリティ差異): この関数はTLGImageDecoder.
// DecodeToFile（ImageConverterを介さない低レベルなデコード専用ユーティリ
// ティ）専用であり、ImageConverterのlosslessAlpha設定を受け取らない。
// losslessAlphaを設定する手段が無いこの関数の性質上、アルファ精度を暗黙に
// 欠落させるよりも安全側に倒し、imageHasAlpha(img)がtrueなら常に
// Lossless=trueにする意図的な選択である。ImageConverter経由の変換
// （Convert/ConvertFromImage）は既にlosslessAlphaを正しく反映するsaveAsWebp
// を使うため、この差異はTLGImageDecoder.DecodeToFileを直接呼ぶ経路にのみ
// 影響する。
func encodeImageToFile(img image.Image, dest string, quality int) error {
	f, err := os.Create(dest) //nolint:gosec // ビルド成果物の出力用途のため妥当
	if err != nil {
		return fmt.Errorf("出力ファイルの作成に失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	ext := strings.ToLower(filepath.Ext(dest))

	switch ext {
	case ".png":
		if err := png.Encode(f, img); err != nil {
			return fmt.Errorf("PNGエンコードに失敗しました: %w", err)
		}
	case ".webp":
		opts := webp.Options{Quality: quality, Method: webp.DefaultMethod}
		if imageHasAlpha(img) {
			opts.Lossless = true
		}

		if err := webp.Encode(f, img, opts); err != nil {
			return fmt.Errorf("WebPエンコードに失敗しました: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, ext)
	}

	return nil
}

// ImageConverter はBMP/JPG/PNG/TLG形式の画像をPNG/WebP形式に変換するConverter。
type ImageConverter struct {
	outputFormat  OutputFormat
	quality       int
	losslessAlpha bool
	tlgDecoder    *TLGImageDecoder
}

// NewImageConverter はImageConverterを初期化する。出力形式はPNG固定
// （krkrsdl2互換のためのデフォルト。WebP出力が必要な場合はNewImageConverter
// WithFormatを使う）。
// qualityはQualityPresetの値（95/85/70）または任意の0-100の整数を指定する。
// qualityが0以下の場合はQualityHighを既定値として使用する。
func NewImageConverter(quality int, losslessAlpha bool) *ImageConverter {
	return NewImageConverterWithFormat(OutputFormatPNG, quality, losslessAlpha)
}

// NewImageConverterWithFormat はoutputFormatを明示的に指定してImageConverter
// を初期化する。
//
// why not(呼び出し元互換): 既存呼び出し元(internal/pipeline)はNewImage
// Converter(quality, losslessAlpha)の2引数シグネチャに依存しているため、
// 出力形式選択はこの別コンストラクタとして追加し、既存シグネチャを変更
// しない。
func NewImageConverterWithFormat(outputFormat OutputFormat, quality int, losslessAlpha bool) *ImageConverter {
	if quality <= 0 {
		quality = int(QualityHigh)
	}

	return &ImageConverter{
		outputFormat:  outputFormat,
		quality:       quality,
		losslessAlpha: losslessAlpha,
		tlgDecoder:    NewTLGImageDecoder(),
	}
}

// OutputFormat は出力形式を返す。
func (c *ImageConverter) OutputFormat() OutputFormat { return c.outputFormat }

// Quality はWebP品質値を返す。
func (c *ImageConverter) Quality() int { return c.quality }

// LosslessAlpha はロスレスアルファ設定を返す。
func (c *ImageConverter) LosslessAlpha() bool { return c.losslessAlpha }

// SupportedExtensions は対応する拡張子の一覧を返す。
//
// JPEG/PNG/BMPはkrkrsdl2でネイティブサポートのため変換対象外
// （feat/exe-icon-extraction 680b27fより。誤ってPNGに変換されたJPEGを
// krkrsdl2のTVPLoadJPEGへ渡すとSIGSEGVでクラッシュする不具合の修正）。
func (c *ImageConverter) SupportedExtensions() []string {
	return []string{".tlg"}
}

// GetOutputExtension は出力形式に応じた拡張子（.pngまたは.webp）を返す。
func (c *ImageConverter) GetOutputExtension(_ string) string {
	if c.outputFormat == OutputFormatPNG {
		return ".png"
	}

	return ".webp"
}

// CanConvert はfilePathが変換可能かを拡張子で判定する。
func (c *ImageConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(c.SupportedExtensions(), ext)
}

// Convert は画像ファイルを指定された形式に変換し、destへ出力する。
//
// why not: 他のConverterと異なり、validateSource・TLG未実装エラー・画像
// デコードの失敗を自身で捕捉せず、呼び出し元(ConversionManager)へerrとして
// 伝播させる。これらの失敗はConversionResultではなくerrとして返す。
//
// why not(decodeSourceの対応拡張子): CanConvert/SupportedExtensionsは.tlg
// のみだが、Convert()自体は.bmp/.jpg/.jpeg/.png/.tlgを直接処理できる
// （ConversionManager経由では.tlg以外はルーティングされないが、
// ConvertFromImage等、呼び出し元がConvertを直接呼ぶ経路のためにdecodeSource
// の分岐は維持する）。
func (c *ImageConverter) Convert(source, dest string) (ConversionResult, error) {
	if err := validateSource(source); err != nil {
		return ConversionResult{}, err
	}

	bytesBefore := getFileSize(source)

	img, err := c.decodeSource(source)
	if err != nil {
		return ConversionResult{}, err
	}

	if c.outputFormat == OutputFormatPNG {
		return c.saveAsPNG(img, dest, source, bytesBefore)
	}

	return c.saveAsWebp(img, dest, source, bytesBefore)
}

// ConvertFromImage はメモリ上のimage.Imageを設定済み出力形式で保存する。
// TLGデコード後の画像変換等、既にデコード済みの画像を直接保存する用途。
func (c *ImageConverter) ConvertFromImage(img image.Image, dest string) (ConversionResult, error) {
	if c.outputFormat == OutputFormatPNG {
		return c.saveAsPNG(img, dest, dest, 0)
	}

	return c.saveAsWebp(img, dest, dest, 0)
}

func (c *ImageConverter) decodeSource(source string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(source))
	if ext == ".tlg" {
		return c.tlgDecoder.Decode(source)
	}

	f, err := os.Open(source) //nolint:gosec // validateSourceで存在確認済みのアセットパスを読む用途のため妥当
	if err != nil {
		return nil, fmt.Errorf("画像ファイルのオープンに失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	switch ext {
	case ".bmp":
		img, err := bmp.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("BMP画像のデコードに失敗しました: %w", err)
		}

		return img, nil
	case ".jpg", ".jpeg":
		img, err := jpeg.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("JPEG画像のデコードに失敗しました: %w", err)
		}

		return img, nil
	case ".png":
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("PNG画像のデコードに失敗しました: %w", err)
		}

		return img, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, ext)
	}
}

// saveAsPNG は画像をPNG形式で保存する内部メソッド。
//
// why not: Goのimage/png.Encodeは任意のimage.Imageを受け付け、そのカラー
// モデルに応じて適切なPNGを書き出すため、モード変換分岐は不要
// （image/pngが吸収する）。
func (c *ImageConverter) saveAsPNG(img image.Image, dest, source string, bytesBefore int64) (ConversionResult, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	f, err := os.Create(dest) //nolint:gosec // ビルド成果物の出力用途のため妥当
	if err != nil {
		return ConversionResult{}, fmt.Errorf("出力ファイルの作成に失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := png.Encode(f, img); err != nil {
		return ConversionResult{}, fmt.Errorf("PNGエンコードに失敗しました: %w", err)
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  getFileSize(dest),
	}, nil
}

// saveAsWebp は画像をWebP形式で保存する内部メソッド。
//
// why not: github.com/gen2brain/webpのEncodeはimage.Imageを内部で必要な形式へ
// 変換するため、明示的なRGB変換を行わずEncodeへ委譲する。
func (c *ImageConverter) saveAsWebp(img image.Image, dest, source string, bytesBefore int64) (ConversionResult, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	f, err := os.Create(dest) //nolint:gosec // ビルド成果物の出力用途のため妥当
	if err != nil {
		return ConversionResult{}, fmt.Errorf("出力ファイルの作成に失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	opts := webp.Options{Quality: c.quality, Method: webp.DefaultMethod}
	if imageHasAlpha(img) && c.losslessAlpha {
		opts.Lossless = true
	}

	if err := webp.Encode(f, img, opts); err != nil {
		return ConversionResult{}, fmt.Errorf("WebPエンコードに失敗しました: %w", err)
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  getFileSize(dest),
	}, nil
}

// opaquer はOpaque() boolを実装する画像型（image.NRGBA等stdlibのほとんどの
// 具象型が実装する）を表す。
type opaquer interface {
	Opaque() bool
}

// imageHasAlpha はimgが実質的なアルファチャンネルを持つかを判定する。
//
// why not: 画像フォーマットが仕様上アルファチャンネルを持つかを判定したい
// （全ピクセルが不透明であっても、フォーマット上アルファチャンネルを持つ
// なら「アルファあり」として扱いたい）。Goのimage.Imageには統一的な
// モード情報が無いため、まずimage/png.Decodeの具象型で判定する: stdlib
// image/pngはカラータイプ4/6
// （グレースケール+アルファ／トゥルーカラー+アルファ）およびtRNSチャンクに
// よる色キー透過を*image.NRGBA/*image.NRGBA64としてのみデコードする
// （アルファの無いカラータイプ0/2/3はGray/RGBA/RGBA64/Palettedになる）ため、
// これらの型はフォーマット上アルファチャンネルを持つPNGを全ピクセル不透明
// でも正しく検出できる。
//
// この型判定に当てはまらない場合はOpaque()（stdlib各具象型が実装）へ
// フォールバックする。golang.org/x/image/bmpの24bpp BMPデコード結果は
// alpha=0xff固定の*image.RGBA（アルファ格納可能な型だが実質不透明）を返す
// ため、型情報だけで判定すると不透明なBMP/JPEGを誤って「アルファあり」と
// 判定してしまう——Opaque()フォールバックはこのBMP/JPEGのケースを正しく
// falseにするために必要。
//
// 既知の残差: 32bpp BMPでアルファチャンネルを許可しない場合
// (golang.org/x/image/bmp decodeNRGBAのallowAlpha=false)も*image.NRGBAで
// 返るため、実質不透明でもhasAlpha=trueになる。本パッケージのテストが対象とする
// 24bpp BMP/PNGのケースでは発生しない。
//
// TLG5デコード結果（internal/converter/tlg.TLG5Decoder.Decode）はcolorsに
// 応じて具象型を使い分ける（RGB(3チャンネル)は*image.RGBA、RGBA(4チャンネル)
// は*image.NRGBA。詳細はtlg5.go createImageFromChannelsのwhy notコメント
// 参照）。そのため本関数の判定はTLG5由来の画像に対しても2つの経路で
// 正しく動作する: RGBA(*image.NRGBA)はColorModel()がNRGBAModelに一致し
// 上のswitchで即座にtrue（Opaque()は評価されない）。RGB(*image.RGBA)は
// ColorModel()がRGBAModelでありswitchに一致しないためOpaque()フォール
// バックへ進み、A=255固定であることからOpaque()==trueとなりfalseを返す。
func imageHasAlpha(img image.Image) bool {
	switch img.ColorModel() {
	case color.NRGBAModel, color.NRGBA64Model:
		return true
	}

	if o, ok := img.(opaquer); ok {
		return !o.Opaque()
	}

	return false
}
