package builder_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// minimalManifestXML はTemplatePreparerのフルパイプライン(Prepare)を通す
// テストで使う最小限の有効なAndroidManifest.xml。
const minimalManifestXML = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application></application>
</manifest>
`

// addMinimalManifest はprojectDirにminimalManifestXMLを書き込む。
// Prepare()はupdateManifestステップでAndroidManifest.xmlの存在を要求するため、
// 個別ステップ（.so抽出等）だけを検証したいテストでも完走できるようにする。
func addMinimalManifest(t *testing.T, projectDir string) {
	t.Helper()

	manifestDir := filepath.Join(projectDir, "app", "src", "main")
	require.NoError(t, os.MkdirAll(manifestDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(minimalManifestXML), 0o600))
}

// addMinimalBuildGradle はprojectDirに最小限のbuild.gradleを書き込む。
func addMinimalBuildGradle(t *testing.T, projectDir string) {
	t.Helper()

	appDir := filepath.Join(projectDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
}

// realForkKirikiriSDL2ActivityJava はfork krkrsdl2リポジトリ
// (android-project/app/src/main/java/pw/uyjulian/krkrsdl2/KirikiriSDL2Activity.java)
// の実際の内容そのもの（onCreateオーバーライドによるWindowInsetsリスナー
// 登録を含む）。テンプレートzipがprojectDir配下に展開した状態を模した
// フィクスチャとして使う（updateJavaSourceの入力契約の結合確認）。
const realForkKirikiriSDL2ActivityJava = `package pw.uyjulian.krkrsdl2;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.DialogInterface;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.view.DisplayCutout;
import android.view.View;
import android.view.WindowInsets;

import org.libsdl.app.SDLActivity;

public class KirikiriSDL2Activity extends SDLActivity {

    // Screen orientation

    /**
     * This can be overridden (see SDLActivity.setOrientationBis()).
     * SDLActivity.setOrientation() (called from native code via JNI) always
     * dispatches through this instance method rather than calling
     * setRequestedOrientation() itself, specifically so subclasses can
     * intervene -- overriding here, rather than setOrientation(), is the
     * documented hook point.
     *
     * Why not let SDL decide: setOrientationBis() calls
     * setRequestedOrientation(SCREEN_ORIENTATION_FULL_SENSOR) whenever
     * SDL_HINT_ORIENTATIONS isn't set, which silently overrides any
     * orientation lock declared in AndroidManifest.xml (e.g. a
     * build-injected android:screenOrientation="sensorLandscape") the
     * moment SDL starts up. When the Manifest declares an explicit
     * orientation for this Activity, keep it and skip SDL's override
     * entirely; only fall back to SDL's own heuristic (window aspect ratio
     * / SDL_HINT_ORIENTATIONS) when the Manifest leaves it unspecified.
     */
    @Override
    public void setOrientationBis(int w, int h, boolean resizable, String hint) {
        try {
            int manifestOrientation = getPackageManager()
                    .getActivityInfo(getComponentName(), 0).screenOrientation;
            if (manifestOrientation != ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED) {
                return;
            }
        } catch (PackageManager.NameNotFoundException ex) {
            ex.printStackTrace();
        }
        super.setOrientationBis(w, h, resizable, hint);
    }

    // Safe-area insets (cutout + system bars)

