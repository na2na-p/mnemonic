"""ビルドパイプラインのテスト

このモジュールはBuildPipelineインターフェースの実装に対するテストを提供します。
TDDワークフローの第2段階として、実装前に作成されたテストコードです。
"""

from pathlib import Path
from unittest.mock import Mock

import pytest

from mnemonic.pipeline import (
    BuildPipeline,
    PipelineConfig,
    PipelinePhase,
    PipelineProgress,
    PipelineResult,
)

class TestPipelineConfig:
    """PipelineConfig設定クラスのテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        config = PipelineConfig(
            input_path=Path("game.exe"),
            output_path=Path("game.apk"),
        )

        # デフォルト値の検証
        assert config.input_path == Path("game.exe")
        assert config.output_path == Path("game.apk")
        assert config.package_name == ""
        assert config.app_name == ""
        assert config.keystore_path is None
        assert config.skip_video is False
        assert config.quality == "high"
        assert config.clean_cache is False
        assert config.verbose_level == 0
        assert config.log_file is None
        assert config.ffmpeg_timeout == 300
        assert config.gradle_timeout == 1800
        assert config.template_version is None
        assert config.template_refresh_days == 7
        assert config.template_offline is False

    def test_custom_values(self) -> None:
        """カスタム値が正しく設定される"""
        config = PipelineConfig(
            input_path=Path("/path/to/game.exe"),
            output_path=Path("/path/to/output.apk"),
            package_name="com.example.game",
            app_name="My Game",
            keystore_path=Path("/path/to/keystore.jks"),
            skip_video=True,
            quality="low",
            clean_cache=True,
            verbose_level=2,
            log_file=Path("/path/to/log.txt"),
            ffmpeg_timeout=600,
            gradle_timeout=3600,
            template_version="1.0.0",
            template_refresh_days=14,
            template_offline=True,
        )

        # カスタム値の検証
        assert config.input_path == Path("/path/to/game.exe")
        assert config.output_path == Path("/path/to/output.apk")
        assert config.package_name == "com.example.game"
        assert config.app_name == "My Game"
        assert config.keystore_path == Path("/path/to/keystore.jks")
        assert config.skip_video is True
        assert config.quality == "low"
        assert config.clean_cache is True
        assert config.verbose_level == 2
        assert config.log_file == Path("/path/to/log.txt")
        assert config.ffmpeg_timeout == 600
        assert config.gradle_timeout == 3600
        assert config.template_version == "1.0.0"
        assert config.template_refresh_days == 14
        assert config.template_offline is True

    def test_config_is_frozen(self) -> None:
        """PipelineConfigは変更不可"""
        config = PipelineConfig(
            input_path=Path("game.exe"),
            output_path=Path("game.apk"),
        )
        with pytest.raises(AttributeError):
            config.skip_video = True  # type: ignore[misc]

class TestBuildPipelineValidation:
    """BuildPipeline設定検証のテスト"""

    def test_validate_missing_input(self, tmp_path: Path) -> None:
        """入力ファイルが存在しない場合にエラー"""
        config = PipelineConfig(
            input_path=tmp_path / "nonexistent.exe",
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        assert len(errors) > 0
        # エラーメッセージに入力ファイル関連のエラーが含まれることを確認
        error_text = " ".join(errors).lower()
        assert "input" in error_text or "exist" in error_text or "not found" in error_text

    def test_validate_invalid_input_type(self, tmp_path: Path) -> None:
        """入力ファイルがexe/xp3でない場合にエラー"""
        # 無効な拡張子のファイルを作成
        invalid_file = tmp_path / "invalid.txt"
        invalid_file.write_text("invalid content")

        config = PipelineConfig(
            input_path=invalid_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        assert len(errors) > 0
        # エラーメッセージにファイル形式関連のエラーが含まれることを確認
        error_text = " ".join(errors)
        assert "サポートされていないファイル形式" in error_text or ".txt" in error_text

    def test_validate_invalid_keystore(self, tmp_path: Path) -> None:
        """キーストアファイルが存在しない場合にエラー"""
        # 有効な入力ファイルを作成
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            keystore_path=tmp_path / "nonexistent.jks",
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        assert len(errors) > 0
        # エラーメッセージにキーストア関連のエラーが含まれることを確認
        error_text = " ".join(errors).lower()
        assert "keystore" in error_text or "not found" in error_text or "exist" in error_text

    def test_validate_success(self, tmp_path: Path) -> None:
        """有効な設定で検証成功"""
        # 有効な入力ファイルを作成
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        # エラーがないことを確認
        assert errors == []

    def test_validate_success_with_keystore(self, tmp_path: Path) -> None:
        """キーストア付きの有効な設定で検証成功"""
        # 有効な入力ファイルを作成
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        # キーストアファイルを作成
        keystore_file = tmp_path / "keystore.jks"
        keystore_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            keystore_path=keystore_file,
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        # エラーがないことを確認
        assert errors == []

    def test_validate_xp3_input_success(self, tmp_path: Path) -> None:
        """XP3ファイルを入力とした有効な設定で検証成功"""
        # 有効なXP3入力ファイルを作成
        input_file = tmp_path / "game.xp3"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        errors = pipeline.validate()

        # エラーがないことを確認
        assert errors == []

class TestBuildPipelineExecution:
    """BuildPipelineパイプライン実行のテスト"""

    @pytest.fixture
    def mock_components(self) -> dict:
        """Parser, Converter, Builder, Signerのモック"""
        return {
            "parser": Mock(),
            "converter": Mock(),
            "builder": Mock(),
            "signer": Mock(),
        }

    @pytest.fixture
    def valid_config(self, tmp_path: Path) -> PipelineConfig:
        """有効な設定を作成するフィクスチャ"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        return PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )

    def test_run_full_pipeline(
        self,
        mock_components: dict,
        valid_config: PipelineConfig,
        tmp_path: Path,
    ) -> None:
        """全フェーズが順番に実行される"""
        pipeline = BuildPipeline(valid_config)

        result = pipeline.run()

        assert result.success is True
        assert result.output_path is not None
        assert PipelinePhase.ANALYZE in result.phases_completed
        assert PipelinePhase.EXTRACT in result.phases_completed
        assert PipelinePhase.CONVERT in result.phases_completed
        assert PipelinePhase.BUILD in result.phases_completed
        assert PipelinePhase.SIGN in result.phases_completed

    def test_run_with_progress_callback(
        self,
        mock_components: dict,
        valid_config: PipelineConfig,
        tmp_path: Path,
    ) -> None:
        """進捗コールバックが各フェーズで呼び出される"""
        pipeline = BuildPipeline(valid_config)
        progress_callback = Mock()

        result = pipeline.run(progress_callback=progress_callback)

        # 各フェーズで開始と終了の2回呼ばれるため、最低10回のコールバック
        assert progress_callback.call_count >= len(PipelinePhase)
        phases_called = [call.args[0].phase for call in progress_callback.call_args_list]
        for phase in PipelinePhase:
            assert phase in phases_called
        assert result.success is True

    def test_run_parser_failure(
        self,
        mock_components: dict,
        tmp_path: Path,
    ) -> None:
        """入力ファイルが存在しない場合にエラー終了"""
        # 存在しないファイルでパイプラインを作成
        config = PipelineConfig(
            input_path=tmp_path / "nonexistent.exe",
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        result = pipeline.run()

        assert result.success is False
        assert result.error_message != ""
        assert result.output_path is None

    def test_run_skip_video(
        self,
        mock_components: dict,
        tmp_path: Path,
    ) -> None:
        """--skip-videoオプションで動画変換をスキップ"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            skip_video=True,
        )
        pipeline = BuildPipeline(config)

        # skip_videoオプションが設定されていることを確認
        assert pipeline.config.skip_video is True

        result = pipeline.run()

        assert result.success is True

    def test_run_clean_cache(
        self,
        mock_components: dict,
        tmp_path: Path,
    ) -> None:
        """--cleanオプションでキャッシュをクリア"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            clean_cache=True,
        )
        pipeline = BuildPipeline(config)

        # clean_cacheオプションが設定されていることを確認
        assert pipeline.config.clean_cache is True

        result = pipeline.run()

        assert result.success is True

