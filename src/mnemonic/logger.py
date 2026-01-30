"""進捗表示およびログ出力のインターフェース定義

このモジュールは、Mnemonicのビルド進捗表示とログ出力のためのインターフェースを定義する。
VerboseLevel (詳細ログレベル)に応じた出力制御を行い、
CLIでのビルド進捗をユーザーにわかりやすく表示するために使用される。
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import IntEnum
from pathlib import Path
from typing import TYPE_CHECKING, Protocol

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

    def __init__(self, config: LogConfig) -> None:
        """ロガーを初期化する

        Args:
            config: ログ設定
        """
        self._config = config

    @property
    def config(self) -> LogConfig:
        """ログ設定を取得する

        Returns:
            現在のログ設定
        """
        return self._config

    def info(self, message: str) -> None:
        """情報メッセージを出力する（NORMAL以上）

        Args:
            message: 出力するメッセージ
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def verbose(self, message: str) -> None:
        """詳細メッセージを出力する（VERBOSE以上）

        Args:
            message: 出力するメッセージ
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def debug(self, message: str) -> None:
        """デバッグメッセージを出力する（DEBUG以上）

        Args:
            message: 出力するメッセージ
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def error(self, message: str) -> None:
        """エラーメッセージを出力する（常に出力）

        Args:
            message: 出力するメッセージ
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def warning(self, message: str) -> None:
        """警告メッセージを出力する（QUIET以上）

        Args:
            message: 出力するメッセージ
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def create_progress(self) -> ProgressDisplay:
        """進捗表示インスタンスを作成する

        Returns:
            進捗表示インスタンス
        """
        # スタブ実装: I-03-02で実装予定
        raise NotImplementedError("進捗表示作成はI-03-02で実装予定")

    def log_command(self, command: list[str], output: str) -> None:
        """外部コマンド実行をログする（DEBUG以上）

        Args:
            command: 実行したコマンドとその引数
            output: コマンドの出力
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def log_conversion(self, source: Path, dest: Path, status: str) -> None:
        """ファイル変換をログする（VERBOSE以上）

        Args:
            source: 変換元ファイルパス
            dest: 変換先ファイルパス
            status: 変換ステータス
        """
        # スタブ実装: I-03-02で実装予定
        pass

    def log_summary(self, statistics: dict) -> None:
        """ビルドサマリを出力する（NORMAL以上）

        Args:
            statistics: ビルド統計情報
        """
        # スタブ実装: I-03-02で実装予定
        pass
