"""ConversionManagerのテスト"""

import time
from pathlib import Path
from threading import Lock
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.converter import (
    BaseConverter,
    ConversionManager,
    ConversionResult,
    ConversionStatus,
    ConversionSummary,
    ConversionTask,
    RetryConfig,
)

class MockConverter(BaseConverter):
    """テスト用のConverterモック"""

    def __init__(
        self,
        extensions: tuple[str, ...] = (".txt",),
        fail_count: int = 0,
        raise_exception: bool = False,
    ) -> None:
        self._extensions = extensions
        self._fail_count = fail_count
        self._call_count = 0
        self._raise_exception = raise_exception

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        return self._extensions

    def can_convert(self, file_path: Path) -> bool:
        return file_path.suffix.lower() in self._extensions

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        self._call_count += 1

        if self._raise_exception:
            raise RuntimeError("変換中にエラーが発生しました")

        if self._call_count <= self._fail_count:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message="変換失敗",
            )

        return ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            message="",
            bytes_before=100,
            bytes_after=80,
        )

class TestRetryConfig:
    """RetryConfigデータクラスのテスト"""

    def test_default_values(self) -> None:
        """正常系: デフォルト値のテスト"""
        config = RetryConfig()

        assert config.max_attempts == 3
        assert config.backoff_base == 1.0
        assert config.backoff_multiplier == 2.0

    @pytest.mark.parametrize(
        "max_attempts,backoff_base,backoff_multiplier",
        [
            pytest.param(5, 2.0, 3.0, id="正常系: カスタム値1"),
            pytest.param(1, 0.5, 1.5, id="正常系: カスタム値2"),
            pytest.param(10, 0.1, 4.0, id="正常系: カスタム値3"),
        ],
    )
    def test_custom_values(
        self, max_attempts: int, backoff_base: float, backoff_multiplier: float
    ) -> None:
        """正常系: カスタム値のテスト"""
        config = RetryConfig(
            max_attempts=max_attempts,
            backoff_base=backoff_base,
            backoff_multiplier=backoff_multiplier,
        )

        assert config.max_attempts == max_attempts
        assert config.backoff_base == backoff_base
        assert config.backoff_multiplier == backoff_multiplier

class TestConversionTask:
    """ConversionTaskデータクラスのテスト"""

    def test_creation(self, tmp_path: Path) -> None:
        """正常系: タスク作成のテスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        converter = MockConverter()

        task = ConversionTask(source=source, dest=dest, converter=converter)

        assert task.source == source
        assert task.dest == dest
        assert task.converter is converter
        assert task.retry_count == 0

    def test_creation_with_retry_count(self, tmp_path: Path) -> None:
        """正常系: リトライカウント指定でのタスク作成テスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        converter = MockConverter()

        task = ConversionTask(source=source, dest=dest, converter=converter, retry_count=2)

        assert task.retry_count == 2

class TestConversionSummary:
    """ConversionSummaryデータクラスのテスト"""

    def test_default_values(self) -> None:
        """正常系: デフォルト値のテスト"""
        summary = ConversionSummary()

        assert summary.total == 0
        assert summary.success == 0
        assert summary.failed == 0
        assert summary.skipped == 0
        assert summary.results == []

    def test_creation_with_values(self, tmp_path: Path) -> None:
        """正常系: 値を指定してのサマリー作成テスト"""
        result = ConversionResult(
            source_path=tmp_path / "test.txt",
            dest_path=tmp_path / "dest.txt",
            status=ConversionStatus.SUCCESS,
        )

        summary = ConversionSummary(total=5, success=3, failed=1, skipped=1, results=[result])

        assert summary.total == 5
        assert summary.success == 3
        assert summary.failed == 1
        assert summary.skipped == 1
        assert len(summary.results) == 1

    def test_summary_is_mutable(self, tmp_path: Path) -> None:
        """正常系: サマリーが可変であることのテスト"""
        summary = ConversionSummary()

        summary.total = 10
        summary.success = 5
        summary.failed = 3
        summary.skipped = 2

        result = ConversionResult(
            source_path=tmp_path / "test.txt",
            dest_path=tmp_path / "dest.txt",
            status=ConversionStatus.SUCCESS,
        )
        summary.results.append(result)

        assert summary.total == 10
        assert summary.success == 5
        assert summary.failed == 3
        assert summary.skipped == 2
        assert len(summary.results) == 1

