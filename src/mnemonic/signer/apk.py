"""APK署名関連の機能

このモジュールはAPKファイルの署名に関連する機能を提供します。
zipalignによるアラインメント最適化とapksignerによる署名のインターフェースを定義します。
"""

from __future__ import annotations

import os
import shutil
import subprocess
from dataclasses import dataclass
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

class ApkSignerError(Exception):
    """apksigner実行に関する基本例外クラス

    apksignerコマンドの実行時に発生するエラーを表します。
    署名処理の失敗、検証エラー、コマンド不在などを含みます。
    """

    pass

@dataclass(frozen=True)
class KeystoreConfig:
    """キーストア設定を表す不変データクラス

    APK署名に必要なキーストアの設定情報を保持します。
    すべてのフィールドは不変であり、設定後の変更はできません。

    Attributes:
        keystore_path: キーストアファイルのパス
        key_alias: キーのエイリアス名
        keystore_password: キーストアのパスワード
        key_password: キーのパスワード（省略時はkeystore_passwordを使用）
    """

    keystore_path: Path
    key_alias: str
    keystore_password: str
    key_password: str | None = None

class ApkSignerRunner(Protocol):
    """apksignerコマンドを実行するためのインターフェース

    APKファイルの署名と検証を行うapksignerコマンドの
    実行機能を抽象化したProtocolです。
    """

    def sign(self, apk_path: Path, keystore_config: KeystoreConfig) -> Path:
        """APKファイルに署名を適用する

        指定されたAPKファイルに対してapksignerを実行し、
        キーストア設定を使用して署名を適用します。

        Args:
            apk_path: 署名対象のAPKファイルのパス
            keystore_config: キーストア設定

        Returns:
            署名されたAPKファイルのパス

        Raises:
            ApkSignerError: 署名処理に失敗した場合
        """
        ...

    def verify(self, apk_path: Path) -> bool:
        """APKファイルの署名を検証する

        指定されたAPKファイルの署名が有効かどうかを検証します。

        Args:
            apk_path: 検証対象のAPKファイルのパス

        Returns:
            署名が有効な場合はTrue、無効な場合はFalse

        Raises:
            ApkSignerError: 検証処理に失敗した場合
        """
        ...

    def find_apksigner(self) -> Path | None:
        """apksignerコマンドのパスを検索する

        Android SDKからapksignerコマンドを検索します。
        ANDROID_HOME環境変数やシステムPATHを参照して検索を行います。

        Returns:
            apksignerコマンドのパス。見つからない場合はNone。
        """
        ...

