"""ゲーム構成検出モジュール

吉里吉里2などのゲームエンジンの構成を検出し、
ゲーム内のスクリプト、画像、音声などのリソースを特定する。
"""

import re
from dataclasses import dataclass
from enum import Enum
from pathlib import Path


class EngineType(Enum):
    """検出可能なエンジン種別

    サポートされるゲームエンジンの種類を表す列挙型。
    """

    KIRIKIRI2 = "kirikiri2"
    KIRIKIRI2_KAG3 = "kirikiri2_kag3"
    UNKNOWN = "unknown"


@dataclass(frozen=True)
class GameStructure:
    """ゲーム構成情報

    ゲームディレクトリの解析結果を保持する不変データクラス。

    Attributes:
        engine: 検出されたゲームエンジンの種別
        title: ゲームタイトル（Config.tjsから取得、取得できない場合はNone）
        scripts: 検出されたスクリプトファイルのパス一覧
        script_encoding: スクリプトファイルのエンコーディング（検出できない場合はNone）
        images: 検出された画像ファイルのパス一覧
        audio: 検出された音声ファイルのパス一覧
        video: 検出された動画ファイルのパス一覧
        plugins: 検出されたプラグインファイルのパス一覧
    """

    engine: EngineType
    title: str | None
    scripts: list[str]
    script_encoding: str | None
    images: list[str]
    audio: list[str]
    video: list[str]
    plugins: list[str]


