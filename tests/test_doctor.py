"""依存ツールチェッカーのテスト"""

import pytest

from mnemonic.doctor import (
    DEPENDENCIES,
    CheckResult,
    DependencyChecker,
    DependencyInfo,
    check_all_dependencies,
    check_dependency,
)


class TestCheckResult:
    """CheckResult データクラスのテスト"""

    @pytest.mark.parametrize(
        "name,required,found,version,message",
        [
            pytest.param(
                "Python",
                True,
                True,
                "3.12.0",
                None,
                id="正常系: バージョン有り・メッセージ無し",
            ),
            pytest.param(
                "FFmpeg",
                True,
                False,
                None,
                "コマンドが見つかりません",
                id="正常系: バージョン無し・メッセージ有り",
            ),
            pytest.param(
                "OptionalTool",
                False,
                True,
                "1.0.0",
                "オプションツール",
                id="正常系: オプショナルツール",
            ),
        ],
    )
    def test_check_result_creation(
        self,
        name: str,
        required: bool,
        found: bool,
        version: str | None,
        message: str | None,
    ) -> None:
        """CheckResultが正しく生成される"""
        result = CheckResult(
            name=name,
            required=required,
            found=found,
            version=version,
            message=message,
        )

        assert result.name == name
        assert result.required == required
        assert result.found == found
        assert result.version == version
        assert result.message == message

    def test_check_result_is_immutable(self) -> None:
        """CheckResultはイミュータブル"""
        result = CheckResult(
            name="Test",
            required=True,
            found=True,
            version="1.0",
            message=None,
        )

        with pytest.raises(AttributeError):
            result.name = "Modified"  # type: ignore[misc]


class TestDependencyInfo:
    """DependencyInfo データクラスのテスト"""

    @pytest.mark.parametrize(
        "name,command,version_flag,required,min_version",
        [
            pytest.param(
                "Python",
                "python",
                "--version",
                True,
                "3.12",
                id="正常系: 最小バージョン指定有り",
            ),
            pytest.param(
                "FFmpeg",
                "ffmpeg",
                "-version",
                True,
                None,
                id="正常系: 最小バージョン指定無し",
            ),
            pytest.param(
                "OptionalTool",
                "opt-tool",
                "--version",
                False,
                "1.0",
                id="正常系: オプショナルツール",
            ),
        ],
    )
    def test_dependency_info_creation(
        self,
        name: str,
        command: str,
        version_flag: str,
        required: bool,
        min_version: str | None,
    ) -> None:
        """DependencyInfoが正しく生成される"""
        info = DependencyInfo(
            name=name,
            command=command,
            version_flag=version_flag,
            required=required,
            min_version=min_version,
        )

        assert info.name == name
        assert info.command == command
        assert info.version_flag == version_flag
        assert info.required == required
        assert info.min_version == min_version

    def test_dependency_info_default_min_version(self) -> None:
        """min_versionのデフォルト値はNone"""
        info = DependencyInfo(
            name="Test",
            command="test",
            version_flag="--version",
            required=True,
        )

        assert info.min_version is None

    def test_dependency_info_is_immutable(self) -> None:
        """DependencyInfoはイミュータブル"""
        info = DependencyInfo(
            name="Test",
            command="test",
            version_flag="--version",
            required=True,
        )

        with pytest.raises(AttributeError):
            info.name = "Modified"  # type: ignore[misc]


class TestDependencies:
    """DEPENDENCIESリストのテスト"""

    def test_dependencies_is_not_empty(self) -> None:
        """DEPENDENCIESリストは空ではない"""
        assert len(DEPENDENCIES) > 0

    def test_dependencies_count(self) -> None:
        """DEPENDENCIESは5つの依存ツールを含む"""
        assert len(DEPENDENCIES) == 5

    @pytest.mark.parametrize(
        "expected_name,expected_command,expected_required",
        [
            pytest.param("Python", "python", True, id="Python"),
            pytest.param("Java JDK", "java", True, id="Java JDK"),
            pytest.param("Android SDK", "sdkmanager", True, id="Android SDK"),
            pytest.param("Android NDK", "ndk-build", True, id="Android NDK"),
            pytest.param("FFmpeg", "ffmpeg", True, id="FFmpeg"),
        ],
    )
    def test_dependencies_contains_required_tools(
        self,
        expected_name: str,
        expected_command: str,
        expected_required: bool,
    ) -> None:
        """DEPENDENCIESに必要なツールが含まれている"""
        matching = [d for d in DEPENDENCIES if d.name == expected_name]
        assert len(matching) == 1

        info = matching[0]
        assert info.command == expected_command
        assert info.required == expected_required

    def test_all_dependencies_have_version_flag(self) -> None:
        """全ての依存ツールにversion_flagが設定されている"""
        for dep in DEPENDENCIES:
            assert dep.version_flag is not None
            assert len(dep.version_flag) > 0

    def test_all_dependencies_are_dependency_info(self) -> None:
        """DEPENDENCIESの全要素がDependencyInfo型"""
        for dep in DEPENDENCIES:
            assert isinstance(dep, DependencyInfo)


