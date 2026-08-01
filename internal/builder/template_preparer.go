package builder

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// センチネルエラー群。
var (
	// ErrTemplatePreparer はテンプレート準備に関する基本エラー。
	ErrTemplatePreparer = errors.New("テンプレートの準備に失敗しました")
	// ErrJniLibsNotFound はJNIライブラリが見つからない場合のエラー。
	ErrJniLibsNotFound = errors.New("JNIライブラリが見つかりません")
	// ErrSDL2SourceFetch はSDL2 Javaソースの取得に失敗した場合のエラー。
	ErrSDL2SourceFetch = errors.New("SDL2 Javaソースの取得に失敗しました")
)

// TemplatePreparer関連の定数。
const (
	// TargetSDKVersion は推奨ターゲットSDKバージョン。
	TargetSDKVersion = 34
	// CompileSDKVersion は推奨コンパイルSDKバージョン。
	CompileSDKVersion = 34
	// MinSDKVersion は推奨最小SDKバージョン。
	MinSDKVersion = 21
)

// supportedABIs はサポートするABI一覧。
var supportedABIs = map[string]struct{}{
	"arm64-v8a":   {},
	"armeabi-v7a": {},
	"x86":         {},
	"x86_64":      {},
}

// TemplatePreparer はAndroidプロジェクトテンプレートを準備する。
//
// 以下の処理を行う:
//  1. krkrsdl2_universal.apkから.soファイルを抽出してjniLibsに配置
//  2. SDL2 Javaソースをダウンロードして配置
//  3. krkrsdl2プラグイン(.so)をjniLibsに配置
//  4. KirikiriSDL2Activity.javaをassetsコピー機能付きに置き換え
//  5. app/build.gradleを更新（targetSdkVersion=34、namespace追加）
//  6. AndroidManifest.xmlを更新（android:exported="true"追加）
//  7. res/values/strings.xmlを作成（app_name設定）
type TemplatePreparer struct {
	projectDir string
	sdl2Cache  *SDL2SourceCache
}

// NewTemplatePreparer はTemplatePreparerを初期化する。
// sdl2CacheがnilでもSDL2 Javaソースの取得自体は行われる（キャッシュ未使用で
// 毎回ダウンロードする）。
func NewTemplatePreparer(projectDir string, sdl2Cache *SDL2SourceCache) *TemplatePreparer {
	return &TemplatePreparer{projectDir: projectDir, sdl2Cache: sdl2Cache}
}

// Prepare はテンプレートを準備する。
// assetsDir/iconPathが空文字列の場合、対応する処理はスキップされる
// （iconPathはファイルが存在しない場合もスキップされ、代わりにデフォルト
// アイコンを生成する）。pluginsInfoがnilの場合、プラグインのjniLibs配置は
// スキップされる（呼び出し元が事前にPluginFetcherで取得したものを渡す
// 想定。why not: テストが実ネットワークに触れないようにするため、
// pluginsInfo=nilの場合に自前でダウンロードを試みるフォールバックは
// 持たない。呼び出し元（internal/pipeline）が明示的にfetchPlugins()の
// 結果を渡す設計に一本化した）。
func (p *TemplatePreparer) Prepare(packageName, appName, assetsDir, iconPath string, pluginsInfo *PluginsInfo) error {
	if err := p.extractJNILibs(); err != nil {
		return err
	}

	if err := p.fetchSDL2Sources(); err != nil {
		return err
	}

	if err := p.copyPluginsToJNILibs(pluginsInfo); err != nil {
		return err
	}

	if err := p.updateJavaSource(packageName); err != nil {
		return err
	}

	if err := p.updateBuildGradle(packageName); err != nil {
		return err
	}

	if err := p.updateManifest(); err != nil {
		return err
	}

	if err := p.updateStringsXML(appName); err != nil {
		return err
	}

	if assetsDir != "" {
		if err := p.copyAssets(assetsDir); err != nil {
			return err
		}
	}

	if iconPath != "" {
		if _, err := os.Stat(iconPath); err == nil {
			return p.updateIcon(iconPath)
		}
	}

	return p.createDefaultIcon()
}