class GameDetector:
    """ゲーム構成を検出するクラス

    指定されたゲームディレクトリを解析し、
    使用されているエンジンの種類やリソースファイルを検出する。
    """

    # ファイル拡張子による分類
    _SCRIPT_EXTENSIONS = {".ks", ".tjs"}
    _IMAGE_EXTENSIONS = {".tlg", ".bmp", ".jpg", ".jpeg", ".png"}
    _AUDIO_EXTENSIONS = {".ogg", ".wav", ".mp3"}
    _VIDEO_EXTENSIONS = {".mpg", ".mpeg", ".wmv", ".avi"}
    _PLUGIN_EXTENSIONS = {".dll"}

    def __init__(self, game_dir: Path) -> None:
        """ゲームディレクトリを指定して初期化

        Args:
            game_dir: 解析対象のゲームディレクトリのパス

        Raises:
            FileNotFoundError: 指定されたディレクトリが存在しない場合
        """
        if not game_dir.exists():
            raise FileNotFoundError(f"ディレクトリが存在しません: {game_dir}")
        if not game_dir.is_dir():
            raise FileNotFoundError(f"ディレクトリではありません: {game_dir}")

        self._game_dir = game_dir
        self._structure: GameStructure | None = None

    def detect(self) -> GameStructure:
        """ゲーム構成を検出して返す

        ゲームディレクトリを走査し、エンジン種別の判定と
        各種リソースファイルの検出を行う。

        Returns:
            検出されたゲーム構成情報

        Raises:
            ValueError: ゲームディレクトリが空の場合
        """
        # ファイル一覧を取得
        all_files = self._collect_files()

        if not all_files:
            raise ValueError(f"ディレクトリが空です: {self._game_dir}")

        # ファイルを分類
        scripts = self._filter_by_extensions(all_files, self._SCRIPT_EXTENSIONS)
        images = self._filter_by_extensions(all_files, self._IMAGE_EXTENSIONS)
        audio = self._filter_by_extensions(all_files, self._AUDIO_EXTENSIONS)
        video = self._filter_by_extensions(all_files, self._VIDEO_EXTENSIONS)
        plugins = self._filter_by_extensions(all_files, self._PLUGIN_EXTENSIONS)

        # エンジンタイプを検出
        engine = self._detect_engine(all_files, scripts)

        # ゲームタイトルを取得
        title = self._detect_title()

        # スクリプトの文字コードを推定
        script_encoding = self._detect_script_encoding(scripts)

        self._structure = GameStructure(
            engine=engine,
            title=title,
            scripts=scripts,
            script_encoding=script_encoding,
            images=images,
            audio=audio,
            video=video,
            plugins=plugins,
        )

        return self._structure

    def get_summary(self) -> str:
        """検出結果のサマリー文字列を返す

        CLI表示用に検出結果を人間が読みやすい形式で整形する。

        Returns:
            検出結果のサマリー文字列
        """
        if self._structure is None:
            self.detect()

        assert self._structure is not None

        engine_name = self._get_engine_display_name(self._structure.engine)
        encoding_info = (
            f" (detected: {self._structure.script_encoding})"
            if self._structure.script_encoding
            else ""
        )

        video_count = len(self._structure.video)
        video_suffix = "s" if video_count != 1 else ""

        lines = [
            f"Engine: {engine_name}",
            f"Scripts: {len(self._structure.scripts)} files{encoding_info}",
            f"Images: {len(self._structure.images)} files",
            f"Audio: {len(self._structure.audio)} files",
            f"Video: {video_count} file{video_suffix}",
            f"Plugins: {len(self._structure.plugins)} files",
        ]

        return "\n".join(lines)

    def _collect_files(self) -> list[str]:
        """ゲームディレクトリ内の全ファイルを収集

        Returns:
            相対パス文字列のリスト
        """
        files: list[str] = []
        for file_path in self._game_dir.rglob("*"):
            if file_path.is_file() and file_path.name != ".gitkeep":
                relative_path = file_path.relative_to(self._game_dir)
                files.append(str(relative_path))
        return sorted(files)

    def _filter_by_extensions(self, files: list[str], extensions: set[str]) -> list[str]:
        """指定された拡張子でファイルをフィルタリング

        Args:
            files: ファイルパスのリスト
            extensions: フィルタリングする拡張子のセット

        Returns:
            フィルタリングされたファイルパスのリスト
        """
        return [f for f in files if Path(f).suffix.lower() in extensions]

    def _detect_engine(self, all_files: list[str], scripts: list[str]) -> EngineType:
        """ゲームエンジンの種別を検出

        Args:
            all_files: 全ファイルパスのリスト
            scripts: スクリプトファイルのリスト

        Returns:
            検出されたエンジン種別
        """
        file_names = {Path(f).name.lower() for f in all_files}

        # 吉里吉里2の特徴的なファイルをチェック
        is_kirikiri2 = "data.xp3" in file_names or "game.exe" in file_names

        if is_kirikiri2:
            # .ksファイルの存在でKAG3を判定
            has_ks_files = any(f.endswith(".ks") for f in scripts)
            if has_ks_files:
                return EngineType.KIRIKIRI2_KAG3
            return EngineType.KIRIKIRI2

        return EngineType.UNKNOWN

    def _detect_script_encoding(self, scripts: list[str]) -> str | None:
        """スクリプトファイルの文字コードを推定

        Args:
            scripts: スクリプトファイルのリスト

        Returns:
            推定された文字コード、またはNone
        """
        if not scripts:
            return None

        import chardet

        # 最初のスクリプトファイルをサンプルとして使用
        for script_path in scripts:
            full_path = self._game_dir / script_path
            if full_path.exists():
                with open(full_path, "rb") as f:
                    raw_data = f.read()
                    if raw_data:
                        result = chardet.detect(raw_data)
                        if result["encoding"]:
                            return result["encoding"].lower()

        return None

    def _detect_title(self) -> str | None:
        """Config.tjsからゲームタイトルを取得する

        Returns:
            ゲームタイトル、取得できない場合はNone
        """
        config_paths = [
            self._game_dir / "system" / "Config.tjs",
            self._game_dir / "Config.tjs",
        ]

        for config_path in config_paths:
            if config_path.exists():
                try:
                    for encoding in ["cp932", "utf-8"]:
                        try:
                            content = config_path.read_text(encoding=encoding)
                            match = re.search(r';System\.title\s*=\s*"([^"]+)"', content)
                            if match:
                                return match.group(1)
                        except UnicodeDecodeError:
                            continue
                except OSError:
                    continue

        return None

    def _get_engine_display_name(self, engine: EngineType) -> str:
        """エンジン種別の表示名を取得

        Args:
            engine: エンジン種別

        Returns:
            表示用のエンジン名
        """
        display_names = {
            EngineType.KIRIKIRI2: "Kirikiri 2",
            EngineType.KIRIKIRI2_KAG3: "Kirikiri 2 (KAG3)",
            EngineType.UNKNOWN: "Unknown",
        }
        return display_names.get(engine, "Unknown")