    private static volatile int[] cachedSafeAreaInsets = new int[]{0, 0, 0, 0};

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            getWindow().getDecorView().setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener() {
                @Override
                public WindowInsets onApplyWindowInsets(View v, WindowInsets insets) {
                    updateCachedSafeAreaInsets(insets);
                    return insets;
                }
            });
        }
    }

    private static void updateCachedSafeAreaInsets(WindowInsets insets) {
        int left = insets.getSystemWindowInsetLeft();
        int top = insets.getSystemWindowInsetTop();
        int right = insets.getSystemWindowInsetRight();
        int bottom = insets.getSystemWindowInsetBottom();

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            DisplayCutout cutout = insets.getDisplayCutout();
            if (cutout != null) {
                left = Math.max(left, cutout.getSafeInsetLeft());
                top = Math.max(top, cutout.getSafeInsetTop());
                right = Math.max(right, cutout.getSafeInsetRight());
                bottom = Math.max(bottom, cutout.getSafeInsetBottom());
            }
        }

        cachedSafeAreaInsets = new int[]{left, top, right, bottom};
    }

    /**
     * This method is called by SDL using JNI.
     * @return {left, top, right, bottom} safe-area insets in px
     */
    public static int[] getSafeAreaInsets() {
        return cachedSafeAreaInsets;
    }

    // Select-list dialog

    /** Result of the current select-list dialog. Also used for blocking the calling thread. */
    private static final int[] selectListSelection = new int[1];
    /**
     * Set alongside a notify() on selectListSelection, and only there --
     * guards against a spurious wakeup returning from wait() before
     * onDismiss (or the show()-failure fallback below) actually ran.
     */
    private static final boolean[] selectListDone = new boolean[1];

    /**
     * This method is called by SDL using JNI.
     * Shows an Android-native, scrollable, cancelable list dialog on the UI
     * thread and blocks the calling thread until an item is tapped or the
     * dialog is dismissed (back key, outside tap). This replaces
     * SDLActivity's own messagebox for list selection: that dialog lays
     * buttons out in a single non-scrolling horizontal row and hard-codes
     * setCancelable(false), which on-device pushes a handful of items'
     * cancel button off-screen and unreachable.
     * @param title dialog title
     * @param items list item labels
     * @return the tapped 0-based index, or -1 if cancelled
     */
    public static int showSelectList(final String title, final String[] items) {
        selectListSelection[0] = -1;
        selectListDone[0] = false;

        final Activity activity = (Activity) getContext();
        if (activity == null) {
            return -1;
        }

        // trigger Dialog creation on UI thread

        activity.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                // Building/showing the dialog can throw (e.g. the activity
                // is finishing and the window token is no longer valid) --
                // unlike SDLActivity's own messagebox, this must not let
                // that exception skip the notify(), or the calling thread
                // (already inside wait()) would hang forever with no
                // dialog on screen to eventually dismiss it.
                try {
                    AlertDialog.Builder builder = new AlertDialog.Builder(activity);
                    builder.setTitle(title);
                    builder.setItems(items, new DialogInterface.OnClickListener() {
                        @Override
                        public void onClick(DialogInterface dialog, int which) {
                            selectListSelection[0] = which;
                        }
                    });
                    builder.setCancelable(true);
                    builder.setOnCancelListener(new DialogInterface.OnCancelListener() {
                        @Override
                        public void onCancel(DialogInterface dialog) {
                            selectListSelection[0] = -1;
                        }
                    });

                    AlertDialog dialog = builder.create();
                    dialog.setOnDismissListener(new DialogInterface.OnDismissListener() {
                        @Override
                        public void onDismiss(DialogInterface unused) {
                            synchronized (selectListSelection) {
                                selectListDone[0] = true;
                                selectListSelection.notify();
                            }
                        }
                    });
                    dialog.show();
                } catch (Exception ex) {
                    ex.printStackTrace();
                    synchronized (selectListSelection) {
                        selectListSelection[0] = -1;
                        selectListDone[0] = true;
                        selectListSelection.notify();
                    }
                }
            }
        });

        // block the calling thread

        synchronized (selectListSelection) {
            while (!selectListDone[0]) {
                try {
                    selectListSelection.wait();
                } catch (InterruptedException ex) {
                    ex.printStackTrace();
                    return -1;
                }
            }
        }

        // return selected value

        return selectListSelection[0];
    }
}
`

// addForkJavaSource はprojectDir配下に、CIがfork krkrsdl2から梱包する
// テンプレートzipが展開済みであることを模して、fork版
// KirikiriSDL2Activity.javaをapp/src/main/java/pw/uyjulian/krkrsdl2/へ
// 配置する。updateJavaSourceはこのファイルの存在を前提とする。
func addForkJavaSource(t *testing.T, projectDir string) {
	t.Helper()

	forkJavaDir := filepath.Join(projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2")
	require.NoError(t, os.MkdirAll(forkJavaDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(forkJavaDir, "KirikiriSDL2Activity.java"), []byte(realForkKirikiriSDL2ActivityJava), 0o600))
}

// testSDL2Cache はSDL2 Javaソース一式を持つ、有効なSDL2SourceCacheを返す。
//
// why: TemplatePreparer.Prepareはfetch_sdl2_sources()を無条件に呼ぶため、
// 実ネットワークに触れずにテストを完走させるには、事前に有効なキャッシュを
// 用意してSDL2SourceFetcher.Fetchがキャッシュ復元経路(Cache.RestoreTo)を
// 通るようにする必要がある（実ネットワークへのアクセス禁止という制約への
// 対応。builder.SDL2SourceCache.Save/IsValidの正規経路を使う）。
func testSDL2Cache(t *testing.T) *builder.SDL2SourceCache {
	t.Helper()

	sourcesDir := t.TempDir()
	appDir := filepath.Join(sourcesDir, "org", "libsdl", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o750))

	for _, name := range builder.SDL2RequiredFiles {
		require.NoError(t, os.WriteFile(filepath.Join(appDir, name), []byte("dummy content"), 0o600))
	}

	cache := builder.NewSDL2SourceCache(t.TempDir())
	require.NoError(t, cache.Save(sourcesDir))

	return cache
}

func writeZipFile(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // テスト用の固定パス
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

func TestTemplatePreparer_Prepare(t *testing.T) {
	t.Parallel()

	t.Run("正常系: すべての処理が成功するケース", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"lib/arm64-v8a/libmain.so":   []byte("fake so content"),
			"lib/armeabi-v7a/libmain.so": []byte("fake so content"),
		})

		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte(`
