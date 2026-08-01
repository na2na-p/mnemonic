package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// javaSourceGenerator はfork版KirikiriSDL2Activity.javaを起点に、パッケージ名
// 書き換えのみの素通しファイルと、mnemonic独自機能（アセットコピー等）を
// 実装するKirikiriSDL2GameActivityサブクラスを生成する。
type javaSourceGenerator struct {
	projectDir string
}

// newJavaSourceGenerator はjavaSourceGeneratorを初期化する。
func newJavaSourceGenerator(projectDir string) *javaSourceGenerator {
	return &javaSourceGenerator{projectDir: projectDir}
}

// forkJavaSourceRelPath はCIがfork krkrsdl2からテンプレートzipへ梱包する
// KirikiriSDL2Activity.javaの、projectDirからの相対パス。
// テンプレート展開後・Generate呼び出し前の時点で存在している前提。
var forkJavaSourceRelPath = filepath.Join("app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2", "KirikiriSDL2Activity.java")

// Generate はfork版KirikiriSDL2Activity.javaを起点にパッケージ名の書き換えのみを
// 行った素通しファイルと、mnemonic独自機能（アセットコピー等）を実装する
// KirikiriSDL2GameActivityサブクラスの2ファイルを、対象パッケージの
// ディレクトリへ出力する。
//
// why not（fork版ファイルへの独自メンバ直接注入をやめた理由）: fork側が
// 独自にonCreateをオーバーライドするようになった場合（krkrsdl2 fork側で
// WindowInsetsリスナー登録のため実際に追加された）、生成後のクラスに
// onCreateが2つ定義されjavacのメソッド二重定義エラーになる。
// KirikiriSDL2Activityをextendsする別クラスへmnemonic独自メンバを分離する
// ことで、両ファイルのメソッド一覧が重複しない構造にする。
//
// why not（fork版ファイルそのものを読み込んで変換する理由）: Go定数への
// 全文手動移植は、fork側でメソッドが追加/変更されても追従できず、JNI経由で
// 呼ばれるメソッド（showSelectList等）がsilentに欠落する構造的リスクを
// 常に抱える。fork版ファイルそのものを読み込んで変換する方式にすることで、
// fork側の変更が自動的に反映されるようにする。
//
// why not（削除と書き込みの順序）: 生成物の書き込みを終えてから
// oldJavaDir（fork版ファイルの元ディレクトリ）を削除すると、packageNameが
// "pw.uyjulian.krkrsdl2"自身またはその配下を指すケースでjavaDirと
// oldJavaDirが同一/包含関係になり、書いたばかりの生成物ごと削除されて
// しまう（エラーは返らずファイルだけが消える）。fork版ソースは削除前に
// メモリへ読み込み済みであるため、oldJavaDirの削除を生成物の書き込みより
// 先に行っても情報は失われない。この順序（読み込み→削除→書き込み）に
// することで、packageNameの値に関わらず安全にする。
func (g *javaSourceGenerator) Generate(packageName string) error {
	packagePath := strings.ReplaceAll(packageName, ".", "/")
	javaDir := filepath.Join(g.projectDir, "app", "src", "main", "java", packagePath)

	forkJavaFile := filepath.Join(g.projectDir, forkJavaSourceRelPath)

	forkSource, err := os.ReadFile(forkJavaFile) //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("%w: fork版KirikiriSDL2Activity.javaが見つかりません: %s: %w", ErrTemplatePreparer, forkJavaFile, err)
	}

	forkContent, err := generateActivityJava(string(forkSource), packageName)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	gameActivityContent, err := generateGameActivityJava(packageName)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	// fork版ソースはここまでで読み込み済みのため、oldJavaDirの削除を
	// javaDirの作成・書き込みより先に行ってよい（package書き換え後の
	// パスがoldJavaDirと同一/包含関係になるケースへの対処。上のwhy not参照）。
	oldJavaDir := filepath.Join(g.projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2")
	if err := os.RemoveAll(oldJavaDir); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	if err := os.MkdirAll(javaDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	javaFile := filepath.Join(javaDir, forkActivityClassName+".java")
	if err := os.WriteFile(javaFile, []byte(forkContent), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	gameActivityFile := filepath.Join(javaDir, gameActivityClassName+".java")
	if err := os.WriteFile(gameActivityFile, []byte(gameActivityContent), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}

// forkActivityClassName はfork版KirikiriSDL2Activity.javaのクラス名
// （出力ファイル名にも使う）。fork krkrsdl2リポジトリ側のファイル名と
// 結びついているため、fork側で変わらない前提を他の箇所（ファイル名決め打ち等）
// でも既に置いている。
const forkActivityClassName = "KirikiriSDL2Activity"

// gameActivityClassName はforkActivityClassNameをextendsする、mnemonic独自の
// アセットコピー・起動引数設定を実装するサブクラスのクラス名
// （出力ファイル名にも使う）。ゲーム固有の名前やmnemonic固有の名前を含めない
// 汎用名にすることで、生成元プロジェクトに依存しない安定した命名にする。
const gameActivityClassName = "KirikiriSDL2GameActivity"

// mnemonicJavaImports はKirikiriSDL2GameActivity.javaが要求するimport群。
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

// gameActivityMembers はKirikiriSDL2GameActivityのクラス本体
// （フィールド/メソッド）。gameActivityClassJavadoc直後のクラス宣言に続けて
// そのまま出力される。
const gameActivityMembers = `
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

// javaPackageNamePattern はJavaパッケージ名として妥当な文字列にマッチする。
var javaPackageNamePattern = regexp.MustCompile(`^[A-Za-z_][\w.]*$`)

// generateActivityJava はfork版KirikiriSDL2Activity.javaのソース(forkSource)を
// パッケージ名の書き換えのみで素通しする。
//
// why not: mnemonic独自メンバ（アセットコピー等）をここに注入する方式は、
// fork側が独自にonCreateをオーバーライドした場合（krkrsdl2 fork側で
// WindowInsetsリスナー登録のため実際に追加された）にjavacのメソッド
// 二重定義エラーを起こす。mnemonic独自機能はgenerateGameActivityJavaが
// 生成する別クラス（forkActivityClassNameをextendsするサブクラス）へ分離し、
// このファイルはfork側の変更をそのまま反映する素通し出力に徹する。
func generateActivityJava(forkSource, packageName string) (string, error) {
	if !javaPackageDeclPattern.MatchString(forkSource) {
		return "", fmt.Errorf("%w: fork版Javaソースにpackage宣言が見つかりません", ErrTemplatePreparer)
	}

	return javaPackageDeclPattern.ReplaceAllString(forkSource, fmt.Sprintf("package %s;", packageName)), nil
}

// gameActivityClassJavadoc はKirikiriSDL2GameActivityクラスのクラスレベル
// docコメント。
//
// why not（サブクラス化）: mnemonic独自のonCreate等をfork版
// KirikiriSDL2Activity.java（パッケージ名書き換えのみで素通し出力される。
// generateActivityJava参照）へ直接注入すると、fork側が独自にonCreateを
// オーバーライドした場合にjavacのメソッド二重定義エラーになる。onCreateは
// アセットコピー後にsuper.onCreate()を呼ぶ構成にすることで、fork側の
// onCreate（存在する場合）へ連鎖させる。
//
// why not（JNI互換性）: krkrsdl2ネイティブはSDL_AndroidGetActivity()で
// 得たjobjectをGetObjectClassに渡して実行時クラス（このサブクラス）を
// 解決し、そのクラスに対しGetStaticMethodID("showSelectList", ...)等を
// 呼ぶ。showSelectList/setOrientationBis等はスーパークラスである
// KirikiriSDL2Activity（fork版）にのみ定義されているが、JNI仕様上
// GetMethodID/GetStaticMethodIDは指定したクラスだけでなくスーパークラスの
// 継承済みメンバも解決対象に含むため、サブクラス化してもJNI経由の呼び出しは
// 壊れない。
const gameActivityClassJavadoc = `/**
 * KirikiriSDL2用のメインアクティビティ
 *
 * アプリ起動時にassets/data/配下のゲームファイルを
 * 内部ストレージにコピーしてkrkrsdl2が読み込めるようにする。
 */
`

// generateGameActivityJava はKirikiriSDL2GameActivity.javaの完全なソースを
// 生成する。フィールド/メソッドはmnemonic独自の固定内容であるため、
// fork版ソースを読み込む必要はない。
func generateGameActivityJava(packageName string) (string, error) {
	if !javaPackageNamePattern.MatchString(packageName) {
		return "", fmt.Errorf("%w: パッケージ名が不正です: %s", ErrTemplatePreparer, packageName)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "package %s;\n\n", packageName)
	b.WriteString(strings.Join(mnemonicJavaImports, "\n"))
	b.WriteString("\n\n")
	b.WriteString(gameActivityClassJavadoc)
	fmt.Fprintf(&b, "public class %s extends %s {\n", gameActivityClassName, forkActivityClassName)
	b.WriteString(gameActivityMembers)
	b.WriteString("}\n")

	return b.String(), nil
}