class TestDependencyCheckerProtocol:
    """DependencyChecker Protocolのテスト"""

    def test_protocol_has_check_all_method(self) -> None:
        """DependencyCheckerにcheck_allメソッドが定義されている"""
        assert hasattr(DependencyChecker, "check_all")

    def test_protocol_has_check_one_method(self) -> None:
        """DependencyCheckerにcheck_oneメソッドが定義されている"""
        assert hasattr(DependencyChecker, "check_one")


class TestCheckDependency:
    """check_dependency関数のテスト"""

    def test_check_dependency_is_callable(self) -> None:
        """check_dependencyは呼び出し可能"""
        assert callable(check_dependency)

    def test_check_dependency_returns_check_result(self) -> None:
        """check_dependencyはCheckResultを返す"""
        test_info = DependencyInfo(
            name="Python",
            command="python",
            version_flag="--version",
            required=True,
        )

        result = check_dependency(test_info)

        assert isinstance(result, CheckResult)
        assert result.name == "Python"

    @pytest.mark.parametrize(
        "command,version_flag,expected_found",
        [
            pytest.param(
                "python",
                "--version",
                True,
                id="正常系: Pythonが見つかる",
            ),
            pytest.param(
                "nonexistent_command_xyz123",
                "--version",
                False,
                id="異常系: 存在しないコマンド",
            ),
        ],
    )
    def test_check_dependency_found_status(
        self,
        command: str,
        version_flag: str,
        expected_found: bool,
    ) -> None:
        """コマンドの存在有無が正しく検出される"""
        test_info = DependencyInfo(
            name="Test",
            command=command,
            version_flag=version_flag,
            required=True,
        )

        result = check_dependency(test_info)

        assert result.found == expected_found

    def test_check_dependency_extracts_version(self) -> None:
        """バージョン情報が抽出される"""
        test_info = DependencyInfo(
            name="Python",
            command="python",
            version_flag="--version",
            required=True,
        )

        result = check_dependency(test_info)

        assert result.found is True
        assert result.version is not None
        assert len(result.version) > 0

    def test_check_dependency_not_found_has_message(self) -> None:
        """コマンドが見つからない場合はメッセージが設定される"""
        test_info = DependencyInfo(
            name="NotFound",
            command="nonexistent_command_xyz123",
            version_flag="--version",
            required=True,
        )

        result = check_dependency(test_info)

        assert result.found is False
        assert result.message is not None


class TestCheckAllDependencies:
    """check_all_dependencies関数のテスト"""

    def test_check_all_dependencies_is_callable(self) -> None:
        """check_all_dependenciesは呼び出し可能"""
        assert callable(check_all_dependencies)

    def test_check_all_dependencies_returns_list(self) -> None:
        """check_all_dependenciesはリストを返す"""
        results = check_all_dependencies()

        assert isinstance(results, list)
        assert len(results) == len(DEPENDENCIES)

    def test_check_all_dependencies_contains_check_results(self) -> None:
        """check_all_dependenciesの結果は全てCheckResult型"""
        results = check_all_dependencies()

        for result in results:
            assert isinstance(result, CheckResult)

    def test_check_all_dependencies_covers_all_dependencies(self) -> None:
        """check_all_dependenciesは全てのDEPENDENCIESをチェックする"""
        results = check_all_dependencies()

        result_names = {r.name for r in results}
        expected_names = {d.name for d in DEPENDENCIES}

        assert result_names == expected_names