android {
    compileSdkVersion 30
    defaultConfig {
        applicationId "pw.uyjulian.krkrsdl2"
        minSdkVersion 16
        targetSdkVersion 30
    }
}
`), 0o600))

		manifestDir := filepath.Join(appDir, "src", "main")
		require.NoError(t, os.MkdirAll(manifestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`), 0o600))
		addForkJavaSource(t, projectDir)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		jniLibsDir := filepath.Join(projectDir, "app", "src", "main", "jniLibs")
		assert.FileExists(t, filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		assert.FileExists(t, javaFile)

		stringsXML := filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml")
		content, err := os.ReadFile(stringsXML)
		require.NoError(t, err)
		assert.Contains(t, string(content), "My Game")

		// デフォルトアイコンが生成されていることを確認（icon_path未指定のため）
		resDir := filepath.Join(projectDir, "app", "src", "main", "res")
		assert.FileExists(t, filepath.Join(resDir, "mipmap-mdpi", "ic_launcher.png"))
	})

	t.Run("異常系: APKファイルが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrJniLibsNotFound)
		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "ベースAPKが見つかりません")
	})
}

func TestTemplatePreparer_ExtractJNILibs(t *testing.T) {
	t.Parallel()

	t.Run("正常系: APKから.soファイルを抽出", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"lib/arm64-v8a/libmain.so":   []byte("arm64 so content"),
			"lib/armeabi-v7a/libmain.so": []byte("armeabi so content"),
			"lib/x86/libmain.so":         []byte("x86 so content"),
			"lib/x86_64/libmain.so":      []byte("x86_64 so content"),
		})
		addMinimalBuildGradle(t, projectDir)
		addMinimalManifest(t, projectDir)
		addForkJavaSource(t, projectDir)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		jniLibsDir := filepath.Join(projectDir, "app", "src", "main", "jniLibs")
		assert.FileExists(t, filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "armeabi-v7a", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "x86", "libmain.so"))
		assert.FileExists(t, filepath.Join(jniLibsDir, "x86_64", "libmain.so"))

		content, err := os.ReadFile(filepath.Join(jniLibsDir, "arm64-v8a", "libmain.so"))
		require.NoError(t, err)
		assert.Equal(t, "arm64 so content", string(content))
	})

	t.Run("異常系: APK内に.soファイルがない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))

		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
			"AndroidManifest.xml": []byte("manifest content"),
			"classes.dex":         []byte("dex content"),
		})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrJniLibsNotFound)
		assert.ErrorContains(t, err, ".soファイルが見つかりません")
	})

	t.Run("異常系: APKファイルが不正な場合", func(t *testing.T) {
		t.Parallel()

		projectDir := filepath.Join(t.TempDir(), "project")
		require.NoError(t, os.MkdirAll(projectDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(projectDir, "krkrsdl2_universal.apk"), []byte("invalid zip content"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "無効なAPKファイルです")
	})

	t.Run("正常系: サポートされるABIのみ抽出される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name              string
			abi               string
			expectedExtracted bool
		}{
			{name: "正常系: arm64-v8a はサポート対象", abi: "arm64-v8a", expectedExtracted: true},
			{name: "正常系: armeabi-v7a はサポート対象", abi: "armeabi-v7a", expectedExtracted: true},
			{name: "正常系: x86 はサポート対象", abi: "x86", expectedExtracted: true},
			{name: "正常系: x86_64 はサポート対象", abi: "x86_64", expectedExtracted: true},
			{name: "正常系: mips はサポート対象外", abi: "mips", expectedExtracted: false},
			{name: "正常系: armeabi はサポート対象外", abi: "armeabi", expectedExtracted: false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := filepath.Join(t.TempDir(), "project")
				require.NoError(t, os.MkdirAll(projectDir, 0o750))

				writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
					"lib/" + tc.abi + "/libtest.so": []byte("test so content"),
					"lib/arm64-v8a/libmain.so":      []byte("arm64 so content"),
				})
				addMinimalBuildGradle(t, projectDir)
				addMinimalManifest(t, projectDir)
				addForkJavaSource(t, projectDir)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

				soFile := filepath.Join(projectDir, "app", "src", "main", "jniLibs", tc.abi, "libtest.so")
				if tc.expectedExtracted {
					assert.FileExists(t, soFile, "ABI: %s", tc.abi)
				} else {
					assert.NoFileExists(t, soFile, "ABI: %s", tc.abi)
				}
			})
		}
	})
}