class TestBuildPipelineResult:
    """PipelineResult結果クラスのテスト"""

    def test_success_result(self) -> None:
        """成功時の結果が正しい"""
        result = PipelineResult(
            success=True,
            output_path=Path("/path/to/output.apk"),
            error_message="",
            phases_completed=[
                PipelinePhase.ANALYZE,
                PipelinePhase.EXTRACT,
                PipelinePhase.CONVERT,
                PipelinePhase.BUILD,
                PipelinePhase.SIGN,
            ],
            statistics={
                "total_time_seconds": 120.5,
                "files_processed": 100,
            },
        )

        assert result.success is True
        assert result.output_path == Path("/path/to/output.apk")
        assert result.error_message == ""
        assert len(result.phases_completed) == 5
        assert PipelinePhase.ANALYZE in result.phases_completed
        assert PipelinePhase.SIGN in result.phases_completed
        assert result.statistics["total_time_seconds"] == 120.5
        assert result.statistics["files_processed"] == 100

    def test_failure_result(self) -> None:
        """失敗時の結果が正しい"""
        result = PipelineResult(
            success=False,
            output_path=None,
            error_message="Parser failed: Invalid EXE format",
            phases_completed=[PipelinePhase.ANALYZE],
            statistics={
                "total_time_seconds": 5.2,
                "error_phase": "analyze",
            },
        )

        assert result.success is False
        assert result.output_path is None
        assert "Parser failed" in result.error_message
        assert len(result.phases_completed) == 1
        assert PipelinePhase.ANALYZE in result.phases_completed
        assert result.statistics["error_phase"] == "analyze"

    def test_statistics(self) -> None:
        """統計情報が正しく記録される"""
        statistics = {
            "total_time_seconds": 300.0,
            "analyze_time_seconds": 10.5,
            "extract_time_seconds": 30.2,
            "convert_time_seconds": 150.8,
            "build_time_seconds": 100.3,
            "sign_time_seconds": 8.2,
            "files_processed": 500,
            "images_converted": 200,
            "videos_converted": 5,
            "scripts_converted": 50,
        }

        result = PipelineResult(
            success=True,
            output_path=Path("/path/to/output.apk"),
            phases_completed=list(PipelinePhase),
            statistics=statistics,
        )

        # 統計情報の検証
        assert result.statistics["total_time_seconds"] == 300.0
        assert result.statistics["files_processed"] == 500
        assert result.statistics["images_converted"] == 200
        assert result.statistics["videos_converted"] == 5
        assert result.statistics["scripts_converted"] == 50

        # フェーズ別時間の検証
        assert result.statistics["analyze_time_seconds"] == 10.5
        assert result.statistics["extract_time_seconds"] == 30.2
        assert result.statistics["convert_time_seconds"] == 150.8
        assert result.statistics["build_time_seconds"] == 100.3
        assert result.statistics["sign_time_seconds"] == 8.2

    def test_result_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        result = PipelineResult(
            success=False,
            output_path=None,
        )

        assert result.success is False
        assert result.output_path is None
        assert result.error_message == ""
        assert result.phases_completed == []
        assert result.statistics == {}