class TestConversionManagerInit:
    """ConversionManager初期化のテスト"""

    def test_init_with_defaults(self) -> None:
        """正常系: デフォルト値での初期化テスト"""
        converters = [MockConverter()]

        with patch.object(ConversionManager, "calculate_workers", return_value=4):
            manager = ConversionManager(converters=converters)

        assert manager.converters == converters
        assert isinstance(manager.retry_config, RetryConfig)
        assert manager.max_workers == 4
        assert manager.progress_callback is None

    def test_init_with_custom_values(self) -> None:
        """正常系: カスタム値での初期化テスト"""
        converters = [MockConverter()]
        retry_config = RetryConfig(max_attempts=5)
        progress_callback = MagicMock()

        manager = ConversionManager(
            converters=converters,
            retry_config=retry_config,
            max_workers=8,
            progress_callback=progress_callback,
        )

        assert manager.converters == converters
        assert manager.retry_config.max_attempts == 5
        assert manager.max_workers == 8
        assert manager.progress_callback is progress_callback

class TestConversionManagerGetConverterForFile:
    """ConversionManager.get_converter_for_fileのテスト"""

    def test_get_converter_for_matching_file(self, tmp_path: Path) -> None:
        """正常系: マッチするConverterが見つかる場合"""
        txt_converter = MockConverter(extensions=(".txt",))
        jpg_converter = MockConverter(extensions=(".jpg", ".jpeg"))

        manager = ConversionManager(converters=[txt_converter, jpg_converter], max_workers=1)

        txt_file = tmp_path / "test.txt"
        converter = manager.get_converter_for_file(txt_file)

        assert converter is txt_converter

    def test_get_converter_returns_none_for_unsupported(self, tmp_path: Path) -> None:
        """正常系: サポートされていないファイル形式の場合Noneを返す"""
        txt_converter = MockConverter(extensions=(".txt",))

        manager = ConversionManager(converters=[txt_converter], max_workers=1)

        pdf_file = tmp_path / "test.pdf"
        converter = manager.get_converter_for_file(pdf_file)

        assert converter is None

    def test_get_converter_returns_first_match(self, tmp_path: Path) -> None:
        """正常系: 複数のConverterがマッチする場合、最初のものを返す"""
        first_converter = MockConverter(extensions=(".txt",))
        second_converter = MockConverter(extensions=(".txt", ".md"))

        manager = ConversionManager(converters=[first_converter, second_converter], max_workers=1)

        txt_file = tmp_path / "test.txt"
        converter = manager.get_converter_for_file(txt_file)

        assert converter is first_converter

