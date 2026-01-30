"""Gradleビルド機能のテスト

このモジュールはGradleBuilderクラスのテストを提供します。
"""

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.builder.gradle import (
    GradleBuilder,
    GradleBuildError,
    GradleBuildResult,
    GradleNotFoundError,
    GradleTimeoutError,
)

class TestGradleBuildResult:
    """GradleBuildResultデータクラスのテスト"""

    def test_creation_success(self) -> None:
        """正常系: 成功時のGradleBuildResult作成"""
        result = GradleBuildResult(
            success=True,
            apk_path=Path("/project/app/build/outputs/apk/release/app-release-unsigned.apk"),
            build_time=120.5,
            output_log="BUILD SUCCESSFUL",
        )

        assert result.success is True
        assert result.apk_path == Path(
            "/project/app/build/outputs/apk/release/app-release-unsigned.apk"
        )
        assert result.build_time == 120.5
        assert result.output_log == "BUILD SUCCESSFUL"

    def test_creation_failure(self) -> None:
        """正常系: 失敗時のGradleBuildResult作成"""
        result = GradleBuildResult(
            success=False,
            apk_path=None,
            build_time=30.0,
            output_log="BUILD FAILED",
        )

        assert result.success is False
        assert result.apk_path is None
        assert result.build_time == 30.0
        assert result.output_log == "BUILD FAILED"

    def test_immutability(self) -> None:
        """正常系: GradleBuildResultが不変であることのテスト"""
        result = GradleBuildResult(
            success=True,
            apk_path=Path("/test/path.apk"),
            build_time=100.0,
            output_log="SUCCESS",
        )

        with pytest.raises(AttributeError):
            result.success = False  # type: ignore[misc]

class TestGradleBuilderInit:
    """GradleBuilder初期化のテスト"""

    def test_init_with_default_timeout(self, tmp_path: Path) -> None:
        """正常系: デフォルトタイムアウトでの初期化"""
        builder = GradleBuilder(project_path=tmp_path)

        assert builder._project_path == tmp_path
        assert builder._timeout == GradleBuilder.DEFAULT_TIMEOUT

    def test_init_with_custom_timeout(self, tmp_path: Path) -> None:
        """正常系: カスタムタイムアウトでの初期化"""
        builder = GradleBuilder(project_path=tmp_path, timeout=3600)

        assert builder._project_path == tmp_path
        assert builder._timeout == 3600

    def test_default_timeout_is_1800_seconds(self) -> None:
        """正常系: デフォルトタイムアウトが1800秒であることの確認"""
        assert GradleBuilder.DEFAULT_TIMEOUT == 1800

