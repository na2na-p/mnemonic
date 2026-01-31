"""進捗表示およびログ出力のインターフェース定義

このモジュールは、Mnemonicのビルド進捗表示とログ出力のためのインターフェースを定義する。
VerboseLevel (詳細ログレベル)に応じた出力制御を行い、
CLIでのビルド進捗をユーザーにわかりやすく表示するために使用される。
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from datetime import datetime
from enum import IntEnum
from pathlib import Path
from typing import TYPE_CHECKING, Any, Protocol, TextIO

if TYPE_CHECKING:
    from mnemonic.pipeline import PipelinePhase


class VerboseLevel(IntEnum):
    """詳細ログレベル

    ログ出力の詳細度を制御するための列挙型。
    QUIET: エラーのみ出力
    NORMAL: 進捗バーとサマリ出力
    VERBOSE: 変換ファイル一覧も出力（-vオプション）
    DEBUG: 外部コマンド実行ログも出力（-vvオプション）
    """

    QUIET = -1
    NORMAL = 0
    VERBOSE = 1
    DEBUG = 2


class ProgressDisplay(Protocol):
    """進捗表示のプロトコル

    ビルドパイプラインの各フェーズの進捗を表示するためのインターフェース。
    CLI UIやログ出力などで進捗を可視化する際に使用する。
    """

    def start(self, phase: PipelinePhase, total: int) -> None:
        """フェーズ開始を表示する

        Args:
            phase: 開始するパイプラインフェーズ
            total: 処理対象の総数
        """
        ...

    def update(self, current: int, message: str = "") -> None:
        """進捗を更新する

        Args:
            current: 現在の進捗（処理済みアイテム数）
            message: 追加の進捗メッセージ（オプション）
        """
        ...

    def finish(self, success: bool, message: str = "") -> None:
        """フェーズ終了を表示する

        Args:
            success: フェーズが成功したか
            message: 終了メッセージ（オプション）
        """
        ...


@dataclass
class LogConfig:
    """ログ設定

    ログ出力の動作を制御するための設定データクラス。
    詳細レベル、ファイル出力、色やemojiの使用有無を設定できる。

    Attributes:
        verbose_level: ログの詳細度レベル
        log_file: ログ出力先ファイルパス（Noneの場合はファイル出力なし）
        use_color: カラー出力を使用するか
        use_emoji: emoji表示を使用するか
    """

    verbose_level: VerboseLevel = VerboseLevel.NORMAL
    log_file: Path | None = None
    use_color: bool = True
    use_emoji: bool = True


class BuildLogger:
    """ビルドログ出力クラス

    ビルドパイプラインのログ出力を管理するクラス。
    VerboseLevelに応じてメッセージのフィルタリングを行い、
    進捗表示インスタンスの作成も担当する。

    使用例:
        >>> config = LogConfig(verbose_level=VerboseLevel.VERBOSE)
        >>> logger = BuildLogger(config)
        >>> logger.info("ビルドを開始します")
        >>> logger.verbose("game.exe を処理中")
    """

    _ANSI_ESCAPE_PATTERN = re.compile(r"\x1b\[[0-9;]*m")

    def __init__(self, config: LogConfig) -> None:
        """ロガーを初期化する

        Args:
            config: ログ設定
        """
        self._config = config
        self._log_file: TextIO | None = None
        if config.log_file:
            # クラス自体がコンテキストマネージャとして動作し、__exit__でファイルを閉じる
            self._log_file = open(config.log_file, "w", encoding="utf-8")  # noqa: SIM115

    def __enter__(self) -> BuildLogger:
        """コンテキストマネージャのエントリポイント"""
        return self

    def __exit__(self, *args: object) -> None:
        """コンテキストマネージャの終了処理"""
        if self._log_file:
            self._log_file.close()
            self._log_file = None

    @property
    def config(self) -> LogConfig:
        """ログ設定を取得する

        Returns:
            現在のログ設定
        """
        return self._config

    def _print(self, message: str, file: TextIO | None = None) -> None:
        """メッセージを出力する

        Args:
            message: 出力するメッセージ
            file: 出力先（Noneの場合は標準出力）
        """
        if file is None:
            file = sys.stdout
        print(message, file=file)

    def _log_to_file(self, level: str, message: str) -> None:
        """ファイルにログ出力する

        Args:
            level: ログレベル文字列
            message: 出力するメッセージ
        """
        if self._log_file:
            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            clean_message = self._strip_ansi(message)
            self._log_file.write(f"[{timestamp}] {level}: {clean_message}\n")
            self._log_file.flush()

    def _strip_ansi(self, text: str) -> str:
        """ANSIエスケープシーケンスを除去する

        Args:
            text: 処理対象のテキスト

        Returns:
            ANSIエスケープシーケンスを除去したテキスト
        """
        return self._ANSI_ESCAPE_PATTERN.sub("", text)

    def info(self, message: str) -> None:
        """情報メッセージを出力する（NORMAL以上）

        Args:
            message: 出力するメッセージ
        """
        if self._config.verbose_level >= VerboseLevel.NORMAL:
            self._print(message)
        self._log_to_file("INFO", message)

    def verbose(self, message: str) -> None:
        """詳細メッセージを出力する（VERBOSE以上）

        Args:
            message: 出力するメッセージ
        """
        if self._config.verbose_level >= VerboseLevel.VERBOSE:
            self._print(message)
        self._log_to_file("VERBOSE", message)

    def debug(self, message: str) -> None:
        """デバッグメッセージを出力する（DEBUG以上）

        Args:
            message: 出力するメッセージ
        """
        if self._config.verbose_level >= VerboseLevel.DEBUG:
            self._print(message)
        self._log_to_file("DEBUG", message)

    def error(self, message: str) -> None:
        """エラーメッセージを出力する（常に出力）

        Args:
            message: 出力するメッセージ
        """
        self._print(f"エラー: {message}", file=sys.stderr)
        self._log_to_file("ERROR", message)

    def warning(self, message: str) -> None:
        """警告メッセージを出力する（QUIET以上）

        Args:
            message: 出力するメッセージ
        """
        if self._config.verbose_level > VerboseLevel.QUIET:
            self._print(f"警告: {message}")
        self._log_to_file("WARNING", message)

    def create_progress(self) -> ProgressDisplay:
        """進捗表示インスタンスを作成する

        Returns:
            進捗表示インスタンス
        """
        return ConsoleProgressDisplay(
            use_color=self._config.use_color,
            use_emoji=self._config.use_emoji,
        )

    def log_command(self, command: list[str], output: str) -> None:
        """外部コマンド実行をログする（DEBUG以上）

        Args:
            command: 実行したコマンドとその引数
            output: コマンドの出力
        """
        cmd_str = " ".join(command)
        self.debug(f"実行: {cmd_str}")
        if output:
            for line in output.splitlines():
                self.debug(f"  > {line}")

    def log_conversion(self, source: Path, dest: Path, status: str) -> None:
        """ファイル変換をログする（VERBOSE以上）

        Args:
            source: 変換元ファイルパス
            dest: 変換先ファイルパス
            status: 変換ステータス
        """
        self.verbose(f"変換: {source.name} -> {dest.name} [{status}]")

    def log_summary(self, statistics: dict[str, Any]) -> None:
        """ビルドサマリを出力する（NORMAL以上）

        Args:
            statistics: ビルド統計情報
        """
        emoji = "\u2705" if self._config.use_emoji else "[OK]"
        self.info(f"{emoji} Build complete!")
        if "output_path" in statistics:
            size_mb = statistics.get("output_size", 0) / (1024 * 1024)
            self.info(f"   Output: {statistics['output_path']} ({size_mb:.1f} MB)")
        if "package_name" in statistics:
            self.info(f"   Package: {statistics['package_name']}")


class ConsoleProgressDisplay:
    """コンソール進捗表示

    ビルドパイプラインの各フェーズの進捗をコンソールに表示するクラス。
    進捗バーと絵文字を使用して視覚的にわかりやすい表示を行う。
    """

    # TYPE_CHECKINGブロック外でPipelinePhaseをインポートする必要があるため遅延インポート
    PHASE_EMOJI: dict[str, str] = {
        "analyze": "\U0001f50d",
        "extract": "\U0001f4e6",
        "convert": "\U0001f504",
        "build": "\U0001f528",
        "sign": "\U0001f50f",
    }

    PHASE_NAME: dict[str, str] = {
        "analyze": "Analyzing game structure",
        "extract": "Extracting assets",
        "convert": "Converting assets",
        "build": "Building APK",
        "sign": "Signing APK",
    }

    def __init__(self, use_color: bool = True, use_emoji: bool = True) -> None:
        """進捗表示を初期化する

        Args:
            use_color: カラー出力を使用するか
            use_emoji: 絵文字を使用するか
        """
        self._use_color = use_color
        self._use_emoji = use_emoji
        self._phase: PipelinePhase | None = None
        self._total = 0
        self._current = 0

    def start(self, phase: PipelinePhase, total: int) -> None:
        """フェーズ開始を表示する

        Args:
            phase: 開始するパイプラインフェーズ
            total: 処理対象の総数
        """
        self._phase = phase
        self._total = total
        self._current = 0
        emoji = self.PHASE_EMOJI.get(phase.value, "") if self._use_emoji else ""
        name = self.PHASE_NAME.get(phase.value, str(phase))
        prefix = f"{emoji} " if emoji else ""
        print(f"{prefix}{name}...")

    def update(self, current: int, message: str = "") -> None:
        """進捗を更新する

        Args:
            current: 現在の進捗（処理済みアイテム数）
            message: 追加の進捗メッセージ（オプション）
        """
        self._current = current
        if self._total > 0:
            percent = int((current / self._total) * 100)
            bar_width = 40
            filled = int(bar_width * current / self._total)
            bar = "\u2588" * filled + "\u2591" * (bar_width - filled)
            msg_part = f" {message}" if message else ""
            print(f"\r   [{bar}] {percent}%{msg_part}", end="", flush=True)

    def finish(self, success: bool, message: str = "") -> None:
        """フェーズ終了を表示する

        Args:
            success: フェーズが成功したか
            message: 終了メッセージ（オプション）
        """
        bar_width = 40
        full_bar = "\u2588" * bar_width
        if success:
            mark = "\u2713" if self._use_emoji else "done"
            print(f"\r   [{full_bar}] 100% {mark}")
        else:
            mark = "\u2717" if self._use_emoji else "failed"
            msg_part = f": {message}" if message else ""
            print(f"\r   [{full_bar}] {mark}{msg_part}")