// fetchSDL2Sources はSDL2のJavaソースファイル（SDLActivity.java等）を取得して
// 配置する。キャッシュが有効な場合はキャッシュから復元する。
func (p *TemplatePreparer) fetchSDL2Sources() error {
	javaDir := filepath.Join(p.projectDir, "app", "src", "main", "java")
	if err := os.MkdirAll(javaDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	fetcher := NewSDL2SourceFetcher(0, p.sdl2Cache)
	if err := fetcher.Fetch(javaDir); err != nil {
		return fmt.Errorf("%w: %w: %w", ErrTemplatePreparer, ErrSDL2SourceFetch, err)
	}

	return nil
}

// copyPluginsToJNILibs はkrkrsdl2プラグインをjniLibsディレクトリにコピーする。
//
// プラグイン(.so)を各ABI用のjniLibsディレクトリに配置する。jniLibsに配置
// されたプラグインはAPKビルド時に自動的にlib/{abi}/配下に含まれ、
// System.loadLibraryで読み込み可能になる。スクリプト変換時にlibプレフィックス
// 付きのフルファイル名を指定するため、libプレフィックス付きのファイルのみ
// 配置すれば良い。pluginsInfoがnilの場合は何も行わない（Prepareのdocコメント
// 参照）。プラグインファイルが見つからない場合はスキップする
// （ベストエフォートとしてエラーを握りつぶす）。
func (p *TemplatePreparer) copyPluginsToJNILibs(pluginsInfo *PluginsInfo) error {
	if pluginsInfo == nil {
		return nil
	}

	jniLibsDir := filepath.Join(p.projectDir, "app", "src", "main", "jniLibs")

	for _, abi := range SupportedABIs {
		abiDir := filepath.Join(jniLibsDir, abi)
		if err := os.MkdirAll(abiDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		for _, srcPath := range pluginsInfo.GetAllPathsForABI(abi) {
			if _, err := os.Stat(srcPath); err != nil {
				continue
			}

			destPath := filepath.Join(abiDir, filepath.Base(srcPath))
			if err := copyFile(srcPath, destPath); err != nil {
				return fmt.Errorf("%w: プラグインのコピーに失敗しました: %s: %w", ErrTemplatePreparer, srcPath, err)
			}
		}
	}

	return nil
}

// extractJNILibs はkrkrsdl2_universal.apkから.soファイルを抽出する。
func (p *TemplatePreparer) extractJNILibs() error {
	baseAPK := filepath.Join(p.projectDir, "krkrsdl2_universal.apk")
	if _, err := os.Stat(baseAPK); err != nil {
		return fmt.Errorf("%w: %w: ベースAPKが見つかりません: %s", ErrTemplatePreparer, ErrJniLibsNotFound, baseAPK)
	}

	jniLibsDir := filepath.Join(p.projectDir, "app", "src", "main", "jniLibs")
	if err := os.MkdirAll(jniLibsDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	zr, err := zip.OpenReader(baseAPK)
	if err != nil {
		return fmt.Errorf("%w: 無効なAPKファイルです: %s", ErrTemplatePreparer, baseAPK)
	}
	defer func() { _ = zr.Close() }()

	extracted := 0

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "lib/") || !strings.HasSuffix(f.Name, ".so") {
			continue
		}

		parts := strings.Split(f.Name, "/")
		if len(parts) < 3 {
			continue
		}

		abi := parts[1]
		soName := parts[2]

		if _, ok := supportedABIs[abi]; !ok {
			continue
		}

		destDir := filepath.Join(jniLibsDir, abi)
		if err := os.MkdirAll(destDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		if err := extractZipFileEntry(f, filepath.Join(destDir, soName)); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("%w: APK内に.soファイルが見つかりません: %s", ErrJniLibsNotFound, baseAPK)
	}

	return nil
}

func extractZipFileEntry(f *zip.File, destPath string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // ABI名はsupportedABIsで許可リスト検証済みのため妥当
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil { //nolint:gosec // ベースAPKは信頼済みのビルド成果物でありサイズ上限は設けない
		return err
	}

	return nil
}

// forkJavaSourceRelPath はCIがfork krkrsdl2からテンプレートzipへ梱包する
// KirikiriSDL2Activity.javaの、projectDirからの相対パス。
// テンプレート展開後・updateJavaSource呼び出し前の時点で存在している前提。
var forkJavaSourceRelPath = filepath.Join("app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2", "KirikiriSDL2Activity.java")

// updateJavaSource はfork版KirikiriSDL2Activity.javaを起点に、パッケージ名の
// 書き換えとmnemonic独自メンバの注入を行い、対象パッケージのディレクトリへ
// 出力する。
//
// why not: Go定数への全文手動移植は、fork側でメソッドが追加/変更されても
// 追従できず、JNI経由で呼ばれるメソッド（showSelectList等）がsilentに
// 欠落する構造的リスクを常に抱える。fork版ファイルそのものを読み込んで
// 変換する方式にすることで、fork側の変更が自動的に反映されるようにする。
//
// why not（削除と書き込みの順序）: 生成物の書き込みを終えてから
// oldJavaDir（fork版ファイルの元ディレクトリ）を削除すると、packageNameが
// "pw.uyjulian.krkrsdl2"自身またはその配下を指すケースでjavaDirと
// oldJavaDirが同一/包含関係になり、書いたばかりの生成物ごと削除されて
// しまう（エラーは返らずファイルだけが消える）。fork版ソースは削除前に
// メモリへ読み込み済みであるため、oldJavaDirの削除を生成物の書き込みより
// 先に行っても情報は失われない。この順序（読み込み→削除→書き込み）に
// することで、packageNameの値に関わらず安全にする。
func (p *TemplatePreparer) updateJavaSource(packageName string) error {
	packagePath := strings.ReplaceAll(packageName, ".", "/")
	javaDir := filepath.Join(p.projectDir, "app", "src", "main", "java", packagePath)

	forkJavaFile := filepath.Join(p.projectDir, forkJavaSourceRelPath)

	forkSource, err := os.ReadFile(forkJavaFile) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: fork版KirikiriSDL2Activity.javaが見つかりません: %s: %w", ErrTemplatePreparer, forkJavaFile, err)
	}

	javaContent, err := generateActivityJava(string(forkSource), packageName)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	// fork版ソースはここまでで読み込み済みのため、oldJavaDirの削除を
	// javaDirの作成・書き込みより先に行ってよい（package書き換え後の
	// パスがoldJavaDirと同一/包含関係になるケースへの対処。上のwhy not参照）。
	oldJavaDir := filepath.Join(p.projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2")
	if err := os.RemoveAll(oldJavaDir); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	if err := os.MkdirAll(javaDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	javaFile := filepath.Join(javaDir, "KirikiriSDL2Activity.java")
	if err := os.WriteFile(javaFile, []byte(javaContent), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// mnemonicJavaImports はfork版ソースには存在しない、mnemonic独自機能
// （アセットコピー等）が要求するimport群。
var mnemonicJavaImports = []string{
	"import android.os.Bundle;",
	"import android.content.pm.ApplicationInfo;",
	"import android.content.res.AssetManager;",
	"import android.util.Log;",
	"import java.io.File;",
	"import java.io.FileOutputStream;",
	"import java.io.IOException;",
	"import java.io.InputStream;",
	"import java.io.OutputStream;",
}

// mnemonicActivityMembers はmnemonic独自のクラスメンバ（フィールド/メソッド）。
// fork版KirikiriSDL2Activity.javaのクラス閉じ括弧の直前に注入される。
const mnemonicActivityMembers = `
    private static final String TAG = "KirikiriSDL2";
    private static final String ASSETS_DATA_DIR = "data";
    private static String sNativeLibDir = null;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        // ネイティブライブラリのディレクトリを保存
        sNativeLibDir = getApplicationInfo().nativeLibraryDir;
        Log.i(TAG, "Native library directory: " + sNativeLibDir);

        copyAssetsToInternal();
        super.onCreate(savedInstanceState);
    }

    /**
     * krkrsdl2に渡すコマンドライン引数を設定する
     * - プラグイン検索パス: ネイティブライブラリディレクトリを指定
     * - holdalpha: アルファチャンネル保持を有効化（透過表示の互換性向上）
     * - cpuXXX=no: SIMD最適化を無効化してC実装を使用
     *   ARMデバイスではSIMDe経由のSSE2エミュレーションに問題があるため、
     *   純粋なC実装のブレンド関数を使用することでアルファブレンディングの
     *   互換性を確保する
     */
    @Override
    protected String[] getArguments() {
        if (sNativeLibDir != null) {
            Log.i(TAG, "Setting plugin search path: " + sNativeLibDir);
            return new String[]{
                "-krkrsdl2_pluginsearchpath=" + sNativeLibDir,
                "-holdalpha=yes",
                "-cpummx=no",
                "-cpusse=no",
                "-cpusse2=no"
            };
        }
        return new String[]{
            "-holdalpha=yes",
            "-cpummx=no",
            "-cpusse=no",
            "-cpusse2=no"
        };
    }

    /**
     * assets/data/配下のファイルを内部ストレージにコピーする
     * 既存ファイルはスキップする（初回のみコピー）
     */
    private void copyAssetsToInternal() {
        AssetManager assetManager = getAssets();
        File destDir = getFilesDir();

        try {
            copyAssetFolder(assetManager, ASSETS_DATA_DIR, destDir);
            Log.i(TAG, "Assets copied to: " + destDir.getAbsolutePath());
        } catch (IOException e) {
            Log.e(TAG, "Failed to copy assets", e);
        }
    }

    /**
     * アセットフォルダを再帰的にコピーする
     *
     * @param assetManager アセットマネージャー
     * @param srcPath コピー元のアセットパス
     * @param destDir コピー先のディレクトリ
     * @throws IOException コピーに失敗した場合
     */
    private void copyAssetFolder(AssetManager assetManager, String srcPath, File destDir)
            throws IOException {
        String[] files = assetManager.list(srcPath);
        if (files == null || files.length == 0) {
            // ファイルの場合
            copyAssetFile(assetManager, srcPath, destDir);
            return;
        }

        // ディレクトリの場合
        for (String file : files) {
            String srcFilePath = srcPath + "/" + file;
            File destFile = new File(destDir, file);

            String[] subFiles = assetManager.list(srcFilePath);
            if (subFiles != null && subFiles.length > 0) {
                // サブディレクトリ
                destFile.mkdirs();
                copyAssetFolder(assetManager, srcFilePath, destFile);
            } else {
                // ファイル
                copyAssetFile(assetManager, srcFilePath, destDir);
            }
        }
    }

    /**
     * アセットファイルを単一コピーする
     * 既に存在するファイルはスキップする
     *
     * @param assetManager アセットマネージャー
     * @param srcPath コピー元のアセットパス
     * @param destDir コピー先のディレクトリ
     * @throws IOException コピーに失敗した場合
     */
    private void copyAssetFile(AssetManager assetManager, String srcPath, File destDir)
            throws IOException {
        String fileName = srcPath.contains("/")
                ? srcPath.substring(srcPath.lastIndexOf("/") + 1)
                : srcPath;
        File destFile = new File(destDir, fileName);

        // 既存ファイルはスキップ
        if (destFile.exists()) {
            return;
        }

        destFile.getParentFile().mkdirs();

        try (InputStream in = assetManager.open(srcPath);
             OutputStream out = new FileOutputStream(destFile)) {
            byte[] buffer = new byte[8192];
            int read;
            while ((read = in.read(buffer)) != -1) {
                out.write(buffer, 0, read);
            }
        }
    }
`

// javaPackageDeclPattern はJavaソース先頭のpackage宣言にマッチする。
var javaPackageDeclPattern = regexp.MustCompile(`^package\s+[A-Za-z_][\w.]*;`)

// javaImportLinePattern はJavaのimport文1行にマッチする。
var javaImportLinePattern = regexp.MustCompile(`(?m)^import\s+[\w.]+;\s*$`)

// activityClassDeclPattern はKirikiriSDL2Activityのクラス宣言行にマッチする。
// クラス名はファイル名（KirikiriSDL2Activity.java）およびmnemonicActivityMembers
// が注入する独自メンバのシグネチャと結びついているため、このクラス名自体は
// fork側で変わらない前提を既に他の箇所（ファイル名決め打ち等）でも置いている。
// そのため、この文字列をJavadoc挿入位置のアンカーに使うことは既存の前提の
// 範囲内であり、追加のリスクにはならない。
var activityClassDeclPattern = regexp.MustCompile(`(?m)^public class KirikiriSDL2Activity extends SDLActivity \{`)

// activityClassJavadoc はKirikiriSDL2Activityクラスのクラスレベルdocコメント。
// mnemonic独自機能（起動時のアセットコピー）を説明する、mnemonic独自の記述。
const activityClassJavadoc = `/**
 * KirikiriSDL2用のメインアクティビティ
 *
 * アプリ起動時にassets/data/配下のゲームファイルを
 * 内部ストレージにコピーしてkrkrsdl2が読み込めるようにする。
 */
`

// generateActivityJava はfork版KirikiriSDL2Activity.javaのソース(forkSource)を
// 起点に、パッケージ名の書き換えとmnemonic独自メンバの注入を行う。
//
// why not: 注入アンカーとして「クラス宣言の直後」や「特定メソッドの直前」の
// ような相対位置を使うと、fork側のフィールド追加やコメント整形の変化で
// 注入位置がずれる恐れがある。本ファイルは単一のトップレベルpublicクラスの
// みを含むJavaファイルであるため、ファイル内で最後に現れる閉じ括弧は
// 必ずそのクラス自身の閉じ括弧になる。この位置をアンカーにすることで、
// fork側の内部的な変更（フィールド追加、コメント変更等）に対して頑健になる。
func generateActivityJava(forkSource, packageName string) (string, error) {
	if !javaPackageDeclPattern.MatchString(forkSource) {
		return "", fmt.Errorf("%w: fork版Javaソースにpackage宣言が見つかりません", ErrTemplatePreparer)
	}

	content := javaPackageDeclPattern.ReplaceAllString(forkSource, fmt.Sprintf("package %s;", packageName))
	content = injectMissingJavaImports(content, mnemonicJavaImports)
	content = injectActivityClassJavadoc(content)

	lastBrace := strings.LastIndex(content, "}")
	if lastBrace < 0 {
		return "", fmt.Errorf("%w: fork版Javaソースにクラスの閉じ括弧が見つかりません", ErrTemplatePreparer)
	}

	return content[:lastBrace] + mnemonicActivityMembers + content[lastBrace:], nil
}

// injectActivityClassJavadoc はKirikiriSDL2Activityのクラス宣言直前へ
// activityClassJavadocを挿入する（冪等: 既に挿入済み、またはアンカーとなる
// クラス宣言が見つからない場合は何もしない）。
func injectActivityClassJavadoc(content string) string {
	if strings.Contains(content, activityClassJavadoc) {
		return content
	}

	idx := activityClassDeclPattern.FindStringIndex(content)
	if idx == nil {
		return content
	}

	return content[:idx[0]] + activityClassJavadoc + content[idx[0]:]
}

// injectMissingJavaImports はcontent内にまだ存在しないimport文を、最後の
// import文の直後（import文が無い場合はpackage宣言の直後）へ追加する。
// 既に存在するimportは追加しない（冪等）。
func injectMissingJavaImports(content string, imports []string) string {
	var missing []string
	for _, imp := range imports {
		if !strings.Contains(content, imp) {
			missing = append(missing, imp)
		}
	}
	if len(missing) == 0 {
		return content
	}

	insertAt := len(content)
	if matches := javaImportLinePattern.FindAllStringIndex(content, -1); len(matches) > 0 {
		insertAt = matches[len(matches)-1][1]
	} else if idx := javaPackageDeclPattern.FindStringIndex(content); idx != nil {
		insertAt = idx[1]
	}

	insertion := "\n" + strings.Join(missing, "\n")

	return content[:insertAt] + insertion + content[insertAt:]
}

var (
	buildGradleNamespaceCheckPattern = regexp.MustCompile(`namespace`)
	androidBlockStartPattern         = regexp.MustCompile(`android\s*\{`)
	compileSdkVersionPattern         = regexp.MustCompile(`compileSdkVersion\s+\d+`)
	minSdkVersionPattern             = regexp.MustCompile(`minSdkVersion\s+\d+`)
	targetSdkVersionPattern          = regexp.MustCompile(`targetSdkVersion\s+\d+`)
	applicationIDCheckPattern        = regexp.MustCompile(`applicationId`)
	applicationIDValuePattern        = regexp.MustCompile(`applicationId\s+"[^"]+"`)
	cmakeExternalNativeBuildPattern  = regexp.MustCompile(`(?s)\s*externalNativeBuild\s*\{[^}]*cmake\s*\{[^}]*\}[^}]*\}`)
	ndkExternalNativeBuildPattern    = regexp.MustCompile(`(?s)\s*externalNativeBuild\s*\{[^}]*ndk\s*\{[^}]*\}[^}]*\}`)
	standaloneNdkAbiFiltersPattern   = regexp.MustCompile(`(?s)\s*ndk\s*\{[^}]*abiFilters[^}]*\}`)
)

// updateBuildGradle はapp/build.gradleを更新する
// （namespace追加、SDKバージョン更新、CMake設定の削除）。
func (p *TemplatePreparer) updateBuildGradle(packageName string) error {
	buildGradle := filepath.Join(p.projectDir, "app", "build.gradle")

	content, err := os.ReadFile(buildGradle) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: build.gradleが見つかりません: %s", ErrTemplatePreparer, buildGradle)
	}

	text := string(content)

	if !buildGradleNamespaceCheckPattern.MatchString(text) {
		text = androidBlockStartPattern.ReplaceAllStringFunc(text, func(match string) string {
			return match + fmt.Sprintf("\n    namespace \"%s\"", packageName)
		})
	}

	text = compileSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("compileSdkVersion %d", CompileSDKVersion))
	text = minSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("minSdkVersion %d", MinSDKVersion))
	text = targetSdkVersionPattern.ReplaceAllString(text, fmt.Sprintf("targetSdkVersion %d", TargetSDKVersion))

	if applicationIDCheckPattern.MatchString(text) {
		text = applicationIDValuePattern.ReplaceAllStringFunc(text, func(string) string {
			return fmt.Sprintf(`applicationId "%s"`, packageName)
		})
	}

	text = cmakeExternalNativeBuildPattern.ReplaceAllString(text, "")
	text = ndkExternalNativeBuildPattern.ReplaceAllString(text, "")
	text = standaloneNdkAbiFiltersPattern.ReplaceAllString(text, "")

	if err := os.WriteFile(buildGradle, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

var (
	manifestPackageAttrPattern = regexp.MustCompile(`\s*package="[^"]*"`)
	activityTagPattern         = regexp.MustCompile(`<activity[^>]*(?:>|/>)`)
	serviceTagPattern          = regexp.MustCompile(`<service[^>]*(?:>|/>)`)
	receiverTagPattern         = regexp.MustCompile(`<receiver[^>]*(?:>|/>)`)
	screenOrientationPattern   = regexp.MustCompile(`android:screenOrientation="[^"]*"`)
)

var applicationTagPattern = regexp.MustCompile(`<application[^>]*>`)

// ScreenOrientationSensorLandscape は起動activityに固定する画面向きの値。
//
// why not: 横向き固定（ゲーム画面が4:3のため横向きの方が大きく表示される）
// とし、plain landscapeではなくsensorLandscapeを選ぶ。寝転んでプレイする際に
// 上下逆さまに持ち替えても追従してほしい（180度反転）というユーザー要望を、
// 横向き固定を保ったまま満たすため。
const ScreenOrientationSensorLandscape = "sensorLandscape"

// updateManifest はAndroidManifest.xmlを更新する
// （package属性の削除、android:exported="true"の付与、
// android:extractNativeLibs="true"の付与、activityへの
// android:screenOrientation="sensorLandscape"の付与）。
func (p *TemplatePreparer) updateManifest() error {
	manifestPath := filepath.Join(p.projectDir, "app", "src", "main", "AndroidManifest.xml")

	content, err := os.ReadFile(manifestPath) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: AndroidManifest.xmlが見つかりません: %s", ErrTemplatePreparer, manifestPath)
	}

	text := string(content)
	text = manifestPackageAttrPattern.ReplaceAllString(text, "")

	// applicationタグにextractNativeLibs="true"を追加する。これにより
	// ネイティブライブラリがAPKから展開され、dlopen（krkrsdl2プラグインの
	// 動的読み込み）でアクセス可能になる。
	text = applicationTagPattern.ReplaceAllStringFunc(text, addExtractNativeLibsIfMissing)

	text = activityTagPattern.ReplaceAllStringFunc(text, func(tag string) string {
		return setScreenOrientation(addExportedIfMissing(tag))
	})
	text = serviceTagPattern.ReplaceAllStringFunc(text, addExportedIfMissing)
	text = receiverTagPattern.ReplaceAllStringFunc(text, addExportedIfMissing)

	if err := os.WriteFile(manifestPath, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// setScreenOrientation はactivityタグにandroid:screenOrientation属性を
// ScreenOrientationSensorLandscapeで設定する。既に属性が存在する場合は
// 値を置き換える（冪等）。
func setScreenOrientation(tag string) string {
	if screenOrientationPattern.MatchString(tag) {
		return screenOrientationPattern.ReplaceAllString(tag, fmt.Sprintf(`android:screenOrientation="%s"`, ScreenOrientationSensorLandscape))
	}

	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + fmt.Sprintf(` android:screenOrientation="%s"/>`, ScreenOrientationSensorLandscape)
	}

	return tag[:len(tag)-1] + fmt.Sprintf(` android:screenOrientation="%s">`, ScreenOrientationSensorLandscape)
}

// addExtractNativeLibsIfMissing はapplicationタグにandroid:extractNativeLibs
// 属性が無い場合"true"で追加する。
func addExtractNativeLibsIfMissing(tag string) string {
	if strings.Contains(tag, "android:extractNativeLibs") || !strings.HasSuffix(tag, ">") {
		return tag
	}

	return tag[:len(tag)-1] + ` android:extractNativeLibs="true">`
}

// addExportedIfMissing はタグにandroid:exported属性が無い場合"true"で追加する。
func addExportedIfMissing(tag string) string {
	if strings.Contains(tag, "android:exported") {
		return tag
	}

	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + ` android:exported="true"/>`
	}

	return tag[:len(tag)-1] + ` android:exported="true">`
}

// xmlAttrEscaper はXML属性値向けのエスケープ処理を行う。
//
// why not: 標準ライブラリのhtml.EscapeStringは"を&#34;、'を&#39;という
// 異なる数値文字参照でエスケープする。"を&quot;、'を&#x27;にエスケープする
// 独自の変換表を持つreplacerを自前で用意する。
var xmlAttrEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)

var stringsXMLAppNamePattern = regexp.MustCompile(`(<string name="app_name">)[^<]*(</string>)`)

// updateStringsXML はres/values/strings.xmlを作成/更新する。
func (p *TemplatePreparer) updateStringsXML(appName string) error {
	valuesDir := filepath.Join(p.projectDir, "app", "src", "main", "res", "values")
	if err := os.MkdirAll(valuesDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	stringsXML := filepath.Join(valuesDir, "strings.xml")
	escapedAppName := xmlAttrEscaper.Replace(appName)

	if content, err := os.ReadFile(stringsXML); err == nil { //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
		text := stringsXMLAppNamePattern.ReplaceAllStringFunc(string(content), func(string) string {
			return "<string name=\"app_name\">" + escapedAppName + "</string>"
		})
		if err := os.WriteFile(stringsXML, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		return nil
	}

	content := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">%s</string>
</resources>
`, escapedAppName)

	if err := os.WriteFile(stringsXML, []byte(content), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// copyAssets はゲームファイルをapp/src/main/assets/dataにコピーする（既存ファイルはマージ）。
func (p *TemplatePreparer) copyAssets(assetsDir string) error {
	destDir := filepath.Join(p.projectDir, "app", "src", "main", "assets", "data")
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	err := filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return err
		}

		return copyFile(path, destPath)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// iconMipmapDensities はアイコンを配置するmipmapディレクトリの解像度一覧。
var iconMipmapDensities = []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

// updateIcon はアプリアイコンを各解像度のmipmapディレクトリへコピーする。
func (p *TemplatePreparer) updateIcon(iconPath string) error {
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

// createDefaultIcon はデフォルトアイコンを生成する。
//
// アイコンが提供されない場合のフォールバックとして、単色の正方形アイコンを
// 各解像度で生成する。
func (p *TemplatePreparer) createDefaultIcon() error {
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