class TestConversionManagerConvertFiles:
    """ConversionManager.convert_filesのテスト"""

    def test_convert_single_file(self, tmp_path: Path) -> None:
        """正常系: 単一ファイルの変換テスト"""
        source = tmp_path / "source.txt"
        source.write_text("test content")
        dest = tmp_path / "dest.txt"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_files([(source, dest)])

        assert summary.total == 1
        assert summary.success == 1
        assert summary.failed == 0
        assert len(summary.results) == 1
        assert summary.results[0].status == ConversionStatus.SUCCESS

    def test_convert_multiple_files(self, tmp_path: Path) -> None:
        """正常系: 複数ファイルの変換テスト"""
        files = []
        for i in range(3):
            source = tmp_path / f"source_{i}.txt"
            source.write_text(f"content {i}")
            dest = tmp_path / f"dest_{i}.txt"
            files.append((source, dest))

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=2)

        summary = manager.convert_files(files)

        assert summary.total == 3
        assert summary.success == 3
        assert summary.failed == 0
        assert len(summary.results) == 3

    def test_convert_files_parallel_execution(self, tmp_path: Path) -> None:
        """正常系: 並列実行の検証テスト"""
        files = []
        for i in range(4):
            source = tmp_path / f"source_{i}.txt"
            source.write_text(f"content {i}")
            dest = tmp_path / f"dest_{i}.txt"
            files.append((source, dest))

        execution_start_times: list[float] = []
        lock = Lock()

        class TimingConverter(MockConverter):
            def convert(self, source: Path, dest: Path) -> ConversionResult:
                with lock:
                    execution_start_times.append(time.time())
                time.sleep(0.05)
                return super().convert(source, dest)

        converter = TimingConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=4)

        summary = manager.convert_files(files)

        assert summary.success == 4
        # 並列実行なら、開始時刻がほぼ同時（0.1秒以内）になるはず
        if len(execution_start_times) >= 2:
            time_spread = max(execution_start_times) - min(execution_start_times)
            # 直列実行なら各タスク0.05秒かかるので、4タスクで少なくとも0.15秒以上の開きがある
            # 並列実行なら開始時刻がほぼ同時（0.1秒以内）
            assert time_spread < 0.15

    def test_convert_files_with_unsupported_file(self, tmp_path: Path) -> None:
        """正常系: サポートされていないファイルはスキップされる"""
        supported = tmp_path / "source.txt"
        supported.write_text("content")
        dest_supported = tmp_path / "dest.txt"

        unsupported = tmp_path / "source.pdf"
        unsupported.write_text("pdf content")
        dest_unsupported = tmp_path / "dest.pdf"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_files(
            [(supported, dest_supported), (unsupported, dest_unsupported)]
        )

        assert summary.total == 2
        assert summary.success == 1
        assert summary.skipped == 1

    def test_convert_files_progress_callback(self, tmp_path: Path) -> None:
        """正常系: 進捗コールバックの呼び出しテスト"""
        files = []
        for i in range(3):
            source = tmp_path / f"source_{i}.txt"
            source.write_text(f"content {i}")
            dest = tmp_path / f"dest_{i}.txt"
            files.append((source, dest))

        callback = MagicMock()
        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(
            converters=[converter], max_workers=1, progress_callback=callback
        )

        manager.convert_files(files)

        # コールバックが呼ばれたことを確認
        assert callback.call_count >= 3
        # 最終呼び出しは (3, 3) であるはず
        final_calls = [call for call in callback.call_args_list if call[0][0] == 3]
        assert len(final_calls) > 0

