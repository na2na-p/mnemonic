"""APKマージビルド機能のテスト

このモジュールはApkMergeBuilderクラスのテストを提供します。
"""

import struct
import zipfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.builder.apk_merge import (
    ApkMergeBuilder,
    ApkMergeConfig,
    ApkMergeError,
    ApkMergeResult,
    ApkNotFoundError,
    ApktoolError,
    InvalidApkError,
    ManifestUpdateError,
)


def create_minimal_binary_manifest() -> bytes:
    """最小限のバイナリAndroidManifest.xmlを作成する

    実際のバイナリXML形式をシミュレートします。
    """
    # AXMLマジックナンバー（0x00080003）
    header = struct.pack("<I", 0x00080003)
    # ファイルサイズ（仮）
    file_size = struct.pack("<I", 100)
    # 文字列プールを含むダミーデータ
    # パッケージ名を含むデータ
    package_name = b"org.kirikiri.krkrsdl2"
    padding = b"\x00" * (100 - len(header) - len(file_size) - len(package_name))

    return header + file_size + package_name + padding


def create_mock_apk(apk_path: Path, include_manifest: bool = True) -> None:
    """テスト用のモックAPKを作成する

    Args:
        apk_path: 作成するAPKのパス
        include_manifest: AndroidManifest.xmlを含めるか
    """
    with zipfile.ZipFile(apk_path, "w") as zf:
        if include_manifest:
            zf.writestr("AndroidManifest.xml", create_minimal_binary_manifest())
        zf.writestr("classes.dex", b"mock dex content")
        zf.writestr("resources.arsc", b"mock resources")
        zf.writestr("lib/arm64-v8a/libSDL2.so", b"mock native library")


