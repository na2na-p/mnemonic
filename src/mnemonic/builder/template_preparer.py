"""テンプレート準備機能

このモジュールはAndroidプロジェクトテンプレートを準備する機能を提供します。
krkrsdl2_universal.apkから.soファイルを抽出し、Javaコードを拡張し、
ビルド設定を更新してGradleビルド用のプロジェクトを構成します。
"""

from __future__ import annotations

import asyncio
import html
import re
import shutil
import zipfile
from pathlib import Path
from typing import Final

from PIL import Image

from mnemonic.builder.plugin_fetcher import (
    SUPPORTED_ABIS,
    PluginDownloadError,
    PluginFetcher,
    PluginsInfo,
)
from mnemonic.builder.sdl2_sources import (
    SDL2SourceCache,
    SDL2SourceFetcher,
    SDL2SourceFetcherError,
)


class TemplatePreparerError(Exception):
    """テンプレート準備に関する基本例外クラス"""

    pass


class JniLibsNotFoundError(TemplatePreparerError):
    """JNIライブラリが見つからない場合の例外"""

    pass


class SDL2SourceFetchError(TemplatePreparerError):
    """SDL2 Java ソースの取得に失敗した場合の例外"""

    pass


class PluginFetchError(TemplatePreparerError):
    """プラグインの取得に失敗した場合の例外"""

    pass