func newProjectWithAPK(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o750))
	writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{
		"lib/arm64-v8a/libmain.so": []byte("so content"),
	})
	addForkJavaSource(t, projectDir)

	return projectDir
}

func TestTemplatePreparer_UpdateJavaSource(t *testing.T) {
	t.Parallel()

	t.Run("正常系: パッケージ名が正しく反映される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			packageName string
		}{
			{name: "正常系: 標準的なパッケージ名", packageName: "com.example.game"},
			{name: "正常系: 日本ドメインのパッケージ名", packageName: "jp.example.mygame"},
			{name: "正常系: 数字を含むパッケージ名", packageName: "org.test.app123"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := newFullyPreparableProject(t)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare(tc.packageName, "My Game", "", "", nil))

				packagePath := filepath.Join(strings.Split(tc.packageName, ".")...)
				javaFile := filepath.Join(projectDir, "app", "src", "main", "java", packagePath, "KirikiriSDL2Activity.java")
				assert.FileExists(t, javaFile)

				content, err := os.ReadFile(javaFile)
				require.NoError(t, err)
				assert.Contains(t, string(content), "package "+tc.packageName+";")

				gameActivityFile := filepath.Join(projectDir, "app", "src", "main", "java", packagePath, "KirikiriSDL2GameActivity.java")
				assert.FileExists(t, gameActivityFile)

				gameActivityContent, err := os.ReadFile(gameActivityFile)
				require.NoError(t, err)
				assert.Contains(t, string(gameActivityContent), "package "+tc.packageName+";")
			})
		}
	})

	t.Run("正常系: fork版ソースのディレクトリ(pw/uyjulian/krkrsdl2)が消費後に削除される", func(t *testing.T) {
		t.Parallel()

		// newFullyPreparableProjectがテンプレートzip展開後の状態を模して
		// 配置するfork版ソース(addForkJavaSource)は、変換元として読み込まれた
		// 後は不要になる。生成後のAndroidプロジェクトに重複したソースディレクトリが
		// 残らないことを確認する。
		projectDir := newFullyPreparableProject(t)
		oldJavaDir := filepath.Join(projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2")
		require.DirExists(t, oldJavaDir)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		assert.NoDirExists(t, oldJavaDir)
	})

	t.Run("正常系: KirikiriSDL2GameActivityのgetArgumentsにholdalpha=yesが含まれる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2GameActivity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		assert.Contains(t, string(content), `"-holdalpha=yes"`)
	})

	t.Run("正常系: KirikiriSDL2GameActivityのgetArgumentsにSIMD無効化フラグが含まれる（C実装を使用）", func(t *testing.T) {
		t.Parallel()

		// ARMデバイスではSIMDeエミュレーションに問題があるため、純粋なC実装の
		// ブレンド関数を使用することでアルファブレンディングの互換性を確保する
		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2GameActivity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		assert.Contains(t, text, `"-cpummx=no"`)
		assert.Contains(t, text, `"-cpusse=no"`)
		assert.Contains(t, text, `"-cpusse2=no"`)
	})

	t.Run("正常系: showSelectListメソッドが含まれる（krkrsdl2ネイティブがJNI経由で呼び出す）", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		assert.Contains(t, text, "public static int showSelectList(final String title, final String[] items)")
	})

	t.Run("正常系: fork側Java(pw/uyjulian/krkrsdl2/KirikiriSDL2Activity.java)の安全性・機能に不可欠な断片が生成テンプレートから欠落していない", func(t *testing.T) {
		t.Parallel()

		// このリストはfork側の実装を手で移植した箇所のピン留め。fork側で
		// メソッドが追加/削除された場合、このテストは自動検知できないため
		// 別途「fork側の全メンバ列挙とGo定数側との突合」を都度行う必要がある。
		requiredFragments := []string{
			// setOrientationBis: Manifestの画面向き指定をSDLのsetOrientationBis()
			// による上書きから守るオーバーライド
			"import android.content.pm.ActivityInfo;",
			"import android.content.pm.PackageManager;",
			"public void setOrientationBis(int w, int h, boolean resizable, String hint)",
			"ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED",
			"PackageManager.NameNotFoundException",
			// getSafeAreaInsets: krkrsdl2ネイティブがJNI経由で呼び出すセーフエリア取得
			"public static int[] getSafeAreaInsets()",
			// showSelectList: krkrsdl2ネイティブがJNI経由で呼び出すダイアログ実装
			"public static int showSelectList(",
			"setCancelable(true)",
		}

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		for _, fragment := range requiredFragments {
			assert.Contains(t, text, fragment, "fork側の断片が生成テンプレートから欠落しています: %s", fragment)
		}
	})

	t.Run("正常系: fork版がonCreateを持っていても生成一式がjavac的に衝突しない（onCreate定義が各ファイルに1つずつ）", func(t *testing.T) {
		t.Parallel()

		// realForkKirikiriSDL2ActivityJavaはfork krkrsdl2で追加された
		// onCreateオーバーライド（WindowInsetsリスナー登録）を含む。
		// KirikiriSDL2Activity.java（fork版、素通し出力）と
		// KirikiriSDL2GameActivity.java（mnemonic独自、サブクラス）の
		// それぞれにonCreateが1つずつ定義され、同一ファイル内でメソッドが
		// 重複しないことを確認する（回帰: 過去にonCreate二重定義でjavac
		// エラーが発生した）。
		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		forkJavaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2Activity.java")
		forkContent, err := os.ReadFile(forkJavaFile)
		require.NoError(t, err)

		gameActivityFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2GameActivity.java")
		gameActivityContent, err := os.ReadFile(gameActivityFile)
		require.NoError(t, err)

		forkText := string(forkContent)
		gameActivityText := string(gameActivityContent)

		assert.Equal(t, 1, countOccurrences(forkText, "protected void onCreate("), "fork版ファイルのonCreateが重複または欠落している")
		assert.Equal(t, 1, countOccurrences(gameActivityText, "protected void onCreate("), "サブクラスファイルのonCreateが重複または欠落している")
	})

	t.Run("異常系: fork版KirikiriSDL2Activity.javaがテンプレートに存在しない場合", func(t *testing.T) {
		t.Parallel()

		// app/src/main/java/pw/uyjulian/krkrsdl2/以下にfork版ソースが無い状態
		// （テンプレートzip展開に失敗した等）を再現する。
		projectDir := filepath.Join(t.TempDir(), "project")
		appDir := filepath.Join(projectDir, "app")
		manifestDir := filepath.Join(appDir, "src", "main")
		require.NoError(t, os.MkdirAll(manifestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(minimalManifestXML), 0o600))
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "fork版KirikiriSDL2Activity.javaが見つかりません")
	})

	t.Run("正常系: packageNameがpw.uyjulian.krkrsdl2自身でも生成ファイルが残る", func(t *testing.T) {
		t.Parallel()

		// 新Java書き込み後にoldJavaDir(pw/uyjulian/krkrsdl2)を無条件削除する
		// 実装だと、packageNameが"pw.uyjulian.krkrsdl2"自身の場合は
		// javaDir==oldJavaDirとなり、書いたばかりの生成ファイルがエラーなしで
		// 削除されてしまう（Prepareはnilを返すのにファイルが存在しない状態に
		// なる）。
		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("pw.uyjulian.krkrsdl2", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2", "KirikiriSDL2Activity.java")
		require.FileExists(t, javaFile)

		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)
		text := string(content)
		assert.Contains(t, text, "package pw.uyjulian.krkrsdl2;")
		assert.Contains(t, text, "public static int showSelectList(")

		gameActivityFile := filepath.Join(projectDir, "app", "src", "main", "java", "pw", "uyjulian", "krkrsdl2", "KirikiriSDL2GameActivity.java")
		require.FileExists(t, gameActivityFile)

		gameActivityContent, err := os.ReadFile(gameActivityFile)
		require.NoError(t, err)
		gameActivityText := string(gameActivityContent)
		assert.Contains(t, gameActivityText, "package pw.uyjulian.krkrsdl2;")
		assert.Contains(t, gameActivityText, "protected void onCreate(Bundle savedInstanceState)")
	})
}