class DefaultApkSignerRunner:
    """apksignerコマンドを実行するデフォルト実装

    Android SDKのapksignerコマンドを使用してAPKファイルの
    署名と検証を行います。
    """

    def sign(self, apk_path: Path, keystore_config: KeystoreConfig) -> Path:
        """APKファイルに署名を適用する

        指定されたAPKファイルに対してapksigner signを実行し、
        キーストア設定を使用して署名を適用します。

        Args:
            apk_path: 署名対象のAPKファイルのパス
            keystore_config: キーストア設定

        Returns:
            署名されたAPKファイルのパス

        Raises:
            ApkSignerError: APKファイルが存在しない、キーストアが存在しない、
                          apksignerが見つからない、または署名処理に失敗した場合
        """
        if not apk_path.exists():
            raise ApkSignerError(f"APK file not found: {apk_path}")

        if not keystore_config.keystore_path.exists():
            raise ApkSignerError(f"Keystore file not found: {keystore_config.keystore_path}")

        apksigner_path = self.find_apksigner()
        if apksigner_path is None:
            raise ApkSignerError("apksigner command not found")

        key_password = (
            keystore_config.key_password
            if keystore_config.key_password is not None
            else keystore_config.keystore_password
        )

        result = subprocess.run(
            [
                str(apksigner_path),
                "sign",
                "--ks",
                str(keystore_config.keystore_path),
                "--ks-key-alias",
                keystore_config.key_alias,
                "--ks-pass",
                f"pass:{keystore_config.keystore_password}",
                "--key-pass",
                f"pass:{key_password}",
                str(apk_path),
            ],
            capture_output=True,
            text=True,
        )

        if result.returncode != 0:
            raise ApkSignerError(f"apksigner sign failed: {result.stderr}")

        return apk_path

    def verify(self, apk_path: Path) -> bool:
        """APKファイルの署名を検証する

        apksigner verifyコマンドを使用してAPKファイルの
        署名の有効性を検証します。

        Args:
            apk_path: 検証対象のAPKファイルのパス

        Returns:
            署名が有効な場合はTrue、無効な場合はFalse

        Raises:
            ApkSignerError: ファイルが存在しない、apksignerが見つからない、
                          またはコマンド実行中にエラーが発生した場合
        """
        if not apk_path.exists():
            raise ApkSignerError(f"APK file not found: {apk_path}")

        apksigner_path = self.find_apksigner()
        if apksigner_path is None:
            raise ApkSignerError("apksigner command not found")

        try:
            result = subprocess.run(
                [str(apksigner_path), "verify", str(apk_path)],
                capture_output=True,
                text=True,
            )
            return result.returncode == 0
        except subprocess.SubprocessError as e:
            raise ApkSignerError(f"apksigner verify failed: {e}") from e

    def find_apksigner(self) -> Path | None:
        """apksignerコマンドのパスを検索する

        Android SDKのbuild-toolsディレクトリからapksignerコマンドを検索します。
        複数のバージョンがある場合は最新バージョンを選択します。
        ANDROID_HOMEが設定されていない、またはapksignerが見つからない場合は
        システムPATHから検索します。

        Returns:
            apksignerコマンドのパス。見つからない場合はNone。
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
                    apksigner_path = version_dir / "apksigner"
                    if apksigner_path.exists():
                        return apksigner_path

        which_result = shutil.which("apksigner")
        if which_result:
            return Path(which_result)

        return None

class PasswordError(Exception):
    """パスワード取得に関する基本例外クラス

    キーストアパスワードの取得時に発生するエラーを表します。
    環境変数が未設定、対話的入力の失敗、キャンセルなどを含みます。
    """

    pass

class PasswordProvider(Protocol):
    """キーストアパスワードを取得するためのインターフェース

    APK署名時に必要なキーストアパスワードを
    様々なソース（対話的入力、環境変数など）から取得する機能を
    抽象化したProtocolです。
    """

    def get_password(self, prompt: str = "Enter keystore password: ") -> str:
        """対話的にパスワードを取得する

        ユーザーに対してプロンプトを表示し、パスワードの入力を求めます。
        入力されたパスワードは画面に表示されません。

        Args:
            prompt: パスワード入力を求める際に表示するプロンプト文字列

        Returns:
            入力されたパスワード文字列

        Raises:
            PasswordError: パスワードの取得に失敗した場合
        """
        ...

    def get_password_from_env(self, env_var: str = "MNEMONIC_KEYSTORE_PASS") -> str | None:
        """環境変数からパスワードを取得する

        指定された環境変数からパスワードを取得します。
        環境変数が設定されていない場合はNoneを返します。

        Args:
            env_var: パスワードが格納されている環境変数名

        Returns:
            環境変数に設定されたパスワード。未設定の場合はNone。
        """
        ...

class DefaultPasswordProvider:
    """キーストアパスワードを取得するデフォルト実装

    対話的入力（getpass）と環境変数からのパスワード取得をサポートします。
    """

    def get_password(self, prompt: str = "Enter keystore password: ") -> str:
        """対話的にパスワードを取得する

        ユーザーに対してプロンプトを表示し、パスワードの入力を求めます。
        入力されたパスワードは画面に表示されません。

        Args:
            prompt: パスワード入力を求める際に表示するプロンプト文字列

        Returns:
            入力されたパスワード文字列

        Raises:
            PasswordError: パスワードが空、または入力がキャンセルされた場合
        """
        import getpass

        try:
            password = getpass.getpass(prompt)
        except KeyboardInterrupt as e:
            raise PasswordError("Password input cancelled by user interrupt") from e
        except EOFError as e:
            raise PasswordError("Password input failed: EOF received") from e

        if not password:
            raise PasswordError("Password cannot be empty")

        return password

    def get_password_from_env(self, env_var: str = "MNEMONIC_KEYSTORE_PASS") -> str | None:
        """環境変数からパスワードを取得する

        指定された環境変数からパスワードを取得します。
        環境変数が設定されていない、または空文字列の場合はNoneを返します。

        Args:
            env_var: パスワードが格納されている環境変数名

        Returns:
            環境変数に設定されたパスワード。未設定または空の場合はNone。
        """
        password = os.environ.get(env_var)
        if not password:
            return None
        return password
