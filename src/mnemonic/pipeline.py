"""ビルドパイプラインの統合インターフェース定義

このモジュールは、MnemonicのビルドパイプラインをオーケストレーションするためのINTERFACEを定義する。
Parser -> Converter -> Builder -> Signer の各コンポーネントを連携させ、
Windows EXE/XP3からAndroid APKを生成するパイプラインを構成する。
"""

from __future__ import annotations

import re
import shutil
import tempfile
import zipfile
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import TYPE_CHECKING, Any, Protocol

import mnemonic.cache as cache_module
from mnemonic.builder.gradle import GradleBuilder
from mnemonic.builder.template import (
    TemplateCache,
    TemplateDownloader,
)
from mnemonic.builder.template_preparer import TemplatePreparer
from mnemonic.converter.encoding import EncodingConverter
from mnemonic.converter.image import ImageConverter
from mnemonic.converter.manager import ConversionManager
from mnemonic.converter.video import VideoConverter
from mnemonic.parser.detector import GameDetector, GameStructure
from mnemonic.parser.exe import EmbeddedXP3Extractor
from mnemonic.parser.icon import ExeIconExtractor
from mnemonic.parser.xp3 import XP3Archive, XP3EncryptionChecker
from mnemonic.signer.apk import DefaultApkSignerRunner, DefaultZipalignRunner, KeystoreConfig

if TYPE_CHECKING:
    from mnemonic.cache import CacheInfo

# Java予約語リスト（パッケージ名生成時のフォールバック用）
JAVA_RESERVED_WORDS = {
    "abstract",
    "assert",
    "boolean",
    "break",
    "byte",
    "case",
    "catch",
    "char",
    "class",
    "const",
    "continue",
    "default",
    "do",
    "double",
    "else",
    "enum",
    "extends",
    "false",
    "final",
    "finally",
    "float",
    "for",
    "goto",
    "if",
    "implements",
    "import",
    "instanceof",
    "int",
    "interface",
    "long",
    "native",
    "new",
    "null",
    "package",
    "private",
    "protected",
    "public",
    "return",
    "short",
    "static",
    "strictfp",
    "super",
    "switch",
    "synchronized",
    "this",
    "throw",
    "throws",
    "transient",
    "true",
    "try",
    "void",
    "volatile",
    "while",
}


class DefaultCacheManager:
    """デフォルトのキャッシュマネージャー実装

    CacheManager Protocolの具象実装を提供する。
    """

    def get_cache_dir(self) -> Path:
        """OSごとのキャッシュディレクトリを取得する"""
        return cache_module.get_cache_dir()

    def get_template_cache_path(self, version: str) -> Path:
        """テンプレートキャッシュパスを取得する"""
        return cache_module.get_template_cache_path(version)

    def is_cache_valid(self, path: Path, max_age_days: int) -> bool:
        """キャッシュの有効性をチェックする"""
        return cache_module.is_cache_valid(path, max_age_days)

    def clear_cache(self, template_only: bool = False) -> None:
        """キャッシュをクリアする"""
        cache_module.clear_cache(template_only)

    def get_cache_info(self) -> CacheInfo:
        """キャッシュ情報を取得する"""
        return cache_module.get_cache_info()


class PipelinePhase(Enum):
    """パイプラインフェーズ

    ビルドパイプラインの各段階を表す列挙型。
    パイプラインは以下の順序で実行される:
    1. ANALYZE: ゲーム構造解析
    2. EXTRACT: アセット抽出
    3. CONVERT: アセット変換
    4. BUILD: APKビルド
    5. SIGN: APK署名
    """

    ANALYZE = "analyze"
    EXTRACT = "extract"
    CONVERT = "convert"
    BUILD = "build"
    SIGN = "sign"


@dataclass(frozen=True)
class PipelineProgress:
    """パイプライン進捗情報

    パイプラインの実行進捗を表すデータクラス。
    進捗コールバックを通じて、各フェーズの進捗状況を通知するために使用される。

    Attributes:
        phase: 現在実行中のパイプラインフェーズ
        current: 現在の進捗（処理済みアイテム数）
        total: 総数（処理対象アイテム数）
        message: 追加の進捗メッセージ（オプション）
    """

    phase: PipelinePhase
    current: int
    total: int
    message: str = ""


