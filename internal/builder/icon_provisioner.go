package builder

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// iconMipmapDensities はアイコンを配置するmipmapディレクトリの解像度一覧。
var iconMipmapDensities = []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

// defaultIconDensitySizes はデフォルトアイコン生成時の密度ごとのサイズ(px)。
var defaultIconDensitySizes = map[string]int{
	"mdpi":    48,
	"hdpi":    72,
	"xhdpi":   96,
	"xxhdpi":  144,
	"xxxhdpi": 192,
}

// defaultIconColor はデフォルトアイコンの色（吉里吉里のテーマカラーに近い青紫）。
var defaultIconColor = color.RGBA{R: 100, G: 80, B: 160, A: 255}

// iconProvisioner はアプリアイコンを各解像度のmipmapディレクトリへ配置する。
// 提供アイコンが無い場合は単色のデフォルトアイコンを生成する。
//
// why not（UpdateIcon/CreateDefaultを別の型に分けない理由）: 両者は
// 「起動activityが読み込むic_launcher.pngを各密度のmipmapディレクトリに
// 揃える」という同一目的の二者択一（提供アイコンがあれば配置、無ければ
// 生成）であり、対象ディレクトリ・密度一覧という同じ不変条件を共有する。
// 目的が分かれていない（アイコンを用意したい、という1つの目的の実現手段が
// 2通りあるだけ）ため、1つの型にまとめる。
type iconProvisioner struct {
	projectDir string
}

// newIconProvisioner はiconProvisionerを初期化する。
func newIconProvisioner(projectDir string) *iconProvisioner {
	return &iconProvisioner{projectDir: projectDir}
}

// UpdateIcon はアプリアイコンを各解像度のmipmapディレクトリへコピーする。
func (p *iconProvisioner) UpdateIcon(iconPath string) error {
	resDir := filepath.Join(p.projectDir, "app", "src", "main", "res")

	for _, density := range iconMipmapDensities {
		mipmapDir := filepath.Join(resDir, "mipmap-"+density)
		if err := os.MkdirAll(mipmapDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		destPath := filepath.Join(mipmapDir, "ic_launcher.png")
		if err := copyFile(iconPath, destPath); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}
	}

	return nil
}

// CreateDefault はデフォルトアイコンを生成する。
//
// アイコンが提供されない場合のフォールバックとして、単色の正方形アイコンを
// 各解像度で生成する。
func (p *iconProvisioner) CreateDefault() error {
	resDir := filepath.Join(p.projectDir, "app", "src", "main", "res")

	for _, density := range iconMipmapDensities {
		mipmapDir := filepath.Join(resDir, "mipmap-"+density)
		if err := os.MkdirAll(mipmapDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		destPath := filepath.Join(mipmapDir, "ic_launcher.png")
		if err := writeSolidColorPNG(destPath, defaultIconDensitySizes[density], defaultIconColor); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}
	}

	return nil
}

// writeSolidColorPNG はsize x sizeの単色PNG画像をpathへ書き出す。
func writeSolidColorPNG(path string, size int, c color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)

	f, err := os.Create(path) //nolint:gosec // ビルド成果物の出力用途のため妥当
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return png.Encode(f, img)
}
