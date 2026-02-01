"""テンプレート準備機能のテスト

TemplatePreparer クラスと関連する例外クラスのユニットテストを提供します。
"""

import zipfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.builder.template_preparer import (
    JniLibsNotFoundError,
    SDL2SourceFetchError,
    TemplatePreparer,
    TemplatePreparerError,
)


@pytest.fixture
def mock_sdl2_fetcher():
    """SDL2SourceFetcher のモック fixture"""
    with patch("mnemonic.builder.template_preparer.SDL2SourceFetcher") as mock_class:
        mock_instance = MagicMock()

        async def mock_fetch(dest_dir: Path):
            # SDL2 Java ソースファイルを模擬作成
            sdl_app_dir = dest_dir / "org" / "libsdl" / "app"
            sdl_app_dir.mkdir(parents=True, exist_ok=True)
            (sdl_app_dir / "SDLActivity.java").write_text("mock content", encoding="utf-8")

        mock_instance.fetch = mock_fetch
        mock_class.return_value = mock_instance
        yield mock_class


class TestTemplatePreparerInit:
    """TemplatePreparer初期化のテスト"""

    def test_init_sets_project_dir(self, tmp_path: Path) -> None:
        """正常系: プロジェクトディレクトリが正しく設定される"""
        preparer = TemplatePreparer(project_dir=tmp_path)

        assert preparer._project_dir == tmp_path

    @pytest.mark.parametrize(
        "project_dir_name",
        [
            pytest.param("my_project", id="正常系: 通常のディレクトリ名"),
            pytest.param("project-with-dashes", id="正常系: ハイフン付きディレクトリ名"),
            pytest.param("project_123", id="正常系: 数字付きディレクトリ名"),
        ],
    )
    def test_init_with_various_project_dirs(self, tmp_path: Path, project_dir_name: str) -> None:
        """正常系: 様々なディレクトリ名で初期化できる"""
        project_dir = tmp_path / project_dir_name
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        assert preparer._project_dir == project_dir


