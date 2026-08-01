// Package config は mnemonic.yml 設定ファイルの読み込みとデフォルト値マージを提供する。
package config

// Config はビルド設定のルート値。
//
// PackageName / AppName は未指定を明示するためポインタで表現する
// （空文字列と未指定を区別する必要があるため、ゼロ値の空文字列を
// sentinelとして扱わない）。
type Config struct {
	PackageName     *string
	AppName         *string
	VersionCode     int
	VersionName     string
	Image           ImageConfig
	Video           VideoConfig
	Encoding        EncodingConfig
	ConversionRules []ConversionRule
	Exclude         []string
	Timeouts        TimeoutConfig
}

// ImageConfig は画像変換設定を表す。
type ImageConfig struct {
	Format        string
	Quality       Quality
	LosslessAlpha bool
}

// Quality は画像品質設定を表す。
//
// Goには合併型がないため、プリセット文字列（"high"等）か0-100の整数の
// いずれかを保持する値型として表現する。
type Quality struct {
	// Preset はIsInt=falseの場合に有効なプリセット文字列。
	Preset string
	// Level はIsInt=trueの場合に有効な0-100の整数値。
	Level int
	IsInt bool
}

// VideoConfig は動画変換設定を表す。
type VideoConfig struct {
	Codec      string
	Profile    string
	AudioCodec string
}

// EncodingConfig は文字コード設定を表す。
type EncodingConfig struct {
	// Source は未指定の場合に自動検出することを表す。
	Source *string
	Target string
}

// ConversionRule はカスタム変換ルールを表す。
type ConversionRule struct {
	Pattern   string
	Converter string
}

// TimeoutConfig はタイムアウト設定を表す。
type TimeoutConfig struct {
	Ffmpeg int
	Gradle int
}

// Default はデフォルト設定を返す。
func Default() Config {
	return Config{
		VersionCode: 1,
		VersionName: "1.0.0",
		Image: ImageConfig{
			Format:        "webp",
			Quality:       Quality{Preset: "high"},
			LosslessAlpha: true,
		},
		Video: VideoConfig{
			Codec:      "h264",
			Profile:    "baseline",
			AudioCodec: "aac",
		},
		Encoding: EncodingConfig{
			Target: "utf-8",
		},
		Timeouts: TimeoutConfig{
			Ffmpeg: 300,
			Gradle: 1800,
		},
	}
}
