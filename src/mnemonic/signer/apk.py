"""APK署名関連の機能

このモジュールはAPKファイルの署名に関連する機能を提供します。
zipalignによるアラインメント最適化のインターフェースを定義します。
"""

from __future__ import annotations

import os
import shutil
import subprocess
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

class DefaultZipalignRunner:
    """zipalignコマンドを実行するデフォルト実装

    Android SDKのzipalignコマンドを使用してAPKファイルの
    アラインメント最適化を行います。
    """

    def align(self, input_path: Path, output_path: Path) -> Path:
        """APKファイルにアラインメント最適化を適用する

        指定された入力APKファイルに対してzipalign -p -f 4を実行し、
        アラインメント最適化されたAPKを出力パスに生成します。

        Args:
            input_path: 入力APKファイルのパス
            output_path: 出力APKファイルのパス

        Returns:
            アラインメント最適化されたAPKファイルのパス

        Raises:
            ZipalignError: 入力ファイルが存在しない、zipalignが見つからない、
                          またはzipalign実行に失敗した場合
        """
        if not input_path.exists():
            raise ZipalignError(f"Input file not found: {input_path}")

        zipalign_path = self.find_zipalign()
        if zipalign_path is None:
            raise ZipalignError("zipalign command not found")

        result = subprocess.run(
            [str(zipalign_path), "-p", "-f", "4", str(input_path), str(output_path)],
            capture_output=True,
            text=True,
        )

        if result.returncode != 0:
            raise ZipalignError(f"zipalign failed: {result.stderr}")

        return output_path

    def find_zipalign(self) -> Path | None:
        """zipalignコマンドのパスを検索する

        Android SDKのbuild-toolsディレクトリからzipalignコマンドを検索します。
        複数のバージョンがある場合は最新バージョンを選択します。
        ANDROID_HOMEが設定されていない、またはzipalignが見つからない場合は
        システムPATHから検索します。

        Returns:
            zipalignコマンドのパス。見つからない場合はNone。
        """
        android_home = os.environ.get("ANDROID_HOME")

        if android_home:
            android_home_path = Path(android_home)
            build_tools_dir = android_home_path / "build-tools"

            if build_tools_dir.exists():
                versions = sorted(
                    [d for d in build_tools_dir.iterdir() if d.is_dir()],
                    key=lambda x: x.name,
                    reverse=True,
                )

                for version_dir in versions:
                    zipalign_path = version_dir / "zipalign"
                    if zipalign_path.exists():
                        return zipalign_path

        which_result = shutil.which("zipalign")
        if which_result:
            return Path(which_result)

        return None

    def is_aligned(self, apk_path: Path) -> bool:
        """APKファイルがアラインメント済みかどうかを確認する

        zipalign -c -v 4コマンドを使用してAPKファイルの
        アラインメント状態を確認します。

        Args:
            apk_path: 確認対象のAPKファイルのパス

        Returns:
            アラインメント済みの場合はTrue、そうでない場合はFalse

        Raises:
            ZipalignError: ファイルが存在しない、zipalignが見つからない、
                          またはコマンド実行中にエラーが発生した場合
        """
        if not apk_path.exists():
            raise ZipalignError(f"APK file not found: {apk_path}")

        zipalign_path = self.find_zipalign()
        if zipalign_path is None:
            raise ZipalignError("zipalign command not found")

        try:
            result = subprocess.run(
                [str(zipalign_path), "-c", "-v", "4", str(apk_path)],
                capture_output=True,
                text=True,
            )
            return result.returncode == 0
        except subprocess.SubprocessError as e:
            raise ZipalignError(f"zipalign verification failed: {e}") from e
