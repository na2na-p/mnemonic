package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"

	"github.com/goccy/go-yaml"
)

// ErrNotFound は設定ファイルが存在しない場合のエラー。
var ErrNotFound = errors.New("設定ファイルが見つかりません")

// ErrReadFailed は設定ファイルは存在するが読み込みに失敗した場合のエラー
// （権限不足、ディレクトリをファイルとして指定した等、存在しないこと以外の原因）。
var ErrReadFailed = errors.New("設定ファイルの読み込みに失敗しました")

// ErrInvalidYAML はYAML解析に失敗した場合のエラー。
var ErrInvalidYAML = errors.New("YAML解析エラー")

// ErrInvalidFormat は設定ファイルのルートがYAMLマッピングでない場合のエラー。
var ErrInvalidFormat = errors.New("設定ファイルはYAMLのマッピング形式である必要があります")

// Load は設定ファイルを読み込み、デフォルト値とマージして返す。
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // ビルド対象ゲームのユーザー指定パスを読み込む用途のため妥当
	if err != nil {
		// 「存在しない」と「存在するが読めない（権限不足・ディレクトリ指定等）」は
		// 原因も対処法も異なるため、fs.ErrNotExistかどうかで別のセンチネルにする。
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("%w: %s", ErrNotFound, path)
		}

		return Config{}, fmt.Errorf("%w: %s: %w", ErrReadFailed, path, err)
	}

	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalidYAML, err)
	}

	// 空ファイルはデフォルト設定として扱う（goccy/go-yamlのUnmarshalは空文字列に
	// 対してdocをnilのままにする）。
	if doc == nil {
		doc = map[string]any{}
	}

	data, ok := doc.(map[string]any)
	if !ok {
		return Config{}, ErrInvalidFormat
	}

	return merge(data, Default()), nil
}

// merge はYAMLから得た生データをデフォルト値とマージする。
//
// Goは静的型付けのため、型が一致しないキーはエラーにせず無視してデフォルト値を
// 維持する（不正な設定値でCLI実行が失敗しないようにするための意図的な設計）。
func merge(data map[string]any, def Config) Config {
	cfg := def

	if v, ok := stringValue(data, "package_name"); ok {
		cfg.PackageName = &v
	}
	if v, ok := stringValue(data, "app_name"); ok {
		cfg.AppName = &v
	}
	if v, ok := intValue(data, "version_code"); ok {
		cfg.VersionCode = v
	}
	if v, ok := stringValue(data, "version_name"); ok {
		cfg.VersionName = v
	}

	cfg.Image = mergeImage(mapValue(data, "image"), def.Image)
	cfg.Video = mergeVideo(mapValue(data, "video"), def.Video)
	cfg.Encoding = mergeEncoding(mapValue(data, "encoding"), def.Encoding)
	cfg.Timeouts = mergeTimeouts(mapValue(data, "timeouts"), def.Timeouts)
	cfg.ConversionRules = parseConversionRules(sliceValue(data, "conversion_rules"))

	if v, ok := stringSliceValue(data, "exclude"); ok {
		cfg.Exclude = v
	}

	return cfg
}

func mergeImage(data map[string]any, def ImageConfig) ImageConfig {
	cfg := def
	if v, ok := stringValue(data, "format"); ok {
		cfg.Format = v
	}
	if v, ok := data["quality"]; ok {
		cfg.Quality = parseQuality(v, def.Quality)
	}
	if v, ok := boolValue(data, "lossless_alpha"); ok {
		cfg.LosslessAlpha = v
	}

	return cfg
}

func mergeVideo(data map[string]any, def VideoConfig) VideoConfig {
	cfg := def
	if v, ok := stringValue(data, "codec"); ok {
		cfg.Codec = v
	}
	if v, ok := stringValue(data, "profile"); ok {
		cfg.Profile = v
	}
	if v, ok := stringValue(data, "audio_codec"); ok {
		cfg.AudioCodec = v
	}

	return cfg
}

func mergeEncoding(data map[string]any, def EncodingConfig) EncodingConfig {
	cfg := def
	if v, ok := stringValue(data, "source"); ok {
		cfg.Source = &v
	}
	if v, ok := stringValue(data, "target"); ok {
		cfg.Target = v
	}

	return cfg
}

func mergeTimeouts(data map[string]any, def TimeoutConfig) TimeoutConfig {
	cfg := def
	if v, ok := intValue(data, "ffmpeg"); ok {
		cfg.Ffmpeg = v
	}
	if v, ok := intValue(data, "gradle"); ok {
		cfg.Gradle = v
	}

	return cfg
}

// parseQuality はquality値をQuality型に変換する。数値はLevel、文字列はPresetとして扱う。
func parseQuality(v any, def Quality) Quality {
	if i, ok := toInt(v); ok {
		return Quality{Level: i, IsInt: true}
	}
	if s, ok := v.(string); ok {
		return Quality{Preset: s}
	}

	return def
}

// parseConversionRules はconversion_rulesのリストを変換する。
// pattern/converterのいずれかを欠く要素はスキップする。
func parseConversionRules(data []any) []ConversionRule {
	if data == nil {
		return nil
	}

	rules := make([]ConversionRule, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pattern, hasPattern := m["pattern"].(string)
		converter, hasConverter := m["converter"].(string)
		if !hasPattern || !hasConverter {
			continue
		}
		rules = append(rules, ConversionRule{Pattern: pattern, Converter: converter})
	}

	return rules
}

func mapValue(data map[string]any, key string) map[string]any {
	v, ok := data[key].(map[string]any)
	if !ok {
		return nil
	}

	return v
}

func sliceValue(data map[string]any, key string) []any {
	v, ok := data[key].([]any)
	if !ok {
		return nil
	}

	return v
}

func stringValue(data map[string]any, key string) (string, bool) {
	v, ok := data[key].(string)

	return v, ok
}

func boolValue(data map[string]any, key string) (bool, bool) {
	v, ok := data[key].(bool)

	return v, ok
}

func intValue(data map[string]any, key string) (int, bool) {
	v, ok := data[key]
	if !ok {
		return 0, false
	}

	return toInt(v)
}

func stringSliceValue(data map[string]any, key string) ([]string, bool) {
	raw, ok := data[key].([]any)
	if !ok {
		return nil, false
	}

	values := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			continue
		}
		values = append(values, s)
	}

	return values, true
}

// toInt はgoccy/go-yamlがデコードする数値型（int64/uint64/float64）をintへ正規化する。
// version_code等の設定値はint型の範囲に収まる想定だが、YAML上は任意の大きさの
// 整数を書けてしまうため、int変換でオーバーフローする値は不正値として拒否する
// （黙ってラップアラウンドさせると符号反転した意図しない値になりかねないため）。
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		if n < math.MinInt || n > math.MaxInt {
			return 0, false
		}

		return int(n), true
	case uint64:
		if n > math.MaxInt {
			return 0, false
		}

		return int(n), true
	case float64:
		// NaN/Infはint変換が未定義動作になり、有限だが範囲外の値
		// （例: 1.0e30）はint(n)で符号反転・飽和した無関係の値になるため、
		// いずれも変換失敗として拒否しデフォルト値へフォールバックさせる。
		// float64(math.MaxInt)は2^63へ切り上がり表現されるため、境界は >= で弾く。
		if math.IsNaN(n) || math.IsInf(n, 0) || n >= float64(math.MaxInt) || n < float64(math.MinInt) {
			return 0, false
		}

		return int(n), true
	default:
		return 0, false
	}
}
