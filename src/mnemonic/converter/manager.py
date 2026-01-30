"""ConversionManager モジュール

複数ファイルの並列変換、リトライ、進捗管理を行うConversionManagerを提供する。
"""

import os
import time
from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from pathlib import Path
from threading import Lock

from mnemonic.converter.base import (
    BaseConverter,
    ConversionResult,
    ConversionStatus,
)

@dataclass
class RetryConfig:
    """リトライ設定

    変換失敗時のリトライ動作を制御するための設定。
    指数バックオフを使用してリトライ間隔を計算する。

    Attributes:
        max_attempts: 最大試行回数（初回含む）
        backoff_base: バックオフの基本秒数
        backoff_multiplier: バックオフの乗数
    """

    max_attempts: int = 3
    backoff_base: float = 1.0
    backoff_multiplier: float = 2.0

@dataclass
class ConversionTask:
    """変換タスク

    単一ファイルの変換タスクを表すデータクラス。

    Attributes:
        source: 変換元ファイルのパス
        dest: 変換先ファイルのパス
        converter: 使用するConverter
        retry_count: 現在のリトライ回数
    """

    source: Path
    dest: Path
    converter: BaseConverter
    retry_count: int = 0

@dataclass
class ConversionSummary:
    """変換サマリー

    複数ファイルの変換結果のサマリーを保持するデータクラス。
    mutableとして定義し、結果を蓄積できるようにする。

    Attributes:
        total: 変換対象の総ファイル数
        success: 変換成功数
        failed: 変換失敗数
        skipped: スキップ数
        results: 個々の変換結果のリスト
    """

    total: int = 0
    success: int = 0
    failed: int = 0
    skipped: int = 0
    results: list[ConversionResult] = field(default_factory=list)

# 進捗コールバックの型エイリアス
ProgressCallback = Callable[[int, int], None]

# 1ワーカーあたりのメモリ使用量（MB）
MEMORY_PER_WORKER_MB = 500

