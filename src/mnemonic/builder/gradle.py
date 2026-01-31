"""Gradleビルド機能

このモジュールはGradle assembleReleaseを実行してAPKを生成する機能を提供します。
"""

from __future__ import annotations

import platform
import stat
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path


class GradleBuildError(Exception):
    """Gradleビルドに関する基本例外クラス"""

    pass


class GradleTimeoutError(GradleBuildError):
    """Gradleビルドがタイムアウトした場合の例外"""

    pass


class GradleNotFoundError(GradleBuildError):
    """Gradle wrapperが見つからない場合の例外"""

    pass


@dataclass(frozen=True)
class GradleBuildResult:
    """Gradleビルド結果を表す不変オブジェクト

    Attributes:
        success: ビルドが成功したかどうか
        apk_path: 生成されたAPKファイルのパス。ビルド失敗時はNone。
        build_time: ビルドにかかった時間（秒）
        output_log: Gradleの出力ログ
    """

    success: bool
    apk_path: Path | None
    build_time: float
    output_log: str


class GradleBuilder:
    """Gradleビルドを実行するクラス

    このクラスはAndroidプロジェクトのGradleビルドを実行し、
    APKを生成する機能を提供します。
    """

    DEFAULT_TIMEOUT = 1800  # デフォルトのタイムアウト（秒）

    def __init__(self, project_path: Path, timeout: int = 1800) -> None:
        """GradleBuilderを初期化する

        Args:
            project_path: Androidプロジェクトのルートパス
            timeout: ビルドのタイムアウト時間（秒）。デフォルトは1800秒（30分）。
        """
        self._project_path = project_path
        self._timeout = timeout
        self._disable_gradle_caching()

    def _disable_gradle_caching(self) -> None:
        """gradle.propertiesにキャッシュ無効化設定を追加する

        一時ディレクトリでのビルドで発生するincremental build問題を回避するため、
        Gradleのキャッシュ機能とファイルシステムウォッチングを無効化する。
        """
        gradle_props = self._project_path / "gradle.properties"
        settings = [
            "org.gradle.caching=false",
            "org.gradle.vfs.watch=false",
        ]

        if gradle_props.exists():
            content = gradle_props.read_text()
            additions = []
            for setting in settings:
                key = setting.split("=")[0]
                if key not in content:
                    additions.append(setting)
            if additions:
                with gradle_props.open("a") as f:
                    f.write("\n" + "\n".join(additions) + "\n")
        else:
            gradle_props.write_text("\n".join(settings) + "\n")

    def _get_gradle_command(self) -> Path:
        """プラットフォームに応じたGradle Wrapperコマンドを取得する

        Returns:
            Gradle Wrapperのパス

        Raises:
            GradleNotFoundError: Gradle wrapperが見つからない場合
        """
        if platform.system() == "Windows":
            gradlew = self._project_path / "gradlew.bat"
        else:
            gradlew = self._project_path / "gradlew"

        if not gradlew.exists():
            raise GradleNotFoundError(f"Gradle wrapper not found at {gradlew}")

        # ZIPから展開した場合に実行権限がないことがあるため付与
        if platform.system() != "Windows":
            current_mode = gradlew.stat().st_mode
            if not (current_mode & stat.S_IXUSR):
                gradlew.chmod(current_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

        return gradlew

    def _run_gradle(self, *args: str) -> subprocess.CompletedProcess[str]:
        """Gradleコマンドを実行する

        Args:
            *args: Gradleに渡す引数

        Returns:
            subprocess.CompletedProcess

        Raises:
            GradleNotFoundError: Gradle wrapperが見つからない場合
            GradleTimeoutError: タイムアウトした場合
        """
        gradlew = self._get_gradle_command()
        cmd = [
            str(gradlew),
            *args,
            "--no-daemon",
            "--no-build-cache",
            "--rerun-tasks",
            "--stacktrace",
        ]

        # ロケール関連の問題を回避するため、C.utf8 を設定
        import os

        env = os.environ.copy()
        env["LC_ALL"] = "C.utf8"
        env["LANG"] = "C.utf8"

        try:
            result = subprocess.run(
                cmd,
                cwd=self._project_path,
                capture_output=True,
                text=True,
                timeout=self._timeout,
                env=env,
            )
            return result
        except subprocess.TimeoutExpired as e:
            raise GradleTimeoutError(
                f"Gradle command timed out after {self._timeout} seconds"
            ) from e

    def build(self, build_type: str = "release") -> GradleBuildResult:
        """Gradleビルドを実行する

        Args:
            build_type: ビルドタイプ（"release" または "debug"）。デフォルトは "release"。

        Returns:
            ビルド結果を表すGradleBuildResult

        Raises:
            GradleBuildError: ビルド処理中にエラーが発生した場合
            GradleTimeoutError: ビルドがタイムアウトした場合
            GradleNotFoundError: Gradle wrapperが見つからない場合
        """
        task = f"assemble{build_type.capitalize()}"
        start_time = time.time()

        result = self._run_gradle(task)
        build_time = time.time() - start_time

        output_log = result.stdout + result.stderr

        if result.returncode != 0:
            raise GradleBuildError(
                f"Gradle build failed with exit code {result.returncode}: {output_log}"
            )

        apk_path = self.get_apk_path(build_type)

        return GradleBuildResult(
            success=True,
            apk_path=apk_path,
            build_time=build_time,
            output_log=output_log,
        )

    def clean(self) -> None:
        """ビルドキャッシュをクリアする

        Gradleのcleanタスクを実行してビルドキャッシュを削除します。

        Raises:
            GradleBuildError: クリーン処理中にエラーが発生した場合
            GradleNotFoundError: Gradle wrapperが見つからない場合
        """
        result = self._run_gradle("clean")

        if result.returncode != 0:
            output_log = result.stdout + result.stderr
            raise GradleBuildError(
                f"Gradle clean failed with exit code {result.returncode}: {output_log}"
            )

    def check_gradle_wrapper(self) -> bool:
        """Gradle wrapperの存在を確認する

        プロジェクトディレクトリにgradlew（またはgradlew.bat）が
        存在するかを確認します。

        Returns:
            Gradle wrapperが存在する場合はTrue、存在しない場合はFalse
        """
        if platform.system() == "Windows":
            gradlew = self._project_path / "gradlew.bat"
        else:
            gradlew = self._project_path / "gradlew"

        return gradlew.exists()

    def get_apk_path(self, build_type: str = "release") -> Path | None:
        """生成されたAPKファイルのパスを取得する

        Args:
            build_type: ビルドタイプ（"release" または "debug"）。デフォルトは "release"。

        Returns:
            APKファイルのパス。ファイルが存在しない場合はNone。
        """
        if build_type == "release":
            apk_name = "app-release-unsigned.apk"
        else:
            apk_name = f"app-{build_type}.apk"

        apk_path = self._project_path / "app" / "build" / "outputs" / "apk" / build_type / apk_name

        if apk_path.exists():
            return apk_path
        return None
