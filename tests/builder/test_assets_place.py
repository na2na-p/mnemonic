"""アセット配置機能のテスト"""

from pathlib import Path

import pytest

from mnemonic.builder.template import (
    AssetConfig,
    AssetPlacementError,
    AssetPlacementResult,
    AssetPlacer,
)

class TestAssetConfig:
    """AssetConfig データクラスのテスト"""

    def test_creation_with_all_fields(self) -> None:
        """正常系: 全フィールドを指定してAssetConfigを作成"""
        config = AssetConfig(
            no_compress_extensions=[".ogg", ".mp3", ".wav"],
            exclude_patterns=["*.bak", "thumbs.db"],
        )

        assert config.no_compress_extensions == [".ogg", ".mp3", ".wav"]
        assert config.exclude_patterns == ["*.bak", "thumbs.db"]

    def test_immutability(self) -> None:
        """正常系: AssetConfigが不変であることのテスト"""
        config = AssetConfig(
            no_compress_extensions=[".ogg"],
            exclude_patterns=["*.bak"],
        )

        with pytest.raises(AttributeError):
            config.no_compress_extensions = [".mp3"]  # type: ignore[misc]

    @pytest.mark.parametrize(
        "no_compress,exclude_patterns",
        [
            pytest.param(
                [".ogg", ".mp3"],
                [],
                id="正常系: 除外パターンなし",
            ),
            pytest.param(
                [],
                ["*.bak", "*.tmp"],
                id="正常系: noCompress拡張子なし",
            ),
            pytest.param(
                [],
                [],
                id="正常系: 両方空リスト",
            ),
            pytest.param(
                [".ogg", ".mp3", ".wav", ".flac", ".opus"],
                ["*.bak", "thumbs.db", ".DS_Store", "*.tmp"],
                id="正常系: 複数の拡張子とパターン",
            ),
        ],
    )
    def test_creation_with_various_values(
        self,
        no_compress: list[str],
        exclude_patterns: list[str],
    ) -> None:
        """正常系: 様々な値でAssetConfigを作成できる"""
        config = AssetConfig(
            no_compress_extensions=no_compress,
            exclude_patterns=exclude_patterns,
        )

        assert config.no_compress_extensions == no_compress
        assert config.exclude_patterns == exclude_patterns

class TestAssetPlacementResult:
    """AssetPlacementResult データクラスのテスト"""

    def test_creation_with_all_fields(self) -> None:
        """正常系: 全フィールドを指定してAssetPlacementResultを作成"""
        placed_files = [Path("file1.txt"), Path("file2.ogg")]
        result = AssetPlacementResult(
            total_files=2,
            total_size=1024,
            placed_files=placed_files,
        )

        assert result.total_files == 2
        assert result.total_size == 1024
        assert result.placed_files == placed_files

    def test_immutability(self) -> None:
        """正常系: AssetPlacementResultが不変であることのテスト"""
        result = AssetPlacementResult(
            total_files=1,
            total_size=512,
            placed_files=[Path("file.txt")],
        )

        with pytest.raises(AttributeError):
            result.total_files = 5  # type: ignore[misc]

    @pytest.mark.parametrize(
        "total_files,total_size,placed_files_count",
        [
            pytest.param(0, 0, 0, id="正常系: ファイルなし"),
            pytest.param(1, 100, 1, id="正常系: 1ファイル"),
            pytest.param(100, 1024 * 1024, 100, id="正常系: 多数のファイル"),
        ],
    )
    def test_creation_with_various_values(
        self,
        total_files: int,
        total_size: int,
        placed_files_count: int,
    ) -> None:
        """正常系: 様々な値でAssetPlacementResultを作成できる"""
        placed_files = [Path(f"file{i}.txt") for i in range(placed_files_count)]
        result = AssetPlacementResult(
            total_files=total_files,
            total_size=total_size,
            placed_files=placed_files,
        )

        assert result.total_files == total_files
        assert result.total_size == total_size
        assert len(result.placed_files) == placed_files_count

