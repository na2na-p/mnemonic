"""進捗表示およびログ出力のテスト

BuildLoggerとConsoleProgressDisplayの動作を検証するテストスイート。
"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from mnemonic.logger import (
    BuildLogger,
    ConsoleProgressDisplay,
    LogConfig,
    VerboseLevel,
)
from mnemonic.pipeline import PipelinePhase

if TYPE_CHECKING:
    from pytest import CaptureFixture


class TestLogConfig:
    """LogConfig設定クラスのテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        config = LogConfig()
        assert config.verbose_level == VerboseLevel.NORMAL
        assert config.log_file is None
        assert config.use_color is True
        assert config.use_emoji is True

    @pytest.mark.parametrize(
        "verbose_level",
        [
            pytest.param(VerboseLevel.QUIET, id="QUIETレベル"),
            pytest.param(VerboseLevel.NORMAL, id="NORMALレベル"),
            pytest.param(VerboseLevel.VERBOSE, id="VERBOSEレベル"),
            pytest.param(VerboseLevel.DEBUG, id="DEBUGレベル"),
        ],
    )
    def test_verbose_level_can_be_set(self, verbose_level: VerboseLevel) -> None:
        """verbose_levelを設定できる"""
        config = LogConfig(verbose_level=verbose_level)
        assert config.verbose_level == verbose_level

    def test_log_file_can_be_set(self, tmp_path: Path) -> None:
        """log_fileを設定できる"""
        log_file = tmp_path / "test.log"
        config = LogConfig(log_file=log_file)
        assert config.log_file == log_file

    def test_use_color_can_be_disabled(self) -> None:
        """use_colorを無効にできる"""
        config = LogConfig(use_color=False)
        assert config.use_color is False

    def test_use_emoji_can_be_disabled(self) -> None:
        """use_emojiを無効にできる"""
        config = LogConfig(use_emoji=False)
        assert config.use_emoji is False


class TestVerboseLevel:
    """VerboseLevelのテスト"""

    def test_quiet_is_lowest(self) -> None:
        """QUIETが最低レベル"""
        assert VerboseLevel.QUIET < VerboseLevel.NORMAL
        assert VerboseLevel.QUIET < VerboseLevel.VERBOSE
        assert VerboseLevel.QUIET < VerboseLevel.DEBUG

    def test_level_ordering(self) -> None:
        """レベルの順序が正しい"""
        assert VerboseLevel.QUIET < VerboseLevel.NORMAL < VerboseLevel.VERBOSE < VerboseLevel.DEBUG


