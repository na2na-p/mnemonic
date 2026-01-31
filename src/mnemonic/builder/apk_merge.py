"""APKマージビルド機能

このモジュールは既存のベースAPKに対してassetsを追加し、
AndroidManifest.xmlのパッケージ名を更新してAPKを再パッケージする機能を提供します。
apktoolを使用してAPKをデコード/再エンコードします。
"""

from __future__ import annotations

import re
import shutil
import subprocess
import tempfile
import zipfile
from dataclasses import dataclass
from pathlib import Path


class ApkMergeError(Exception):
    """APKマージに関する基本例外クラス"""

    pass


class ApkNotFoundError(ApkMergeError):
    """ベースAPKが見つからない場合の例外"""

    pass


class InvalidApkError(ApkMergeError):
    """APKが無効な形式の場合の例外"""

    pass


class ManifestUpdateError(ApkMergeError):
    """AndroidManifest.xmlの更新に失敗した場合の例外"""

    pass


class ApktoolError(ApkMergeError):
    """apktoolの実行に失敗した場合の例外"""

    pass


@dataclass(frozen=True)
class ApkMergeConfig:
    """APKマージ設定を表す不変オブジェクト

    Attributes:
        base_apk_path: ベースとなるAPKファイルのパス（krkrsdl2_universal.apk）
        assets_dir: 追加するassetsディレクトリのパス
        package_name: 新しいパッケージ名
        app_name: アプリケーション表示名
        output_path: 出力APKファイルのパス
        icon_path: アプリアイコンのパス（オプション）
    """

    base_apk_path: Path
    assets_dir: Path
    package_name: str
    app_name: str
    output_path: Path
    icon_path: Path | None = None


@dataclass(frozen=True)
class ApkMergeResult:
    """APKマージ結果を表す不変オブジェクト

    Attributes:
        success: マージが成功したかどうか
        apk_path: 生成されたAPKファイルのパス。マージ失敗時はNone。
        error_message: エラーメッセージ。成功時はNone。
        files_added: 追加されたファイル数
        total_size: 生成されたAPKの合計サイズ（バイト）
    """

    success: bool
    apk_path: Path | None
    error_message: str | None
    files_added: int = 0
    total_size: int = 0