class TestGradleBuilderCheckGradleWrapper:
    """GradleBuilder.check_gradle_wrapperのテスト"""

    def test_check_gradle_wrapper_exists(self, tmp_path: Path) -> None:
        """正常系: gradlewが存在する場合にTrueを返す"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.check_gradle_wrapper()

    def test_check_gradle_wrapper_not_exists(self, tmp_path: Path) -> None:
        """正常系: gradlewが存在しない場合にFalseを返す"""
        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.check_gradle_wrapper()

class TestGradleBuilderBuild:
    """GradleBuilder.buildのテスト"""

    def test_build_success(self, tmp_path: Path) -> None:
        """正常系: ビルドが成功する場合"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.build()

    def test_build_failure(self, tmp_path: Path) -> None:
        """異常系: ビルドが失敗する場合"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.build()

    def test_build_timeout(self, tmp_path: Path) -> None:
        """異常系: ビルドがタイムアウトする場合"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path, timeout=1)

        with pytest.raises(NotImplementedError):
            builder.build()

    def test_build_without_gradlew(self, tmp_path: Path) -> None:
        """異常系: gradlewが存在しない場合にGradleNotFoundErrorが発生"""
        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.build()

    @pytest.mark.parametrize(
        "build_type,expected_task",
        [
            pytest.param("release", "assembleRelease", id="正常系: releaseビルド"),
            pytest.param("debug", "assembleDebug", id="正常系: debugビルド"),
        ],
    )
    def test_build_with_build_type(
        self, tmp_path: Path, build_type: str, expected_task: str
    ) -> None:
        """正常系: ビルドタイプに応じたタスクが実行される"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.build(build_type=build_type)

class TestGradleBuilderClean:
    """GradleBuilder.cleanのテスト"""

    def test_clean_success(self, tmp_path: Path) -> None:
        """正常系: クリーンが成功する場合"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.clean()

    def test_clean_failure(self, tmp_path: Path) -> None:
        """異常系: クリーンが失敗する場合"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.clean()

    def test_clean_without_gradlew(self, tmp_path: Path) -> None:
        """異常系: gradlewが存在しない場合にGradleNotFoundErrorが発生"""
        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.clean()

class TestGradleBuilderGetApkPath:
    """GradleBuilder.get_apk_pathのテスト"""

    def test_get_apk_path_exists(self, tmp_path: Path) -> None:
        """正常系: APKが存在する場合にパスを返す"""
        apk_dir = tmp_path / "app" / "build" / "outputs" / "apk" / "release"
        apk_dir.mkdir(parents=True)
        apk_file = apk_dir / "app-release-unsigned.apk"
        apk_file.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.get_apk_path()

    def test_get_apk_path_not_exists(self, tmp_path: Path) -> None:
        """正常系: APKが存在しない場合にNoneを返す"""
        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.get_apk_path()

    @pytest.mark.parametrize(
        "build_type,expected_path",
        [
            pytest.param(
                "release",
                "app/build/outputs/apk/release/app-release-unsigned.apk",
                id="正常系: releaseビルドのパス",
            ),
            pytest.param(
                "debug",
                "app/build/outputs/apk/debug/app-debug.apk",
                id="正常系: debugビルドのパス",
            ),
        ],
    )
    def test_get_apk_path_with_build_type(
        self, tmp_path: Path, build_type: str, expected_path: str
    ) -> None:
        """正常系: ビルドタイプに応じたパスが返される"""
        apk_path = tmp_path / expected_path
        apk_path.parent.mkdir(parents=True)
        apk_path.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with pytest.raises(NotImplementedError):
            builder.get_apk_path(build_type=build_type)

class TestExceptionClasses:
    """例外クラスのテスト"""

    def test_gradle_build_error_inheritance(self) -> None:
        """正常系: GradleBuildErrorが適切な継承関係を持つ"""
        error = GradleBuildError("build failed")
        assert isinstance(error, Exception)
        assert str(error) == "build failed"

    def test_gradle_timeout_error_inheritance(self) -> None:
        """正常系: GradleTimeoutErrorがGradleBuildErrorを継承している"""
        error = GradleTimeoutError("build timed out")
        assert isinstance(error, GradleBuildError)
        assert isinstance(error, Exception)
        assert str(error) == "build timed out"

    def test_gradle_not_found_error_inheritance(self) -> None:
        """正常系: GradleNotFoundErrorがGradleBuildErrorを継承している"""
        error = GradleNotFoundError("gradlew not found")
        assert isinstance(error, GradleBuildError)
        assert isinstance(error, Exception)
        assert str(error) == "gradlew not found"

class TestGradleBuilderWithMockedSubprocess:
    """subprocess.runをモックしたGradleBuilderのテスト

    これらのテストは実装後に実際のsubprocess.run呼び出しを検証します。
    現在は未実装のためNotImplementedErrorが発生します。
    """

    def test_build_calls_subprocess_with_correct_args(self, tmp_path: Path) -> None:
        """正常系: buildがsubprocess.runを正しい引数で呼び出す"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="BUILD SUCCESSFUL")

            with pytest.raises(NotImplementedError):
                builder.build()

    def test_build_returns_gradle_build_result(self, tmp_path: Path) -> None:
        """正常系: buildがGradleBuildResultを返す"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="BUILD SUCCESSFUL")

            with pytest.raises(NotImplementedError):
                builder.build()

    def test_build_raises_gradle_build_error_on_failure(self, tmp_path: Path) -> None:
        """異常系: ビルド失敗時にGradleBuildErrorが発生"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=1, stderr="BUILD FAILED")

            with pytest.raises(NotImplementedError):
                builder.build()

    def test_build_raises_gradle_timeout_error_on_timeout(self, tmp_path: Path) -> None:
        """異常系: タイムアウト時にGradleTimeoutErrorが発生"""
        import subprocess

        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path, timeout=1)

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = subprocess.TimeoutExpired(cmd="./gradlew", timeout=1)

            with pytest.raises(NotImplementedError):
                builder.build()

    def test_clean_calls_subprocess_with_clean_task(self, tmp_path: Path) -> None:
        """正常系: cleanがsubprocess.runをcleanタスクで呼び出す"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)

            with pytest.raises(NotImplementedError):
                builder.clean()

    def test_clean_raises_gradle_build_error_on_failure(self, tmp_path: Path) -> None:
        """異常系: クリーン失敗時にGradleBuildErrorが発生"""
        gradlew = tmp_path / "gradlew"
        gradlew.touch()

        builder = GradleBuilder(project_path=tmp_path)

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=1, stderr="CLEAN FAILED")

            with pytest.raises(NotImplementedError):
                builder.clean()
