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
        mocker,
    ) -> None:
        """全フェーズが順番に実行される（モック使用）"""
        pipeline = BuildPipeline(valid_config)

        # _execute_phase をモックして実際の処理をスキップ
        mocker.patch.object(pipeline, "_execute_phase")

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
        mocker,
    ) -> None:
        """進捗コールバックが各フェーズで呼び出される（モック使用）"""
        pipeline = BuildPipeline(valid_config)

        # _execute_phase をモックして実際の処理をスキップ
        mocker.patch.object(pipeline, "_execute_phase")

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
        mocker,
    ) -> None:
        """--skip-videoオプションで動画変換をスキップ（モック使用）"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            skip_video=True,
        )
        pipeline = BuildPipeline(config)

        # _execute_phase をモックして実際の処理をスキップ
        mocker.patch.object(pipeline, "_execute_phase")

        # skip_videoオプションが設定されていることを確認
        assert pipeline.config.skip_video is True

        result = pipeline.run()

        assert result.success is True

    def test_run_clean_cache(
        self,
        mock_components: dict,
        tmp_path: Path,
        mocker,
    ) -> None:
        """--cleanオプションでキャッシュをクリア（モック使用）"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
            clean_cache=True,
        )
        pipeline = BuildPipeline(config)

        # _execute_phase をモックして実際の処理をスキップ
        mocker.patch.object(pipeline, "_execute_phase")

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


class TestBuildPipelineSanitizeName:
    """BuildPipeline._sanitize_name メソッドのテスト"""

    @pytest.mark.parametrize(
        "input_name, expected",
        [
            pytest.param(
                "TRUE REMEMBRANCE",
                "true_remembrance",
                id="正常系: スペースをアンダースコアに変換",
            ),
            pytest.param(
                "true",
                "game_true",
                id="正常系: Java予約語にプレフィックス追加",
            ),
            pytest.param(
                "false",
                "game_false",
                id="正常系: Java予約語falseにプレフィックス追加",
            ),
            pytest.param(
                "null",
                "game_null",
                id="正常系: Java予約語nullにプレフィックス追加",
            ),
            pytest.param(
                "123game",
                "_123game",
                id="正常系: 数字始まりにプレフィックス追加",
            ),
            pytest.param(
                "game!@#$%",
                "game",
                id="正常系: 特殊文字を削除",
            ),
            pytest.param(
                "My Game 2",
                "my_game_2",
                id="正常系: スペースと数字の組み合わせ",
            ),
        ],
    )
    def test_sanitize_name(self, input_name: str, expected: str, tmp_path: Path) -> None:
        """_sanitize_nameが正しくパッケージ名を生成する"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        result = pipeline._sanitize_name(input_name)

        assert result == expected


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


class TestBuildPipelineFindGameIcon:
    """BuildPipeline._find_game_iconのテスト"""

    def test_find_game_icon_returns_none_when_extract_dir_is_none(self, tmp_path: Path) -> None:
        """抽出ディレクトリがNoneの場合はNoneを返す"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # _extract_dirはNoneの状態
        result = pipeline._find_game_icon()

        assert result is None

    @pytest.mark.parametrize(
        "icon_name",
        [
            pytest.param("icon.png", id="正常系: icon.pngを検出"),
            pytest.param("icon.ico", id="正常系: icon.icoを検出"),
            pytest.param("icon.bmp", id="正常系: icon.bmpを検出"),
        ],
    )
    def test_find_game_icon_returns_prioritized_icon(self, tmp_path: Path, icon_name: str) -> None:
        """優先度の高いアイコンファイルを返す"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # 抽出ディレクトリをセット
        extract_dir = tmp_path / "extract"
        extract_dir.mkdir()
        pipeline._extract_dir = extract_dir

        # アイコンファイルを作成
        icon_path = extract_dir / icon_name
        icon_path.write_bytes(b"\x89PNG\r\n\x1a\n")

        result = pipeline._find_game_icon()

        assert result == icon_path

    def test_find_game_icon_prefers_png_over_ico(self, tmp_path: Path) -> None:
        """icon.pngがicon.icoより優先される"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        extract_dir = tmp_path / "extract"
        extract_dir.mkdir()
        pipeline._extract_dir = extract_dir

        # 両方のアイコンファイルを作成
        png_path = extract_dir / "icon.png"
        png_path.write_bytes(b"\x89PNG\r\n\x1a\n")
        ico_path = extract_dir / "icon.ico"
        ico_path.write_bytes(b"\x00\x00\x01\x00")

        result = pipeline._find_game_icon()

        assert result == png_path

    def test_find_game_icon_falls_back_to_any_ico(self, tmp_path: Path) -> None:
        """優先アイコンがない場合は任意の.icoファイルを返す"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        extract_dir = tmp_path / "extract"
        extract_dir.mkdir()
        pipeline._extract_dir = extract_dir

        # 優先ファイル名ではないicoファイルを作成
        custom_ico = extract_dir / "game_icon.ico"
        custom_ico.write_bytes(b"\x00\x00\x01\x00")

        result = pipeline._find_game_icon()

        assert result == custom_ico

    def test_find_game_icon_returns_none_when_no_icon(self, tmp_path: Path) -> None:
        """アイコンファイルが存在しない場合はNoneを返す"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        extract_dir = tmp_path / "extract"
        extract_dir.mkdir()
        pipeline._extract_dir = extract_dir

        # アイコンファイルは作成しない
        (extract_dir / "data.xp3").write_bytes(b"XP3")

        result = pipeline._find_game_icon()

        assert result is None

    def test_extracts_icon_from_exe_when_no_icon_file(self, tmp_path: Path, mocker) -> None:
        """アイコンファイルがない場合はEXEからアイコン抽出を試みる"""
        # EXE入力ファイルを作成
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # 抽出ディレクトリをセット（アイコンファイルなし）
        extract_dir = tmp_path / "extract"
        extract_dir.mkdir()
        pipeline._extract_dir = extract_dir

        # ExeIconExtractorをモック
        mock_icon_extractor = mocker.patch("mnemonic.pipeline.ExeIconExtractor")
        extracted_icon = extract_dir / "extracted_icon.png"
        extracted_icon.write_bytes(b"\x89PNG\r\n\x1a\n")
        mock_icon_extractor.return_value.extract.return_value = extracted_icon

        result = pipeline._find_game_icon()

        # EXEからアイコン抽出が呼ばれたことを確認
        mock_icon_extractor.return_value.extract.assert_called_once_with(input_file, extract_dir)
        assert result == extracted_icon