class TestAssetPlacerInit:
    """AssetPlacer 初期化のテスト"""

    def test_init_with_project_path(self, tmp_path: Path) -> None:
        """正常系: プロジェクトパスで初期化"""
        project_path = tmp_path / "android_project"
        project_path.mkdir()

        placer = AssetPlacer(project_path=project_path)

        assert placer._project_path == project_path

class TestAssetPlacerPlaceAssets:
    """AssetPlacer.place_assets のテスト"""

    @pytest.fixture
    def android_project(self, tmp_path: Path) -> Path:
        """Androidプロジェクト構造を作成するフィクスチャ"""
        project_path = tmp_path / "android_project"
        project_path.mkdir()

        # app/src/main/assets ディレクトリ構造を作成
        assets_dir = project_path / "app" / "src" / "main" / "assets"
        assets_dir.mkdir(parents=True)

        # app/build.gradle を作成
        build_gradle = project_path / "app" / "build.gradle"
        build_gradle.write_text(
            """android {
    namespace "com.example.game"
    defaultConfig {
        applicationId "com.example.game"
    }
}""",
            encoding="utf-8",
        )

        return project_path

    @pytest.fixture
    def source_assets(self, tmp_path: Path) -> Path:
        """ソースアセットを作成するフィクスチャ"""
        source_dir = tmp_path / "source_assets"
        source_dir.mkdir()

        # テスト用アセットファイルを作成
        (source_dir / "data.xp3").write_bytes(b"xp3 archive content")
        (source_dir / "bgm.ogg").write_bytes(b"ogg audio content")
        (source_dir / "startup.tjs").write_text("// startup script", encoding="utf-8")

        # サブディレクトリも作成
        subdir = source_dir / "images"
        subdir.mkdir()
        (subdir / "title.png").write_bytes(b"png image content")

        return source_dir

    def test_place_assets_copies_all_files_to_assets_directory(
        self,
        android_project: Path,
        source_assets: Path,
    ) -> None:
        """正常系: 全てのアセットが app/src/main/assets/ に配置される"""
        placer = AssetPlacer(project_path=android_project)

        with pytest.raises(NotImplementedError):
            placer.place_assets(source_dir=source_assets)

    def test_place_assets_preserves_directory_structure(
        self,
        android_project: Path,
        source_assets: Path,
    ) -> None:
        """正常系: ディレクトリ構造が保持される"""
        placer = AssetPlacer(project_path=android_project)

        with pytest.raises(NotImplementedError):
            placer.place_assets(source_dir=source_assets)

    def test_place_assets_returns_correct_result(
        self,
        android_project: Path,
        source_assets: Path,
    ) -> None:
        """正常系: 配置結果（ファイル数、サイズ）が正しく返される"""
        placer = AssetPlacer(project_path=android_project)

        with pytest.raises(NotImplementedError):
            placer.place_assets(source_dir=source_assets)

    @pytest.mark.parametrize(
        "exclude_patterns,expected_excluded",
        [
            pytest.param(
                ["*.bak"],
                ["backup.bak"],
                id="正常系: bakファイルを除外",
            ),
            pytest.param(
                ["thumbs.db", ".DS_Store"],
                ["thumbs.db", ".DS_Store"],
                id="正常系: システムファイルを除外",
            ),
            pytest.param(
                ["*.tmp", "*.bak"],
                ["temp.tmp", "backup.bak"],
                id="正常系: 複数のパターンを除外",
            ),
        ],
    )
    def test_place_assets_excludes_matching_patterns(
        self,
        tmp_path: Path,
        android_project: Path,
        exclude_patterns: list[str],
        expected_excluded: list[str],
    ) -> None:
        """正常系: 除外パターンに一致するファイルは配置されない"""
        # ソースアセットを作成
        source_dir = tmp_path / "source_with_excludes"
        source_dir.mkdir()
        (source_dir / "data.xp3").write_bytes(b"data")

        # 除外対象ファイルを作成
        for excluded_file in expected_excluded:
            (source_dir / excluded_file).write_bytes(b"excluded")

        placer = AssetPlacer(project_path=android_project)

        with pytest.raises(NotImplementedError):
            placer.place_assets(source_dir=source_dir)