// TestTemplatePreparer_GameActivity はKirikiriSDL2GameActivity.java
// （mnemonic独自機能のサブクラス）の生成内容を検証する。
func TestTemplatePreparer_GameActivity(t *testing.T) {
	t.Parallel()

	t.Run("正常系: KirikiriSDL2ActivityをextendsするサブクラスとしてonCreate/getArguments/copyAssets系メソッドが生成される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		javaFile := filepath.Join(projectDir, "app", "src", "main", "java", "com", "example", "game", "KirikiriSDL2GameActivity.java")
		content, err := os.ReadFile(javaFile)
		require.NoError(t, err)

		text := string(content)
		assert.Contains(t, text, "public class KirikiriSDL2GameActivity extends KirikiriSDL2Activity {")
		assert.Contains(t, text, "protected void onCreate(Bundle savedInstanceState)")
		assert.Contains(t, text, "protected String[] getArguments()")
		assert.Contains(t, text, "private void copyAssetsToInternal()")
		assert.Contains(t, text, "super.onCreate(savedInstanceState);")
	})
}

func TestTemplatePreparer_UpdateBuildGradle(t *testing.T) {
	t.Parallel()

	newBuildGradleProject := func(t *testing.T, content string) string {
		t.Helper()

		projectDir := filepath.Join(t.TempDir(), "project")
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte(content), 0o600))
		addMinimalManifest(t, projectDir)
		addForkJavaSource(t, projectDir)

		return projectDir
	}

	t.Run("正常系: namespaceが追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `namespace "com.example.game"`)
	})

	t.Run("正常系: compileSdkVersionが34に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "compileSdkVersion 34")
	})

	t.Run("正常系: targetSdkVersionが34に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        targetSdkVersion 30\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "targetSdkVersion 34")
	})

	t.Run("正常系: minSdkVersionが21に更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        minSdkVersion 16\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "minSdkVersion 21")
	})

	t.Run("正常系: CMake設定が削除される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, `
android {
    compileSdkVersion 30
    externalNativeBuild {
        cmake {
            path "CMakeLists.txt"
        }
    }
    defaultConfig {
        externalNativeBuild {
            ndk {
                abiFilters 'arm64-v8a', 'armeabi-v7a'
            }
        }
    }
}
`)
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		text := string(content)
		assert.NotContains(t, strings.ToLower(text), "cmake")
		assert.NotContains(t, text, "externalNativeBuild")
	})

	t.Run("正常系: applicationIdが更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newBuildGradleProject(t, "\nandroid {\n    compileSdkVersion 30\n    defaultConfig {\n        applicationId \"pw.uyjulian.krkrsdl2\"\n    }\n}\n")
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "build.gradle"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `applicationId "com.example.game"`)
	})

	t.Run("異常系: build.gradleが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := newProjectWithAPK(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "build.gradleが見つかりません")
	})
}

func TestTemplatePreparer_UpdateManifest(t *testing.T) {
	t.Parallel()

	newManifestProject := func(t *testing.T, content string) string {
		t.Helper()

		projectDir := filepath.Join(t.TempDir(), "project")
		manifestDir := filepath.Join(projectDir, "app", "src", "main")
		require.NoError(t, os.MkdirAll(manifestDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(content), 0o600))
		writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
		addForkJavaSource(t, projectDir)

		return projectDir
	}

	t.Run("正常系: package属性が削除される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), `package="`)
	})

	t.Run("正常系: 起動activityのandroid:nameがKirikiriSDL2GameActivityへ書き換えられる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			packageName  string
			activityName string
			wantName     string
		}{
			// 先頭ドット省略形（テスト全体で使われている表記）。相対解決形は
			// build.gradleのnamespace（packageName）を通じて解決されるため、
			// プレフィックスをそのまま保持してよい。
			{name: "正常系: 先頭ドット省略形", packageName: "com.example.game", activityName: ".KirikiriSDL2Activity", wantName: ".KirikiriSDL2GameActivity"},
			// パッケージ省略形（fork krkrsdl2実テンプレートの実際の表記）
			{name: "正常系: パッケージ省略形", packageName: "com.example.game", activityName: "KirikiriSDL2Activity", wantName: "KirikiriSDL2GameActivity"},
			// 完全修飾かつpackageNameがfork版パッケージと異なる場合。
			// updateJavaSourceは生成クラスを常にpackageName配下へ配置するため、
			// 書き換え後もpackageNameを指す必要がある（元のfork版パッケージ
			// 接頭辞pw.uyjulian.krkrsdl2.を保持すると実在しないクラスを
			// 指してしまう。rewriteActivityNameのwhy-not参照）。
			{name: "正常系: 完全修飾（packageNameが異なる）", packageName: "com.example.game", activityName: "pw.uyjulian.krkrsdl2.KirikiriSDL2Activity", wantName: "com.example.game.KirikiriSDL2GameActivity"},
			// 完全修飾かつpackageNameがfork版パッケージ自身と一致する場合
			{name: "正常系: 完全修飾（packageNameがfork版と一致）", packageName: "pw.uyjulian.krkrsdl2", activityName: "pw.uyjulian.krkrsdl2.KirikiriSDL2Activity", wantName: "pw.uyjulian.krkrsdl2.KirikiriSDL2GameActivity"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name="`+tc.activityName+`">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
`)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare(tc.packageName, "My Game", "", "", nil))

				content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
				require.NoError(t, err)
				text := string(content)
				assert.Contains(t, text, `android:name="`+tc.wantName+`"`)
				if tc.activityName != tc.wantName {
					assert.NotContains(t, text, `android:name="`+tc.activityName+`"`)
				}
				// android.intent.action.MAINのandroid:name属性まで誤って
				// 書き換えられていないことを確認する
				assert.Contains(t, text, `android:name="android.intent.action.MAIN"`)

				// 書き換え後のandroid:nameが実際に生成されたクラスファイルを
				// 指していることを確認する（ActivityNotFoundException回帰防止）。
				packagePath := filepath.Join(strings.Split(tc.packageName, ".")...)
				gameActivityFile := filepath.Join(projectDir, "app", "src", "main", "java", packagePath, "KirikiriSDL2GameActivity.java")
				assert.FileExists(t, gameActivityFile)
			})
		}
	})

	t.Run("正常系: android:exported=trueがactivityに追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:exported="true"`)
	})

	t.Run("正常系: android:exported=trueがserviceに追加される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <service android:name=".MyService">
        </service>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:exported="true"`)
	})

	t.Run("正常系: 既にexportedがある場合は重複追加しない", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity" android:exported="false">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Equal(t, 1, countOccurrences(string(content), `android:exported="`))
	})

	t.Run("異常系: AndroidManifest.xmlが見つからない場合", func(t *testing.T) {
		t.Parallel()

		projectDir := newProjectWithAPK(t)
		appDir := filepath.Join(projectDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))

		err := p.Prepare("com.example.game", "My Game", "", "", nil)

		require.ErrorIs(t, err, builder.ErrTemplatePreparer)
		assert.ErrorContains(t, err, "AndroidManifest.xmlが見つかりません")
	})

	t.Run("正常系: screenOrientation属性が無いactivityにsensorLandscapeが注入される", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:screenOrientation="sensorLandscape"`)
	})

	t.Run("正常系: 既存のscreenOrientation属性はsensorLandscapeへ置き換えられる", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity" android:screenOrientation="portrait">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)
		assert.Contains(t, string(content), `android:screenOrientation="sensorLandscape"`)
		assert.Equal(t, 1, countOccurrences(string(content), `android:screenOrientation="`))
	})

	t.Run("正常系: screenOrientation注入後も妥当なXMLである", func(t *testing.T) {
		t.Parallel()

		projectDir := newManifestProject(t, `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
`)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "AndroidManifest.xml"))
		require.NoError(t, err)

		decoder := xml.NewDecoder(bytes.NewReader(content))
		for {
			_, tokenErr := decoder.Token()
			if errors.Is(tokenErr, io.EOF) {
				break
			}
			require.NoError(t, tokenErr, "注入後のAndroidManifest.xmlが妥当なXMLではありません")
		}
	})
}