class ProgressCallback(Protocol):
    """進捗コールバックのプロトコル

    パイプライン実行中の進捗通知を受け取るためのコールバックインターフェース。
    UIやログ出力などで進捗を表示する際に使用する。
    """

    def __call__(self, progress: PipelineProgress) -> None:
        """進捗情報を受け取るコールバック

        Args:
            progress: 現在の進捗情報
        """
        ...


@dataclass(frozen=True)
class PipelineConfig:
    """パイプライン設定

    ビルドパイプラインの実行に必要な設定を保持するデータクラス。
    CLIオプションやコンフィグファイルからの設定値をまとめて管理する。

    Attributes:
        input_path: 入力ファイルパス（EXEまたはXP3ファイル）
        output_path: 出力APKファイルパス
        package_name: Androidパッケージ名（空の場合は自動生成）
        app_name: アプリケーション表示名（空の場合は入力ファイル名から生成）
        keystore_path: APK署名用キーストアファイルパス（Noneの場合はデバッグキーを使用）
        skip_video: 動画変換をスキップするか
        quality: 変換品質（"low", "medium", "high"）
        clean_cache: キャッシュをクリアしてから実行するか
        verbose_level: 詳細出力レベル（0=通常, 1=詳細, 2=デバッグ）
        log_file: ログ出力ファイルパス（Noneの場合はファイル出力なし）
        ffmpeg_timeout: FFmpeg処理のタイムアウト秒数
        gradle_timeout: Gradleビルドのタイムアウト秒数
        template_version: 使用するAndroidテンプレートのバージョン（Noneの場合は最新）
        template_refresh_days: テンプレートキャッシュの有効期間（日数）
        template_offline: オフラインモードでテンプレートを使用するか
    """

    input_path: Path
    output_path: Path
    package_name: str = ""
    app_name: str = ""
    keystore_path: Path | None = None
    skip_video: bool = False
    quality: str = "high"
    clean_cache: bool = False
    verbose_level: int = 0
    log_file: Path | None = None
    ffmpeg_timeout: int = 300
    gradle_timeout: int = 1800
    template_version: str | None = None
    template_refresh_days: int = 7
    template_offline: bool = False


@dataclass
class PipelineResult:
    """パイプライン実行結果

    パイプライン実行の結果を保持するデータクラス。
    成功/失敗の状態、出力ファイルパス、統計情報などを含む。

    Attributes:
        success: パイプライン実行が成功したか
        output_path: 生成されたAPKファイルパス（失敗時はNone）
        error_message: エラーメッセージ（成功時は空文字列）
        phases_completed: 完了したフェーズのリスト
        statistics: 実行統計情報（処理時間、ファイル数など）
    """

    success: bool
    output_path: Path | None
    error_message: str = ""
    phases_completed: list[PipelinePhase] = field(default_factory=list)
    statistics: dict[str, Any] = field(default_factory=dict)


