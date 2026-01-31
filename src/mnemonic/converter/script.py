"""スクリプト調整モジュール

KiriKiriZ向けにゲームスクリプト(.ks, .tjs)を調整するためのコンバーター。
プラグインDLL読み込みの無効化やエンコーディングディレクティブの追加を行う。
"""

import re
from dataclasses import dataclass
from pathlib import Path

from mnemonic.converter.base import BaseConverter, ConversionResult, ConversionStatus


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
        AdjustmentRule(
            pattern=r"saveDataLocation\s*=\s*System\.exePath\s*\+\s*saveDataLocation",
            replacement="saveDataLocation = System.dataPath",
            description="セーブデータパスをdataPathに変更（Android対応）",
        ),
        AdjustmentRule(
            pattern=r"MIDISoundBuffer",
            replacement="WaveSoundBuffer",
            description="MIDISoundBufferをWaveSoundBufferに変換（krkrsdl2対応）",
        ),
        AdjustmentRule(
            pattern=r'(["\'])([^"\']*\.mid)(["\'])',
            replacement=r"\1\2.ogg\3",
            description="MIDI参照をOGGに変換（.mid → .mid.ogg）",
        ),
        AdjustmentRule(
            pattern=r'(["\'])([^"\']*\.midi)(["\'])',
            replacement=r"\1\2.ogg\3",
            description="MIDI参照をOGGに変換（.midi → .midi.ogg）",
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
        suffix = file_path.suffix.lower()
        return suffix in self.supported_extensions

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
        if not source.exists():
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"変換元ファイルが見つかりません: {source}",
            )

        try:
            content = source.read_text(encoding="utf-8-sig")  # BOM を自動除去
            bytes_before = len(content.encode("utf-8"))

            adjusted_content, adjustment_count = self.adjust_content(content, source.name)

            # startup.tjsの場合、エンコーディングディレクティブを追加
            is_startup = source.name.lower() == "startup.tjs"
            if is_startup and self._add_encoding_directive:
                adjusted_content = self.add_startup_directive(adjusted_content)
                adjustment_count += 1

            # 調整がなければスキップ
            if adjustment_count == 0:
                return ConversionResult(
                    source_path=source,
                    dest_path=None,
                    status=ConversionStatus.SKIPPED,
                    message="調整が不要なファイルです",
                    bytes_before=bytes_before,
                    bytes_after=bytes_before,
                )

            # 出力先ディレクトリを作成
            dest.parent.mkdir(parents=True, exist_ok=True)

            # 調整後の内容を書き込み（Kirikiri スクリプトは UTF-8 BOM が必要）
            dest.write_text(adjusted_content, encoding="utf-8-sig")
            bytes_after = len(adjusted_content.encode("utf-8")) + 3  # BOM 3バイト

            return ConversionResult(
                source_path=source,
                dest_path=dest,
                status=ConversionStatus.SUCCESS,
                message=f"{adjustment_count}箇所を調整しました",
                bytes_before=bytes_before,
                bytes_after=bytes_after,
            )

        except Exception as e:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=str(e),
            )

    def adjust_content(self, content: str, filename: str = "") -> tuple[str, int]:
        """スクリプト内容を調整する

        与えられたスクリプト内容に調整ルールを適用する。

        Args:
            content: 調整するスクリプト内容
            filename: ファイル名（startup.tjs判定用）

        Returns:
            (調整後の内容, 調整回数)のタプル
        """
        total_count = 0
        result = content

        for rule in self._rules:
            pattern = re.compile(rule.pattern, re.MULTILINE)
            new_result, count = pattern.subn(rule.replacement, result)
            total_count += count
            result = new_result

        return result, total_count

    def add_startup_directive(self, content: str) -> str:
        """startup.tjsにエンコーディングディレクティブとプラットフォームスタブを追加する

        KiriKiriZ用のエンコーディング指定ディレクティブと、
        Android で欠落しているクラス（MenuItem等）のスタブをスクリプト先頭に追加する。

        Args:
            content: 元のスクリプト内容

        Returns:
            ディレクティブとスタブを追加した内容
        """
        directive = """// krkrsdl2 polyfill initialization
Scripts.execStorage("system/polyfillinitialize.tjs");

"""
        return directive + content