class TestTemplatePreparerPrepare:
    """TemplatePreparer.prepareのテスト"""

    def test_prepare_executes_all_steps_successfully(
        self, tmp_path: Path, mock_sdl2_fetcher: MagicMock
    ) -> None:
        """正常系: すべての処理が成功するケース"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # APK ファイルを作成（.so ファイルを含む ZIP）
        apk_path = project_dir / "krkrsdl2_universal.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr("lib/arm64-v8a/libmain.so", b"fake so content")
            zf.writestr("lib/armeabi-v7a/libmain.so", b"fake so content")

        # app/build.gradle を作成
        app_dir = project_dir / "app"
        app_dir.mkdir()
        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
    defaultConfig {
        applicationId "pw.uyjulian.krkrsdl2"
        minSdkVersion 16
        targetSdkVersion 30
    }
}
""",
            encoding="utf-8",
        )

        # AndroidManifest.xml を作成
        manifest_dir = app_dir / "src" / "main"
        manifest_dir.mkdir(parents=True)
        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer.prepare(
            package_name="com.example.game",
            app_name="My Game",
        )

        # jniLibs が作成されていることを確認
        jni_libs_dir = project_dir / "app" / "src" / "main" / "jniLibs"
        assert jni_libs_dir.exists()
        assert (jni_libs_dir / "arm64-v8a" / "libmain.so").exists()

        # Java ファイルが作成されていることを確認
        java_file = (
            project_dir
            / "app"
            / "src"
            / "main"
            / "java"
            / "com"
            / "example"
            / "game"
            / "KirikiriSDL2Activity.java"
        )
        assert java_file.exists()

        # strings.xml が作成されていることを確認
        strings_xml = project_dir / "app" / "src" / "main" / "res" / "values" / "strings.xml"
        assert strings_xml.exists()
        assert "My Game" in strings_xml.read_text(encoding="utf-8")

        # デフォルトアイコンが生成されていることを確認（icon_path=Noneの場合）
        res_dir = project_dir / "app" / "src" / "main" / "res"
        assert (res_dir / "mipmap-mdpi" / "ic_launcher.png").exists()

    def test_prepare_creates_default_icon_when_no_icon_provided(
        self, tmp_path: Path, mock_sdl2_fetcher: MagicMock
    ) -> None:
        """正常系: アイコンが提供されない場合にデフォルトアイコンが生成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # APK ファイルを作成
        apk_path = project_dir / "krkrsdl2_universal.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr("lib/arm64-v8a/libmain.so", b"fake so content")

        # app/build.gradle を作成
        app_dir = project_dir / "app"
        app_dir.mkdir()
        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
    defaultConfig {
        applicationId "pw.uyjulian.krkrsdl2"
        minSdkVersion 16
        targetSdkVersion 30
    }
}
""",
            encoding="utf-8",
        )

        # AndroidManifest.xml を作成
        manifest_dir = app_dir / "src" / "main"
        manifest_dir.mkdir(parents=True)
        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        # icon_path を指定しない
        preparer.prepare(
            package_name="com.example.game",
            app_name="My Game",
            icon_path=None,
        )

        # デフォルトアイコンが生成されていることを確認
        res_dir = project_dir / "app" / "src" / "main" / "res"
        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]
        for density in densities:
            icon_file = res_dir / f"mipmap-{density}" / "ic_launcher.png"
            assert icon_file.exists(), f"mipmap-{density} にアイコンがありません"

    def test_prepare_raises_error_when_apk_not_found(self, tmp_path: Path) -> None:
        """異常系: APK ファイルが見つからない場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(JniLibsNotFoundError) as exc_info:
            preparer.prepare(
                package_name="com.example.game",
                app_name="My Game",
            )

        assert "ベースAPKが見つかりません" in str(exc_info.value)


class TestTemplatePreparerExtractJniLibs:
    """TemplatePreparer._extract_jni_libsのテスト"""

    def test_extract_jni_libs_extracts_so_files(self, tmp_path: Path) -> None:
        """正常系: APK から .so ファイルを抽出"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # APK ファイルを作成
        apk_path = project_dir / "krkrsdl2_universal.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr("lib/arm64-v8a/libmain.so", b"arm64 so content")
            zf.writestr("lib/armeabi-v7a/libmain.so", b"armeabi so content")
            zf.writestr("lib/x86/libmain.so", b"x86 so content")
            zf.writestr("lib/x86_64/libmain.so", b"x86_64 so content")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._extract_jni_libs()

        jni_libs_dir = project_dir / "app" / "src" / "main" / "jniLibs"
        assert (jni_libs_dir / "arm64-v8a" / "libmain.so").exists()
        assert (jni_libs_dir / "armeabi-v7a" / "libmain.so").exists()
        assert (jni_libs_dir / "x86" / "libmain.so").exists()
        assert (jni_libs_dir / "x86_64" / "libmain.so").exists()

        # ファイル内容が正しいか確認
        assert (jni_libs_dir / "arm64-v8a" / "libmain.so").read_bytes() == b"arm64 so content"

    def test_extract_jni_libs_raises_error_when_apk_not_exists(self, tmp_path: Path) -> None:
        """異常系: APK ファイルが存在しない場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(JniLibsNotFoundError) as exc_info:
            preparer._extract_jni_libs()

        assert "ベースAPKが見つかりません" in str(exc_info.value)

    def test_extract_jni_libs_raises_error_when_no_so_files(self, tmp_path: Path) -> None:
        """異常系: APK 内に .so ファイルがない場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # .so ファイルを含まない APK を作成
        apk_path = project_dir / "krkrsdl2_universal.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr("AndroidManifest.xml", b"manifest content")
            zf.writestr("classes.dex", b"dex content")

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(JniLibsNotFoundError) as exc_info:
            preparer._extract_jni_libs()

        assert ".soファイルが見つかりません" in str(exc_info.value)

    def test_extract_jni_libs_raises_error_when_apk_is_invalid(self, tmp_path: Path) -> None:
        """異常系: APK ファイルが不正な場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # 不正な APK ファイルを作成
        apk_path = project_dir / "krkrsdl2_universal.apk"
        apk_path.write_bytes(b"invalid zip content")

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(TemplatePreparerError) as exc_info:
            preparer._extract_jni_libs()

        assert "無効なAPKファイルです" in str(exc_info.value)

    @pytest.mark.parametrize(
        "abi,expected_extracted",
        [
            pytest.param("arm64-v8a", True, id="正常系: arm64-v8a はサポート対象"),
            pytest.param("armeabi-v7a", True, id="正常系: armeabi-v7a はサポート対象"),
            pytest.param("x86", True, id="正常系: x86 はサポート対象"),
            pytest.param("x86_64", True, id="正常系: x86_64 はサポート対象"),
            pytest.param("mips", False, id="正常系: mips はサポート対象外"),
            pytest.param("armeabi", False, id="正常系: armeabi はサポート対象外"),
        ],
    )
    def test_extract_jni_libs_filters_supported_abis(
        self, tmp_path: Path, abi: str, expected_extracted: bool
    ) -> None:
        """正常系: サポートされる ABI のみ抽出される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # APK ファイルを作成（サポート対象の ABI も含める）
        apk_path = project_dir / "krkrsdl2_universal.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr(f"lib/{abi}/libtest.so", b"test so content")
            # 少なくとも 1 つのサポート対象 ABI を含める
            zf.writestr("lib/arm64-v8a/libmain.so", b"arm64 so content")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._extract_jni_libs()

        jni_libs_dir = project_dir / "app" / "src" / "main" / "jniLibs"
        so_file = jni_libs_dir / abi / "libtest.so"

        assert so_file.exists() == expected_extracted