func newFullyPreparableProject(t *testing.T) string {
	t.Helper()

	projectDir := filepath.Join(t.TempDir(), "project")
	appDir := filepath.Join(projectDir, "app")
	manifestDir := filepath.Join(appDir, "src", "main")
	require.NoError(t, os.MkdirAll(manifestDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "build.gradle"), []byte("android {\n}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "AndroidManifest.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application></application>
</manifest>
`), 0o600))
	writeZipFile(t, filepath.Join(projectDir, "krkrsdl2_universal.apk"), map[string][]byte{"lib/arm64-v8a/libmain.so": []byte("x")})
	addForkJavaSource(t, projectDir)

	return projectDir
}

func TestTemplatePreparer_UpdateStringsXML(t *testing.T) {
	t.Parallel()

	t.Run("正常系: strings.xmlが作成される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", "", nil))

		stringsXML := filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml")
		content, err := os.ReadFile(stringsXML)
		require.NoError(t, err)
		assert.Contains(t, string(content), `<string name="app_name">My Game</string>`)
	})

	t.Run("正常系: 既存のstrings.xmlが更新される", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		valuesDir := filepath.Join(projectDir, "app", "src", "main", "res", "values")
		require.NoError(t, os.MkdirAll(valuesDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "strings.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">Old Name</string>
    <string name="other">Other Value</string>
</resources>
`), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "New Game Name", "", "", nil))

		content, err := os.ReadFile(filepath.Join(valuesDir, "strings.xml"))
		require.NoError(t, err)
		text := string(content)
		assert.Contains(t, text, `<string name="app_name">New Game Name</string>`)
		assert.NotContains(t, text, "Old Name")
		assert.Contains(t, text, "Other Value")
	})

	t.Run("正常系: XML特殊文字がエスケープされる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name           string
			appName        string
			expectedEscape string
		}{
			{name: "正常系: アンパサンドのエスケープ", appName: "Game & Play", expectedEscape: "Game &amp; Play"},
			{name: "正常系: 山括弧のエスケープ", appName: "Game <Test>", expectedEscape: "Game &lt;Test&gt;"},
			{name: "正常系: ダブルクオートのエスケープ", appName: `Game "Quote"`, expectedEscape: "Game &quot;Quote&quot;"},
			{name: "正常系: シングルクオートのエスケープ", appName: "Game 'Single'", expectedEscape: "Game &#x27;Single&#x27;"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				projectDir := newFullyPreparableProject(t)

				p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
				require.NoError(t, p.Prepare("com.example.game", tc.appName, "", "", nil))

				content, err := os.ReadFile(filepath.Join(projectDir, "app", "src", "main", "res", "values", "strings.xml"))
				require.NoError(t, err)
				assert.Contains(t, string(content), tc.expectedEscape)
			})
		}
	})
}