class TestBuildPipelineNormalizeCriticalFilenames:
    """BuildPipeline._normalize_critical_filenames メソッドのテスト"""

    def test_normalizes_uppercase_filenames(self, tmp_path: Path) -> None:
        """大文字ファイル名を小文字に変換"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # 大文字ファイル名でファイルを作成
        (test_dir / "Data.XP3").write_bytes(b"data")
        (test_dir / "Config.TJS").write_text("config")
        (test_dir / "README.TXT").write_text("readme")

        # 正規化を実行
        pipeline._normalize_critical_filenames(test_dir)

        # 小文字に変換されていることを確認
        assert (test_dir / "data.xp3").exists()
        assert (test_dir / "config.tjs").exists()
        assert (test_dir / "readme.txt").exists()

        # 元の大文字ファイルは存在しない
        assert not (test_dir / "Data.XP3").exists()
        assert not (test_dir / "Config.TJS").exists()
        assert not (test_dir / "README.TXT").exists()

    def test_keeps_lowercase_filenames_unchanged(self, tmp_path: Path) -> None:
        """小文字ファイル名はそのまま"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # 小文字ファイル名でファイルを作成
        (test_dir / "data.xp3").write_bytes(b"data")
        (test_dir / "config.tjs").write_text("config")

        # 正規化を実行
        pipeline._normalize_critical_filenames(test_dir)

        # ファイルがそのまま存在することを確認
        assert (test_dir / "data.xp3").exists()
        assert (test_dir / "config.tjs").exists()

    def test_normalizes_nested_directory_files(self, tmp_path: Path) -> None:
        """ネストしたディレクトリ内のファイルも正規化"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリ構造を作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()
        nested_dir = test_dir / "system"
        nested_dir.mkdir()
        deep_dir = nested_dir / "plugins"
        deep_dir.mkdir()

        # 各階層にファイルを作成
        (test_dir / "Data.XP3").write_bytes(b"data")
        (nested_dir / "MainWindow.TJS").write_text("mainwindow")
        (deep_dir / "Plugin.DLL").write_bytes(b"plugin")

        # 正規化を実行
        pipeline._normalize_critical_filenames(test_dir)

        # 全階層で小文字に変換されていることを確認
        assert (test_dir / "data.xp3").exists()
        assert (nested_dir / "mainwindow.tjs").exists()
        assert (deep_dir / "plugin.dll").exists()

        # 元の大文字ファイルは存在しない
        assert not (test_dir / "Data.XP3").exists()
        assert not (nested_dir / "MainWindow.TJS").exists()
        assert not (deep_dir / "Plugin.DLL").exists()


class TestBuildPipelineAdjustScripts:
    """BuildPipeline._adjust_scripts メソッドのテスト"""

    def test_adjusts_startup_tjs(self, tmp_path: Path, mocker) -> None:
        """startup.tjs にポリフィル読み込みを追加"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # startup.tjs を作成
        startup_file = test_dir / "startup.tjs"
        startup_file.write_text("// original content")

        # ScriptAdjusterをモック
        mock_adjuster = mocker.patch("mnemonic.pipeline.ScriptAdjuster")
        mock_adjuster_instance = mock_adjuster.return_value

        # スクリプト調整を実行
        pipeline._adjust_scripts(test_dir)

        # ScriptAdjuster.convertが呼ばれたことを確認
        mock_adjuster_instance.convert.assert_called_once_with(startup_file, startup_file)

    @pytest.mark.parametrize(
        "variant",
        [
            pytest.param("Startup.tjs", id="正常系: Startup.tjs（先頭大文字）"),
            pytest.param("STARTUP.TJS", id="正常系: STARTUP.TJS（全大文字）"),
            pytest.param("StartUp.tjs", id="正常系: StartUp.tjs（キャメルケース）"),
        ],
    )
    def test_detects_startup_case_variants(self, tmp_path: Path, mocker, variant: str) -> None:
        """Startup.tjs, STARTUP.TJS などを検出"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # バリエーションのファイルを作成
        startup_file = test_dir / variant
        startup_file.write_text("// original content")

        # ScriptAdjusterをモック
        mock_adjuster = mocker.patch("mnemonic.pipeline.ScriptAdjuster")
        mock_adjuster_instance = mock_adjuster.return_value

        # スクリプト調整を実行
        pipeline._adjust_scripts(test_dir)

        # ScriptAdjuster.convertが呼ばれたことを確認
        mock_adjuster_instance.convert.assert_called_once_with(startup_file, startup_file)


class TestBuildPipelineCopyPolyfillFiles:
    """BuildPipeline._copy_polyfill_files メソッドのテスト"""

    def test_creates_system_directory(self, tmp_path: Path, mocker) -> None:
        """system ディレクトリが作成される"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成（systemディレクトリなし）
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # importlib.resourcesをモック
        mocker.patch("importlib.resources.files")

        # _copy_font_fileをモック（フォントダウンロードを回避）
        mocker.patch.object(pipeline, "_copy_font_file")

        # ポリフィルファイルコピーを実行
        pipeline._copy_polyfill_files(test_dir)

        # systemディレクトリが作成されたことを確認
        assert (test_dir / "system").exists()
        assert (test_dir / "system").is_dir()

    def test_copies_all_polyfill_files(self, tmp_path: Path, mocker) -> None:
        """全ポリフィルファイルがコピーされる"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # モックリソースファイルを作成
        polyfill_files = [
            "PolyfillInitialize.tjs",
            "MenuItem_stub.tjs",
            "KAGParser.tjs",
            "MIDISoundBuffer_stub.tjs",
        ]

        # importlib.resourcesをモックしてファイル内容を返す
        mock_files = mocker.patch("importlib.resources.files")
        mock_package = mocker.MagicMock()
        mock_files.return_value = mock_package

        # 各ファイルのモックを設定
        def create_mock_resource(filename: str):
            mock_resource = mocker.MagicMock()
            mock_file = mocker.MagicMock()
            mock_file.read.return_value = f"// {filename} content".encode()
            mock_resource.open.return_value.__enter__ = mocker.MagicMock(return_value=mock_file)
            mock_resource.open.return_value.__exit__ = mocker.MagicMock(return_value=False)
            return mock_resource

        mock_package.joinpath.side_effect = lambda f: create_mock_resource(f)

        # _copy_font_fileをモック（フォントダウンロードを回避）
        mocker.patch.object(pipeline, "_copy_font_file")

        # ポリフィルファイルコピーを実行
        pipeline._copy_polyfill_files(test_dir)

        # systemディレクトリが作成されたことを確認
        system_dir = test_dir / "system"
        assert system_dir.exists()

        # 各ポリフィルファイルがコピーされたことを確認
        for filename in polyfill_files:
            assert (system_dir / filename).exists()
            content = (system_dir / filename).read_text()
            assert f"// {filename} content" in content


class TestBuildPipelineRemovePluginDirectory:
    """BuildPipeline._remove_plugin_directory メソッドのテスト"""

    def test_removes_lowercase_plugin_directory(self, tmp_path: Path) -> None:
        """正常系: 小文字のpluginディレクトリが削除される"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # pluginディレクトリを作成
        plugin_dir = test_dir / "plugin"
        plugin_dir.mkdir()
        (plugin_dir / "test.dll").write_bytes(b"\x00" * 10)

        # プラグインディレクトリ削除を実行
        pipeline._remove_plugin_directory(test_dir)

        # pluginディレクトリが削除されたことを確認
        assert not plugin_dir.exists()

    @pytest.mark.parametrize(
        "dir_name",
        [
            pytest.param("Plugin", id="正常系: 先頭大文字Pluginが削除される"),
            pytest.param("PLUGIN", id="正常系: 全大文字PLUGINが削除される"),
            pytest.param("Plugins", id="正常系: 複数形Pluginsが削除される"),
            pytest.param("plugins", id="正常系: 小文字複数形pluginsが削除される"),
            pytest.param("PLUGINS", id="正常系: 全大文字複数形PLUGINSが削除される"),
        ],
    )
    def test_removes_plugin_directory_case_variants(self, tmp_path: Path, dir_name: str) -> None:
        """正常系: 大文字小文字のバリエーションが削除される"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # プラグインディレクトリを作成
        plugin_dir = test_dir / dir_name
        plugin_dir.mkdir()
        (plugin_dir / "wuvorbis.dll").write_bytes(b"\x00" * 10)

        # プラグインディレクトリ削除を実行
        pipeline._remove_plugin_directory(test_dir)

        # ディレクトリが削除されたことを確認
        assert not plugin_dir.exists()

    def test_does_nothing_when_no_plugin_directory(self, tmp_path: Path) -> None:
        """正常系: pluginディレクトリがない場合は何もしない"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成（pluginディレクトリなし）
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # 他のディレクトリを作成
        other_dir = test_dir / "other"
        other_dir.mkdir()
        (other_dir / "test.txt").write_text("test")

        # プラグインディレクトリ削除を実行（例外が発生しないことを確認）
        pipeline._remove_plugin_directory(test_dir)

        # 他のディレクトリは残っていることを確認
        assert other_dir.exists()
        assert (other_dir / "test.txt").exists()

    def test_removes_nested_dll_files(self, tmp_path: Path) -> None:
        """正常系: ネストされたDLLファイルも含めて削除される"""
        input_file = tmp_path / "game.exe"
        input_file.write_bytes(b"\x00" * 100)

        config = PipelineConfig(
            input_path=input_file,
            output_path=tmp_path / "output.apk",
        )
        pipeline = BuildPipeline(config)

        # テスト用ディレクトリを作成
        test_dir = tmp_path / "test_dir"
        test_dir.mkdir()

        # pluginディレクトリとサブディレクトリを作成
        plugin_dir = test_dir / "plugin"
        plugin_dir.mkdir()
        (plugin_dir / "wuvorbis.dll").write_bytes(b"\x00" * 10)
        (plugin_dir / "krwinmm.dll").write_bytes(b"\x00" * 10)

        sub_dir = plugin_dir / "subdir"
        sub_dir.mkdir()
        (sub_dir / "other.dll").write_bytes(b"\x00" * 10)

        # プラグインディレクトリ削除を実行
        pipeline._remove_plugin_directory(test_dir)

        # pluginディレクトリ全体が削除されたことを確認
        assert not plugin_dir.exists()
