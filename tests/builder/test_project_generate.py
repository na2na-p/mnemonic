"""プロジェクト生成機能のテスト"""

import zipfile
from pathlib import Path

import pytest

from mnemonic.builder.template import (
    InvalidTemplateError,
    ProjectConfig,
    ProjectGenerationError,
    ProjectGenerator,
)

class TestProjectConfig:
    """ProjectConfig初期化のテスト"""

    def test_init_with_valid_values(self) -> None:
        """正常系: 有効な値で初期化"""
        config = ProjectConfig(
            package_name="com.example.game",
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        assert config.package_name == "com.example.game"
        assert config.app_name == "My Game"
        assert config.version_code == 1
        assert config.version_name == "1.0.0"

    def test_config_is_immutable(self) -> None:
        """正常系: 設定は不変オブジェクト"""
        config = ProjectConfig(
            package_name="com.example.game",
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        with pytest.raises(AttributeError):
            config.package_name = "com.other.game"  # type: ignore[misc]

    @pytest.mark.parametrize(
        "package_name,app_name,version_code,version_name",
        [
            pytest.param(
                "com.krkr.gamename",
                "Test Game",
                1,
                "1.0.0",
                id="正常系: 標準的なパッケージ名",
            ),
            pytest.param(
                "jp.example.app.game",
                "Long Package Name Game",
                100,
                "2.5.3",
                id="正常系: 長いパッケージ名",
            ),
            pytest.param(
                "com.game",
                "Short",
                999,
                "10.20.30",
                id="正常系: 短いパッケージ名",
            ),
        ],
    )
    def test_init_with_various_valid_values(
        self,
        package_name: str,
        app_name: str,
        version_code: int,
        version_name: str,
    ) -> None:
        """正常系: 様々な有効な値で初期化できる"""
        config = ProjectConfig(
            package_name=package_name,
            app_name=app_name,
            version_code=version_code,
            version_name=version_name,
        )

        assert config.package_name == package_name
        assert config.app_name == app_name
        assert config.version_code == version_code
        assert config.version_name == version_name

class TestProjectGeneratorInit:
    """ProjectGenerator初期化のテスト"""

    def test_init_with_template_path(self, tmp_path: Path) -> None:
        """正常系: テンプレートパスで初期化"""
        template_path = tmp_path / "template.zip"
        template_path.write_bytes(b"test")

        generator = ProjectGenerator(template_path=template_path)

        assert generator._template_path == template_path

class TestProjectGeneratorValidateTemplate:
    """ProjectGenerator.validate_templateのテスト"""

    def test_validate_template_returns_true_for_valid_template(self, tmp_path: Path) -> None:
        """正常系: 有効なテンプレートでTrueを返す"""
        template_path = tmp_path / "template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr("app/build.gradle", "android { }")
            zf.writestr("app/src/main/AndroidManifest.xml", "<manifest />")
            zf.writestr("settings.gradle", "include ':app'")
            zf.writestr("build.gradle", "buildscript { }")

        generator = ProjectGenerator(template_path=template_path)

        result = generator.validate_template()
        assert result is True

    def test_validate_template_raises_error_for_missing_files(self, tmp_path: Path) -> None:
        """異常系: 必要なファイルが欠けている場合にInvalidTemplateErrorを返す"""
        template_path = tmp_path / "incomplete_template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr("README.md", "incomplete template")

        generator = ProjectGenerator(template_path=template_path)

        with pytest.raises(InvalidTemplateError):
            generator.validate_template()

    @pytest.mark.parametrize(
        "files_to_include,expected_valid",
        [
            pytest.param(
                [
                    "app/build.gradle",
                    "app/src/main/AndroidManifest.xml",
                    "settings.gradle",
                    "build.gradle",
                ],
                True,
                id="正常系: 全ての必須ファイルが存在",
            ),
            pytest.param(
                ["app/build.gradle", "settings.gradle"],
                False,
                id="異常系: AndroidManifest.xmlが欠落",
            ),
            pytest.param(
                ["app/src/main/AndroidManifest.xml"],
                False,
                id="異常系: build.gradleが欠落",
            ),
            pytest.param(
                [],
                False,
                id="異常系: 空のテンプレート",
            ),
        ],
    )
    def test_validate_template_checks_required_files(
        self,
        tmp_path: Path,
        files_to_include: list[str],
        expected_valid: bool,
    ) -> None:
        """正常系/異常系: 必須ファイルの存在確認"""
        template_path = tmp_path / "template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            for file_path in files_to_include:
                zf.writestr(file_path, f"content of {file_path}")

        generator = ProjectGenerator(template_path=template_path)

        if expected_valid:
            result = generator.validate_template()
            assert result is True
        else:
            with pytest.raises(InvalidTemplateError):
                generator.validate_template()

class TestProjectGeneratorGenerate:
    """ProjectGenerator.generateのテスト"""

    @pytest.fixture
    def valid_template(self, tmp_path: Path) -> Path:
        """有効なテンプレートZIPを作成するフィクスチャ"""
        template_path = tmp_path / "template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr(
                "app/build.gradle",
                """android {
    namespace "com.krkrsdl2.template"
    defaultConfig {
        applicationId "com.krkrsdl2.template"
        versionCode 1
        versionName "1.0"
    }
}""",
            )
            zf.writestr(
                "app/src/main/AndroidManifest.xml",
                """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.krkrsdl2.template">
    <application
        android:label="Template App">
    </application>
</manifest>""",
            )
            zf.writestr("settings.gradle", "include ':app'")
            zf.writestr("build.gradle", "buildscript { }")
            zf.writestr("app/src/main/java/com/krkrsdl2/template/MainActivity.java", "")

        return template_path

    @pytest.fixture
    def valid_config(self) -> ProjectConfig:
        """有効なProjectConfigを作成するフィクスチャ"""
        return ProjectConfig(
            package_name="com.example.mygame",
            app_name="My Game",
            version_code=10,
            version_name="2.0.0",
        )

    def test_generate_creates_project_in_output_directory(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: 指定ディレクトリにプロジェクトが生成される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)

        result = generator.generate(output_dir=output_dir, config=valid_config)

        assert result == output_dir
        assert (output_dir / "app" / "build.gradle").exists()
        assert (output_dir / "app" / "src" / "main" / "AndroidManifest.xml").exists()
        assert (output_dir / "settings.gradle").exists()
        assert (output_dir / "build.gradle").exists()

    def test_generate_sets_package_name_correctly(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: package_nameが正しく設定される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)
        generator.generate(output_dir=output_dir, config=valid_config)

        # AndroidManifest.xmlのpackage属性を検証
        manifest_path = output_dir / "app" / "src" / "main" / "AndroidManifest.xml"
        manifest_content = manifest_path.read_text(encoding="utf-8")
        assert f'package="{valid_config.package_name}"' in manifest_content

        # build.gradleのapplicationIdとnamespaceを検証
        gradle_path = output_dir / "app" / "build.gradle"
        gradle_content = gradle_path.read_text(encoding="utf-8")
        assert f'applicationId "{valid_config.package_name}"' in gradle_content
        assert f'namespace "{valid_config.package_name}"' in gradle_content

    def test_generate_sets_app_name_correctly(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: app_nameが正しく設定される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)
        generator.generate(output_dir=output_dir, config=valid_config)

        manifest_path = output_dir / "app" / "src" / "main" / "AndroidManifest.xml"
        manifest_content = manifest_path.read_text(encoding="utf-8")
        assert f'android:label="{valid_config.app_name}"' in manifest_content

    def test_generate_sets_version_code_and_name_correctly(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: version_codeとversion_nameが正しく設定される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)
        generator.generate(output_dir=output_dir, config=valid_config)

        gradle_path = output_dir / "app" / "build.gradle"
        gradle_content = gradle_path.read_text(encoding="utf-8")
        assert f"versionCode {valid_config.version_code}" in gradle_content
        assert f'versionName "{valid_config.version_name}"' in gradle_content

    def test_generate_updates_android_manifest(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: AndroidManifest.xmlが正しく更新される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)
        generator.generate(output_dir=output_dir, config=valid_config)

        manifest_path = output_dir / "app" / "src" / "main" / "AndroidManifest.xml"
        manifest_content = manifest_path.read_text(encoding="utf-8")

        # 元のテンプレート値が置換されていることを確認
        assert "com.krkrsdl2.template" not in manifest_content
        assert "Template App" not in manifest_content
        assert valid_config.package_name in manifest_content
        assert valid_config.app_name in manifest_content

    def test_generate_updates_build_gradle(
        self,
        tmp_path: Path,
        valid_template: Path,
        valid_config: ProjectConfig,
    ) -> None:
        """正常系: build.gradleが正しく更新される"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ProjectGenerator(template_path=valid_template)
        generator.generate(output_dir=output_dir, config=valid_config)

        gradle_path = output_dir / "app" / "build.gradle"
        gradle_content = gradle_path.read_text(encoding="utf-8")

        # 元のテンプレート値が置換されていることを確認
        assert "com.krkrsdl2.template" not in gradle_content
        # versionCode が新しい値に更新されていることを確認
        assert f"versionCode {valid_config.version_code}" in gradle_content
        assert 'versionName "1.0"' not in gradle_content

    @pytest.mark.parametrize(
        "package_name,app_name,version_code,version_name",
        [
            pytest.param(
                "com.krkr.visualnovel",
                "Visual Novel",
                1,
                "1.0.0",
                id="正常系: 標準的な設定",
            ),
            pytest.param(
                "jp.example.game.adventure",
                "Adventure Game",
                50,
                "3.2.1",
                id="正常系: 長いパッケージ名",
            ),
            pytest.param(
                "com.test",
                "Test",
                999,
                "99.99.99",
                id="正常系: 大きいバージョン番号",
            ),
        ],
    )
    def test_generate_with_various_configs(
        self,
        tmp_path: Path,
        valid_template: Path,
        package_name: str,
        app_name: str,
        version_code: int,
        version_name: str,
    ) -> None:
        """正常系: 様々な設定でプロジェクトを生成できる"""
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        config = ProjectConfig(
            package_name=package_name,
            app_name=app_name,
            version_code=version_code,
            version_name=version_name,
        )
        generator = ProjectGenerator(template_path=valid_template)

        result = generator.generate(output_dir=output_dir, config=config)

        assert result == output_dir

        # AndroidManifest.xmlの検証
        manifest_path = output_dir / "app" / "src" / "main" / "AndroidManifest.xml"
        manifest_content = manifest_path.read_text(encoding="utf-8")
        assert f'package="{package_name}"' in manifest_content
        assert f'android:label="{app_name}"' in manifest_content

        # build.gradleの検証
        gradle_path = output_dir / "app" / "build.gradle"
        gradle_content = gradle_path.read_text(encoding="utf-8")
        assert f'applicationId "{package_name}"' in gradle_content
        assert f"versionCode {version_code}" in gradle_content
        assert f'versionName "{version_name}"' in gradle_content

class TestProjectGeneratorErrorCases:
    """ProjectGenerator エラーケースのテスト"""

    def test_generate_raises_error_for_invalid_template(self, tmp_path: Path) -> None:
        """異常系: 無効なテンプレートでInvalidTemplateError"""
        template_path = tmp_path / "invalid_template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr("README.md", "not a valid template")

        output_dir = tmp_path / "output"
        output_dir.mkdir()

        config = ProjectConfig(
            package_name="com.example.game",
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        generator = ProjectGenerator(template_path=template_path)

        with pytest.raises(InvalidTemplateError):
            generator.generate(output_dir=output_dir, config=config)

    def test_generate_handles_nonexistent_output_directory(self, tmp_path: Path) -> None:
        """異常系: 出力ディレクトリが存在しない場合の処理"""
        template_path = tmp_path / "template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr("app/build.gradle", "android { }")
            zf.writestr("app/src/main/AndroidManifest.xml", "<manifest />")
            zf.writestr("settings.gradle", "include ':app'")
            zf.writestr("build.gradle", "buildscript { }")

        output_dir = tmp_path / "nonexistent" / "output"

        config = ProjectConfig(
            package_name="com.example.game",
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        generator = ProjectGenerator(template_path=template_path)

        with pytest.raises(ProjectGenerationError):
            generator.generate(output_dir=output_dir, config=config)

    @pytest.mark.parametrize(
        "invalid_package_name,expected_error_type",
        [
            pytest.param(
                "",
                (ValueError, ProjectGenerationError),
                id="異常系: 空のパッケージ名",
            ),
            pytest.param(
                "invalid",
                (ValueError, ProjectGenerationError),
                id="異常系: ドットなしのパッケージ名",
            ),
            pytest.param(
                "com.123invalid",
                (ValueError, ProjectGenerationError),
                id="異常系: 数字で始まるパッケージ名",
            ),
            pytest.param(
                "com.example.class",
                (ValueError, ProjectGenerationError),
                id="異常系: 予約語を含むパッケージ名",
            ),
            pytest.param(
                "com..double.dot",
                (ValueError, ProjectGenerationError),
                id="異常系: 連続するドットを含むパッケージ名",
            ),
        ],
    )
    def test_generate_raises_error_for_invalid_package_name(
        self,
        tmp_path: Path,
        invalid_package_name: str,
        expected_error_type: tuple[type[Exception], ...],
    ) -> None:
        """異常系: 不正なpackage_nameでエラー"""
        template_path = tmp_path / "template.zip"

        with zipfile.ZipFile(template_path, "w") as zf:
            zf.writestr("app/build.gradle", "android { }")
            zf.writestr("app/src/main/AndroidManifest.xml", "<manifest />")
            zf.writestr("settings.gradle", "include ':app'")
            zf.writestr("build.gradle", "buildscript { }")

        output_dir = tmp_path / "output"
        output_dir.mkdir()

        config = ProjectConfig(
            package_name=invalid_package_name,
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        generator = ProjectGenerator(template_path=template_path)

        with pytest.raises(expected_error_type):
            generator.generate(output_dir=output_dir, config=config)

    def test_generate_raises_error_when_template_not_exists(self, tmp_path: Path) -> None:
        """異常系: テンプレートファイルが存在しない場合"""
        template_path = tmp_path / "nonexistent.zip"
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        config = ProjectConfig(
            package_name="com.example.game",
            app_name="My Game",
            version_code=1,
            version_name="1.0.0",
        )

        generator = ProjectGenerator(template_path=template_path)

        with pytest.raises(ProjectGenerationError):
            generator.generate(output_dir=output_dir, config=config)

class TestProjectGenerationExceptions:
    """プロジェクト生成例外のテスト"""

    def test_project_generation_error_inheritance(self) -> None:
        """正常系: ProjectGenerationErrorが適切な継承関係を持つ"""
        error = ProjectGenerationError("generation error")
        assert isinstance(error, Exception)
        assert str(error) == "generation error"

    def test_invalid_template_error_inheritance(self) -> None:
        """正常系: InvalidTemplateErrorがProjectGenerationErrorを継承"""
        error = InvalidTemplateError("invalid template")
        assert isinstance(error, ProjectGenerationError)
        assert isinstance(error, Exception)
        assert str(error) == "invalid template"

    def test_invalid_template_error_with_context(self) -> None:
        """正常系: InvalidTemplateErrorがコンテキスト情報を保持"""
        original_error = FileNotFoundError("file not found")
        error = InvalidTemplateError("Template validation failed")
        error.__cause__ = original_error

        assert error.__cause__ is original_error
        assert "Template validation failed" in str(error)
