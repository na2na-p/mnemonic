package parser_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// writeExeFixture はcontentを一意なEXEファイルとしてt.TempDir()配下へ書き出し、
// そのパスを返す。
func writeExeFixture(t *testing.T, content []byte) string {
	t.Helper()

	exePath := filepath.Join(t.TempDir(), "game.exe")
	require.NoError(t, os.WriteFile(exePath, content, 0o600))

	return exePath
}

// decodePNGFile は指定パスのPNGファイルを読み込みimage.Imageへデコードする。
func decodePNGFile(t *testing.T, path string) image.Image {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
	require.NoError(t, err)

	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	return img
}

func TestExeIconExtractor_Extract(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 存在しないEXEファイルでErrEXENotFound", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(t.TempDir(), "nonexistent.exe")
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(nonexistent, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrEXENotFound)
	})

	t.Run("異常系: 破損したEXEファイルでErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		exePath := writeExeFixture(t, []byte("INVALID"))
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: アイコンリソースが無いPEでErrNoIconsAvailable", func(t *testing.T) {
		t.Parallel()

		peBytes := buildMinimalPE(t, buildEmptyRsrcDir())
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrNoIconsAvailable)
	})

	t.Run("異常系: GRPICONDIRのidTypeが不正な場合ErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{{id: 101, width: 32, height: 32, bitCount: 32, data: build32bppDIB(32, 32, solidColor(255, 0, 0, 255))}}
		rsrc := buildRsrcWithIconGroupInvalidType(t, images)

		peBytes := buildMinimalPE(t, rsrc)
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("正常系: 単一フレーム32bppアイコンをPNGとして保存する", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{
			{id: 101, width: 32, height: 32, bitCount: 32, data: build32bppDIB(32, 32, solidColor(200, 10, 20, 128))},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(outputDir, "icon.png"), result)
		assert.FileExists(t, result)

		img := decodePNGFile(t, result)
		assert.Equal(t, 32, img.Bounds().Dx())
		assert.Equal(t, 32, img.Bounds().Dy())
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 200, G: 10, B: 20, A: 128})
	})

	t.Run("正常系: 複数サイズのアイコンがある場合、最大サイズを選択する", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{
			{id: 10, width: 16, height: 16, bitCount: 32, data: build32bppDIB(16, 16, solidColor(255, 0, 0, 255))},
			{id: 20, width: 0, height: 0, bitCount: 32, data: build32bppDIB(256, 256, solidColor(0, 255, 0, 255))}, // 0は256を表す
			{id: 30, width: 48, height: 48, bitCount: 32, data: build32bppDIB(48, 48, solidColor(0, 0, 255, 255))},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assert.Equal(t, 256, img.Bounds().Dx())
		assert.Equal(t, 256, img.Bounds().Dy())
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 255})
	})

	t.Run("正常系: 出力ディレクトリが存在しない場合に作成する", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{{id: 101, width: 16, height: 16, bitCount: 32, data: build32bppDIB(16, 16, solidColor(1, 2, 3, 255))}}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "deep", "nested", "out")
		require.NoDirExists(t, outputDir)
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		assert.DirExists(t, outputDir)
		assert.FileExists(t, result)
	})

	t.Run("正常系: PNG形式で埋め込まれたアイコンフレームをデコードできる", func(t *testing.T) {
		t.Parallel()

		var pngBuf bytes.Buffer
		src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		for y := range 4 {
			for x := range 4 {
				src.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
			}
		}
		require.NoError(t, png.Encode(&pngBuf, src))

		images := []fixtureIconImage{{id: 101, width: 4, height: 4, bitCount: 32, data: pngBuf.Bytes()}}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assert.Equal(t, 4, img.Bounds().Dx())
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	})

	t.Run("正常系: 24bppアイコンはアルファ無し(不透明)としてデコードする", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 24, data: build24bppDIB(8, 8, func(int, int) (byte, byte, byte) { return 50, 60, 70 })},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assertNRGBAEqual(t, img, 3, 3, color.NRGBA{R: 50, G: 60, B: 70, A: 255})
	})

	t.Run("正常系: 8bppパレットアイコンはANDマスクの透明画素を反映する", func(t *testing.T) {
		t.Parallel()

		palette := [][3]byte{{255, 0, 0}, {0, 255, 0}}
		idx := func(x, y int) byte {
			if x == 1 && y == 1 {
				return 1
			}

			return 0
		}
		transparent := func(x, y int) bool { return x == 1 && y == 1 }

		images := []fixtureIconImage{
			{id: 101, width: 2, height: 2, bitCount: 8, data: build8bppDIB(2, 2, palette, idx, transparent)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		assertNRGBAEqual(t, img, 1, 1, color.NRGBA{R: 0, G: 255, B: 0, A: 0})
	})

	t.Run("正常系: 1bppアイコンをデコードできる", func(t *testing.T) {
		t.Parallel()

		palette := [2][3]byte{{0, 0, 0}, {255, 255, 255}}
		bit := func(x, y int) byte {
			if x == 0 && y == 0 {
				return 1
			}

			return 0
		}

		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 1, data: build1bppDIB(8, 8, palette, bit)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		assertNRGBAEqual(t, img, 1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	})

	t.Run("正常系: 4bppアイコンはANDマスクの透明画素を反映する", func(t *testing.T) {
		t.Parallel()

		palette := [][3]byte{{255, 0, 0}, {0, 255, 0}}
		idx := func(x, y int) byte {
			if x == 1 && y == 1 {
				return 1
			}

			return 0
		}
		transparent := func(x, y int) bool { return x == 1 && y == 1 }

		images := []fixtureIconImage{
			{id: 101, width: 2, height: 2, bitCount: 4, data: build4bppDIB(2, 2, palette, idx, transparent)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		assertNRGBAEqual(t, img, 1, 1, color.NRGBA{R: 0, G: 255, B: 0, A: 0})
	})

	t.Run("正常系: リソースをセクション名でなくデータディレクトリRVAで解決する", func(t *testing.T) {
		t.Parallel()

		// Windowsローダー/pefile/icoextractはIMAGE_DIRECTORY_ENTRY_RESOURCEの
		// RVAでリソーステーブルを解決し、セクション名は見ない。セクションを
		// ".rsrc"以外(".data")へリネームしても、データディレクトリを
		// 正しく設定していれば抽出できることを確認する回帰テスト
		// (レビューで実証: 名前だけで探す実装は同じPEのセクションを
		// リネームしただけで抽出に失敗していた)。
		images := []fixtureIconImage{{id: 101, width: 16, height: 16, bitCount: 32, data: build32bppDIB(16, 16, solidColor(9, 8, 7, 255))}}
		rsrc := buildRsrcWithIconGroup(t, images)
		peBytes := buildMinimalPEWithSection(t, ".data", rsrc, true)
		exePath := writeExeFixture(t, peBytes)
		outputDir := filepath.Join(t.TempDir(), "out")
		extractor := parser.NewExeIconExtractor()

		result, err := extractor.Extract(exePath, outputDir)

		require.NoError(t, err)
		img := decodePNGFile(t, result)
		assertNRGBAEqual(t, img, 0, 0, color.NRGBA{R: 9, G: 8, B: 7, A: 255})
	})

	t.Run("異常系: 攻撃的なbiWidth/biHeightでもpanicせずErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		// biWidth/biHeightにint32上限付近の値(2147483646)を書き込んだ
		// 壊れたDIB。次元チェックがimage.NewNRGBAより前に無いと、
		// makeslice panicまたは実データサイズに見合わない巨大確保を
		// 引き起こす(レビューで実証済みのケース)。
		const attackDimension = 2147483646
		images := []fixtureIconImage{
			{id: 101, width: 32, height: 32, bitCount: 32, data: buildRawDIBHeader(attackDimension, attackDimension, 32, 0)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: DIBの次元が上限を超える場合ErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		// maxICODimension(1024)を超えるが、int32の乗算オーバーフローは
		// 起こさない程度の値。上限チェック自体を単独で検証する。
		const overCapDimension = 5000
		images := []fixtureIconImage{
			{id: 101, width: 32, height: 32, bitCount: 32, data: buildRawDIBHeader(overCapDimension, overCapDimension, 32, 0)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: ピクセルデータが不足している場合ErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		// ヘッダーのみでXORピクセルデータを含まない32bpp DIB。
		// decodeDIBのエラー(ErrIconInvalidDIB)がExtract()の3種類の
		// センチネルエラー契約(ErrEXENotFound/ErrIconInvalidPEFile/
		// ErrNoIconsAvailable)から漏れずErrIconInvalidPEFileへ
		// 包まれることを確認する。
		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 32, data: buildRawDIBHeader(8, 16, 32, 0)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: 未対応ビット深度(16bpp)でErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 16, data: buildRawDIBHeader(8, 16, 16, 0)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: 圧縮あり(RLE)DIBでErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		const biRLE8 = 1
		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 8, data: buildRawDIBHeader(8, 16, 8, biRLE8)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})

	t.Run("異常系: biHeightが奇数の場合ErrIconInvalidPEFile", func(t *testing.T) {
		t.Parallel()

		images := []fixtureIconImage{
			{id: 101, width: 8, height: 8, bitCount: 32, data: buildRawDIBHeader(8, 17, 32, 0)},
		}
		peBytes := buildMinimalPE(t, buildRsrcWithIconGroup(t, images))
		exePath := writeExeFixture(t, peBytes)
		extractor := parser.NewExeIconExtractor()

		_, err := extractor.Extract(exePath, filepath.Join(t.TempDir(), "out"))

		require.ErrorIs(t, err, parser.ErrIconInvalidPEFile)
	})
}

// solidColor はテスト用の単色32bpp DIBを塗りつぶすための画素関数を返す。
func solidColor(r, g, b, a byte) func(x, y int) (byte, byte, byte, byte) {
	return func(int, int) (byte, byte, byte, byte) { return r, g, b, a }
}

// assertNRGBAEqual はimgの(x, y)画素がwantと一致することを検証する。
//
// why not(img.At(x,y).RGBA()の生の16bit値を直接シフトしない理由):
// color.Color.RGBA()はアルファ事前乗算(premultiplied alpha)の16bit値を
// 返す契約のため、単純な右シフトでは非事前乗算の8bit値に戻せない
// (例: A=128の画素はR成分が約半分に減衰したまま返る)。
// image/pngは完全不透明な*image.NRGBAを色タイプ2(アルファ無し)として
// 書き出すことがあり、その場合デコード結果は*image.RGBAになるため
// 具象型への型アサーションにも頼れない。color.NRGBAModel.Convertは
// 事前乗算からの復元(かつ既にNRGBAならそのまま)を正しく行うため、
// これを経由することで両ケースを吸収する。
func assertNRGBAEqual(t *testing.T, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()

	got, ok := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	require.True(t, ok)

	assert.Equal(t, want, got)
}