class BuildPipeline:
    """ビルドパイプラインオーケストレーター

    Parser -> Converter -> Builder -> Signer の各コンポーネントを連携させ、
    Windows EXE/XP3からAndroid APKを生成するパイプラインを管理する。

    使用例:
        >>> config = PipelineConfig(
        ...     input_path=Path("game.exe"),
        ...     output_path=Path("game.apk"),
        ... )
        >>> pipeline = BuildPipeline(config)
        >>> errors = pipeline.validate()
        >>> if not errors:
        ...     result = pipeline.run()
    """

    def __init__(self, config: PipelineConfig) -> None:
        """パイプラインを初期化する

        Args:
            config: パイプライン設定
        """
        self._config = config
        self._temp_dirs: list[Path] = []
        self._extract_dir: Path | None = None
        self._convert_dir: Path | None = None
        self._project_dir: Path | None = None
        self._unsigned_apk: Path | None = None
        self._game_structure: GameStructure | None = None

    @property
    def config(self) -> PipelineConfig:
        """パイプライン設定を取得する

        Returns:
            現在のパイプライン設定
        """
        return self._config

    def run(self, progress_callback: ProgressCallback | None = None) -> PipelineResult:
        """パイプラインを実行する

        各フェーズ（解析、抽出、変換、ビルド、署名）を順次実行し、
        最終的にAPKファイルを生成する。

        Args:
            progress_callback: 進捗通知用コールバック（オプション）

        Returns:
            パイプライン実行結果

        Raises:
            ValueError: 設定が無効な場合
        """
        import time

        start_time = time.time()

        # 検証
        errors = self.validate()
        if errors:
            return PipelineResult(
                success=False,
                output_path=None,
                error_message=errors[0],
            )

        phases_completed: list[PipelinePhase] = []
        statistics: dict[str, Any] = {}

        try:
            # 各フェーズを実行
            for phase in PipelinePhase:
                phase_start = time.time()

                # 進捗コールバックを呼び出し
                if progress_callback is not None:
                    progress = PipelineProgress(
                        phase=phase,
                        current=0,
                        total=1,
                        message=f"{phase.value}フェーズを開始...",
                    )
                    progress_callback(progress)

                # 各フェーズの処理を実行
                self._execute_phase(phase)

                # フェーズ完了
                phases_completed.append(phase)
                phase_time = time.time() - phase_start
                statistics[f"{phase.value}_time_seconds"] = round(phase_time, 2)

                # 完了時の進捗コールバック
                if progress_callback is not None:
                    progress = PipelineProgress(
                        phase=phase,
                        current=1,
                        total=1,
                        message=f"{phase.value}フェーズが完了",
                    )
                    progress_callback(progress)

            total_time = time.time() - start_time
            statistics["total_time_seconds"] = round(total_time, 2)

            return PipelineResult(
                success=True,
                output_path=self._config.output_path,
                phases_completed=phases_completed,
                statistics=statistics,
            )
        except Exception as e:
            return PipelineResult(
                success=False,
                output_path=None,
                error_message=str(e),
                phases_completed=phases_completed,
            )
        finally:
            self._cleanup_temp_dirs()

    def _execute_phase(self, phase: PipelinePhase) -> None:
        """個別フェーズを実行する

        Args:
            phase: 実行するフェーズ

        Raises:
            ValueError: フェーズ実行中にエラーが発生した場合
        """
        match phase:
            case PipelinePhase.ANALYZE:
                self._execute_analyze()
            case PipelinePhase.EXTRACT:
                self._execute_extract()
            case PipelinePhase.CONVERT:
                self._execute_convert()
            case PipelinePhase.BUILD:
                self._execute_build()
            case PipelinePhase.SIGN:
                self._execute_sign()

    def _execute_analyze(self) -> None:
        """ANALYZEフェーズ: ゲーム構造解析

        入力ファイルの形式を確認し、必要に応じて暗号化チェックを行う。

        Raises:
            ValueError: サポートされていない入力形式、または暗号化されている場合
        """
        input_path = self._config.input_path
        suffix = input_path.suffix.lower()

        if suffix == ".exe":
            # EXEの場合: 埋め込みXP3を検出
            extractor = EmbeddedXP3Extractor(input_path)
            xp3_list = extractor.find_embedded_xp3()
            if not xp3_list:
                raise ValueError(f"EXEファイル内にXP3アーカイブが見つかりません: {input_path}")
        elif suffix == ".xp3":
            # XP3の場合: 暗号化チェック
            checker = XP3EncryptionChecker(input_path)
            checker.raise_if_encrypted()

    def _execute_extract(self) -> None:
        """EXTRACTフェーズ: アセット抽出

        XP3アーカイブを展開し、ゲーム構造を解析する。
        EXEファイルの場合は、埋め込みXP3を抽出してから展開する。

        Raises:
            ValueError: 抽出に失敗した場合
        """
        input_path = self._config.input_path
        suffix = input_path.suffix.lower()

        # 一時ディレクトリ作成
        self._extract_dir = Path(tempfile.mkdtemp(prefix="mnemonic_extract_"))
        self._temp_dirs.append(self._extract_dir)

        if suffix == ".exe":
            # EXEから埋め込みXP3を抽出
            extractor = EmbeddedXP3Extractor(input_path)
            xp3_files = extractor.extract_all(self._extract_dir)

            # 各XP3を展開
            for xp3_file in xp3_files:
                archive = XP3Archive(xp3_file)
                archive.extract_all(self._extract_dir)
        else:
            # XP3を直接展開
            archive = XP3Archive(input_path)
            archive.extract_all(self._extract_dir)

        # ゲーム構造解析
        detector = GameDetector(self._extract_dir)
        self._game_structure = detector.detect()

    def _execute_convert(self) -> None:
        """CONVERTフェーズ: アセット変換

        抽出されたアセットをAndroid互換形式に変換する。
        まず全ファイルをコピーし（ゲームコアファイルを含む）、
        その後変換対象ファイルを変換（上書き）する。

        Raises:
            ValueError: 抽出フェーズが完了していない場合
        """
        if self._extract_dir is None:
            raise ValueError("抽出フェーズが完了していません")

        self._convert_dir = Path(tempfile.mkdtemp(prefix="mnemonic_convert_"))
        self._temp_dirs.append(self._convert_dir)

        # まず全ファイルをコピー（data.xp3等のゲームコアファイルを含む）
        shutil.copytree(self._extract_dir, self._convert_dir, dirs_exist_ok=True)

        # Androidのファイルシステムは大文字小文字を区別するため、
        # 重要なファイル名を正規化（小文字化）
        self._normalize_critical_filenames(self._convert_dir)

        # コンバーターを設定
        converters: list[Any] = [
            EncodingConverter(),
            ImageConverter(),
        ]

        if not self._config.skip_video:
            converters.append(VideoConverter(timeout=self._config.ffmpeg_timeout))

        # 変換対象ファイルを変換（上書き）
        manager = ConversionManager(converters=converters)
        manager.convert_directory(self._extract_dir, self._convert_dir)

    def _normalize_critical_filenames(self, directory: Path) -> None:
        """Kirikiri の重要なファイル名を正規化（小文字化）する

        Windows はファイル名の大文字小文字を区別しないが、Android は区別する。
        Kirikiri エンジンが期待するファイル名（小文字）に合わせてリネームする。

        Args:
            directory: 処理対象のディレクトリ
        """
        # system ディレクトリ内のファイルを小文字化
        system_dir = directory / "system"
        if system_dir.exists():
            for file_path in system_dir.iterdir():
                if file_path.is_file():
                    lower_name = file_path.name.lower()
                    if file_path.name != lower_name:
                        new_path = file_path.parent / lower_name
                        file_path.rename(new_path)

        # startup.tjs も小文字化（ルートディレクトリ）
        for startup_variant in ["Startup.tjs", "STARTUP.TJS", "StartUp.tjs"]:
            startup_file = directory / startup_variant
            if startup_file.exists():
                startup_file.rename(directory / "startup.tjs")
                break

    def _execute_build(self) -> None:
        """BUILDフェーズ: APKビルド

        Gradleビルドを使用してAPKを生成する。
        テンプレートを展開し、ゲームファイルをassetsに配置してビルドする。

        Raises:
            ValueError: 変換フェーズが完了していない、またはビルド失敗の場合
        """
        import asyncio

        if self._convert_dir is None:
            raise ValueError("変換フェーズが完了していません")

        self._project_dir = Path(tempfile.mkdtemp(prefix="mnemonic_project_"))
        self._temp_dirs.append(self._project_dir)

        # テンプレート取得
        cache_manager = DefaultCacheManager()
        template_cache = TemplateCache(
            cache_manager=cache_manager,
            refresh_days=self._config.template_refresh_days,
        )

        template_path = template_cache.get_cached_template(self._config.template_version)

        if template_path is None and not self._config.template_offline:
            downloader = TemplateDownloader()
            loop = asyncio.new_event_loop()
            try:
                template_path = loop.run_until_complete(
                    downloader.download(self._config.template_version)
                )
            finally:
                loop.close()

            if template_path:
                version = self._config.template_version or "latest"
                template_cache.save_template(template_path, version)
                template_path = template_cache.get_cached_template(self._config.template_version)

        if template_path is None:
            raise ValueError("テンプレートが利用できません。オンラインモードで再実行してください。")

        # テンプレートをプロジェクトディレクトリに展開
        self._extract_template(template_path, self._project_dir)

        # タイトルからパッケージ名を生成（フォールバック: ファイル名）
        if self._game_structure and self._game_structure.title:
            base_name = self._game_structure.title
        else:
            base_name = self._config.input_path.stem

        package_name = self._config.package_name or f"com.krkr.{self._sanitize_name(base_name)}"
        app_name = self._config.app_name or (
            self._game_structure.title
            if self._game_structure and self._game_structure.title
            else base_name
        )

        # ゲームアイコンを検索
        icon_path = self._find_game_icon()

        # テンプレートを準備（jniLibs抽出、Java/Gradle/Manifest更新、assetsコピー、アイコン設定）
        preparer = TemplatePreparer(self._project_dir)
        preparer.prepare(
            package_name=package_name,
            app_name=app_name,
            assets_dir=self._convert_dir,
            icon_path=icon_path,
        )

        # Gradleビルド実行
        builder = GradleBuilder(
            project_path=self._project_dir,
            timeout=self._config.gradle_timeout,
        )
        result = builder.build(build_type="release")

        if not result.success or result.apk_path is None:
            raise ValueError(f"Gradleビルドに失敗しました: {result.output_log}")

        self._unsigned_apk = result.apk_path

    def _extract_template(self, template_path: Path, dest_dir: Path) -> None:
        """テンプレートをプロジェクトディレクトリに展開する

        Args:
            template_path: テンプレートZIPファイルのパス
            dest_dir: 展開先ディレクトリ

        Raises:
            ValueError: テンプレートの展開に失敗した場合
        """
        if not template_path.exists():
            raise ValueError(f"テンプレートが見つかりません: {template_path}")

        try:
            with zipfile.ZipFile(template_path, "r") as zf:
                zf.extractall(dest_dir)
        except zipfile.BadZipFile as e:
            raise ValueError(f"無効なテンプレートファイルです: {template_path}") from e

    def _find_game_icon(self) -> Path | None:
        """ゲームアイコンを検索する

        以下の優先順位でアイコンを検索します:
        1. 抽出ディレクトリからアイコンファイルを検索
        2. EXEファイルから埋め込みアイコンを抽出

        Returns:
            アイコンファイルのパス。見つからない場合はNone。
        """
        if self._extract_dir is None:
            return None

        # 1. 抽出ディレクトリから既存アイコンファイルを検索
        icon_names = ["icon.png", "icon.ico", "icon.bmp"]
        for name in icon_names:
            icon_path = self._extract_dir / name
            if icon_path.exists():
                return icon_path

        # 任意のicoファイルを検索
        for ico_file in self._extract_dir.glob("*.ico"):
            return ico_file

        # 2. EXEファイルからアイコンを抽出（入力がEXEの場合のみ）
        if self._config.input_path.suffix.lower() == ".exe":
            icon_extractor = ExeIconExtractor()
            extracted_icon = icon_extractor.extract(self._config.input_path, self._extract_dir)
            if extracted_icon is not None:
                return extracted_icon

        return None

    def _execute_sign(self) -> None:
        """SIGNフェーズ: APK署名

        ビルドされたAPKにzipalignを適用し、オプションで署名を行う。

        Raises:
            ValueError: ビルドフェーズが完了していない場合
        """
        if self._unsigned_apk is None:
            raise ValueError("ビルドフェーズが完了していません")

        output_path = self._config.output_path
        output_path.parent.mkdir(parents=True, exist_ok=True)

        # Zipalign
        zipaligner = DefaultZipalignRunner()
        aligned_apk = output_path.with_suffix(".aligned.apk")
        zipaligner.align(self._unsigned_apk, aligned_apk)

        if self._config.keystore_path:
            # 署名付きAPK
            from mnemonic.signer.apk import DefaultPasswordProvider

            password_provider = DefaultPasswordProvider()
            password = password_provider.get_password_from_env() or password_provider.get_password()

            keystore_config = KeystoreConfig(
                keystore_path=self._config.keystore_path,
                key_alias="key",
                keystore_password=password,
            )

            signer = DefaultApkSignerRunner()
            shutil.copy2(aligned_apk, output_path)
            signer.sign(output_path, keystore_config)
            aligned_apk.unlink()
        else:
            # デバッグ用キーストアで署名
            debug_keystore = self._create_debug_keystore()
            debug_config = KeystoreConfig(
                keystore_path=debug_keystore,
                key_alias="debug",
                keystore_password="android",
            )
            signer = DefaultApkSignerRunner()
            shutil.copy2(aligned_apk, output_path)
            signer.sign(output_path, debug_config)
            aligned_apk.unlink()

    def _create_debug_keystore(self) -> Path:
        """デバッグ用キーストアを作成する

        keytool コマンドを使用してデバッグ用の自己署名キーストアを生成します。

        Returns:
            生成されたキーストアのパス

        Raises:
            ValueError: keytool コマンドが見つからない場合
        """
        import subprocess

        debug_keystore = Path(tempfile.mkdtemp(prefix="mnemonic_keystore_")) / "debug.keystore"
        self._temp_dirs.append(debug_keystore.parent)

        keytool_cmd = [
            "keytool",
            "-genkeypair",
            "-v",
            "-keystore",
            str(debug_keystore),
            "-storepass",
            "android",
            "-alias",
            "debug",
            "-keypass",
            "android",
            "-keyalg",
            "RSA",
            "-keysize",
            "2048",
            "-validity",
            "10000",
            "-dname",
            "CN=Debug,OU=Debug,O=Debug,L=Debug,ST=Debug,C=US",
        ]

        try:
            result = subprocess.run(
                keytool_cmd,
                capture_output=True,
                text=True,
                timeout=30,
            )
            if result.returncode != 0:
                raise ValueError(f"keytool failed: {result.stderr}")
        except FileNotFoundError as e:
            raise ValueError("keytool command not found. Please install JDK.") from e
        except subprocess.TimeoutExpired as e:
            raise ValueError("keytool command timed out.") from e

        return debug_keystore

    def _sanitize_name(self, name: str) -> str:
        """パッケージ名に使用できる形式に変換

        空白は単語区切りとして保持するためアンダースコアに変換し、
        その他の特殊文字（ハイフン、記号等）はパッケージ名に使用できないため削除する。

        Args:
            name: 変換対象の名前

        Returns:
            パッケージ名として使用可能な文字列
        """
        sanitized = name.replace(" ", "_")
        sanitized = re.sub(r"[^a-zA-Z0-9_]", "", sanitized)
        # 先頭が数字の場合はプレフィックス追加
        if sanitized and sanitized[0].isdigit():
            sanitized = "_" + sanitized
        sanitized = sanitized.lower()

        # Java予約語の場合はプレフィックス追加（フォールバック用）
        if sanitized in JAVA_RESERVED_WORDS:
            sanitized = f"game_{sanitized}"

        return sanitized

    def _cleanup_temp_dirs(self) -> None:
        """一時ディレクトリをクリーンアップする"""
        for temp_dir in self._temp_dirs:
            if temp_dir.exists():
                shutil.rmtree(temp_dir, ignore_errors=True)
        self._temp_dirs.clear()

    def validate(self) -> list[str]:
        """設定を検証し、エラーメッセージのリストを返す

        入力ファイルの存在確認、出力パスの妥当性、
        オプション値の整合性などを検証する。

        Returns:
            エラーメッセージのリスト（エラーがない場合は空リスト）
        """
        errors: list[str] = []

        # 入力ファイル存在チェック
        if not self._config.input_path.exists():
            errors.append(f"入力ファイルが見つかりません: {self._config.input_path}")
            return errors

        # 入力ファイル形式チェック
        suffix = self._config.input_path.suffix.lower()
        if suffix not in (".exe", ".xp3"):
            errors.append(f"サポートされていないファイル形式です: {suffix}")

        # キーストアチェック（指定時のみ）
        if self._config.keystore_path and not self._config.keystore_path.exists():
            errors.append(f"キーストアファイルが見つかりません: {self._config.keystore_path}")

        return errors