class TestBuildLogger:
    """BuildLoggerクラスのテスト"""

    # --- info メソッドのテスト ---

    def test_info_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルでinfoメッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.info("テストメッセージ")
        captured = capsys.readouterr()
        assert "テストメッセージ" in captured.out

    def test_info_quiet_level(self, capsys: CaptureFixture[str]) -> None:
        """QUIETレベルでinfoメッセージが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.QUIET)
        logger = BuildLogger(config)
        logger.info("テストメッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_info_verbose_level(self, capsys: CaptureFixture[str]) -> None:
        """VERBOSEレベルでinfoメッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.VERBOSE)
        logger = BuildLogger(config)
        logger.info("テストメッセージ")
        captured = capsys.readouterr()
        assert "テストメッセージ" in captured.out

    # --- verbose メソッドのテスト ---

    def test_verbose_verbose_level(self, capsys: CaptureFixture[str]) -> None:
        """VERBOSEレベルでverboseメッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.VERBOSE)
        logger = BuildLogger(config)
        logger.verbose("詳細メッセージ")
        captured = capsys.readouterr()
        assert "詳細メッセージ" in captured.out

    def test_verbose_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルでverboseメッセージが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.verbose("詳細メッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_verbose_debug_level(self, capsys: CaptureFixture[str]) -> None:
        """DEBUGレベルでverboseメッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.DEBUG)
        logger = BuildLogger(config)
        logger.verbose("詳細メッセージ")
        captured = capsys.readouterr()
        assert "詳細メッセージ" in captured.out

    # --- debug メソッドのテスト ---

    def test_debug_debug_level(self, capsys: CaptureFixture[str]) -> None:
        """DEBUGレベルでdebugメッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.DEBUG)
        logger = BuildLogger(config)
        logger.debug("デバッグメッセージ")
        captured = capsys.readouterr()
        assert "デバッグメッセージ" in captured.out

    def test_debug_verbose_level(self, capsys: CaptureFixture[str]) -> None:
        """VERBOSEレベルでdebugメッセージが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.VERBOSE)
        logger = BuildLogger(config)
        logger.debug("デバッグメッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""

    def test_debug_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルでdebugメッセージが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.debug("デバッグメッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""

    # --- error メソッドのテスト ---

    def test_error_always_outputs(self, capsys: CaptureFixture[str]) -> None:
        """エラーメッセージは常に出力される"""
        config = LogConfig(verbose_level=VerboseLevel.QUIET)
        logger = BuildLogger(config)
        logger.error("エラーメッセージ")
        captured = capsys.readouterr()
        assert "エラー" in captured.err
        assert "エラーメッセージ" in captured.err

    def test_error_outputs_to_stderr(self, capsys: CaptureFixture[str]) -> None:
        """エラーメッセージは標準エラー出力に出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.error("エラーメッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""
        assert "エラーメッセージ" in captured.err

    # --- warning メソッドのテスト ---

    def test_warning_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルで警告メッセージが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.warning("警告メッセージ")
        captured = capsys.readouterr()
        assert "警告" in captured.out
        assert "警告メッセージ" in captured.out

    def test_warning_quiet_level(self, capsys: CaptureFixture[str]) -> None:
        """QUIETレベルで警告メッセージが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.QUIET)
        logger = BuildLogger(config)
        logger.warning("警告メッセージ")
        captured = capsys.readouterr()
        assert captured.out == ""

    # --- log_command メソッドのテスト ---

    def test_log_command_debug_level(self, capsys: CaptureFixture[str]) -> None:
        """DEBUGレベルでコマンドログが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.DEBUG)
        logger = BuildLogger(config)
        logger.log_command(["ffmpeg", "-i", "input.mp4"], "output line 1\noutput line 2")
        captured = capsys.readouterr()
        assert "ffmpeg -i input.mp4" in captured.out
        assert "output line 1" in captured.out
        assert "output line 2" in captured.out

    def test_log_command_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルでコマンドログが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.log_command(["ffmpeg", "-i", "input.mp4"], "output")
        captured = capsys.readouterr()
        assert captured.out == ""

    # --- log_conversion メソッドのテスト ---

    def test_log_conversion_verbose_level(self, capsys: CaptureFixture[str]) -> None:
        """VERBOSEレベルで変換ログが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.VERBOSE)
        logger = BuildLogger(config)
        logger.log_conversion(Path("input.ogg"), Path("output.mp3"), "OK")
        captured = capsys.readouterr()
        assert "input.ogg" in captured.out
        assert "output.mp3" in captured.out
        assert "OK" in captured.out

    def test_log_conversion_normal_level(self, capsys: CaptureFixture[str]) -> None:
        """NORMALレベルで変換ログが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.log_conversion(Path("input.ogg"), Path("output.mp3"), "OK")
        captured = capsys.readouterr()
        assert captured.out == ""

    # --- log_summary メソッドのテスト ---

    def test_log_summary_with_emoji(self, capsys: CaptureFixture[str]) -> None:
        """emojiありでサマリが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL, use_emoji=True)
        logger = BuildLogger(config)
        logger.log_summary({"output_path": "game.apk", "output_size": 10485760})
        captured = capsys.readouterr()
        assert "Build complete" in captured.out
        assert "game.apk" in captured.out
        assert "10.0 MB" in captured.out

    def test_log_summary_without_emoji(self, capsys: CaptureFixture[str]) -> None:
        """emojiなしでサマリが出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL, use_emoji=False)
        logger = BuildLogger(config)
        logger.log_summary({"output_path": "game.apk", "output_size": 10485760})
        captured = capsys.readouterr()
        assert "[OK]" in captured.out
        assert "Build complete" in captured.out

    def test_log_summary_with_package_name(self, capsys: CaptureFixture[str]) -> None:
        """パッケージ名が出力される"""
        config = LogConfig(verbose_level=VerboseLevel.NORMAL)
        logger = BuildLogger(config)
        logger.log_summary({"package_name": "com.example.game"})
        captured = capsys.readouterr()
        assert "com.example.game" in captured.out

    def test_log_summary_quiet_level(self, capsys: CaptureFixture[str]) -> None:
        """QUIETレベルでサマリが出力されない"""
        config = LogConfig(verbose_level=VerboseLevel.QUIET)
        logger = BuildLogger(config)
        logger.log_summary({"output_path": "game.apk"})
        captured = capsys.readouterr()
        assert captured.out == ""

    # --- create_progress メソッドのテスト ---

    def test_create_progress_returns_progress_display(self) -> None:
        """進捗表示インスタンスを返す"""
        config = LogConfig()
        logger = BuildLogger(config)
        progress = logger.create_progress()
        assert isinstance(progress, ConsoleProgressDisplay)

    def test_create_progress_respects_config(self) -> None:
        """設定が進捗表示に反映される"""
        config = LogConfig(use_color=False, use_emoji=False)
        logger = BuildLogger(config)
        progress = logger.create_progress()
        assert progress._use_color is False
        assert progress._use_emoji is False

    # --- ファイル出力のテスト ---

    def test_log_to_file(self, tmp_path: Path) -> None:
        """ファイルにログが出力される"""
        log_file = tmp_path / "test.log"
        config = LogConfig(verbose_level=VerboseLevel.NORMAL, log_file=log_file)
        with BuildLogger(config) as logger:
            logger.info("テストメッセージ")
        content = log_file.read_text()
        assert "INFO" in content
        assert "テストメッセージ" in content

    def test_log_file_strips_ansi(self, tmp_path: Path) -> None:
        """ファイル出力はANSIエスケープシーケンスを除去する"""
        log_file = tmp_path / "test.log"
        config = LogConfig(verbose_level=VerboseLevel.NORMAL, log_file=log_file)
        with BuildLogger(config) as logger:
            # ANSIエスケープシーケンスを含むメッセージ
            logger.info("\x1b[32mカラーメッセージ\x1b[0m")
        content = log_file.read_text()
        assert "\x1b[" not in content
        assert "カラーメッセージ" in content

    def test_context_manager_closes_file(self, tmp_path: Path) -> None:
        """コンテキストマネージャがファイルを閉じる"""
        log_file = tmp_path / "test.log"
        config = LogConfig(log_file=log_file)
        logger = BuildLogger(config)
        with logger:
            logger.info("テスト")
        # ファイルが閉じられていることを確認
        assert logger._log_file is None or logger._log_file.closed

    def test_all_log_levels_written_to_file(self, tmp_path: Path) -> None:
        """全てのログレベルがファイルに書き込まれる"""
        log_file = tmp_path / "test.log"
        # QUIETでもファイルには書き込まれる
        config = LogConfig(verbose_level=VerboseLevel.QUIET, log_file=log_file)
        with BuildLogger(config) as logger:
            logger.info("INFO message")
            logger.verbose("VERBOSE message")
            logger.debug("DEBUG message")
            logger.warning("WARNING message")
            logger.error("ERROR message")
        content = log_file.read_text()
        assert "INFO: INFO message" in content
        assert "VERBOSE: VERBOSE message" in content
        assert "DEBUG: DEBUG message" in content
        assert "WARNING: WARNING message" in content
        assert "ERROR: ERROR message" in content


class TestConsoleProgressDisplay:
    """ConsoleProgressDisplayのテスト"""

    def test_start_phase_with_emoji(self, capsys: CaptureFixture[str]) -> None:
        """絵文字ありでフェーズ開始を表示"""
        display = ConsoleProgressDisplay(use_emoji=True)
        display.start(PipelinePhase.ANALYZE, 10)
        captured = capsys.readouterr()
        assert "Analyzing game structure" in captured.out

    def test_start_phase_without_emoji(self, capsys: CaptureFixture[str]) -> None:
        """絵文字なしでフェーズ開始を表示"""
        display = ConsoleProgressDisplay(use_emoji=False)
        display.start(PipelinePhase.ANALYZE, 10)
        captured = capsys.readouterr()
        assert "Analyzing game structure" in captured.out

    @pytest.mark.parametrize(
        "phase,expected_name",
        [
            pytest.param(PipelinePhase.ANALYZE, "Analyzing game structure", id="ANALYZE"),
            pytest.param(PipelinePhase.EXTRACT, "Extracting assets", id="EXTRACT"),
            pytest.param(PipelinePhase.CONVERT, "Converting assets", id="CONVERT"),
            pytest.param(PipelinePhase.BUILD, "Building APK", id="BUILD"),
            pytest.param(PipelinePhase.SIGN, "Signing APK", id="SIGN"),
        ],
    )
    def test_start_shows_phase_name(
        self, capsys: CaptureFixture[str], phase: PipelinePhase, expected_name: str
    ) -> None:
        """各フェーズの名前が正しく表示される"""
        display = ConsoleProgressDisplay(use_emoji=False)
        display.start(phase, 10)
        captured = capsys.readouterr()
        assert expected_name in captured.out

    def test_update_progress(self, capsys: CaptureFixture[str]) -> None:
        """進捗が更新される"""
        display = ConsoleProgressDisplay()
        display.start(PipelinePhase.CONVERT, 100)
        _ = capsys.readouterr()  # start の出力をクリア
        display.update(50, "processing...")
        captured = capsys.readouterr()
        assert "50%" in captured.out
        assert "processing" in captured.out

    def test_update_progress_bar(self, capsys: CaptureFixture[str]) -> None:
        """進捗バーが表示される"""
        display = ConsoleProgressDisplay()
        display.start(PipelinePhase.CONVERT, 100)
        _ = capsys.readouterr()  # start の出力をクリア
        display.update(25)
        captured = capsys.readouterr()
        # 25%なので、約10個のバーがあるはず（40 * 0.25 = 10）
        assert captured.out.count("\u2588") == 10  # filled blocks

    def test_finish_success(self, capsys: CaptureFixture[str]) -> None:
        """成功時の終了表示"""
        display = ConsoleProgressDisplay(use_emoji=True)
        display.start(PipelinePhase.BUILD, 10)
        _ = capsys.readouterr()  # start の出力をクリア
        display.finish(success=True)
        captured = capsys.readouterr()
        assert "100%" in captured.out

    def test_finish_success_without_emoji(self, capsys: CaptureFixture[str]) -> None:
        """絵文字なしの成功時の終了表示"""
        display = ConsoleProgressDisplay(use_emoji=False)
        display.start(PipelinePhase.BUILD, 10)
        _ = capsys.readouterr()  # start の出力をクリア
        display.finish(success=True)
        captured = capsys.readouterr()
        assert "done" in captured.out

    def test_finish_failure(self, capsys: CaptureFixture[str]) -> None:
        """失敗時の終了表示"""
        display = ConsoleProgressDisplay(use_emoji=True)
        display.start(PipelinePhase.BUILD, 10)
        _ = capsys.readouterr()  # start の出力をクリア
        display.finish(success=False, message="Build failed")
        captured = capsys.readouterr()
        assert "Build failed" in captured.out

    def test_finish_failure_without_emoji(self, capsys: CaptureFixture[str]) -> None:
        """絵文字なしの失敗時の終了表示"""
        display = ConsoleProgressDisplay(use_emoji=False)
        display.start(PipelinePhase.BUILD, 10)
        _ = capsys.readouterr()  # start の出力をクリア
        display.finish(success=False, message="Build failed")
        captured = capsys.readouterr()
        assert "failed" in captured.out
        assert "Build failed" in captured.out

    def test_internal_state_tracking(self) -> None:
        """内部状態が正しく追跡される"""
        display = ConsoleProgressDisplay()
        display.start(PipelinePhase.CONVERT, 100)
        assert display._phase == PipelinePhase.CONVERT
        assert display._total == 100
        assert display._current == 0

        display.update(50)
        assert display._current == 50
