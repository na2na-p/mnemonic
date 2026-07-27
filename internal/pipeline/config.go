package pipeline

// Configのデフォルト値。Python版Configの各フィールドの
// デフォルト引数値と同一にする。
const (
	DefaultQuality             = "high"
	DefaultFFmpegTimeoutSecs   = 300
	DefaultGradleTimeoutSecs   = 1800
	DefaultTemplateRefreshDays = 7
)

// Config はビルドパイプラインの実行に必要な設定を保持する値。
//
// CLIオプションやコンフィグファイルからの設定値をまとめて管理する。
// Python版は@dataclass(frozen=True)で不変性を保証していたが、Goの構造体は
// 値渡しされるため、呼び出し側でポインタを共有しない限り生成後に意図せず
// 変更されることはない（internal/apperr.Resultと同じ設計方針）。
//
// KeystorePath / LogFile / TemplateVersionはPythonの `Path | None` /
// `str | None` に相当する。KeystorePath / LogFileは空文字列を「未指定」の
// sentinelとして扱う（ファイルパスとして空文字列が有効になることはないため）。
// TemplateVersionは空文字列自体が意味を持たないバージョン文字列ではなく
// 「未指定→最新を使う」という区別が必要なため*stringで表現する。
type Config struct {
	InputPath            string
	OutputPath           string
	PackageName          string
	AppName              string
	KeystorePath         string
	SkipVideo            bool
	Quality              string
	CleanCache           bool
	VerboseLevel         int
	LogFile              string
	FFmpegTimeoutSeconds int
	GradleTimeoutSeconds int
	TemplateVersion      *string
	TemplateRefreshDays  int
	TemplateOffline      bool
}

// NewConfig はinputPath/outputPathを指定し、その他のフィールドを
// Python版のデフォルト引数値で初期化したConfigを返す。
func NewConfig(inputPath, outputPath string) Config {
	return Config{
		InputPath:            inputPath,
		OutputPath:           outputPath,
		Quality:              DefaultQuality,
		FFmpegTimeoutSeconds: DefaultFFmpegTimeoutSecs,
		GradleTimeoutSeconds: DefaultGradleTimeoutSecs,
		TemplateRefreshDays:  DefaultTemplateRefreshDays,
	}
}