class ApkMergeBuilder:
    """APKマージビルドを実行するクラス

    このクラスは既存のベースAPKに対してassetsを追加し、
    AndroidManifest.xmlのパッケージ名を更新してAPKを再パッケージする機能を提供します。

    apktoolを使用してAPKをデコード/再エンコードすることで、
    バイナリXMLの正しい編集を実現します。
    """

    # AndroidManifest.xmlのファイル名
    MANIFEST_FILE = "AndroidManifest.xml"

    # assetsディレクトリの名前
    ASSETS_DIR = "assets"

    # apktool JARのデフォルトパス
    DEFAULT_APKTOOL_PATH = "/tmp/apktool/apktool.jar"

    # 推奨 targetSdkVersion（Android 14対応）
    TARGET_SDK_VERSION = 34

    def __init__(self, apktool_path: Path | None = None) -> None:
        """ApkMergeBuilderを初期化する

        Args:
            apktool_path: apktool JARファイルのパス。Noneの場合はデフォルトパスを使用。
        """
        self._apktool_path = apktool_path or Path(self.DEFAULT_APKTOOL_PATH)

    def merge(self, config: ApkMergeConfig) -> ApkMergeResult:
        """APKをマージしてビルドする

        apktoolを使用してベースAPKをデコードし、assetsを追加し、
        AndroidManifest.xmlを更新してAPKを再ビルドします。

        Args:
            config: マージ設定

        Returns:
            マージ結果を表すApkMergeResult

        Raises:
            ApkNotFoundError: ベースAPKが見つからない場合
            InvalidApkError: APKが無効な形式の場合
            ApktoolError: apktoolの実行に失敗した場合
        """
        # 入力検証
        if not config.base_apk_path.exists():
            raise ApkNotFoundError(f"ベースAPKが見つかりません: {config.base_apk_path}")

        if not zipfile.is_zipfile(config.base_apk_path):
            raise InvalidApkError(f"無効なAPKファイルです: {config.base_apk_path}")

        if not config.assets_dir.exists():
            raise ApkMergeError(f"assetsディレクトリが見つかりません: {config.assets_dir}")

        if not self._apktool_path.exists():
            raise ApktoolError(f"apktoolが見つかりません: {self._apktool_path}")

        # 一時ディレクトリでマージ作業を行う
        with tempfile.TemporaryDirectory(prefix="mnemonic_apk_merge_") as temp_dir:
            temp_path = Path(temp_dir)

            try:
                # 1. apktoolでAPKをデコード
                decode_dir = temp_path / "decoded"
                self._decode_apk(config.base_apk_path, decode_dir)

                # 2. AndroidManifest.xmlを更新
                manifest_path = decode_dir / self.MANIFEST_FILE
                self._update_manifest(
                    manifest_path,
                    config.package_name,
                    config.app_name,
                    self.TARGET_SDK_VERSION,
                )

                # 3. assetsを追加
                assets_dest = decode_dir / self.ASSETS_DIR
                files_added = self._add_assets(assets_dest, config.assets_dir)

                # 4. アイコンを更新（指定されている場合）
                if config.icon_path and config.icon_path.exists():
                    self._update_icon(decode_dir, config.icon_path)

                # 5. apktoolで再ビルド
                built_apk = temp_path / "rebuilt.apk"
                self._build_apk(decode_dir, built_apk)

                # 6. 出力先にコピー
                config.output_path.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(built_apk, config.output_path)

                total_size = config.output_path.stat().st_size

                return ApkMergeResult(
                    success=True,
                    apk_path=config.output_path,
                    error_message=None,
                    files_added=files_added,
                    total_size=total_size,
                )

            except ApkMergeError:
                raise
            except Exception as e:
                return ApkMergeResult(
                    success=False,
                    apk_path=None,
                    error_message=str(e),
                )

    def _decode_apk(self, apk_path: Path, output_dir: Path) -> None:
        """apktoolでAPKをデコードする

        Args:
            apk_path: デコードするAPKのパス
            output_dir: 出力ディレクトリ

        Raises:
            ApktoolError: デコードに失敗した場合
        """
        cmd = [
            "java",
            "-jar",
            str(self._apktool_path),
            "d",
            str(apk_path),
            "-o",
            str(output_dir),
            "-f",  # 強制上書き
        ]

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=120,
            )
            if result.returncode != 0:
                raise ApktoolError(f"apktool decode failed: {result.stderr}")
        except subprocess.TimeoutExpired as e:
            raise ApktoolError("apktool decode timed out") from e
        except FileNotFoundError as e:
            raise ApktoolError("java command not found") from e

    def _build_apk(self, source_dir: Path, output_path: Path) -> None:
        """apktoolでAPKをビルドする

        Args:
            source_dir: ソースディレクトリ
            output_path: 出力APKのパス

        Raises:
            ApktoolError: ビルドに失敗した場合
        """
        cmd = [
            "java",
            "-jar",
            str(self._apktool_path),
            "b",
            str(source_dir),
            "-o",
            str(output_path),
        ]

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=300,
            )
            if result.returncode != 0:
                raise ApktoolError(f"apktool build failed: {result.stderr}")
        except subprocess.TimeoutExpired as e:
            raise ApktoolError("apktool build timed out") from e
        except FileNotFoundError as e:
            raise ApktoolError("java command not found") from e

    def _update_manifest(
        self, manifest_path: Path, package_name: str, app_name: str, target_sdk: int
    ) -> None:
        """AndroidManifest.xmlを更新する

        Args:
            manifest_path: AndroidManifest.xmlのパス
            package_name: 新しいパッケージ名
            app_name: 新しいアプリ名
            target_sdk: 新しいtargetSdkVersion

        Raises:
            ManifestUpdateError: 更新に失敗した場合
        """
        if not manifest_path.exists():
            raise ManifestUpdateError(f"AndroidManifest.xmlが見つかりません: {manifest_path}")

        try:
            content = manifest_path.read_text(encoding="utf-8")

            # パッケージ名を更新
            content = re.sub(
                r'package="[^"]*"',
                f'package="{package_name}"',
                content,
            )

            # android:label を更新（リソース参照も直接値に変更）
            content = re.sub(
                r'android:label="[^"]*"',
                f'android:label="{app_name}"',
                content,
            )

            # uses-sdk 要素がなければ追加、あれば更新
            uses_sdk_element = (
                f'<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="{target_sdk}"/>'
            )

            if "<uses-sdk" in content:
                # 既存の uses-sdk を更新
                content = re.sub(
                    r"<uses-sdk[^/>]*/>",
                    uses_sdk_element,
                    content,
                )
            else:
                # manifest タグの直後に uses-sdk を追加
                content = re.sub(
                    r"(<manifest[^>]*>)",
                    r"\1\n    " + uses_sdk_element,
                    content,
                )

            # 各activityタグを個別に処理し、既にexported属性がないものにのみ追加
            # Android 12 (API 31) 以上では、intent-filterを持つコンポーネントに必須
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

            manifest_path.write_text(content, encoding="utf-8")

            # strings.xml も更新（存在する場合）
            self._update_strings_xml(manifest_path.parent, app_name)

        except OSError as e:
            raise ManifestUpdateError(f"AndroidManifest.xmlの更新に失敗しました: {e}") from e

    def _update_strings_xml(self, project_dir: Path, app_name: str) -> None:
        """strings.xmlのapp_nameを更新する

        Args:
            project_dir: プロジェクトディレクトリ
            app_name: 新しいアプリ名
        """
        strings_path = project_dir / "res" / "values" / "strings.xml"
        if not strings_path.exists():
            return

        try:
            content = strings_path.read_text(encoding="utf-8")
            content = re.sub(
                r'(<string name="app_name">)[^<]*(</string>)',
                rf"\1{app_name}\2",
                content,
            )
            strings_path.write_text(content, encoding="utf-8")
        except OSError:
            pass  # strings.xml の更新失敗は無視

    def _add_assets(self, assets_dest: Path, assets_src: Path) -> int:
        """assetsディレクトリにファイルを追加する

        Args:
            assets_dest: 追加先のassetsディレクトリ
            assets_src: 追加するファイルのソースディレクトリ

        Returns:
            追加されたファイル数
        """
        assets_dest.mkdir(parents=True, exist_ok=True)

        files_added = 0
        for src_file in assets_src.rglob("*"):
            if src_file.is_file():
                rel_path = src_file.relative_to(assets_src)
                dest_file = assets_dest / rel_path
                dest_file.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(src_file, dest_file)
                files_added += 1

        return files_added

    def _update_icon(self, decode_dir: Path, icon_path: Path) -> None:
        """アプリアイコンを更新する

        各解像度のmipmapディレクトリにアイコンファイルをコピーする。
        リサイズは行わず、同じファイルを各ディレクトリにコピーする。

        Args:
            decode_dir: apktoolでデコードしたディレクトリ
            icon_path: 新しいアイコン画像のパス
        """
        res_dir = decode_dir / "res"
        densities = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"]

        for density in densities:
            mipmap_dir = res_dir / f"mipmap-{density}"
            mipmap_dir.mkdir(parents=True, exist_ok=True)
            dest_path = mipmap_dir / "ic_launcher.png"
            shutil.copy2(icon_path, dest_path)

    def find_base_apk_in_template(self, template_dir: Path) -> Path | None:
        """テンプレートディレクトリからベースAPKを検索する

        Args:
            template_dir: テンプレートディレクトリ

        Returns:
            ベースAPKのパス。見つからない場合はNone。
        """
        # krkrsdl2_universal.apk を検索
        for apk_path in template_dir.rglob("*.apk"):
            if "krkrsdl2" in apk_path.name.lower():
                return apk_path

        # 任意のAPKファイルを検索
        for apk_path in template_dir.rglob("*.apk"):
            return apk_path

        return None

    def validate_apk(self, apk_path: Path) -> bool:
        """APKファイルが有効かどうかを検証する

        Args:
            apk_path: 検証するAPKのパス

        Returns:
            APKが有効な場合はTrue
        """
        if not apk_path.exists():
            return False

        if not zipfile.is_zipfile(apk_path):
            return False

        try:
            with zipfile.ZipFile(apk_path, "r") as zf:
                # 必須ファイルの存在確認
                namelist = zf.namelist()
                return "AndroidManifest.xml" in namelist and "classes.dex" in namelist
        except zipfile.BadZipFile:
            return False

    def is_compression_needed(self, file_path: str) -> bool:
        """ファイルを圧縮すべきかどうかを判定する

        Args:
            file_path: ファイルパス

        Returns:
            圧縮が必要な場合はTrue
        """
        # 既に圧縮されているフォーマットや特定のファイルは圧縮しない
        no_compress_extensions = {
            ".jpg",
            ".jpeg",
            ".png",
            ".gif",
            ".wav",
            ".mp2",
            ".mp3",
            ".ogg",
            ".aac",
            ".mpg",
            ".mpeg",
            ".mid",
            ".midi",
            ".smf",
            ".jet",
            ".rtttl",
            ".imy",
            ".xmf",
            ".mp4",
            ".m4a",
            ".m4v",
            ".3gp",
            ".3gpp",
            ".3g2",
            ".3gpp2",
            ".amr",
            ".awb",
            ".wma",
            ".wmv",
            ".webm",
            ".mkv",
        }

        ext = Path(file_path).suffix.lower()
        return ext not in no_compress_extensions