class ConversionManager:
    """変換マネージャー

    複数ファイルの並列変換を管理するクラス。
    リトライ機能、進捗報告、ディレクトリ単位の変換をサポートする。

    Attributes:
        converters: 使用可能なConverterのリスト
        retry_config: リトライ設定
        max_workers: 最大ワーカー数
        progress_callback: 進捗報告用コールバック
    """

    def __init__(
        self,
        converters: list[BaseConverter],
        retry_config: RetryConfig | None = None,
        max_workers: int | None = None,
        progress_callback: ProgressCallback | None = None,
    ) -> None:
        """ConversionManagerを初期化する

        Args:
            converters: 使用可能なConverterのリスト
            retry_config: リトライ設定（Noneの場合はデフォルト設定を使用）
            max_workers: 最大ワーカー数（Noneの場合は自動計算）
            progress_callback: 進捗報告用コールバック関数
        """
        self.converters = converters
        self.retry_config = retry_config or RetryConfig()
        self.max_workers = max_workers or self.calculate_workers()
        self.progress_callback = progress_callback

    def convert_files(self, files: list[tuple[Path, Path]]) -> ConversionSummary:
        """複数ファイルを変換する

        指定されたファイルリストを並列で変換し、サマリーを返す。

        Args:
            files: (変換元パス, 変換先パス)のタプルのリスト

        Returns:
            変換結果のサマリー
        """
        summary = ConversionSummary(total=len(files))
        completed_count = 0
        lock = Lock()

        def convert_with_retry(source: Path, dest: Path) -> ConversionResult:
            """リトライ付きで単一ファイルを変換する"""
            converter = self.get_converter_for_file(source)

            if converter is None:
                return ConversionResult(
                    source_path=source,
                    dest_path=None,
                    status=ConversionStatus.SKIPPED,
                    message="対応するConverterが見つかりません",
                )

            attempt = 0
            last_result: ConversionResult | None = None
            last_error: Exception | None = None

            while attempt < self.retry_config.max_attempts:
                try:
                    result = converter.convert(source, dest)

                    if result.status == ConversionStatus.SUCCESS:
                        return result

                    last_result = result

                except Exception as e:
                    last_error = e
                    last_result = ConversionResult(
                        source_path=source,
                        dest_path=None,
                        status=ConversionStatus.FAILED,
                        message=str(e),
                    )

                attempt += 1

                if attempt < self.retry_config.max_attempts:
                    # 指数バックオフ: backoff_base * (backoff_multiplier ** retry_count)
                    sleep_time = self.retry_config.backoff_base * (
                        self.retry_config.backoff_multiplier ** (attempt - 1)
                    )
                    time.sleep(sleep_time)

            # 最大リトライ回数に達した場合
            if last_error is not None:
                return ConversionResult(
                    source_path=source,
                    dest_path=None,
                    status=ConversionStatus.FAILED,
                    message=f"最大リトライ回数超過: {last_error}",
                )

            return last_result or ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message="変換に失敗しました",
            )

        def process_file(source: Path, dest: Path) -> ConversionResult:
            """ファイルを処理し、進捗を報告する"""
            nonlocal completed_count
            result = convert_with_retry(source, dest)

            with lock:
                completed_count += 1
                if self.progress_callback:
                    self.progress_callback(completed_count, summary.total)

            return result

        # 並列実行
        with ThreadPoolExecutor(max_workers=self.max_workers) as executor:
            futures = {
                executor.submit(process_file, source, dest): (source, dest)
                for source, dest in files
            }

            for future in as_completed(futures):
                result = future.result()
                summary.results.append(result)

                if result.status == ConversionStatus.SUCCESS:
                    summary.success += 1
                elif result.status == ConversionStatus.SKIPPED:
                    summary.skipped += 1
                else:
                    summary.failed += 1

        return summary

    def convert_directory(
        self,
        source_dir: Path,
        dest_dir: Path,
        recursive: bool = True,
    ) -> ConversionSummary:
        """ディレクトリ内のファイルを変換する

        指定されたディレクトリ内のファイルを変換し、サマリーを返す。
        対応するConverterが存在するファイルのみを処理する。

        Args:
            source_dir: 変換元ディレクトリのパス
            dest_dir: 変換先ディレクトリのパス
            recursive: サブディレクトリも再帰的に処理するか

        Returns:
            変換結果のサマリー
        """
        files: list[tuple[Path, Path]] = []

        # ファイルを収集
        pattern = "**/*" if recursive else "*"

        for source_file in source_dir.glob(pattern):
            if not source_file.is_file():
                continue

            # 対応するConverterがあるファイルのみ収集
            if self.get_converter_for_file(source_file) is None:
                continue

            # 変換先パスを計算（ディレクトリ構造を保持）
            relative_path = source_file.relative_to(source_dir)
            dest_file = dest_dir / relative_path

            files.append((source_file, dest_file))

        return self.convert_files(files)

    def get_converter_for_file(self, file_path: Path) -> BaseConverter | None:
        """ファイルに対応するConverterを取得する

        指定されたファイルを変換可能なConverterを検索し、最初にマッチしたものを返す。

        Args:
            file_path: ファイルのパス

        Returns:
            対応するConverter、存在しない場合はNone
        """
        for converter in self.converters:
            if converter.can_convert(file_path):
                return converter
        return None

    @staticmethod
    def calculate_workers(available_memory_mb: int | None = None) -> int:
        """最適なワーカー数を計算する

        メモリ使用量とCPUコア数に基づいて、最適なワーカー数を計算する。
        1ワーカーあたり500MBのメモリを想定する。

        Args:
            available_memory_mb: 使用可能なメモリ（MB）。Noneの場合は自動検出

        Returns:
            最適なワーカー数（最小1）
        """
        cpu_count = os.cpu_count() or 1

        if available_memory_mb is not None:
            memory_based_workers = available_memory_mb // MEMORY_PER_WORKER_MB
            return max(1, min(memory_based_workers, cpu_count))

        # メモリ自動検出を試みる
        available_mb = _get_available_memory_mb()
        if available_mb is not None:
            memory_based_workers = available_mb // MEMORY_PER_WORKER_MB
            return max(1, min(memory_based_workers, cpu_count))

        # psutilが利用できない場合はCPUコア数のみで決定
        return max(1, cpu_count)

def _get_available_memory_mb() -> int | None:
    """利用可能なメモリをMB単位で取得する

    psutilがインストールされている場合のみ動作する。

    Returns:
        利用可能なメモリ（MB）、取得できない場合はNone
    """
    try:
        import psutil

        available_bytes = psutil.virtual_memory().available
        return available_bytes // (1024 * 1024)
    except (ImportError, AttributeError):
        return None