func TestTemplatePreparer_UpdateIcon(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 各解像度にアイコンがコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		iconPath := filepath.Join(t.TempDir(), "icon.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake png content"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", "", iconPath, nil))

		densities := []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}
		resDir := filepath.Join(projectDir, "app", "src", "main", "res")

		for _, density := range densities {
			iconFile := filepath.Join(resDir, "mipmap-"+density, "ic_launcher.png")
			require.FileExists(t, iconFile, "mipmap-%s にアイコンがありません", density)

			content, err := os.ReadFile(iconFile)
			require.NoError(t, err)
			assert.Equal(t, "fake png content", string(content))
		}
	})
}

func TestTemplatePreparer_CopyAssets(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ゲームファイルがコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		assetsSrc := filepath.Join(t.TempDir(), "game_files")
		require.NoError(t, os.MkdirAll(assetsSrc, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "data.xp3"), []byte("xp3 content"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "config.tjs"), []byte("config"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", assetsSrc, "", nil))

		assetsDest := filepath.Join(projectDir, "app", "src", "main", "assets", "data")
		assert.FileExists(t, filepath.Join(assetsDest, "data.xp3"))
		assert.FileExists(t, filepath.Join(assetsDest, "config.tjs"))
	})

	t.Run("正常系: サブディレクトリもコピーされる", func(t *testing.T) {
		t.Parallel()

		projectDir := newFullyPreparableProject(t)
		assetsSrc := filepath.Join(t.TempDir(), "game_files")
		require.NoError(t, os.MkdirAll(filepath.Join(assetsSrc, "scenario"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(assetsSrc, "scenario", "first.ks"), []byte("scenario"), 0o600))

		p := builder.NewTemplatePreparer(projectDir, testSDL2Cache(t))
		require.NoError(t, p.Prepare("com.example.game", "My Game", assetsSrc, "", nil))

		assetsDest := filepath.Join(projectDir, "app", "src", "main", "assets", "data")
		assert.FileExists(t, filepath.Join(assetsDest, "scenario", "first.ks"))
	})
}