class TestApkMergeConfig:
    """ApkMergeConfigデータクラスのテスト"""

    def test_creation(self, tmp_path: Path) -> None:
        """正常系: ApkMergeConfigの作成"""
        config = ApkMergeConfig(
            base_apk_path=tmp_path / "base.apk",
            assets_dir=tmp_path / "assets",
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        assert config.base_apk_path == tmp_path / "base.apk"
        assert config.assets_dir == tmp_path / "assets"
        assert config.package_name == "com.example.game"
        assert config.app_name == "My Game"
        assert config.output_path == tmp_path / "output.apk"

    def test_immutability(self, tmp_path: Path) -> None:
        """正常系: ApkMergeConfigが不変であることのテスト"""
        config = ApkMergeConfig(
            base_apk_path=tmp_path / "base.apk",
            assets_dir=tmp_path / "assets",
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        with pytest.raises(AttributeError):
            config.package_name = "com.other.game"  # type: ignore[misc]


class TestApkMergeResult:
    """ApkMergeResultデータクラスのテスト"""

    def test_creation_success(self, tmp_path: Path) -> None:
        """正常系: 成功時のApkMergeResult作成"""
        result = ApkMergeResult(
            success=True,
            apk_path=tmp_path / "output.apk",
            error_message=None,
            files_added=100,
            total_size=1024 * 1024,
        )

        assert result.success is True
        assert result.apk_path == tmp_path / "output.apk"
        assert result.error_message is None
        assert result.files_added == 100
        assert result.total_size == 1024 * 1024

    def test_creation_failure(self) -> None:
        """正常系: 失敗時のApkMergeResult作成"""
        result = ApkMergeResult(
            success=False,
            apk_path=None,
            error_message="マージに失敗しました",
        )

        assert result.success is False
        assert result.apk_path is None
        assert result.error_message == "マージに失敗しました"
        assert result.files_added == 0
        assert result.total_size == 0

    def test_immutability(self, tmp_path: Path) -> None:
        """正常系: ApkMergeResultが不変であることのテスト"""
        result = ApkMergeResult(
            success=True,
            apk_path=tmp_path / "output.apk",
            error_message=None,
        )

        with pytest.raises(AttributeError):
            result.success = False  # type: ignore[misc]


class TestApkMergeBuilderValidation:
    """ApkMergeBuilder入力検証のテスト"""

    def test_merge_raises_error_when_base_apk_not_found(self, tmp_path: Path) -> None:
        """異常系: ベースAPKが存在しない場合にApkNotFoundErrorが発生"""
        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()

        config = ApkMergeConfig(
            base_apk_path=tmp_path / "nonexistent.apk",
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        builder = ApkMergeBuilder()

        with pytest.raises(ApkNotFoundError) as exc_info:
            builder.merge(config)

        assert "ベースAPKが見つかりません" in str(exc_info.value)

    def test_merge_raises_error_when_base_apk_invalid(self, tmp_path: Path) -> None:
        """異常系: ベースAPKが無効な形式の場合にInvalidApkErrorが発生"""
        base_apk = tmp_path / "invalid.apk"
        base_apk.write_text("not a zip file")

        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        builder = ApkMergeBuilder()

        with pytest.raises(InvalidApkError) as exc_info:
            builder.merge(config)

        assert "無効なAPKファイル" in str(exc_info.value)

    def test_merge_raises_error_when_assets_dir_not_found(self, tmp_path: Path) -> None:
        """異常系: assetsディレクトリが存在しない場合にApkMergeErrorが発生"""
        base_apk = tmp_path / "base.apk"
        create_mock_apk(base_apk)

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=tmp_path / "nonexistent_assets",
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        builder = ApkMergeBuilder()

        with pytest.raises(ApkMergeError) as exc_info:
            builder.merge(config)

        assert "assetsディレクトリが見つかりません" in str(exc_info.value)


def create_mock_decoded_apk(decode_dir: Path) -> None:
    """apktoolでデコードしたAPKをシミュレートするディレクトリ構造を作成する

    Args:
        decode_dir: デコード先ディレクトリ
    """
    decode_dir.mkdir(parents=True, exist_ok=True)

    # AndroidManifest.xml（テキスト形式、apktoolでデコード後の形式）
    manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest package="org.kirikiri.krkrsdl2"
    xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-sdk android:minSdkVersion="21" android:targetSdkVersion="30"/>
    <application android:label="@string/app_name">
    </application>
</manifest>"""
    (decode_dir / "AndroidManifest.xml").write_text(manifest_content, encoding="utf-8")

    # res/values/strings.xml
    res_values_dir = decode_dir / "res" / "values"
    res_values_dir.mkdir(parents=True, exist_ok=True)
    strings_content = """<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">KrkrSDL2</string>
</resources>"""
    (res_values_dir / "strings.xml").write_text(strings_content, encoding="utf-8")


class TestApkMergeBuilderMerge:
    """ApkMergeBuilder.mergeのテスト（apktoolをモック）"""

    @pytest.fixture
    def mock_apktool_path(self, tmp_path: Path) -> Path:
        """モック用のapktoolパスを作成"""
        apktool_path = tmp_path / "apktool.jar"
        apktool_path.write_bytes(b"mock apktool")
        return apktool_path

    def test_merge_creates_output_apk(self, tmp_path: Path, mock_apktool_path: Path) -> None:
        """正常系: マージ後に出力APKが作成される"""
        base_apk = tmp_path / "base.apk"
        create_mock_apk(base_apk)

        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()
        (assets_dir / "game.dat").write_bytes(b"game data")

        output_apk = tmp_path / "output" / "game.apk"

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=output_apk,
        )

        def mock_subprocess_run(cmd: list, **kwargs) -> MagicMock:  # noqa: ANN003
            """subprocess.runをモックしてapktoolの動作をシミュレート"""
            result = MagicMock()
            result.returncode = 0
            result.stdout = ""
            result.stderr = ""

            # cmd[3]がapktoolのサブコマンド（"d"=decode, "b"=build）
            if len(cmd) > 3 and cmd[3] == "d":
                # decode: APKをデコードしてディレクトリを作成
                output_dir = Path(cmd[cmd.index("-o") + 1])
                create_mock_decoded_apk(output_dir)
            elif len(cmd) > 3 and cmd[3] == "b":
                # build: ディレクトリからAPKを作成
                source_dir = Path(cmd[4])
                output_file = Path(cmd[cmd.index("-o") + 1])
                output_file.parent.mkdir(parents=True, exist_ok=True)
                # 簡易的なAPKを作成
                with zipfile.ZipFile(output_file, "w") as zf:
                    zf.writestr("AndroidManifest.xml", b"mock manifest")
                    zf.writestr("classes.dex", b"mock dex")
                    # assetsディレクトリがあればコピー
                    assets_src = source_dir / "assets"
                    if assets_src.exists():
                        for f in assets_src.rglob("*"):
                            if f.is_file():
                                arcname = "assets/" + str(f.relative_to(assets_src))
                                zf.writestr(arcname, f.read_bytes())

            return result

        builder = ApkMergeBuilder(apktool_path=mock_apktool_path)

        with patch("mnemonic.builder.apk_merge.subprocess.run", side_effect=mock_subprocess_run):
            result = builder.merge(config)

        assert result.success is True
        assert result.apk_path is not None
        assert result.apk_path.exists()
        assert result.error_message is None

    def test_merge_adds_assets_to_apk(self, tmp_path: Path, mock_apktool_path: Path) -> None:
        """正常系: assetsがAPKに追加される"""
        base_apk = tmp_path / "base.apk"
        create_mock_apk(base_apk)

        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()
        (assets_dir / "data.xp3").write_bytes(b"xp3 archive")
        (assets_dir / "startup.tjs").write_bytes(b"startup script")
        subdir = assets_dir / "scenario"
        subdir.mkdir()
        (subdir / "first.ks").write_bytes(b"scenario script")

        output_apk = tmp_path / "output.apk"

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=output_apk,
        )

        def mock_subprocess_run(cmd: list, **kwargs) -> MagicMock:  # noqa: ANN003
            """subprocess.runをモックしてapktoolの動作をシミュレート"""
            result = MagicMock()
            result.returncode = 0
            result.stdout = ""
            result.stderr = ""

            # cmd[3]がapktoolのサブコマンド（"d"=decode, "b"=build）
            if len(cmd) > 3 and cmd[3] == "d":
                output_dir = Path(cmd[cmd.index("-o") + 1])
                create_mock_decoded_apk(output_dir)
            elif len(cmd) > 3 and cmd[3] == "b":
                source_dir = Path(cmd[4])
                output_file = Path(cmd[cmd.index("-o") + 1])
                output_file.parent.mkdir(parents=True, exist_ok=True)
                with zipfile.ZipFile(output_file, "w") as zf:
                    zf.writestr("AndroidManifest.xml", b"mock manifest")
                    zf.writestr("classes.dex", b"mock dex")
                    assets_src = source_dir / "assets"
                    if assets_src.exists():
                        for f in assets_src.rglob("*"):
                            if f.is_file():
                                arcname = "assets/" + str(f.relative_to(assets_src))
                                zf.writestr(arcname, f.read_bytes())

            return result

        builder = ApkMergeBuilder(apktool_path=mock_apktool_path)

        with patch("mnemonic.builder.apk_merge.subprocess.run", side_effect=mock_subprocess_run):
            result = builder.merge(config)

        assert result.success is True
        assert result.files_added == 3

        # APKの内容を確認
        with zipfile.ZipFile(output_apk, "r") as zf:
            names = zf.namelist()
            assert "assets/data.xp3" in names
            assert "assets/startup.tjs" in names
            assert "assets/scenario/first.ks" in names

    def test_merge_returns_correct_statistics(
        self, tmp_path: Path, mock_apktool_path: Path
    ) -> None:
        """正常系: 統計情報が正しく返される"""
        base_apk = tmp_path / "base.apk"
        create_mock_apk(base_apk)

        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()
        (assets_dir / "file1.dat").write_bytes(b"data1")
        (assets_dir / "file2.dat").write_bytes(b"data2")

        output_apk = tmp_path / "output.apk"

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=output_apk,
        )

        def mock_subprocess_run(cmd: list, **kwargs) -> MagicMock:  # noqa: ANN003
            result = MagicMock()
            result.returncode = 0
            result.stdout = ""
            result.stderr = ""

            # cmd[3]がapktoolのサブコマンド（"d"=decode, "b"=build）
            if len(cmd) > 3 and cmd[3] == "d":
                output_dir = Path(cmd[cmd.index("-o") + 1])
                create_mock_decoded_apk(output_dir)
            elif len(cmd) > 3 and cmd[3] == "b":
                source_dir = Path(cmd[4])
                output_file = Path(cmd[cmd.index("-o") + 1])
                output_file.parent.mkdir(parents=True, exist_ok=True)
                with zipfile.ZipFile(output_file, "w") as zf:
                    zf.writestr("AndroidManifest.xml", b"mock manifest")
                    zf.writestr("classes.dex", b"mock dex")
                    assets_src = source_dir / "assets"
                    if assets_src.exists():
                        for f in assets_src.rglob("*"):
                            if f.is_file():
                                arcname = "assets/" + str(f.relative_to(assets_src))
                                zf.writestr(arcname, f.read_bytes())

            return result

        builder = ApkMergeBuilder(apktool_path=mock_apktool_path)

        with patch("mnemonic.builder.apk_merge.subprocess.run", side_effect=mock_subprocess_run):
            result = builder.merge(config)

        assert result.success is True
        assert result.files_added == 2
        assert result.total_size > 0

    def test_merge_raises_apktool_error_when_apktool_not_found(self, tmp_path: Path) -> None:
        """異常系: apktoolが見つからない場合にApktoolErrorが発生"""
        base_apk = tmp_path / "base.apk"
        create_mock_apk(base_apk)

        assets_dir = tmp_path / "assets"
        assets_dir.mkdir()

        config = ApkMergeConfig(
            base_apk_path=base_apk,
            assets_dir=assets_dir,
            package_name="com.example.game",
            app_name="My Game",
            output_path=tmp_path / "output.apk",
        )

        builder = ApkMergeBuilder(apktool_path=tmp_path / "nonexistent.jar")

        with pytest.raises(ApktoolError) as exc_info:
            builder.merge(config)

        assert "apktoolが見つかりません" in str(exc_info.value)


class TestApkMergeBuilderValidateApk:
    """ApkMergeBuilder.validate_apkのテスト"""

    def test_validate_returns_true_for_valid_apk(self, tmp_path: Path) -> None:
        """正常系: 有効なAPKに対してTrueを返す"""
        apk_path = tmp_path / "valid.apk"
        create_mock_apk(apk_path)

        builder = ApkMergeBuilder()
        result = builder.validate_apk(apk_path)

        assert result is True

    def test_validate_returns_false_when_file_not_exists(self, tmp_path: Path) -> None:
        """正常系: ファイルが存在しない場合にFalseを返す"""
        builder = ApkMergeBuilder()
        result = builder.validate_apk(tmp_path / "nonexistent.apk")

        assert result is False

    def test_validate_returns_false_for_non_zip_file(self, tmp_path: Path) -> None:
        """正常系: ZIPファイルでない場合にFalseを返す"""
        apk_path = tmp_path / "invalid.apk"
        apk_path.write_text("not a zip")

        builder = ApkMergeBuilder()
        result = builder.validate_apk(apk_path)

        assert result is False

    def test_validate_returns_false_when_manifest_missing(self, tmp_path: Path) -> None:
        """正常系: AndroidManifest.xmlがない場合にFalseを返す"""
        apk_path = tmp_path / "no_manifest.apk"
        create_mock_apk(apk_path, include_manifest=False)

        builder = ApkMergeBuilder()
        result = builder.validate_apk(apk_path)

        assert result is False

    def test_validate_returns_false_when_dex_missing(self, tmp_path: Path) -> None:
        """正常系: classes.dexがない場合にFalseを返す"""
        apk_path = tmp_path / "no_dex.apk"
        with zipfile.ZipFile(apk_path, "w") as zf:
            zf.writestr("AndroidManifest.xml", create_minimal_binary_manifest())
            # classes.dexを含めない

        builder = ApkMergeBuilder()
        result = builder.validate_apk(apk_path)

        assert result is False


class TestApkMergeBuilderFindBaseApk:
    """ApkMergeBuilder.find_base_apk_in_templateのテスト"""

    def test_find_apk_in_root(self, tmp_path: Path) -> None:
        """正常系: テンプレートルートにあるAPKを見つける"""
        apk_path = tmp_path / "krkrsdl2_universal.apk"
        create_mock_apk(apk_path)

        builder = ApkMergeBuilder()
        result = builder.find_base_apk_in_template(tmp_path)

        assert result is not None
        assert result.name == "krkrsdl2_universal.apk"

    def test_find_apk_in_subdirectory(self, tmp_path: Path) -> None:
        """正常系: サブディレクトリにあるAPKを見つける"""
        apk_dir = tmp_path / "app" / "build" / "outputs" / "apk" / "release"
        apk_dir.mkdir(parents=True)
        apk_path = apk_dir / "app-release.apk"
        create_mock_apk(apk_path)

        builder = ApkMergeBuilder()
        result = builder.find_base_apk_in_template(tmp_path)

        assert result is not None

    def test_find_apk_returns_none_when_not_found(self, tmp_path: Path) -> None:
        """正常系: APKが見つからない場合にNoneを返す"""
        builder = ApkMergeBuilder()
        result = builder.find_base_apk_in_template(tmp_path)

        assert result is None

    def test_find_apk_prioritizes_krkrsdl2_apk(self, tmp_path: Path) -> None:
        """正常系: krkrsdl2を含むAPKが優先される"""
        other_apk = tmp_path / "other.apk"
        create_mock_apk(other_apk)

        krkrsdl2_apk = tmp_path / "krkrsdl2_universal.apk"
        create_mock_apk(krkrsdl2_apk)

        builder = ApkMergeBuilder()
        result = builder.find_base_apk_in_template(tmp_path)

        assert result is not None
        assert "krkrsdl2" in result.name.lower()


class TestApkMergeBuilderIsCompressionNeeded:
    """ApkMergeBuilder.is_compression_neededのテスト"""

    @pytest.mark.parametrize(
        "file_path,expected",
        [
            pytest.param(
                "lib/arm64-v8a/libSDL2.so", True, id="正常系: ネイティブライブラリ（圧縮対象）"
            ),
            pytest.param("assets/bgm.ogg", False, id="正常系: OGGファイル（圧縮不要）"),
            pytest.param("assets/music.mp3", False, id="正常系: MP3ファイル（圧縮不要）"),
            pytest.param("assets/sound.wav", False, id="正常系: WAVファイル（圧縮不要）"),
            pytest.param("assets/icon.png", False, id="正常系: PNGファイル（圧縮不要）"),
            pytest.param("assets/bg.jpg", False, id="正常系: JPGファイル（圧縮不要）"),
            pytest.param("assets/video.mp4", False, id="正常系: MP4ファイル（圧縮不要）"),
            pytest.param("assets/script.tjs", True, id="正常系: スクリプトファイル（圧縮対象）"),
            pytest.param("classes.dex", True, id="正常系: DEXファイル（圧縮対象）"),
            pytest.param("res/layout/main.xml", True, id="正常系: XMLファイル（圧縮対象）"),
        ],
    )
    def test_is_compression_needed(self, file_path: str, expected: bool) -> None:
        """正常系: ファイル種別に応じた圧縮判定"""
        builder = ApkMergeBuilder()
        result = builder.is_compression_needed(file_path)

        assert result == expected


class TestExceptionClasses:
    """例外クラスのテスト"""

    def test_apk_merge_error_inheritance(self) -> None:
        """正常系: ApkMergeErrorが適切な継承関係を持つ"""
        error = ApkMergeError("merge failed")
        assert isinstance(error, Exception)
        assert str(error) == "merge failed"

    def test_apk_not_found_error_inheritance(self) -> None:
        """正常系: ApkNotFoundErrorがApkMergeErrorを継承している"""
        error = ApkNotFoundError("apk not found")
        assert isinstance(error, ApkMergeError)
        assert isinstance(error, Exception)
        assert str(error) == "apk not found"

    def test_invalid_apk_error_inheritance(self) -> None:
        """正常系: InvalidApkErrorがApkMergeErrorを継承している"""
        error = InvalidApkError("invalid apk")
        assert isinstance(error, ApkMergeError)
        assert str(error) == "invalid apk"

    def test_manifest_update_error_inheritance(self) -> None:
        """正常系: ManifestUpdateErrorがApkMergeErrorを継承している"""
        error = ManifestUpdateError("manifest update failed")
        assert isinstance(error, ApkMergeError)
        assert str(error) == "manifest update failed"


class TestUpdateManifestExported:
    """_update_manifest の android:exported 属性追加のテスト"""

    def test_adds_exported_to_activity_without_exported(self, tmp_path: Path) -> None:
        """正常系: exported属性がないactivityにexported=trueを追加"""
        # Arrange
        manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest package="com.example.test">
    <application>
        <activity android:name=".MainActivity">
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
            </intent-filter>
        </activity>
    </application>
</manifest>"""
        manifest_path = tmp_path / "AndroidManifest.xml"
        manifest_path.write_text(manifest_content, encoding="utf-8")

        builder = ApkMergeBuilder()

        # Act
        builder._update_manifest(manifest_path, "com.example.new", "TestApp", 34)

        # Assert
        result = manifest_path.read_text(encoding="utf-8")
        assert 'android:exported="true"' in result

    def test_does_not_modify_activity_with_existing_exported(self, tmp_path: Path) -> None:
        """正常系: 既にexported属性があるactivityは変更しない"""
        # Arrange
        manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest package="com.example.test">
    <application>
        <activity android:name=".MainActivity" android:exported="false">
        </activity>
    </application>
</manifest>"""
        manifest_path = tmp_path / "AndroidManifest.xml"
        manifest_path.write_text(manifest_content, encoding="utf-8")

        builder = ApkMergeBuilder()

        # Act
        builder._update_manifest(manifest_path, "com.example.new", "TestApp", 34)

        # Assert
        result = manifest_path.read_text(encoding="utf-8")
        assert 'android:exported="false"' in result
        assert result.count("android:exported") == 1

    def test_handles_self_closing_activity_tag(self, tmp_path: Path) -> None:
        """正常系: 自己閉じタグの<activity ... />にも属性を追加"""
        manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest package="com.example.test">
    <application>
        <activity android:name=".MainActivity"/>
    </application>
</manifest>"""
        manifest_path = tmp_path / "AndroidManifest.xml"
        manifest_path.write_text(manifest_content, encoding="utf-8")

        builder = ApkMergeBuilder()

        # Act
        builder._update_manifest(manifest_path, "com.example.new", "TestApp", 34)

        # Assert
        result = manifest_path.read_text(encoding="utf-8")
        assert 'android:exported="true"/>' in result


class TestUpdateIcon:
    """_update_icon のテスト"""

    def test_copies_icon_to_all_density_directories(self, tmp_path: Path) -> None:
        """正常系: 全解像度のmipmapディレクトリにアイコンをコピー"""
        # Arrange
        decode_dir = tmp_path / "decoded"
        decode_dir.mkdir()

        icon_path = tmp_path / "icon.png"
        icon_path.write_bytes(b"fake png content")

        builder = ApkMergeBuilder()

        # Act
        builder._update_icon(decode_dir, icon_path)

        # Assert
        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]
        for density in densities:
            icon_dest = decode_dir / "res" / f"mipmap-{density}" / "ic_launcher.png"
            assert icon_dest.exists(), f"mipmap-{density}/ic_launcher.png should exist"
            assert icon_dest.read_bytes() == b"fake png content"

    def test_creates_mipmap_directories_if_not_exist(self, tmp_path: Path) -> None:
        """正常系: mipmapディレクトリが存在しない場合は作成"""
        # Arrange
        decode_dir = tmp_path / "decoded"
        # res ディレクトリは存在しない状態

        icon_path = tmp_path / "icon.png"
        icon_path.write_bytes(b"test")

        builder = ApkMergeBuilder()

        # Act
        builder._update_icon(decode_dir, icon_path)

        # Assert
        res_dir = decode_dir / "res"
        assert res_dir.exists()
        assert (res_dir / "mipmap-mdpi").exists()
