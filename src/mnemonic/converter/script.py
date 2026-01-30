"""スクリプト調整モジュール

KiriKiriZ向けにゲームスクリプト(.ks, .tjs)を調整するためのコンバーター。
プラグインDLL読み込みの無効化やエンコーディングディレクティブの追加を行う。
"""

from dataclasses import dataclass
from pathlib import Path

from mnemonic.converter.base import BaseConverter, ConversionResult

@dataclass(frozen=True)
class AdjustmentRule:
    """スクリプト調整ルール

    正規表現パターンと置換文字列のペアを保持する不変データクラス。

    Attributes:
        pattern: マッチする正規表現パターン
        replacement: 置換文字列
        description: ルールの説明（日本語）
    """

    pattern: str
    replacement: str
    description: str

class ScriptAdjuster(BaseConverter):
    """スクリプト調整クラス

    KiriKiriZ向けにゲームスクリプトを調整するコンバーター。
    プラグインDLL読み込みの無効化やエンコーディングディレクティブの追加など、
    Android環境での動作に必要な調整を行う。
    """

    DEFAULT_RULES: list[AdjustmentRule] = [
        AdjustmentRule(
            pattern=r'^(\s*)(Plugins\.link\(["\'].*?\.dll["\']\);)',
            replacement=r"\1// \2 // Disabled for Android",
            description="プラグインDLL読み込みの無効化",
        ),
    ]

    def __init__(
        self,
        rules: list[AdjustmentRule] | None = None,
        add_encoding_directive: bool = True,
    ) -> None:
        """ScriptAdjusterを初期化する

        Args:
            rules: 適用する調整ルールのリスト。Noneの場合はDEFAULT_RULESを使用
            add_encoding_directive: startup.tjsにエンコーディングディレクティブを追加するかどうか
        """
        self._rules = rules if rules is not None else self.DEFAULT_RULES.copy()
        self._add_encoding_directive = add_encoding_directive

    @property
    def rules(self) -> list[AdjustmentRule]:
        """適用される調整ルールを返す"""
        return self._rules

    @property
    def add_encoding_directive(self) -> bool:
        """エンコーディングディレクティブを追加するかどうかを返す"""
        return self._add_encoding_directive

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        Returns:
            KAGスクリプト(.ks)とTJSスクリプト(.tjs)の拡張子タプル
        """
        return (".ks", ".tjs")

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            .ksまたは.tjsファイルの場合True、そうでない場合False
        """
        raise NotImplementedError

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """スクリプトファイルを調整する

        指定されたスクリプトファイルに調整ルールを適用し、
        変換先パスに出力する。

        Args:
            source: 変換元ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        raise NotImplementedError

    def adjust_content(self, content: str, filename: str = "") -> tuple[str, int]:
        """スクリプト内容を調整する

        与えられたスクリプト内容に調整ルールを適用する。

        Args:
            content: 調整するスクリプト内容
            filename: ファイル名（startup.tjs判定用）

        Returns:
            (調整後の内容, 調整回数)のタプル
        """
        raise NotImplementedError

    def add_startup_directive(self, content: str) -> str:
        """startup.tjsにエンコーディングディレクティブを追加する

        KiriKiriZ用のエンコーディング指定ディレクティブをスクリプト先頭に追加する。

        Args:
            content: 元のスクリプト内容

        Returns:
            ディレクティブを追加した内容
        """
        raise NotImplementedError