class TestAssetPlacerConfigureBuildGradle:
    """AssetPlacer.configure_build_gradle のテスト"""

    @pytest.fixture
    def groovy_gradle_project(self, tmp_path: Path) -> Path:
        """Groovy形式のbuild.gradleを持つプロジェクトを作成"""
        project_path = tmp_path / "groovy_project"
        project_path.mkdir()

        build_gradle = project_path / "app" / "build.gradle"
        build_gradle.parent.mkdir(parents=True)
        build_gradle.write_text(
            """android {
    namespace "com.example.game"
    defaultConfig {
        applicationId "com.example.game"
    }
}""",
            encoding="utf-8",
        )

        return project_path

    @pytest.fixture
    def kotlin_gradle_project(self, tmp_path: Path) -> Path:
        """Kotlin形式のbuild.gradle.ktsを持つプロジェクトを作成"""
        project_path = tmp_path / "kotlin_project"
        project_path.mkdir()

        build_gradle = project_path / "app" / "build.gradle.kts"
        build_gradle.parent.mkdir(parents=True)
        build_gradle.write_text(
            """android {
    namespace = "com.example.game"
    defaultConfig {
        applicationId = "com.example.game"
    }
}""",
            encoding="utf-8",
        )

        return project_path

    def test_configure_build_gradle_sets_no_compress_groovy(
        self,
        groovy_gradle_project: Path,
    ) -> None:
        """正常系: aaptOptions.noCompress が正しく設定される（Groovy形式）"""
        placer = AssetPlacer(project_path=groovy_gradle_project)
        config = AssetConfig(
            no_compress_extensions=[".ogg", ".mp3"],
            exclude_patterns=[],
        )

        with pytest.raises(NotImplementedError):
            placer.configure_build_gradle(asset_config=config)

    def test_configure_build_gradle_sets_no_compress_kotlin(
        self,
        kotlin_gradle_project: Path,
    ) -> None:
        """正常系: aaptOptions.noCompress が正しく設定される（Kotlin形式）"""
        placer = AssetPlacer(project_path=kotlin_gradle_project)
        config = AssetConfig(
            no_compress_extensions=[".ogg", ".mp3"],
            exclude_patterns=[],
        )

        with pytest.raises(NotImplementedError):
            placer.configure_build_gradle(asset_config=config)

    def test_configure_build_gradle_preserves_existing_settings(
        self,
        tmp_path: Path,
    ) -> None:
        """正常系: 既存の設定を壊さない"""
        project_path = tmp_path / "existing_settings_project"
        project_path.mkdir()

        build_gradle = project_path / "app" / "build.gradle"
        build_gradle.parent.mkdir(parents=True)
        build_gradle.write_text(
            """android {
    namespace "com.example.game"
    defaultConfig {
        applicationId "com.example.game"
        minSdk 21
        targetSdk 34
    }
    buildTypes {
        release {
            minifyEnabled true
        }
    }
}""",
            encoding="utf-8",
        )

        placer = AssetPlacer(project_path=project_path)
        config = AssetConfig(
            no_compress_extensions=[".ogg"],
            exclude_patterns=[],
        )

        with pytest.raises(NotImplementedError):
            placer.configure_build_gradle(asset_config=config)

    @pytest.mark.parametrize(
        "no_compress_extensions",
        [
            pytest.param(
                [".ogg"],
                id="正常系: 単一の拡張子",
            ),
            pytest.param(
                [".ogg", ".mp3", ".wav"],
                id="正常系: 複数の拡張子",
            ),
            pytest.param(
                [".ogg", ".mp3", ".wav", ".flac", ".opus", ".xp3"],
                id="正常系: 多数の拡張子",
            ),
        ],
    )
    def test_configure_build_gradle_with_various_extensions(
        self,
        groovy_gradle_project: Path,
        no_compress_extensions: list[str],
    ) -> None:
        """正常系: 様々なnoCompress拡張子で設定できる"""
        placer = AssetPlacer(project_path=groovy_gradle_project)
        config = AssetConfig(
            no_compress_extensions=no_compress_extensions,
            exclude_patterns=[],
        )

        with pytest.raises(NotImplementedError):
            placer.configure_build_gradle(asset_config=config)

