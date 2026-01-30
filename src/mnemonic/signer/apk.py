"""APK署名関連の機能

このモジュールはAPKファイルの署名に関連する機能を提供します。
zipalignによるアラインメント最適化のインターフェースを定義します。
"""

from __future__ import annotations

from pathlib import Path
from typing import Protocol

class ZipalignError(Exception):
    """zipalign実行に関する基本例外クラス

    zipalignコマンドの実行時に発生するエラーを表します。
    アラインメント処理の失敗、コマンド不在、入出力エラーなどを含みます。
    """

    pass

class ZipalignRunner(Protocol):
    """zipalignコマンドを実行するためのインターフェース

    APKファイルのアラインメント最適化を行うzipalignコマンドの
    実行機能を抽象化したProtocolです。
    """

    def align(self, input_path: Path, output_path: Path) -> Path:
        """APKファイルにアラインメント最適化を適用する

        指定された入力APKファイルに対してzipalignを実行し、
        アラインメント最適化されたAPKを出力パスに生成します。

        Args:
            input_path: 入力APKファイルのパス
            output_path: 出力APKファイルのパス

        Returns:
            アラインメント最適化されたAPKファイルのパス

        Raises:
            ZipalignError: zipalign実行に失敗した場合
        """
        ...

    def find_zipalign(self) -> Path | None:
        """zipalignコマンドのパスを検索する

        Android SDKからzipalignコマンドを検索します。
        ANDROID_HOME環境変数やシステムPATHを参照して検索を行います。

        Returns:
            zipalignコマンドのパス。見つからない場合はNone。
        """
        ...

    def is_aligned(self, apk_path: Path) -> bool:
        """APKファイルがアラインメント済みかどうかを確認する

        指定されたAPKファイルが既にzipalignによる
        アラインメント最適化が適用されているかを確認します。

        Args:
            apk_path: 確認対象のAPKファイルのパス

        Returns:
            アラインメント済みの場合はTrue、そうでない場合はFalse

        Raises:
            ZipalignError: 確認処理に失敗した場合
        """
        ...