class TemplatePreparer:
    """Androidプロジェクトテンプレートを準備するクラス

    このクラスはGradleビルド用のAndroidプロジェクトを準備します。
    以下の処理を行います：
    1. krkrsdl2_universal.apkから.soファイルを抽出してjniLibsに配置
    2. SDL2 Java ソースをダウンロードして配置
    3. krkrsdl2プラグイン(.so)をjniLibsに配置
    4. KirikiriSDL2Activity.javaをassets コピー機能付きに置き換え
    5. app/build.gradleを更新（targetSdkVersion=34、namespace追加）
    6. AndroidManifest.xmlを更新（android:exported="true"追加）
    7. res/values/strings.xmlを作成（app_name設定）
    """

    # 推奨ターゲット/コンパイルSDKバージョン
    TARGET_SDK_VERSION: Final[int] = 34
    COMPILE_SDK_VERSION: Final[int] = 34
    MIN_SDK_VERSION: Final[int] = 21

    # サポートするABI
    SUPPORTED_ABIS: Final[list[str]] = [
        "arm64-v8a",
        "armeabi-v7a",
        "x86",
        "x86_64",
    ]

    def __init__(
        self,
        project_dir: Path,
        sdl2_cache: SDL2SourceCache | None = None,
    ) -> None:
        """TemplatePreparerを初期化する

        Args:
            project_dir: Androidプロジェクトのルートディレクトリ
            sdl2_cache: SDL2 Java ソースのキャッシュ（オプション）
        """
        self._project_dir = project_dir
        self._sdl2_cache = sdl2_cache

    def prepare(
        self,
        package_name: str,
        app_name: str,
        assets_dir: Path | None = None,
        icon_path: Path | None = None,
        plugins_info: PluginsInfo | None = None,
    ) -> None:
        """テンプレートを準備する

        Args:
            package_name: Androidパッケージ名（例: com.example.game）
            app_name: アプリケーション表示名
            assets_dir: ゲームファイルを含むディレクトリ（オプション）
            icon_path: アプリアイコンのパス（オプション）
            plugins_info: プラグイン情報（オプション、指定時はjniLibsに配置）

        Raises:
            TemplatePreparerError: テンプレート準備に失敗した場合
        """
        # 1. ベースAPKから.soファイルを抽出
        self._extract_jni_libs()

        # 2. SDL2 Java ソースを取得
        self._fetch_sdl2_sources()

        # 3. krkrsdl2プラグインをjniLibsに配置
        self._copy_plugins_to_jnilibs(plugins_info)

        # 4. Javaソースを置き換え
        self._update_java_source(package_name)

        # 5. build.gradleを更新
        self._update_build_gradle(package_name)

        # 6. AndroidManifest.xmlを更新
        self._update_manifest()

        # 7. strings.xmlを作成/更新
        self._update_strings_xml(app_name)

        # 8. assetsをコピー（指定されている場合）
        if assets_dir is not None:
            self._copy_assets(assets_dir)

        # 9. アイコンを更新（指定されている場合）、またはデフォルトアイコンを生成
        if icon_path is not None and icon_path.exists():
            self._update_icon(icon_path)
        else:
            self._create_default_icon()

    def _extract_jni_libs(self) -> None:
        """krkrsdl2_universal.apkから.soファイルを抽出する

        Raises:
            JniLibsNotFoundError: APKファイルが見つからない、または.soファイルがない場合
        """
        base_apk = self._project_dir / "krkrsdl2_universal.apk"
        if not base_apk.exists():
            raise JniLibsNotFoundError(f"ベースAPKが見つかりません: {base_apk}")

        jni_libs_dir = self._project_dir / "app" / "src" / "main" / "jniLibs"
        jni_libs_dir.mkdir(parents=True, exist_ok=True)

        so_files_extracted = 0

        try:
            with zipfile.ZipFile(base_apk, "r") as zf:
                for name in zf.namelist():
                    # lib/{abi}/*.so のパターンに一致するファイルを抽出
                    if name.startswith("lib/") and name.endswith(".so"):
                        parts = name.split("/")
                        if len(parts) >= 3:
                            abi = parts[1]
                            so_name = parts[2]

                            if abi in self.SUPPORTED_ABIS:
                                dest_dir = jni_libs_dir / abi
                                dest_dir.mkdir(parents=True, exist_ok=True)
                                dest_file = dest_dir / so_name

                                with zf.open(name) as src, open(dest_file, "wb") as dst:
                                    shutil.copyfileobj(src, dst)
                                so_files_extracted += 1

        except zipfile.BadZipFile as e:
            raise TemplatePreparerError(f"無効なAPKファイルです: {base_apk}") from e

        if so_files_extracted == 0:
            raise JniLibsNotFoundError(f"APK内に.soファイルが見つかりません: {base_apk}")

    def _fetch_sdl2_sources(self) -> None:
        """SDL2 Java ソースを取得して配置する

        SDL2 の Java ソースファイル（SDLActivity.java 等）を
        GitHub からダウンロードしてプロジェクトに配置します。
        キャッシュが有効な場合はキャッシュから復元します。

        Raises:
            SDL2SourceFetchError: SDL2 ソースの取得に失敗した場合
        """
        java_dir = self._project_dir / "app" / "src" / "main" / "java"
        java_dir.mkdir(parents=True, exist_ok=True)

        fetcher = SDL2SourceFetcher(cache=self._sdl2_cache)

        try:
            asyncio.run(fetcher.fetch(java_dir))
        except SDL2SourceFetcherError as e:
            raise SDL2SourceFetchError(f"SDL2 Java ソースの取得に失敗しました: {e}") from e

    def _copy_plugins_to_jnilibs(self, plugins_info: PluginsInfo | None) -> None:
        """krkrsdl2プラグインをjniLibsディレクトリにコピーする

        プラグイン(.so)を各ABI用のjniLibsディレクトリに配置する。
        jniLibsに配置されたプラグインはAPKビルド時に自動的に
        lib/{abi}/配下に含まれ、System.loadLibraryで読み込み可能になる。

        スクリプト変換時にlibプレフィックス付きのフルファイル名を指定するため、
        libプレフィックス付きのファイルのみ配置すれば良い。

        Args:
            plugins_info: プラグイン情報。Noneの場合はダウンロードを試みる。

        Note:
            プラグインの取得に失敗してもビルドは継続する（警告のみ）。
        """
        import logging

        logger = logging.getLogger(__name__)

        jni_libs_dir = self._project_dir / "app" / "src" / "main" / "jniLibs"

        # plugins_infoが指定されていない場合はダウンロードを試みる
        if plugins_info is None:
            try:
                fetcher = PluginFetcher()
                plugins_info = asyncio.run(fetcher.get_plugins())
            except PluginDownloadError as e:
                logger.warning(f"プラグインのダウンロードに失敗しました: {e}")
                return

        # 各ABIのディレクトリにプラグインをコピー
        for abi in SUPPORTED_ABIS:
            abi_dir = jni_libs_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)

            # このABIの全プラグインパスを取得
            plugin_paths = plugins_info.get_all_paths_for_abi(abi)
            for plugin_name, src_path in plugin_paths.items():
                if src_path.exists():
                    dest_path = abi_dir / src_path.name
                    shutil.copy2(src_path, dest_path)
                    logger.debug(f"プラグインをコピーしました: {plugin_name} -> {dest_path}")
                else:
                    logger.warning(f"プラグインファイルが見つかりません: {src_path}")

    def _update_java_source(self, package_name: str) -> None:
        """KirikiriSDL2Activity.javaを拡張版に置き換える

        アプリ起動時にassets/data/配下のファイルを内部ストレージにコピーする
        処理を含むJavaファイルを生成します。

        Args:
            package_name: Androidパッケージ名
        """
        # パッケージ名からディレクトリパスを生成
        package_path = package_name.replace(".", "/")
        java_dir = self._project_dir / "app" / "src" / "main" / "java" / package_path
        java_dir.mkdir(parents=True, exist_ok=True)

        java_file = java_dir / "KirikiriSDL2Activity.java"

        # 古いJavaファイルを削除（元のパッケージ名のディレクトリにある場合）
        old_java_base = self._project_dir / "app" / "src" / "main" / "java" / "pw"
        old_java_dir = old_java_base / "uyjulian" / "krkrsdl2"
        if old_java_dir.exists():
            shutil.rmtree(old_java_dir)

        java_content = self._generate_activity_java(package_name)
        java_file.write_text(java_content, encoding="utf-8")

    def _generate_activity_java(self, package_name: str) -> str:
        """拡張版KirikiriSDL2Activity.javaのソースコードを生成する

        Args:
            package_name: Androidパッケージ名

        Returns:
            Javaソースコード
        """
        return f"""package {package_name};

import android.os.Bundle;
import android.content.pm.ApplicationInfo;
import android.content.res.AssetManager;
import android.util.Log;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import org.libsdl.app.SDLActivity;

/**
 * KirikiriSDL2用のメインアクティビティ
 *
 * アプリ起動時にassets/data/配下のゲームファイルを
 * 内部ストレージにコピーしてkrkrsdl2が読み込めるようにする。
 */
public class KirikiriSDL2Activity extends SDLActivity {{
    private static final String TAG = "KirikiriSDL2";
    private static final String ASSETS_DATA_DIR = "data";
    private static String sNativeLibDir = null;

    @Override
    protected void onCreate(Bundle savedInstanceState) {{
        // ネイティブライブラリのディレクトリを保存
        sNativeLibDir = getApplicationInfo().nativeLibraryDir;
        Log.i(TAG, "Native library directory: " + sNativeLibDir);

        copyAssetsToInternal();
        super.onCreate(savedInstanceState);
    }}

    /**
     * krkrsdl2に渡すコマンドライン引数を設定する
     * プラグイン検索パスにネイティブライブラリディレクトリを追加
     */
    @Override
    protected String[] getArguments() {{
        if (sNativeLibDir != null) {{
            Log.i(TAG, "Setting plugin search path: " + sNativeLibDir);
            return new String[]{{
                "-krkrsdl2_pluginsearchpath=" + sNativeLibDir
            }};
        }}
        return new String[]{{}};
    }}

    /**
     * assets/data/配下のファイルを内部ストレージにコピーする
     * 既存ファイルはスキップする（初回のみコピー）
     */
    private void copyAssetsToInternal() {{
        AssetManager assetManager = getAssets();
        File destDir = getFilesDir();

        try {{
            copyAssetFolder(assetManager, ASSETS_DATA_DIR, destDir);
            Log.i(TAG, "Assets copied to: " + destDir.getAbsolutePath());
        }} catch (IOException e) {{
            Log.e(TAG, "Failed to copy assets", e);
        }}
    }}

    /**
     * アセットフォルダを再帰的にコピーする
     *
     * @param assetManager アセットマネージャー
     * @param srcPath コピー元のアセットパス
     * @param destDir コピー先のディレクトリ
     * @throws IOException コピーに失敗した場合
     */
    private void copyAssetFolder(AssetManager assetManager, String srcPath, File destDir)
            throws IOException {{
        String[] files = assetManager.list(srcPath);
        if (files == null || files.length == 0) {{
            // ファイルの場合
            copyAssetFile(assetManager, srcPath, destDir);
            return;
        }}

        // ディレクトリの場合
        for (String file : files) {{
            String srcFilePath = srcPath + "/" + file;
            File destFile = new File(destDir, file);

            String[] subFiles = assetManager.list(srcFilePath);
            if (subFiles != null && subFiles.length > 0) {{
                // サブディレクトリ
                destFile.mkdirs();
                copyAssetFolder(assetManager, srcFilePath, destFile);
            }} else {{
                // ファイル
                copyAssetFile(assetManager, srcFilePath, destDir);
            }}
        }}
    }}

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
            throws IOException {{
        String fileName = srcPath.contains("/")
                ? srcPath.substring(srcPath.lastIndexOf("/") + 1)
                : srcPath;
        File destFile = new File(destDir, fileName);

        // 既存ファイルはスキップ
        if (destFile.exists()) {{
            return;
        }}

        destFile.getParentFile().mkdirs();

        try (InputStream in = assetManager.open(srcPath);
             OutputStream out = new FileOutputStream(destFile)) {{
            byte[] buffer = new byte[8192];
            int read;
            while ((read = in.read(buffer)) != -1) {{
                out.write(buffer, 0, read);
            }}
        }}
    }}
}}
"""

    def _update_build_gradle(self, package_name: str) -> None:
        """app/build.gradleを更新する

        - namespace追加
        - compileSdkVersion/targetSdkVersionを34に更新
        - minSdkVersionを21に更新
        - CMake設定を削除（Javaのみのプロジェクトに変更）

        Args:
            package_name: Androidパッケージ名
        """
        build_gradle = self._project_dir / "app" / "build.gradle"
        if not build_gradle.exists():
            raise TemplatePreparerError(f"build.gradleが見つかりません: {build_gradle}")

        content = build_gradle.read_text(encoding="utf-8")

        # android { ブロックの先頭にnamespaceを追加（まだない場合）
        if "namespace" not in content:
            content = re.sub(
                r"(android\s*\{)",
                rf'\1\n    namespace "{package_name}"',
                content,
            )

        # compileSdkVersionを更新
        content = re.sub(
            r"compileSdkVersion\s+\d+",
            f"compileSdkVersion {self.COMPILE_SDK_VERSION}",
            content,
        )

        # minSdkVersionを更新
        content = re.sub(
            r"minSdkVersion\s+\d+",
            f"minSdkVersion {self.MIN_SDK_VERSION}",
            content,
        )

        # targetSdkVersionを更新
        content = re.sub(
            r"targetSdkVersion\s+\d+",
            f"targetSdkVersion {self.TARGET_SDK_VERSION}",
            content,
        )

        # applicationIdを更新（存在する場合）
        if "applicationId" in content:
            content = re.sub(
                r'applicationId\s+"[^"]+"',
                f'applicationId "{package_name}"',
                content,
            )

        # CMake関連の設定を削除（externalNativeBuildブロック全体を削除）
        # android内のexternalNativeBuild（cmake path設定）
        content = re.sub(
            r"\s*externalNativeBuild\s*\{[^}]*cmake\s*\{[^}]*\}[^}]*\}",
            "",
            content,
            flags=re.DOTALL,
        )

        # defaultConfig内のexternalNativeBuild（abiFilters設定）
        content = re.sub(
            r"\s*externalNativeBuild\s*\{[^}]*ndk\s*\{[^}]*\}[^}]*\}",
            "",
            content,
            flags=re.DOTALL,
        )

        # 単独のndk { abiFilters設定 }も削除
        content = re.sub(
            r"\s*ndk\s*\{[^}]*abiFilters[^}]*\}",
            "",
            content,
            flags=re.DOTALL,
        )

        build_gradle.write_text(content, encoding="utf-8")

    def _update_manifest(self) -> None:
        """AndroidManifest.xmlを更新する

        - package属性を削除（namespaceで指定するため）
        - android:exported="true"を追加
        - android:extractNativeLibs="true"を追加（プラグインロード用）
        """
        manifest_path = self._project_dir / "app" / "src" / "main" / "AndroidManifest.xml"
        if not manifest_path.exists():
            raise TemplatePreparerError(f"AndroidManifest.xmlが見つかりません: {manifest_path}")

        content = manifest_path.read_text(encoding="utf-8")

        # package属性を削除（AGP 8.0以降はnamespaceで指定）
        content = re.sub(r'\s*package="[^"]*"', "", content)

        # applicationタグにextractNativeLibs="true"を追加
        # これによりネイティブライブラリがAPKから展開され、dlopenでアクセス可能になる
        def add_extract_native_libs(match: re.Match[str]) -> str:
            tag = match.group(0)
            if "android:extractNativeLibs" not in tag and tag.endswith(">"):
                # タグの閉じ部分の直前に属性を挿入
                return tag[:-1] + ' android:extractNativeLibs="true">'
            return tag

        content = re.sub(r"<application[^>]*>", add_extract_native_libs, content)

        # activity/service/receiverタグにandroid:exported属性を追加
        def add_exported_if_missing(match: re.Match[str]) -> str:
            tag = match.group(0)
            if "android:exported" not in tag:
                # タグの閉じ部分の直前にexported属性を挿入
                if tag.endswith("/>"):
                    return tag[:-2] + ' android:exported="true"/>'
                else:
                    return tag[:-1] + ' android:exported="true">'
            return tag

        content = re.sub(r"<activity[^>]*(?:>|/>)", add_exported_if_missing, content)
        content = re.sub(r"<service[^>]*(?:>|/>)", add_exported_if_missing, content)
        content = re.sub(r"<receiver[^>]*(?:>|/>)", add_exported_if_missing, content)

        manifest_path.write_text(content, encoding="utf-8")

    def _update_strings_xml(self, app_name: str) -> None:
        """res/values/strings.xmlを作成/更新する

        Args:
            app_name: アプリケーション表示名
        """
        values_dir = self._project_dir / "app" / "src" / "main" / "res" / "values"
        values_dir.mkdir(parents=True, exist_ok=True)

        strings_xml = values_dir / "strings.xml"

        # XMLインジェクション対策: 特殊文字をエスケープ
        escaped_app_name = html.escape(app_name)

        if strings_xml.exists():
            # 既存のstrings.xmlを更新
            content = strings_xml.read_text(encoding="utf-8")
            content = re.sub(
                r'(<string name="app_name">)[^<]*(</string>)',
                rf"\1{escaped_app_name}\2",
                content,
            )
            strings_xml.write_text(content, encoding="utf-8")
        else:
            # 新規作成
            content = f"""<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">{escaped_app_name}</string>
</resources>
"""
            strings_xml.write_text(content, encoding="utf-8")

    def _copy_assets(self, assets_dir: Path) -> None:
        """ゲームファイルをassets/dataにコピーする

        Args:
            assets_dir: コピー元のゲームファイルディレクトリ
        """
        dest_dir = self._project_dir / "app" / "src" / "main" / "assets" / "data"
        dest_dir.mkdir(parents=True, exist_ok=True)

        # ディレクトリをコピー
        shutil.copytree(assets_dir, dest_dir, dirs_exist_ok=True)

    def _update_icon(self, icon_path: Path) -> None:
        """アプリアイコンを更新する

        各解像度のmipmapディレクトリにアイコンファイルをコピーする。

        Args:
            icon_path: アイコン画像のパス
        """
        res_dir = self._project_dir / "app" / "src" / "main" / "res"
        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]

        for density in densities:
            mipmap_dir = res_dir / f"mipmap-{density}"
            mipmap_dir.mkdir(parents=True, exist_ok=True)
            dest_path = mipmap_dir / "ic_launcher.png"
            shutil.copy2(icon_path, dest_path)

    def _create_default_icon(self) -> None:
        """デフォルトアイコンを生成する

        アイコンが提供されない場合のフォールバックとして、
        単色の正方形アイコンを各解像度で生成します。
        """
        res_dir = self._project_dir / "app" / "src" / "main" / "res"

        # 各密度に対応するアイコンサイズ
        density_sizes = {
            "mdpi": 48,
            "hdpi": 72,
            "xhdpi": 96,
            "xxhdpi": 144,
            "xxxhdpi": 192,
        }

        # デフォルトカラー（吉里吉里のテーマカラーに近い青紫）
        default_color = (100, 80, 160)

        for density, size in density_sizes.items():
            mipmap_dir = res_dir / f"mipmap-{density}"
            mipmap_dir.mkdir(parents=True, exist_ok=True)
            dest_path = mipmap_dir / "ic_launcher.png"

            # 単色の正方形アイコンを生成
            img = Image.new("RGB", (size, size), default_color)
            img.save(str(dest_path), "PNG")
