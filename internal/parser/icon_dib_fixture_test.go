package parser_test

import "encoding/binary"

// このファイルはicon_test.go専用のBITMAPINFOHEADERベースDIB(ICO内RT_ICON
// 生データ)フィクスチャビルダー。decodeDIB(icon_dib.go)が対応する
// 1/8/24/32bppそれぞれについて、テストが必要とする最小限のビルダーのみを
// 用意する(タスク指示: フィクスチャは小さく保つ。共通化より各ビット深度の
// 素直な実装を優先し可読性を優先する)。

func dibRowStrideFixture(width, bitsPerPixel int) int {
	return ((width*bitsPerPixel + 31) / 32) * 4
}

func putDIBHeader(buf []byte, width, height, bitCount int) {
	binary.LittleEndian.PutUint32(buf[0:4], 40) // biSize
	binary.LittleEndian.PutUint32(buf[4:8], uint32(width))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(height)) // XOR+ANDの合計(実際の高さの2倍)
	binary.LittleEndian.PutUint16(buf[12:14], 1)             // biPlanes
	binary.LittleEndian.PutUint16(buf[14:16], uint16(bitCount))
	binary.LittleEndian.PutUint32(buf[16:20], 0) // biCompression: BI_RGB
}

// build32bppDIB はrgbaの各画素(row-major、y=0が画像上端)を32bpp DIBとして
// 組み立てる。32bppはANDマスクを持たない(decodeDIB32が読まないため付与不要)。
func build32bppDIB(width, height int, rgba func(x, y int) (r, g, b, a byte)) []byte {
	stride := width * 4
	xorSize := stride * height
	buf := make([]byte, 40+xorSize)
	putDIBHeader(buf, width, height*2, 32)

	for y := range height {
		destRow := height - 1 - y // DIBは下から上へ格納
		row := buf[40+destRow*stride : 40+(destRow+1)*stride]
		for x := range width {
			r, g, b, a := rgba(x, y)
			row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = b, g, r, a
		}
	}

	return buf
}

// build24bppDIB はrgbの各画素を24bpp DIBとして組み立てる。ANDマスクは
// 付与しない(applyDIBAndMaskはデータ不足時に不透明のまま扱う)。
func build24bppDIB(width, height int, rgb func(x, y int) (r, g, b byte)) []byte {
	stride := dibRowStrideFixture(width, 24)
	xorSize := stride * height
	buf := make([]byte, 40+xorSize)
	putDIBHeader(buf, width, height*2, 24)

	for y := range height {
		destRow := height - 1 - y
		row := buf[40+destRow*stride : 40+(destRow+1)*stride]
		for x := range width {
			r, g, b := rgb(x, y)
			row[x*3], row[x*3+1], row[x*3+2] = b, g, r
		}
	}

	return buf
}

// build8bppDIB はpalette(索引→RGB)・idx(画素ごとの索引)・transparent
// (画素ごとの透明フラグ、ANDマスクへ反映)から8bpp DIBを組み立てる。
func build8bppDIB(width, height int, paletteRGB [][3]byte, idx func(x, y int) byte, transparent func(x, y int) bool) []byte {
	const bitCount = 8
	xorStride := dibRowStrideFixture(width, bitCount)
	andStride := dibRowStrideFixture(width, 1)
	paletteBytes := len(paletteRGB) * 4
	xorSize := xorStride * height
	andSize := andStride * height

	buf := make([]byte, 40+paletteBytes+xorSize+andSize)
	putDIBHeader(buf, width, height*2, bitCount)
	binary.LittleEndian.PutUint32(buf[32:36], uint32(len(paletteRGB))) // biClrUsed

	for i, c := range paletteRGB {
		e := buf[40+i*4 : 40+(i+1)*4]
		e[0], e[1], e[2] = c[2], c[1], c[0] // BGR + reserved
	}

	xorOffset := 40 + paletteBytes
	andOffset := xorOffset + xorSize

	for y := range height {
		destRow := height - 1 - y
		xorRow := buf[xorOffset+destRow*xorStride : xorOffset+(destRow+1)*xorStride]
		andRow := buf[andOffset+destRow*andStride : andOffset+(destRow+1)*andStride]

		for x := range width {
			xorRow[x] = idx(x, y)
			if transparent(x, y) {
				andRow[x/8] |= 1 << (7 - x%8)
			}
		}
	}

	return buf
}

// build1bppDIB はpalette(2色)・bit(画素ごとの索引0/1)から1bpp DIBを
// 組み立てる。ANDマスクは付与しない(不透明のまま扱われることを確認する
// テスト専用)。
func build1bppDIB(width, height int, paletteRGB [2][3]byte, bit func(x, y int) byte) []byte {
	const bitCount = 1
	xorStride := dibRowStrideFixture(width, bitCount)
	xorSize := xorStride * height

	buf := make([]byte, 40+2*4+xorSize)
	putDIBHeader(buf, width, height*2, bitCount)

	for i, c := range paletteRGB {
		e := buf[40+i*4 : 40+(i+1)*4]
		e[0], e[1], e[2] = c[2], c[1], c[0]
	}

	xorOffset := 40 + 2*4
	for y := range height {
		destRow := height - 1 - y
		row := buf[xorOffset+destRow*xorStride : xorOffset+(destRow+1)*xorStride]

		for x := range width {
			if bit(x, y) != 0 {
				row[x/8] |= 1 << (7 - x%8)
			}
		}
	}

	return buf
}
