package converter

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

// ErrUnsupportedImageFormat はstdlib/x-imageで対応していない画像拡張子を
// 指定した場合のエラー。
var ErrUnsupportedImageFormat = errors.New("サポートされていない画像形式です")

// ErrTLGInvalidFormat はTLG5/TLG6/SDSのいずれのマジックバイトにも一致しない
// データに対するエラー。
//
// why not: TLGImageDecoderはこのエラーにファイルパスを付けない。呼び出し側は
// 渡したパスを知っており、ConversionManagerの結果もSourcePathを持つため、
// パスを付けると報告で同じパスが重なる。デコード中の失敗（tlgパッケージの
// エラー）もパスを含まず、それと揃う。
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

// TLGInfo はTLG画像のメタ情報を表す不変値。
type TLGInfo struct {
	Version  TLGVersion
	Width    int
	Height   int
	HasAlpha bool
}

// TLGImageDecoder はTLG形式の画像ファイルを読み込み、image.Imageへ変換する。
// TLG5およびTLG6形式に対応し、SDSコンテナ形式もサポートする。
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
// 読み込みの失敗はclassifyReadErrorで分類したエラーを返す。
func readTLGSource(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // 呼び出し側が指定したアセットパスを読む用途のため妥当
	if err != nil {
		return nil, classifyReadError(filePath, err)
	}

	return unwrapSDS(data), nil
}

// GetInfo はTLG画像のメタ情報をヘッダーだけから取得する。
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
		return TLGInfo{}, ErrTLGInvalidFormat
	}
}

// Decode はTLG画像をデコードしてimage.Imageを返す。
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
		img, decErr := d.tlg6Decoder.Decode(data)
		if decErr != nil {
			return nil, decErr
		}

		return img, nil
	default:
		return nil, ErrTLGInvalidFormat
	}
}

// DecodeToFile はTLG画像をデコードしてファイルに保存する。
// destの拡張子が.pngの場合に保存する。
func (d *TLGImageDecoder) DecodeToFile(source, dest string) error {
	img, err := d.Decode(source)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	return encodeImageToFile(img, dest)
}

// encodeImageToFile はimgをPNG形式でファイルへ書き出す。
func encodeImageToFile(img image.Image, dest string) error {
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
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, ext)
	}

	return nil
}

// ImageConverter はBMP/JPG/PNG/TLG形式の画像をPNG形式に変換するConverter。
type ImageConverter struct {
	tlgDecoder *TLGImageDecoder
}

// NewImageConverter はImageConverterを初期化する。
//
// why not: krkrsdl2はWebPを読み込めないため、出力形式はPNGに固定する。
func NewImageConverter() *ImageConverter {
	return &ImageConverter{tlgDecoder: NewTLGImageDecoder()}
}

// SupportedExtensions は対応する拡張子の一覧を返す。
//
// JPEG/PNG/BMPはkrkrsdl2でネイティブサポートのため変換対象外
// （feat/exe-icon-extraction 680b27fより。誤ってPNGに変換されたJPEGを
// krkrsdl2のTVPLoadJPEGへ渡すとSIGSEGVでクラッシュする不具合の修正）。
func (c *ImageConverter) SupportedExtensions() []string {
	return []string{".tlg"}
}

// GetOutputExtension は常に.pngを返す。
func (c *ImageConverter) GetOutputExtension(_ string) string {
	return ".png"
}

// CanConvert はfilePathが変換可能かを拡張子で判定する。
func (c *ImageConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(c.SupportedExtensions(), ext)
}

// Convert は画像ファイルを指定された形式に変換し、destへ出力する。
//
// 失敗はerrとして返す。変換元の検証(validateSource)の失敗のうち、変換元が
// 存在しない・権限不足で確認できない・ディレクトリである場合と、デコードの失敗
// （壊れたTLG・未対応の拡張子を含む）はErrPermanentFailureでラップして返す。
// それ以外の理由で変換元を確認できない場合、TLGの読み込み失敗（権限不足を除く）・
// TLG以外の変換元のオープン失敗・出力先の作成・PNGエンコードの失敗は再試行対象と
// する。errがnilのとき、StatusはStatusSuccessとなる。
//
// why not(decodeSourceの対応拡張子): CanConvert/SupportedExtensionsは.tlg
// のみだが、Convert()自体は.bmp/.jpg/.jpeg/.png/.tlgを直接処理できる
// （ConversionManager経由では.tlg以外はルーティングされないが、
// ConvertFromImage等、呼び出し元がConvertを直接呼ぶ経路のためにdecodeSource
// の分岐は維持する）。
func (c *ImageConverter) Convert(source, dest string) (ConversionResult, error) {
	if err := validateSource(source); err != nil {
		return ConversionResult{SourcePath: source}, err
	}

	bytesBefore := getFileSize(source)

	img, err := c.decodeSource(source)
	if err != nil {
		return ConversionResult{SourcePath: source}, err
	}

	return c.saveAsPNG(img, dest, source, bytesBefore)
}

// ConvertFromImage はメモリ上のimage.ImageをPNG形式で保存する。
// TLGデコード後の画像変換等、既にデコード済みの画像を直接保存する用途。
func (c *ImageConverter) ConvertFromImage(img image.Image, dest string) (ConversionResult, error) {
	return c.saveAsPNG(img, dest, dest, 0)
}

func (c *ImageConverter) decodeSource(source string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(source))
	if ext == ".tlg" {
		img, err := c.tlgDecoder.Decode(source)
		if err != nil {
			// why not: 読み込みの失敗まで一律に恒久扱いにはしない。読み込みの失敗は
			// readTLGSourceがclassifyReadErrorで恒久か再試行対象かを分類済みのため
			// そのまま返し、恒久扱いにするのはデコードの失敗だけにする。
			if errors.Is(err, ErrSourceNotFound) || errors.Is(err, ErrSourceUnreadable) {
				return nil, err
			}

			return nil, permanentError(err)
		}

		return img, nil
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
			return nil, permanentError(fmt.Errorf("BMP画像のデコードに失敗しました: %w", err))
		}

		return img, nil
	case ".jpg", ".jpeg":
		img, err := jpeg.Decode(f)
		if err != nil {
			return nil, permanentError(fmt.Errorf("JPEG画像のデコードに失敗しました: %w", err))
		}

		return img, nil
	case ".png":
		img, err := png.Decode(f)
		if err != nil {
			return nil, permanentError(fmt.Errorf("PNG画像のデコードに失敗しました: %w", err))
		}

		return img, nil
	default:
		return nil, permanentError(fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, ext))
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