class TestTemplatePreparerUpdateJavaSource:
    """TemplatePreparer._update_java_sourceのテスト"""

    def test_update_java_source_creates_java_file(self, tmp_path: Path) -> None:
        """正常系: Java ファイルが正しく生成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_java_source("com.example.game")

        java_file = (
            project_dir
            / "app"
            / "src"
            / "main"
            / "java"
            / "com"
            / "example"
            / "game"
            / "KirikiriSDL2Activity.java"
        )
        assert java_file.exists()

    @pytest.mark.parametrize(
        "package_name",
        [
            pytest.param("com.example.game", id="正常系: 標準的なパッケージ名"),
            pytest.param("jp.example.mygame", id="正常系: 日本ドメインのパッケージ名"),
            pytest.param("org.test.app123", id="正常系: 数字を含むパッケージ名"),
        ],
    )
    def test_update_java_source_reflects_package_name(
        self, tmp_path: Path, package_name: str
    ) -> None:
        """正常系: パッケージ名が正しく反映される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_java_source(package_name)

        package_path = package_name.replace(".", "/")
        java_file = (
            project_dir
            / "app"
            / "src"
            / "main"
            / "java"
            / package_path
            / "KirikiriSDL2Activity.java"
        )
        assert java_file.exists()

        content = java_file.read_text(encoding="utf-8")
        assert f"package {package_name};" in content

    def test_update_java_source_removes_old_java_dir(self, tmp_path: Path) -> None:
        """正常系: 古い Java ディレクトリが削除される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # 古い Java ディレクトリを作成
        old_java_dir = (
            project_dir / "app" / "src" / "main" / "java" / "pw" / "uyjulian" / "krkrsdl2"
        )
        old_java_dir.mkdir(parents=True)
        old_java_file = old_java_dir / "KirikiriSDL2Activity.java"
        old_java_file.write_text("old content", encoding="utf-8")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_java_source("com.example.game")

        # 古いディレクトリが削除されていることを確認
        assert not old_java_dir.exists()

    def test_update_java_source_includes_holdalpha_argument(self, tmp_path: Path) -> None:
        """正常系: getArguments に -holdalpha=yes が含まれる"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_java_source("com.example.game")

        java_file = (
            project_dir
            / "app"
            / "src"
            / "main"
            / "java"
            / "com"
            / "example"
            / "game"
            / "KirikiriSDL2Activity.java"
        )
        content = java_file.read_text(encoding="utf-8")

        # -holdalpha=yes がgetArguments内に含まれていることを確認
        assert '"-holdalpha=yes"' in content

    def test_update_java_source_includes_simd_disable_arguments(self, tmp_path: Path) -> None:
        """正常系: getArguments に SIMD無効化フラグが含まれる（C実装を使用）"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_java_source("com.example.game")

        java_file = (
            project_dir
            / "app"
            / "src"
            / "main"
            / "java"
            / "com"
            / "example"
            / "game"
            / "KirikiriSDL2Activity.java"
        )
        content = java_file.read_text(encoding="utf-8")

        # SIMD無効化フラグがgetArguments内に含まれていることを確認
        # ARMデバイスではSIMDeエミュレーションに問題があるため、
        # 純粋なC実装のブレンド関数を使用することでアルファブレンディングの互換性を確保
        assert '"-cpummx=no"' in content
        assert '"-cpusse=no"' in content
        assert '"-cpusse2=no"' in content


class TestTemplatePreparerUpdateBuildGradle:
    """TemplatePreparer._update_build_gradleのテスト"""

    def test_update_build_gradle_adds_namespace(self, tmp_path: Path) -> None:
        """正常系: namespace が追加される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
}
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert 'namespace "com.example.game"' in content

    def test_update_build_gradle_updates_compile_sdk_version(self, tmp_path: Path) -> None:
        """正常系: compileSdkVersion が 34 に更新される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
}
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert "compileSdkVersion 34" in content

    def test_update_build_gradle_updates_target_sdk_version(self, tmp_path: Path) -> None:
        """正常系: targetSdkVersion が 34 に更新される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
    defaultConfig {
        targetSdkVersion 30
    }
}
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert "targetSdkVersion 34" in content

    def test_update_build_gradle_updates_min_sdk_version(self, tmp_path: Path) -> None:
        """正常系: minSdkVersion が 21 に更新される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
    defaultConfig {
        minSdkVersion 16
    }
}
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert "minSdkVersion 21" in content

    def test_update_build_gradle_removes_cmake_settings(self, tmp_path: Path) -> None:
        """正常系: CMake 設定が削除される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
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
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert "cmake" not in content.lower()
        assert "externalNativeBuild" not in content

    def test_update_build_gradle_updates_application_id(self, tmp_path: Path) -> None:
        """正常系: applicationId が更新される"""
        project_dir = tmp_path / "project"
        app_dir = project_dir / "app"
        app_dir.mkdir(parents=True)

        build_gradle = app_dir / "build.gradle"
        build_gradle.write_text(
            """
android {
    compileSdkVersion 30
    defaultConfig {
        applicationId "pw.uyjulian.krkrsdl2"
    }
}
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_build_gradle("com.example.game")

        content = build_gradle.read_text(encoding="utf-8")
        assert 'applicationId "com.example.game"' in content

    def test_update_build_gradle_raises_error_when_file_not_found(self, tmp_path: Path) -> None:
        """異常系: build.gradle が見つからない場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(TemplatePreparerError) as exc_info:
            preparer._update_build_gradle("com.example.game")

        assert "build.gradleが見つかりません" in str(exc_info.value)


class TestTemplatePreparerUpdateManifest:
    """TemplatePreparer._update_manifestのテスト"""

    def test_update_manifest_removes_package_attribute(self, tmp_path: Path) -> None:
        """正常系: package 属性が削除される"""
        project_dir = tmp_path / "project"
        manifest_dir = project_dir / "app" / "src" / "main"
        manifest_dir.mkdir(parents=True)

        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="pw.uyjulian.krkrsdl2">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_manifest()

        content = manifest_path.read_text(encoding="utf-8")
        assert 'package="' not in content

    def test_update_manifest_adds_exported_true_to_activity(self, tmp_path: Path) -> None:
        """正常系: android:exported="true" が activity に追加される"""
        project_dir = tmp_path / "project"
        manifest_dir = project_dir / "app" / "src" / "main"
        manifest_dir.mkdir(parents=True)

        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity">
        </activity>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_manifest()

        content = manifest_path.read_text(encoding="utf-8")
        assert 'android:exported="true"' in content

    def test_update_manifest_adds_exported_true_to_service(self, tmp_path: Path) -> None:
        """正常系: android:exported="true" が service に追加される"""
        project_dir = tmp_path / "project"
        manifest_dir = project_dir / "app" / "src" / "main"
        manifest_dir.mkdir(parents=True)

        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <service android:name=".MyService">
        </service>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_manifest()

        content = manifest_path.read_text(encoding="utf-8")
        assert 'android:exported="true"' in content

    def test_update_manifest_does_not_duplicate_exported(self, tmp_path: Path) -> None:
        """正常系: 既に exported がある場合は重複追加しない"""
        project_dir = tmp_path / "project"
        manifest_dir = project_dir / "app" / "src" / "main"
        manifest_dir.mkdir(parents=True)

        manifest_path = manifest_dir / "AndroidManifest.xml"
        manifest_path.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <application>
        <activity android:name=".KirikiriSDL2Activity" android:exported="false">
        </activity>
    </application>
</manifest>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_manifest()

        content = manifest_path.read_text(encoding="utf-8")
        # exported は 1 回だけ出現する
        assert content.count('android:exported="') == 1

    def test_update_manifest_raises_error_when_file_not_found(self, tmp_path: Path) -> None:
        """異常系: AndroidManifest.xml が見つからない場合"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        with pytest.raises(TemplatePreparerError) as exc_info:
            preparer._update_manifest()

        assert "AndroidManifest.xmlが見つかりません" in str(exc_info.value)


class TestTemplatePreparerUpdateStringsXml:
    """TemplatePreparer._update_strings_xmlのテスト"""

    def test_update_strings_xml_creates_file(self, tmp_path: Path) -> None:
        """正常系: strings.xml が作成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_strings_xml("My Game")

        strings_xml = project_dir / "app" / "src" / "main" / "res" / "values" / "strings.xml"
        assert strings_xml.exists()
        content = strings_xml.read_text(encoding="utf-8")
        assert '<string name="app_name">My Game</string>' in content

    def test_update_strings_xml_updates_existing_file(self, tmp_path: Path) -> None:
        """正常系: 既存の strings.xml が更新される"""
        project_dir = tmp_path / "project"
        values_dir = project_dir / "app" / "src" / "main" / "res" / "values"
        values_dir.mkdir(parents=True)

        strings_xml = values_dir / "strings.xml"
        strings_xml.write_text(
            """<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">Old Name</string>
    <string name="other">Other Value</string>
</resources>
""",
            encoding="utf-8",
        )

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_strings_xml("New Game Name")

        content = strings_xml.read_text(encoding="utf-8")
        assert '<string name="app_name">New Game Name</string>' in content
        assert "Old Name" not in content
        assert "Other Value" in content

    @pytest.mark.parametrize(
        "app_name,expected_escaped",
        [
            pytest.param("Game & Play", "Game &amp; Play", id="正常系: アンパサンドのエスケープ"),
            pytest.param("Game <Test>", "Game &lt;Test&gt;", id="正常系: 山括弧のエスケープ"),
            pytest.param(
                'Game "Quote"', "Game &quot;Quote&quot;", id="正常系: ダブルクオートのエスケープ"
            ),
            pytest.param(
                "Game 'Single'",
                "Game &#x27;Single&#x27;",
                id="正常系: シングルクオートのエスケープ",
            ),
        ],
    )
    def test_update_strings_xml_escapes_special_characters(
        self, tmp_path: Path, app_name: str, expected_escaped: str
    ) -> None:
        """正常系: XML 特殊文字がエスケープされる"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_strings_xml(app_name)

        strings_xml = project_dir / "app" / "src" / "main" / "res" / "values" / "strings.xml"
        content = strings_xml.read_text(encoding="utf-8")
        assert expected_escaped in content


class TestTemplatePreparerUpdateIcon:
    """TemplatePreparer._update_iconのテスト"""

    def test_update_icon_copies_to_all_densities(self, tmp_path: Path) -> None:
        """正常系: 各解像度にアイコンがコピーされる"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # テスト用アイコンを作成
        icon_path = tmp_path / "icon.png"
        icon_path.write_bytes(b"fake png content")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_icon(icon_path)

        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]
        res_dir = project_dir / "app" / "src" / "main" / "res"

        for density in densities:
            icon_file = res_dir / f"mipmap-{density}" / "ic_launcher.png"
            assert icon_file.exists(), f"mipmap-{density} にアイコンがありません"
            assert icon_file.read_bytes() == b"fake png content"

    @pytest.mark.parametrize(
        "density",
        [
            pytest.param("mdpi", id="正常系: mdpi 解像度"),
            pytest.param("hdpi", id="正常系: hdpi 解像度"),
            pytest.param("xhdpi", id="正常系: xhdpi 解像度"),
            pytest.param("xxhdpi", id="正常系: xxhdpi 解像度"),
            pytest.param("xxxhdpi", id="正常系: xxxhdpi 解像度"),
        ],
    )
    def test_update_icon_creates_mipmap_directory(self, tmp_path: Path, density: str) -> None:
        """正常系: mipmap ディレクトリが作成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        icon_path = tmp_path / "icon.png"
        icon_path.write_bytes(b"fake png content")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._update_icon(icon_path)

        mipmap_dir = project_dir / "app" / "src" / "main" / "res" / f"mipmap-{density}"
        assert mipmap_dir.exists()


class TestTemplatePreparerCopyAssets:
    """TemplatePreparer._copy_assetsのテスト"""

    def test_copy_assets_copies_files(self, tmp_path: Path) -> None:
        """正常系: ゲームファイルがコピーされる"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        # テスト用アセットディレクトリを作成
        assets_src = tmp_path / "game_files"
        assets_src.mkdir()
        (assets_src / "data.xp3").write_bytes(b"xp3 content")
        (assets_src / "config.tjs").write_text("config", encoding="utf-8")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._copy_assets(assets_src)

        assets_dest = project_dir / "app" / "src" / "main" / "assets" / "data"
        assert (assets_dest / "data.xp3").exists()
        assert (assets_dest / "config.tjs").exists()

    def test_copy_assets_copies_subdirectories(self, tmp_path: Path) -> None:
        """正常系: サブディレクトリもコピーされる"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        assets_src = tmp_path / "game_files"
        assets_src.mkdir()
        (assets_src / "scenario").mkdir()
        (assets_src / "scenario" / "first.ks").write_text("scenario", encoding="utf-8")

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._copy_assets(assets_src)

        assets_dest = project_dir / "app" / "src" / "main" / "assets" / "data"
        assert (assets_dest / "scenario" / "first.ks").exists()


class TestTemplatePreparerCreateDefaultIcon:
    """TemplatePreparer._create_default_iconのテスト"""

    def test_create_default_icon_creates_icons_for_all_densities(self, tmp_path: Path) -> None:
        """正常系: 全ての解像度でデフォルトアイコンが生成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._create_default_icon()

        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]
        res_dir = project_dir / "app" / "src" / "main" / "res"

        for density in densities:
            icon_file = res_dir / f"mipmap-{density}" / "ic_launcher.png"
            assert icon_file.exists(), f"mipmap-{density} にアイコンがありません"

    @pytest.mark.parametrize(
        "density,expected_size",
        [
            pytest.param("mdpi", 48, id="正常系: mdpi は 48x48"),
            pytest.param("hdpi", 72, id="正常系: hdpi は 72x72"),
            pytest.param("xhdpi", 96, id="正常系: xhdpi は 96x96"),
            pytest.param("xxhdpi", 144, id="正常系: xxhdpi は 144x144"),
            pytest.param("xxxhdpi", 192, id="正常系: xxxhdpi は 192x192"),
        ],
    )
    def test_create_default_icon_generates_correct_sizes(
        self, tmp_path: Path, density: str, expected_size: int
    ) -> None:
        """正常系: 各解像度で正しいサイズのアイコンが生成される"""
        from PIL import Image

        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._create_default_icon()

        res_dir = project_dir / "app" / "src" / "main" / "res"
        icon_file = res_dir / f"mipmap-{density}" / "ic_launcher.png"

        with Image.open(icon_file) as img:
            assert img.size == (expected_size, expected_size)

    def test_create_default_icon_generates_valid_png(self, tmp_path: Path) -> None:
        """正常系: 有効なPNGファイルが生成される"""
        from PIL import Image

        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._create_default_icon()

        res_dir = project_dir / "app" / "src" / "main" / "res"
        icon_file = res_dir / "mipmap-mdpi" / "ic_launcher.png"

        # PIL で読み込めれば有効な PNG
        with Image.open(icon_file) as img:
            assert img.format == "PNG"
            assert img.mode == "RGB"