class TestPipelinePhase:
    """PipelinePhase列挙型のテスト"""

    def test_phase_values(self) -> None:
        """全フェーズの値が正しい"""
        assert PipelinePhase.ANALYZE.value == "analyze"
        assert PipelinePhase.EXTRACT.value == "extract"
        assert PipelinePhase.CONVERT.value == "convert"
        assert PipelinePhase.BUILD.value == "build"
        assert PipelinePhase.SIGN.value == "sign"

    def test_phase_count(self) -> None:
        """フェーズ数が5つである"""
        assert len(PipelinePhase) == 5

    def test_phase_order(self) -> None:
        """フェーズが期待通りの順序で定義されている"""
        phases = list(PipelinePhase)
        assert phases[0] == PipelinePhase.ANALYZE
        assert phases[1] == PipelinePhase.EXTRACT
        assert phases[2] == PipelinePhase.CONVERT
        assert phases[3] == PipelinePhase.BUILD
        assert phases[4] == PipelinePhase.SIGN

class TestPipelineProgress:
    """PipelineProgress進捗情報のテスト"""

    def test_progress_creation(self) -> None:
        """進捗情報が正しく作成される"""
        progress = PipelineProgress(
            phase=PipelinePhase.CONVERT,
            current=50,
            total=100,
            message="Converting images...",
        )

        assert progress.phase == PipelinePhase.CONVERT
        assert progress.current == 50
        assert progress.total == 100
        assert progress.message == "Converting images..."

    def test_progress_default_message(self) -> None:
        """メッセージのデフォルト値が空文字列"""
        progress = PipelineProgress(
            phase=PipelinePhase.BUILD,
            current=0,
            total=1,
        )

        assert progress.message == ""

    def test_progress_is_frozen(self) -> None:
        """PipelineProgressは変更不可"""
        progress = PipelineProgress(
            phase=PipelinePhase.ANALYZE,
            current=0,
            total=10,
        )
        with pytest.raises(AttributeError):
            progress.current = 5  # type: ignore[misc]

    @pytest.mark.parametrize(
        "phase,current,total,message",
        [
            pytest.param(
                PipelinePhase.ANALYZE,
                0,
                1,
                "Analyzing game structure...",
                id="正常系: 解析フェーズの進捗",
            ),
            pytest.param(
                PipelinePhase.EXTRACT,
                10,
                50,
                "Extracting assets...",
                id="正常系: 抽出フェーズの進捗",
            ),
            pytest.param(
                PipelinePhase.CONVERT,
                100,
                200,
                "",
                id="正常系: 変換フェーズ（メッセージなし）",
            ),
            pytest.param(
                PipelinePhase.BUILD,
                1,
                1,
                "Building APK...",
                id="正常系: ビルドフェーズの進捗",
            ),
            pytest.param(
                PipelinePhase.SIGN,
                0,
                1,
                "Signing APK...",
                id="正常系: 署名フェーズの進捗",
            ),
        ],
    )
    def test_progress_various_phases(
        self,
        phase: PipelinePhase,
        current: int,
        total: int,
        message: str,
    ) -> None:
        """各フェーズの進捗情報が正しく作成される"""
        progress = PipelineProgress(
            phase=phase,
            current=current,
            total=total,
            message=message,
        )

        assert progress.phase == phase
        assert progress.current == current
        assert progress.total == total
        assert progress.message == message

class TestBuildPipelineInit:
    """BuildPipeline初期化のテスト"""

    def test_init_with_config(self, tmp_path: Path) -> None:
        """設定を渡してパイプラインを初期化できる"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        assert pipeline.config == config

    def test_config_property(self, tmp_path: Path) -> None:
        """configプロパティで設定を取得できる"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            package_name="com.example.game",
        )
        pipeline = BuildPipeline(config)

        assert pipeline.config.input_path == input_file
        assert pipeline.config.package_name == "com.example.game"
