"""ゲーム情報解析モジュールのテスト"""

from pathlib import Path

import pytest

from mnemonic.info import (
    FileStats,
    GameInfo,
    analyze_game,
    collect_file_stats,
    detect_engine,
)

class TestFileStats:
    """FileStatsデータクラスのテスト"""

    @pytest.mark.parametrize(
        "count,extensions,total_size_bytes",
        [
            pytest.param(0, (), 0, id="正常系: 空のファイル統計"),
            pytest.param(10, (".txt",), 1024, id="正常系: 単一拡張子"),
            pytest.param(100, (".png", ".jpg"), 1048576, id="正常系: 複数拡張子"),
        ],
    )
    def test_create_file_stats(
        self, count: int, extensions: tuple[str, ...], total_size_bytes: int
    ) -> None:
        """FileStatsインスタンスを正常に生成できる"""
        stats = FileStats(
            count=count,
            extensions=extensions,
            total_size_bytes=total_size_bytes,
        )
        assert stats.count == count
        assert stats.extensions == extensions
        assert stats.total_size_bytes == total_size_bytes

    def test_extensions_is_tuple(self) -> None:
        """extensionsフィールドがtupleであることを確認"""
        stats = FileStats(count=1, extensions=(".png", ".jpg"), total_size_bytes=100)
        assert isinstance(stats.extensions, tuple)

    def test_file_stats_is_immutable(self) -> None:
        """FileStatsはイミュータブル（frozen=True）である"""
        stats = FileStats(count=1, extensions=(".txt",), total_size_bytes=100)
        with pytest.raises(AttributeError):
            stats.count = 2  # type: ignore[misc]

class TestGameInfo:
    """GameInfoデータクラスのテスト"""

    @pytest.fixture
    def sample_file_stats(self) -> FileStats:
        """テスト用のFileStats"""
        return FileStats(count=5, extensions=(".txt",), total_size_bytes=500)

    @pytest.mark.parametrize(
        "engine,detected_encoding",
        [
            pytest.param("kirikiri", "utf-8", id="正常系: 吉里吉里エンジン"),
            pytest.param("renpy", "shift_jis", id="正常系: Ren'Pyエンジン"),
            pytest.param("unknown", None, id="正常系: 不明なエンジン"),
        ],
    )
    def test_create_game_info(
        self,
        sample_file_stats: FileStats,
        engine: str,
        detected_encoding: str | None,
    ) -> None:
        """GameInfoインスタンスを正常に生成できる"""
        info = GameInfo(
            engine=engine,
            scripts=sample_file_stats,
            images=sample_file_stats,
            audio=sample_file_stats,
            video=sample_file_stats,
            detected_encoding=detected_encoding,
        )
        assert info.engine == engine
        assert info.scripts == sample_file_stats
        assert info.images == sample_file_stats
        assert info.audio == sample_file_stats
        assert info.video == sample_file_stats
        assert info.detected_encoding == detected_encoding

    def test_game_info_is_immutable(self, sample_file_stats: FileStats) -> None:
        """GameInfoはイミュータブル（frozen=True）である"""
        info = GameInfo(
            engine="kirikiri",
            scripts=sample_file_stats,
            images=sample_file_stats,
            audio=sample_file_stats,
            video=sample_file_stats,
            detected_encoding="utf-8",
        )
        with pytest.raises(AttributeError):
            info.engine = "renpy"  # type: ignore[misc]

class TestAnalyzeGame:
    """analyze_game関数のテスト"""

    def test_analyze_game_raises_not_implemented(self, tmp_path: Path) -> None:
        """analyze_gameは未実装のためNotImplementedErrorを発生させる"""
        with pytest.raises(NotImplementedError) as exc_info:
            analyze_game(tmp_path)
        assert "F-07-03" in str(exc_info.value)

class TestDetectEngine:
    """detect_engine関数のテスト"""

    def test_detect_engine_raises_not_implemented(self, tmp_path: Path) -> None:
        """detect_engineは未実装のためNotImplementedErrorを発生させる"""
        with pytest.raises(NotImplementedError) as exc_info:
            detect_engine(tmp_path)
        assert "F-07-03" in str(exc_info.value)

class TestCollectFileStats:
    """collect_file_stats関数のテスト"""

    def test_collect_file_stats_raises_not_implemented(self, tmp_path: Path) -> None:
        """collect_file_statsは未実装のためNotImplementedErrorを発生させる"""
        with pytest.raises(NotImplementedError) as exc_info:
            collect_file_stats(tmp_path, [".txt"])
        assert "F-07-03" in str(exc_info.value)