class TestConversionManagerRetry:
    """ConversionManagerのリトライ機能のテスト"""

    def test_retry_on_failure(self, tmp_path: Path) -> None:
        """正常系: 失敗時にリトライが行われる"""
        source = tmp_path / "source.txt"
        source.write_text("content")
        dest = tmp_path / "dest.txt"

        # 2回失敗してから成功するConverter
        converter = MockConverter(extensions=(".txt",), fail_count=2)
        retry_config = RetryConfig(max_attempts=3)

        manager = ConversionManager(
            converters=[converter], retry_config=retry_config, max_workers=1
        )

        with patch("time.sleep"):
            summary = manager.convert_files([(source, dest)])

        assert summary.success == 1
        assert summary.failed == 0
        assert converter._call_count == 3

    def test_max_retry_attempts_exceeded(self, tmp_path: Path) -> None:
        """正常系: 最大リトライ回数を超えた場合は失敗"""
        source = tmp_path / "source.txt"
        source.write_text("content")
        dest = tmp_path / "dest.txt"

        # 5回失敗するConverter
        converter = MockConverter(extensions=(".txt",), fail_count=5)
        retry_config = RetryConfig(max_attempts=3)

        manager = ConversionManager(
            converters=[converter], retry_config=retry_config, max_workers=1
        )

        with patch("time.sleep"):
            summary = manager.convert_files([(source, dest)])

        assert summary.success == 0
        assert summary.failed == 1
        # 3回試行されるはず
        assert converter._call_count == 3

    def test_exponential_backoff_timing(self, tmp_path: Path) -> None:
        """正常系: 指数バックオフのタイミングテスト"""
        source = tmp_path / "source.txt"
        source.write_text("content")
        dest = tmp_path / "dest.txt"

        converter = MockConverter(extensions=(".txt",), fail_count=2)
        retry_config = RetryConfig(max_attempts=3, backoff_base=1.0, backoff_multiplier=2.0)

        manager = ConversionManager(
            converters=[converter], retry_config=retry_config, max_workers=1
        )

        sleep_calls: list[float] = []
        with patch("time.sleep", side_effect=lambda x: sleep_calls.append(x)):
            manager.convert_files([(source, dest)])

        # リトライ1回目: 1.0 * (2.0 ** 0) = 1.0秒
        # リトライ2回目: 1.0 * (2.0 ** 1) = 2.0秒
        assert len(sleep_calls) == 2
        assert sleep_calls[0] == 1.0
        assert sleep_calls[1] == 2.0

    def test_retry_on_exception(self, tmp_path: Path) -> None:
        """正常系: 例外発生時もリトライが行われる"""
        source = tmp_path / "source.txt"
        source.write_text("content")
        dest = tmp_path / "dest.txt"

        converter = MockConverter(extensions=(".txt",), raise_exception=True)
        retry_config = RetryConfig(max_attempts=2)

        manager = ConversionManager(
            converters=[converter], retry_config=retry_config, max_workers=1
        )

        with patch("time.sleep"):
            summary = manager.convert_files([(source, dest)])

        assert summary.failed == 1
        assert converter._call_count == 2

class TestConversionManagerConvertDirectory:
    """ConversionManager.convert_directoryのテスト"""

    def test_convert_directory_recursive(self, tmp_path: Path) -> None:
        """正常系: 再帰的なディレクトリ変換テスト"""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        # ルートにファイル作成
        (source_dir / "file1.txt").write_text("content1")

        # サブディレクトリにファイル作成
        sub_dir = source_dir / "subdir"
        sub_dir.mkdir()
        (sub_dir / "file2.txt").write_text("content2")

        dest_dir = tmp_path / "dest"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_directory(source_dir, dest_dir, recursive=True)

        assert summary.total == 2
        assert summary.success == 2

    def test_convert_directory_non_recursive(self, tmp_path: Path) -> None:
        """正常系: 非再帰的なディレクトリ変換テスト"""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        # ルートにファイル作成
        (source_dir / "file1.txt").write_text("content1")

        # サブディレクトリにファイル作成
        sub_dir = source_dir / "subdir"
        sub_dir.mkdir()
        (sub_dir / "file2.txt").write_text("content2")

        dest_dir = tmp_path / "dest"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_directory(source_dir, dest_dir, recursive=False)

        # ルートのファイルのみ変換
        assert summary.total == 1
        assert summary.success == 1

    def test_convert_directory_filters_by_converter(self, tmp_path: Path) -> None:
        """正常系: サポートされるファイルのみが変換される"""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        (source_dir / "file1.txt").write_text("text content")
        (source_dir / "file2.pdf").write_text("pdf content")
        (source_dir / "file3.txt").write_text("more text")

        dest_dir = tmp_path / "dest"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_directory(source_dir, dest_dir)

        # .txtファイルのみ変換
        assert summary.total == 2
        assert summary.success == 2

    def test_convert_directory_preserves_structure(self, tmp_path: Path) -> None:
        """正常系: ディレクトリ構造が保持される"""
        source_dir = tmp_path / "source"
        source_dir.mkdir()

        sub_dir = source_dir / "sub" / "nested"
        sub_dir.mkdir(parents=True)
        (sub_dir / "file.txt").write_text("content")

        dest_dir = tmp_path / "dest"

        converter = MockConverter(extensions=(".txt",))
        manager = ConversionManager(converters=[converter], max_workers=1)

        summary = manager.convert_directory(source_dir, dest_dir)

        assert summary.success == 1
        # 変換先パスが正しいディレクトリ構造を持つことを確認
        result = summary.results[0]
        assert "sub" in str(result.dest_path)
        assert "nested" in str(result.dest_path)