class TestTemplatePreparerFetchSDL2Sources:
    """TemplatePreparer._fetch_sdl2_sources のテスト"""

    def test_fetch_sdl2_sources_creates_java_files(
        self, tmp_path: Path, mock_sdl2_fetcher: MagicMock
    ) -> None:
        """正常系: SDL2 Java ソースが作成される"""
        project_dir = tmp_path / "project"
        project_dir.mkdir()

        preparer = TemplatePreparer(project_dir=project_dir)

        preparer._fetch_sdl2_sources()

        sdl_app_dir = project_dir / "app" / "src" / "main" / "java" / "org" / "libsdl" / "app"
        assert sdl_app_dir.exists()
        assert (sdl_app_dir / "SDLActivity.java").exists()

    def test_fetch_sdl2_sources_passes_cache_to_fetcher(
        self, tmp_path: Path, mock_sdl2_fetcher: MagicMock
    ) -> None:
        """正常系: キャッシュが SDL2SourceFetcher に渡される"""
        from mnemonic.builder.sdl2_sources import SDL2SourceCache

        project_dir = tmp_path / "project"
        project_dir.mkdir()

        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")
        preparer = TemplatePreparer(project_dir=project_dir, sdl2_cache=cache)

        preparer._fetch_sdl2_sources()

        mock_sdl2_fetcher.assert_called_once_with(cache=cache)


class TestExceptionClasses:
    """例外クラスのテスト"""

    def test_template_preparer_error_is_exception(self) -> None:
        """正常系: TemplatePreparerError は Exception を継承"""
        error = TemplatePreparerError("test error")
        assert isinstance(error, Exception)
        assert str(error) == "test error"

    def test_jni_libs_not_found_error_is_template_preparer_error(self) -> None:
        """正常系: JniLibsNotFoundError は TemplatePreparerError を継承"""
        error = JniLibsNotFoundError("JNI not found")
        assert isinstance(error, TemplatePreparerError)
        assert str(error) == "JNI not found"

    def test_sdl2_source_fetch_error_is_template_preparer_error(self) -> None:
        """正常系: SDL2SourceFetchError は TemplatePreparerError を継承"""
        error = SDL2SourceFetchError("SDL2 fetch error")
        assert isinstance(error, TemplatePreparerError)
        assert str(error) == "SDL2 fetch error"
