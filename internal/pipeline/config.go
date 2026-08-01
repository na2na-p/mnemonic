package pipeline

// Configの各フィールドのデフォルト値。
const (
	DefaultQuality             = "high"
	DefaultFFmpegTimeoutSecs   = 300
	DefaultGradleTimeoutSecs   = 1800
	DefaultTemplateRefreshDays = 7
)

// Config はビルドパイプラインの実行に必要な設定を保持する値。
//
// CLIオプションやコンフィグファイルからの設定値をまとめて管理する。
// Goの構造体は値渡しされるため、呼び出し側でポインタを共有しない限り
// 生成後に意図せず変更されることはない（internal/apperr.Resultと同じ
// 設計方針）。
//
// KeystorePath / LogFileは空文字列を「未指定」のsentinelとして扱う
// （ファイルパスとして空文字列が有効になることはないため）。
// TemplateVersionは空文字列自体が意味を持たないバージョン文字列ではなく
// 「未指定→最新を使う」という区別が必要なため*stringで表現する。
type Config struct {
	InputPath   string
	OutputPath  string
	PackageName string
	AppName     string
	// SoundfontPath はMIDI変換に使うサウンドフォントのパス。空文字列を
	// 「未指定」のsentinelとして扱い、converter.GetDefaultSoundfontPathによる
	// 既定の解決に委ねる。
	SoundfontPath        string
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

// NewConfig はinputPath/outputPathを指定し、その他のフィールドをデフォルト値
// で初期化したConfigを返す。
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
