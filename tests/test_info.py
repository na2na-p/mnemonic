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


class TestDetectEngine:
    """detect_engine関数のテスト"""

    @pytest.mark.parametrize(
        "files,expected_engine",
        [
            pytest.param(["data.xp3"], "kirikiri", id="正常系: data.xp3があればkirikiri"),
            pytest.param(["video.xp3"], "kirikiri", id="正常系: video.xp3があればkirikiri"),
            pytest.param(["game.xp3"], "kirikiri", id="正常系: 任意の.xp3があればkirikiri"),
            pytest.param(["Game.rgss3a"], "rpgmaker", id="正常系: .rgss3aがあればrpgmaker"),
            pytest.param(["Game.rgssad"], "rpgmaker", id="正常系: .rgssadがあればrpgmaker"),
            pytest.param(["Game.rgss2a"], "rpgmaker", id="正常系: .rgss2aがあればrpgmaker"),
            pytest.param(["readme.txt"], "unknown", id="正常系: 不明なファイルのみはunknown"),
            pytest.param([], "unknown", id="正常系: 空ディレクトリはunknown"),
        ],
    )
    def test_detect_engine(self, tmp_path: Path, files: list[str], expected_engine: str) -> None:
        """エンジン検出が正しく動作する"""
        for filename in files:
            (tmp_path / filename).touch()

        result = detect_engine(tmp_path)
        assert result == expected_engine

    def test_detect_engine_kirikiri_priority(self, tmp_path: Path) -> None:
        """kirikiriとrpgmakerの両方のファイルがある場合、kirikiriを優先する"""
        (tmp_path / "data.xp3").touch()
        (tmp_path / "Game.rgss3a").touch()

        result = detect_engine(tmp_path)
        assert result == "kirikiri"


class TestCollectFileStats:
    """collect_file_stats関数のテスト"""

    def test_collect_file_stats_empty_dir(self, tmp_path: Path) -> None:
        """空ディレクトリの場合、count=0を返す"""
        result = collect_file_stats(tmp_path, [".txt"])
        assert result.count == 0
        assert result.total_size_bytes == 0

    def test_collect_file_stats_single_extension(self, tmp_path: Path) -> None:
        """単一拡張子のファイルを正しく収集する"""
        (tmp_path / "file1.txt").write_text("hello")
        (tmp_path / "file2.txt").write_text("world")
        (tmp_path / "file3.png").write_bytes(b"\x89PNG")

        result = collect_file_stats(tmp_path, [".txt"])
        assert result.count == 2
        assert ".txt" in result.extensions
        assert result.total_size_bytes == 10

    def test_collect_file_stats_multiple_extensions(self, tmp_path: Path) -> None:
        """複数拡張子のファイルを正しく収集する"""
        (tmp_path / "image1.png").write_bytes(b"\x89PNG1234")
        (tmp_path / "image2.jpg").write_bytes(b"\xff\xd8\xff")

        result = collect_file_stats(tmp_path, [".png", ".jpg"])
        assert result.count == 2
        assert ".png" in result.extensions
        assert ".jpg" in result.extensions
        assert result.total_size_bytes == 11

    def test_collect_file_stats_recursive(self, tmp_path: Path) -> None:
        """サブディレクトリ内のファイルも再帰的に収集する"""
        subdir = tmp_path / "subdir"
        subdir.mkdir()
        (tmp_path / "file1.txt").write_text("root")
        (subdir / "file2.txt").write_text("subdir")

        result = collect_file_stats(tmp_path, [".txt"])
        assert result.count == 2
        assert result.total_size_bytes == 10

    def test_collect_file_stats_case_insensitive(self, tmp_path: Path) -> None:
        """拡張子の大文字小文字を区別しない"""
        (tmp_path / "file1.TXT").write_text("upper")
        (tmp_path / "file2.txt").write_text("lower")

        result = collect_file_stats(tmp_path, [".txt"])
        assert result.count == 2

    def test_collect_file_stats_returns_found_extensions(self, tmp_path: Path) -> None:
        """実際に見つかった拡張子のみを返す"""
        (tmp_path / "file1.png").write_bytes(b"\x89PNG")

        result = collect_file_stats(tmp_path, [".png", ".jpg", ".gif"])
        assert result.count == 1
        assert ".png" in result.extensions
        assert ".jpg" not in result.extensions
        assert ".gif" not in result.extensions


class TestAnalyzeGame:
    """analyze_game関数のテスト"""

    def test_analyze_game_kirikiri(self, tmp_path: Path) -> None:
        """吉里吉里ゲームを正しく解析する"""
        (tmp_path / "data.xp3").touch()
        (tmp_path / "script.ks").write_text("スクリプト内容", encoding="utf-8")
        (tmp_path / "image.png").write_bytes(b"\x89PNG12345678")
        (tmp_path / "sound.ogg").write_bytes(b"OggS1234")
        (tmp_path / "movie.mp4").write_bytes(b"ftyp1234")

        result = analyze_game(tmp_path)

        assert result.engine == "kirikiri"
        assert result.scripts.count == 1
        assert result.images.count == 1
        assert result.audio.count == 1
        assert result.video.count == 1

    def test_analyze_game_rpgmaker(self, tmp_path: Path) -> None:
        """RPGツクールゲームを正しく解析する"""
        (tmp_path / "Game.rgss3a").touch()

        result = analyze_game(tmp_path)
        assert result.engine == "rpgmaker"

    def test_analyze_game_unknown(self, tmp_path: Path) -> None:
        """不明なエンジンのゲームを正しく解析する"""
        (tmp_path / "readme.txt").write_text("readme")

        result = analyze_game(tmp_path)
        assert result.engine == "unknown"

    def test_analyze_game_encoding_detection(self, tmp_path: Path) -> None:
        """エンコーディング検出が動作する"""
        (tmp_path / "script.ks").write_text("日本語テキスト", encoding="utf-8")

        result = analyze_game(tmp_path)
        assert result.detected_encoding is not None

    def test_analyze_game_no_scripts_no_encoding(self, tmp_path: Path) -> None:
        """スクリプトがない場合、エンコーディングはNone"""
        result = analyze_game(tmp_path)
        assert result.detected_encoding is None
