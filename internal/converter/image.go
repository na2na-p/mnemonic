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
)

// ErrTLGDecodeNotImplemented はTLG画像のデコードが未実装であることを示す
// センチネルエラー。
//
// why not: TLG5/TLG6の完全なデコード実装はPR4のスコープ外
// （Python版もNotImplementedErrorを送出し、ヘッダのマジックバイト判定のみ実装する）。
var ErrTLGDecodeNotImplemented = errors.New("TLGデコードは未実装です")

// ErrUnsupportedImageFormat はstdlib/x-imageで対応していない画像拡張子を
// 指定した場合のエラー。
var ErrUnsupportedImageFormat = errors.New("サポートされていない画像形式です")

var (
	tlg5Magic = []byte("TLG5.0\x00raw\x1a")
	tlg6Magic = []byte("TLG6.0\x00raw\x1a")
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

// TLGInfo はTLG画像のメタ情報を表す不変値。
type TLGInfo struct {
	Version  TLGVersion
	Width    int
	Height   int
	HasAlpha bool
}

// TLGImageDecoder はTLG形式の画像ファイルのマジックバイト判定を行う。
//
// デコード自体はPython版と同様に未実装（ErrTLGDecodeNotImplementedを返す）。
type TLGImageDecoder struct{}

// NewTLGImageDecoder はTLGImageDecoderを初期化する。
func NewTLGImageDecoder() *TLGImageDecoder {
	return &TLGImageDecoder{}
}

// IsTLGFile はfilePathがTLG5/TLG6形式かどうかをマジックバイトで判定する。
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

	return bytes.Equal(header, tlg5Magic) || bytes.Equal(header, tlg6Magic)
}

// GetInfo はTLG画像のメタ情報を取得する。未実装のためErrTLGDecodeNotImplementedを返す。
func (d *TLGImageDecoder) GetInfo(_ string) (TLGInfo, error) {
	return TLGInfo{}, ErrTLGDecodeNotImplemented
}

// Decode はTLG画像をデコードする。未実装のためErrTLGDecodeNotImplementedを返す。
func (d *TLGImageDecoder) Decode(_ string) (image.Image, error) {
	return nil, ErrTLGDecodeNotImplemented
}

// DecodeToFile はTLG画像をデコードしてファイルに保存する。
// 未実装のためErrTLGDecodeNotImplementedを返す。
func (d *TLGImageDecoder) DecodeToFile(_, _ string) error {
	return ErrTLGDecodeNotImplemented
}

// ImageConverter はBMP/JPG/PNG/TLG形式の画像をWebP形式に変換するConverter。
type ImageConverter struct {
	quality       int
	losslessAlpha bool
	tlgDecoder    *TLGImageDecoder
}

// NewImageConverter はImageConverterを初期化する。
// qualityはQualityPresetの値（95/85/70）または任意の0-100の整数を指定する。
// qualityが0以下の場合はQualityHigh（Python版のデフォルト引数
// quality: QualityPreset | int = QualityPreset.HIGHに相当）を使用する。
func NewImageConverter(quality int, losslessAlpha bool) *ImageConverter {
	if quality <= 0 {
		quality = int(QualityHigh)
	}

	return &ImageConverter{
		quality:       quality,
		losslessAlpha: losslessAlpha,
		tlgDecoder:    NewTLGImageDecoder(),
	}
}

// Quality はWebP品質値を返す。
func (c *ImageConverter) Quality() int { return c.quality }

// LosslessAlpha はロスレスアルファ設定を返す。
func (c *ImageConverter) LosslessAlpha() bool { return c.losslessAlpha }

// SupportedExtensions は対応する拡張子の一覧を返す。
func (c *ImageConverter) SupportedExtensions() []string {
	return []string{".tlg", ".bmp", ".jpg", ".jpeg", ".png"}
}

// CanConvert はfilePathが変換可能かを拡張子で判定する。
func (c *ImageConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(c.SupportedExtensions(), ext)
}

// Convert は画像ファイルをWebP形式に変換し、destへ出力する。
//
// why not: Python版のImageConverter.convert()は_validate_source・TLG未実装
// エラー・PIL.Image.openの失敗を自身で捕捉せず呼び出し元(ConversionManager)へ
// 伝播させる（他のConverterと異なりtry/exceptで囲まれていない）。Go版もこれに
// 倣い、これらの失敗はConversionResultではなくerrとして返す。
func (c *ImageConverter) Convert(source, dest string) (ConversionResult, error) {
	if err := validateSource(source); err != nil {
		return ConversionResult{}, err
	}

	bytesBefore := getFileSize(source)

	img, err := c.decodeSource(source)
	if err != nil {
		return ConversionResult{}, err
	}

	return c.saveAsWebp(img, dest, source, bytesBefore)
}

// ConvertFromImage はメモリ上のimage.ImageをWebP形式で保存する。
// TLGデコード後の画像変換等、既にデコード済みの画像を直接保存する用途。
func (c *ImageConverter) ConvertFromImage(img image.Image, dest string) (ConversionResult, error) {
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

// saveAsWebp は画像をWebP形式で保存する内部メソッド。
//
// why not: Python版はhas_alpha かつ not lossless_alphaの場合と、has_alphaなしの
// 場合とで明示的にimage.convert("RGB")を呼び分けるが、github.com/gen2brain/webp
// のEncodeはimage.Imageを内部で必要な形式へ変換するため、Go版では明示的な
// RGB変換を行わずEncodeへ委譲する（Pillow固有のモード変換要求がGoには無いため）。
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
// why not: PythonのPIL.Image.modeは "RGBA/LA/PA" というフォーマット上の
// チャンネル構成で判定する（全ピクセルが不透明でも"RGBA"ならアルファあり
// 扱い）。Goのimage.Imageには統一的なモード情報が無いため、まず
// image/png.Decodeの具象型で判定する: stdlib image/pngはカラータイプ4/6
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
// 返るため、実質不透明でもhasAlpha=trueになる。Python版テスト・本パッケージの
// テストが対象とする24bpp BMP/PNGのケースでは発生しない。
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