class TestAssetPlacerValidatePlacement:
    """AssetPlacer.validate_placement のテスト"""

    @pytest.fixture
    def project_with_assets(self, tmp_path: Path) -> Path:
        """アセットが配置されたプロジェクトを作成"""
        project_path = tmp_path / "project_with_assets"
        project_path.mkdir()

        assets_dir = project_path / "app" / "src" / "main" / "assets"
        assets_dir.mkdir(parents=True)

        # アセットファイルを配置
        (assets_dir / "data.xp3").write_bytes(b"xp3 content")
        (assets_dir / "startup.tjs").write_text("// script", encoding="utf-8")

        return project_path

    def test_validate_placement_returns_true_when_valid(
        self,
        project_with_assets: Path,
    ) -> None:
        """正常系: 必要なファイルが全て配置されている場合にTrue"""
        placer = AssetPlacer(project_path=project_with_assets)

        with pytest.raises(NotImplementedError):
            placer.validate_placement()

    def test_validate_placement_returns_false_when_missing_files(
        self,
        tmp_path: Path,
    ) -> None:
        """正常系: ファイルが欠けている場合にFalse"""
        project_path = tmp_path / "empty_project"
        project_path.mkdir()

        # assetsディレクトリは存在するが空
        assets_dir = project_path / "app" / "src" / "main" / "assets"
        assets_dir.mkdir(parents=True)

        placer = AssetPlacer(project_path=project_path)

        with pytest.raises(NotImplementedError):
            placer.validate_placement()

class TestAssetPlacerErrorCases:
    """AssetPlacer エラーケースのテスト"""

    def test_place_assets_raises_error_when_source_not_exists(
        self,
        tmp_path: Path,
    ) -> None:
        """異常系: ソースディレクトリが存在しない場合"""
        project_path = tmp_path / "project"
        project_path.mkdir()
        (project_path / "app" / "src" / "main" / "assets").mkdir(parents=True)

        nonexistent_source = tmp_path / "nonexistent_source"

        placer = AssetPlacer(project_path=project_path)

        # 現在は NotImplementedError が発生
        # 実装後は AssetPlacementError が発生する予定
        with pytest.raises((NotImplementedError, AssetPlacementError)):
            placer.place_assets(source_dir=nonexistent_source)

    def test_configure_build_gradle_raises_error_when_gradle_not_exists(
        self,
        tmp_path: Path,
    ) -> None:
        """異常系: build.gradleが存在しない場合"""
        project_path = tmp_path / "no_gradle_project"
        project_path.mkdir()
        (project_path / "app").mkdir()
        # build.gradle を作成しない

        placer = AssetPlacer(project_path=project_path)
        config = AssetConfig(
            no_compress_extensions=[".ogg"],
            exclude_patterns=[],
        )

        with pytest.raises((NotImplementedError, AssetPlacementError)):
            placer.configure_build_gradle(asset_config=config)

    def test_place_assets_raises_error_when_project_path_invalid(
        self,
        tmp_path: Path,
    ) -> None:
        """異常系: プロジェクトパスが無効な場合"""
        invalid_project_path = tmp_path / "nonexistent_project"
        source_dir = tmp_path / "source"
        source_dir.mkdir()
        (source_dir / "data.xp3").write_bytes(b"data")

        placer = AssetPlacer(project_path=invalid_project_path)

        with pytest.raises((NotImplementedError, AssetPlacementError)):
            placer.place_assets(source_dir=source_dir)

class TestAssetPlacementExceptions:
    """AssetPlacement例外クラスのテスト"""

    def test_asset_placement_error_inheritance(self) -> None:
        """正常系: AssetPlacementErrorが適切な継承関係を持つ"""
        error = AssetPlacementError("placement failed")
        assert isinstance(error, Exception)
        assert str(error) == "placement failed"

    def test_asset_placement_error_with_cause(self) -> None:
        """正常系: AssetPlacementErrorが原因を保持できる"""
        original_error = FileNotFoundError("file not found")
        error = AssetPlacementError("Asset placement failed")
        error.__cause__ = original_error

        assert error.__cause__ is original_error
        assert "Asset placement failed" in str(error)
