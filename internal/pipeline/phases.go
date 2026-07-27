package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/parser"
	"github.com/na2na-p/mnemonic/internal/signer"
)

// ErrGradleAPKMissing はGradleビルドが成功終了コードを返したにもかかわらず、
// 期待される場所にAPKファイルが生成されなかった場合のエラー。
var ErrGradleAPKMissing = errors.New("Gradleビルド後にAPKファイルが見つかりません")

// executeAnalyze はANALYZEフェーズを実行する: 入力ファイルの形式を確認し、
// 必要に応じて暗号化チェックを行う。
func (b *BuildPipeline) executeAnalyze() error {
	suffix := strings.ToLower(filepath.Ext(b.config.InputPath))

	switch suffix {
	case ".exe":
		extractor, err := parser.NewEmbeddedXP3Extractor(b.config.InputPath)
		if err != nil {
			return err
		}

		xp3List, err := extractor.FindEmbeddedXP3()
		if err != nil {
			return err
		}
		if len(xp3List) == 0 {
			return fmt.Errorf("EXEファイル内にXP3アーカイブが見つかりません: %s", b.config.InputPath)
		}
	case ".xp3":
		checker := parser.NewXP3EncryptionChecker(b.config.InputPath)
		if err := checker.RaiseIfEncrypted(); err != nil {
			return err
		}
	}

	return nil
}

// executeExtract はEXTRACTフェーズを実行する: XP3アーカイブを展開し、
// ゲーム構造を解析する。EXEファイルの場合は埋め込みXP3を抽出してから展開する。
func (b *BuildPipeline) executeExtract() error {
	extractDir, err := os.MkdirTemp("", "mnemonic_extract_")
	if err != nil {
		return fmt.Errorf("一時ディレクトリの作成に失敗しました: %w", err)
	}
	b.tempDirs = append(b.tempDirs, extractDir)
	b.extractDir = extractDir

	suffix := strings.ToLower(filepath.Ext(b.config.InputPath))

	if suffix == ".exe" {
		extractor, err := parser.NewEmbeddedXP3Extractor(b.config.InputPath)
		if err != nil {
			return err
		}

		xp3Files, err := extractor.ExtractAll(extractDir)
		if err != nil {
			return err
		}

		for _, xp3File := range xp3Files {
			archive, err := parser.NewXP3Archive(xp3File)
			if err != nil {
				return err
			}
			if err := archive.ExtractAll(extractDir); err != nil {
				return err
			}
		}
	} else {
		archive, err := parser.NewXP3Archive(b.config.InputPath)
		if err != nil {
			return err
		}
		if err := archive.ExtractAll(extractDir); err != nil {
			return err
		}
	}

	detector, err := parser.NewGameDetector(extractDir)
	if err != nil {
		return err
	}

	structure, err := detector.Detect()
	if err != nil {
		return err
	}
	b.gameStructure = &structure

	return nil
}

// executeConvert はCONVERTフェーズを実行する: 抽出されたアセットをAndroid
// 互換形式に変換する。まず全ファイルをコピーし（ゲームコアファイルを含む）、
// その後変換対象ファイルを変換（上書き）する。
func (b *BuildPipeline) executeConvert() error {
	if b.extractDir == "" {
		return errors.New("抽出フェーズが完了していません")
	}

	convertDir, err := os.MkdirTemp("", "mnemonic_convert_")
	if err != nil {
		return fmt.Errorf("一時ディレクトリの作成に失敗しました: %w", err)
	}
	b.tempDirs = append(b.tempDirs, convertDir)
	b.convertDir = convertDir

	if err := copyTree(b.extractDir, b.convertDir); err != nil {
		return err
	}

	converters := []converter.Converter{
		converter.NewEncodingConverter("", ""),
		converter.NewImageConverter(0, true),
	}
	if !b.config.SkipVideo {
		timeout := time.Duration(b.config.FFmpegTimeoutSeconds) * time.Second
		converters = append(converters, converter.NewVideoConverter("", "", "", timeout, nil))
	}

	manager := converter.NewConversionManager(converters, nil, 0, nil)

	// why not(PR4申し送り): converter.ConvertDirectoryはsourceDir（ここでは
	// extractDir）が存在しない場合にerrorを返す。Python版のConversionManager.
	// convert_directoryは同じ状況で空のConversionSummaryを返して成功扱いに
	// する既知の齟齬がある（PR4レビュー参照）。extractDirは直前のEXTRACTフェーズが
	// os.MkdirTempで必ず作成しているため、通常の実行経路でこのエラー分岐に
	// 到達することはない。到達するとすればextractDirが実行中に消失した異常系
	// であり、そのケースをPython版のように空サマリーで握りつぶさず、CONVERT
	// フェーズの失敗として明示的に報告する（「エラーはerrorとして呼び出し元へ
	// 伝播する」という他フェーズと同じ契約を踏襲する方が、黙って空の変換結果を
	// 返すより安全なため、Python版の挙動をあえて踏襲しない）。
	if _, err := manager.ConvertDirectory(b.extractDir, b.convertDir, true); err != nil {
		return fmt.Errorf("アセット変換に失敗しました: %w", err)
	}

	return nil
}