class TestCalculateWorkers:
    """ConversionManager.calculate_workersのテスト"""

    def test_memory_based_calculation(self) -> None:
        """正常系: メモリベースのワーカー数計算"""
        # 2000MBなら4ワーカー
        workers = ConversionManager.calculate_workers(available_memory_mb=2000)
        assert workers == 4

    def test_cpu_based_limit(self) -> None:
        """正常系: CPUコア数による制限"""
        # 十分なメモリがあってもCPUコア数で制限される
        with patch("os.cpu_count", return_value=2):
            workers = ConversionManager.calculate_workers(available_memory_mb=10000)
            assert workers == 2

    def test_minimum_one_worker(self) -> None:
        """正常系: 最小ワーカー数は1"""
        workers = ConversionManager.calculate_workers(available_memory_mb=100)
        assert workers == 1

    def test_auto_memory_detection(self) -> None:
        """正常系: メモリ自動検出のテスト"""
        # 4GB = 4096MB
        with (
            patch("mnemonic.converter.manager._get_available_memory_mb", return_value=4096),
            patch("os.cpu_count", return_value=8),
        ):
            workers = ConversionManager.calculate_workers()
            # 4096 // 500 = 8, min(8, 8) = 8
            assert workers == 8

    def test_fallback_without_psutil(self) -> None:
        """正常系: psutilが利用できない場合のフォールバック"""
        with (
            patch("mnemonic.converter.manager._get_available_memory_mb", return_value=None),
            patch("os.cpu_count", return_value=4),
        ):
            # psutilがない場合はCPUコア数のみで計算
            workers = ConversionManager.calculate_workers()
            assert workers == 4

class TestConversionManagerSummary:
    """ConversionManagerのサマリー生成テスト"""

    def test_summary_counts_all_statuses(self, tmp_path: Path) -> None:
        """正常系: すべてのステータスがカウントされる"""
        # 成功用ファイル
        success_file = tmp_path / "success.txt"
        success_file.write_text("content")

        # 失敗用ファイル（fail_countが大きいConverterを使用）
        fail_file = tmp_path / "fail.txt"
        fail_file.write_text("content")

        # スキップ用ファイル（サポートされない形式）
        skip_file = tmp_path / "skip.pdf"
        skip_file.write_text("content")

        class MixedConverter(MockConverter):
            def convert(self, source: Path, dest: Path) -> ConversionResult:
                if "fail" in source.name:
                    return ConversionResult(
                        source_path=source,
                        dest_path=None,
                        status=ConversionStatus.FAILED,
                        message="強制失敗",
                    )
                return super().convert(source, dest)

        converter = MixedConverter(extensions=(".txt",), fail_count=0)
        retry_config = RetryConfig(max_attempts=1)

        manager = ConversionManager(
            converters=[converter], retry_config=retry_config, max_workers=1
        )

        files = [
            (success_file, tmp_path / "out_success.txt"),
            (fail_file, tmp_path / "out_fail.txt"),
            (skip_file, tmp_path / "out_skip.pdf"),
        ]

        summary = manager.convert_files(files)

        assert summary.total == 3
        assert summary.success == 1
        assert summary.failed == 1
        assert summary.skipped == 1
        assert len(summary.results) == 3