// executeBuild はBUILDフェーズを実行する: Gradleビルドを使用してAPKを
// 生成する。テンプレートを展開し、ゲームファイルをassetsに配置してビルドする。
func (b *BuildPipeline) executeBuild() error {
	if b.convertDir == "" {
		return errors.New("変換フェーズが完了していません")
	}

	projectDir, err := os.MkdirTemp("", "mnemonic_project_")
	if err != nil {
		return fmt.Errorf("一時ディレクトリの作成に失敗しました: %w", err)
	}
	b.tempDirs = append(b.tempDirs, projectDir)
	b.projectDir = projectDir

	templatePath, err := b.resolveTemplate()
	if err != nil {
		return err
	}

	if err := extractTemplateZip(templatePath, projectDir); err != nil {
		return err
	}

	baseName := strings.TrimSuffix(filepath.Base(b.config.InputPath), filepath.Ext(b.config.InputPath))
	if b.gameStructure != nil && b.gameStructure.Title != "" {
		baseName = b.gameStructure.Title
	}

	packageName := b.config.PackageName
	if packageName == "" {
		packageName = "com.krkr." + b.sanitizeName(baseName)
	}

	appName := b.config.AppName
	if appName == "" {
		appName = baseName
	}

	preparer := builder.NewTemplatePreparer(projectDir)
	if err := preparer.Prepare(packageName, appName, b.convertDir, b.findGameIcon()); err != nil {
		return err
	}

	gradleTimeout := time.Duration(b.config.GradleTimeoutSeconds) * time.Second

	gradleBuilder, err := builder.NewGradleBuilder(projectDir, gradleTimeout, nil)
	if err != nil {
		return err
	}

	result, err := gradleBuilder.Build("release")
	if err != nil {
		return err
	}
	if !result.Success || result.APKPath == nil {
		return fmt.Errorf("%w: %s", ErrGradleAPKMissing, result.OutputLog)
	}

	b.unsignedAPK = *result.APKPath

	return nil
}

// resolveTemplate はキャッシュ済みテンプレートを解決する。キャッシュが無く
// オフラインモードでない場合はダウンロードしてキャッシュへ保存する。
func (b *BuildPipeline) resolveTemplate() (string, error) {
	cacheManager := builder.NewDefaultCacheManager()
	templateCache := builder.NewTemplateCache(cacheManager, b.config.TemplateRefreshDays)

	templatePath, ok := templateCache.GetCachedTemplate(b.config.TemplateVersion)

	if !ok && !b.config.TemplateOffline {
		downloader := builder.NewTemplateDownloader("", nil)

		downloaded, err := downloader.Download(b.config.TemplateVersion)
		if err != nil {
			return "", err
		}

		version := "latest"
		if b.config.TemplateVersion != nil {
			version = *b.config.TemplateVersion
		}

		if _, err := templateCache.SaveTemplate(downloaded, version); err != nil {
			return "", err
		}

		templatePath, ok = templateCache.GetCachedTemplate(b.config.TemplateVersion)
	}

	if !ok {
		return "", errors.New("テンプレートが利用できません。オンラインモードで再実行してください。")
	}

	return templatePath, nil
}

// executeSign はSIGNフェーズを実行する: ビルドされたAPKにzipalignを適用し、
// キーストア指定時は署名鍵で、未指定時はデバッグ鍵で署名を行う。
func (b *BuildPipeline) executeSign() error {
	if b.unsignedAPK == "" {
		return errors.New("ビルドフェーズが完了していません")
	}

	outputPath := b.config.OutputPath
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	zipaligner := signer.NewDefaultZipalignRunner(nil)
	alignedAPK := withSuffix(outputPath, ".aligned.apk")

	if _, err := zipaligner.Align(b.unsignedAPK, alignedAPK); err != nil {
		return err
	}

	keystoreConfig, err := b.resolveKeystoreConfig()
	if err != nil {
		return err
	}

	if err := copyFile(alignedAPK, outputPath); err != nil {
		return fmt.Errorf("署名前APKのコピーに失敗しました: %w", err)
	}

	apkSigner := signer.NewDefaultApkSignerRunner(nil)
	if _, err := apkSigner.Sign(outputPath, keystoreConfig); err != nil {
		return err
	}

	_ = os.Remove(alignedAPK)

	return nil
}

// resolveKeystoreConfig は署名に使うキーストア設定を決定する。
//
// KeystorePath指定時は環境変数MNEMONIC_KEYSTORE_PASS（未設定なら対話的
// 入力）からパスワードを取得する。未指定時はデバッグ用キーストアを生成する
// （PR6の申し送り: 非対話用途は環境変数、対話用途は端末入力という現行方針を
// そのままCLI経由の実行にも適用する）。
func (b *BuildPipeline) resolveKeystoreConfig() (signer.KeystoreConfig, error) {
	if b.config.KeystorePath == "" {
		debugKeystore, err := b.createDebugKeystore()
		if err != nil {
			return signer.KeystoreConfig{}, err
		}

		return signer.KeystoreConfig{
			KeystorePath:     debugKeystore,
			KeyAlias:         "debug",
			KeystorePassword: "android",
		}, nil
	}

	provider := signer.DefaultPasswordProvider{}

	password, ok := provider.GetPasswordFromEnv("")
	if !ok {
		pw, err := provider.GetPassword("")
		if err != nil {
			return signer.KeystoreConfig{}, err
		}
		password = pw
	}

	return signer.KeystoreConfig{
		KeystorePath:     b.config.KeystorePath,
		KeyAlias:         "key",
		KeystorePassword: password,
	}, nil
}
