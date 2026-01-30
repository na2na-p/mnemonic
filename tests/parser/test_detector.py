"""ゲーム構成検出機能のテスト

GameDetectorクラスの単体テストを提供する。
"""

from pathlib import Path

import pytest

from mnemonic.parser.detector import EngineType, GameDetector, GameStructure

# テストフィクスチャディレクトリのパス
FIXTURES_DIR = Path(__file__).parent.parent / "fixtures" / "parser" / "game_samples"

class TestEngineType:
    """EngineType列挙型のテスト"""

    @pytest.mark.parametrize(
        "engine_type,expected_value",
        [
            pytest.param(
                EngineType.KIRIKIRI2,
                "kirikiri2",
                id="正常系: KIRIKIRI2の値が正しい",
            ),
            pytest.param(
                EngineType.KIRIKIRI2_KAG3,
                "kirikiri2_kag3",
                id="正常系: KIRIKIRI2_KAG3の値が正しい",
            ),
            pytest.param(
                EngineType.UNKNOWN,
                "unknown",
                id="正常系: UNKNOWNの値が正しい",
            ),
        ],
    )
    def test_engine_type_value(self, engine_type: EngineType, expected_value: str) -> None:
        """EngineTypeの値が期待通りである"""
        assert engine_type.value == expected_value

class TestGameStructure:
    """GameStructureデータクラスのテスト"""

    def test_game_structure_creation(self) -> None:
        """GameStructureが正しく生成される"""
        structure = GameStructure(
            engine=EngineType.KIRIKIRI2,
            scripts=["scenario/first.ks"],
            script_encoding="shift_jis",
            images=["image/bg01.tlg"],
            audio=["bgm/op.ogg"],
            video=["video/op.mpg"],
            plugins=["plugin/extrans.dll"],
        )

        assert structure.engine == EngineType.KIRIKIRI2
        assert structure.scripts == ["scenario/first.ks"]
        assert structure.script_encoding == "shift_jis"
        assert structure.images == ["image/bg01.tlg"]
        assert structure.audio == ["bgm/op.ogg"]
        assert structure.video == ["video/op.mpg"]
        assert structure.plugins == ["plugin/extrans.dll"]

    def test_game_structure_is_immutable(self) -> None:
        """GameStructureはイミュータブル"""
        structure = GameStructure(
            engine=EngineType.KIRIKIRI2,
            scripts=[],
            script_encoding=None,
            images=[],
            audio=[],
            video=[],
            plugins=[],
        )

        with pytest.raises(AttributeError):
            structure.engine = EngineType.UNKNOWN  # type: ignore[misc]

    def test_game_structure_with_none_encoding(self) -> None:
        """script_encodingがNoneでも正しく生成される"""
        structure = GameStructure(
            engine=EngineType.UNKNOWN,
            scripts=[],
            script_encoding=None,
            images=[],
            audio=[],
            video=[],
            plugins=[],
        )

        assert structure.script_encoding is None

class TestGameDetectorInit:
    """GameDetector初期化のテスト"""

    def test_init_with_valid_directory(self) -> None:
        """有効なディレクトリで初期化できる"""
        game_dir = FIXTURES_DIR / "kirikiri2_game"
        detector = GameDetector(game_dir)
        assert detector is not None

    def test_init_with_nonexistent_directory(self) -> None:
        """存在しないディレクトリでエラーが発生する"""
        nonexistent_dir = Path("/nonexistent/path/to/game")
        with pytest.raises(FileNotFoundError):
            GameDetector(nonexistent_dir)

class TestGameDetectorDetect:
    """GameDetector.detectメソッドのテスト"""

    @pytest.fixture
    def kirikiri2_game_dir(self) -> Path:
        """吉里吉里2ゲームのテストディレクトリ"""
        return FIXTURES_DIR / "kirikiri2_game"

    @pytest.fixture
    def empty_game_dir(self) -> Path:
        """空のテストディレクトリ"""
        return FIXTURES_DIR / "empty_dir"

    @pytest.fixture
    def non_game_dir(self) -> Path:
        """ゲームではないテストディレクトリ"""
        return FIXTURES_DIR / "non_game_dir"

    def test_detect_kirikiri2_game(self, kirikiri2_game_dir: Path) -> None:
        """吉里吉里2ゲームを正しく検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        assert result.engine in (EngineType.KIRIKIRI2, EngineType.KIRIKIRI2_KAG3)

    def test_detect_kag3_scripts(self, kirikiri2_game_dir: Path) -> None:
        """KAG3スクリプト(.ks)を検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        # .ksファイルがスクリプトとして検出される
        ks_scripts = [s for s in result.scripts if s.endswith(".ks")]
        assert len(ks_scripts) > 0

    def test_detect_tjs_scripts(self, kirikiri2_game_dir: Path) -> None:
        """TJSスクリプト(.tjs)を検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        # .tjsファイルがスクリプトとして検出される
        tjs_scripts = [s for s in result.scripts if s.endswith(".tjs")]
        assert len(tjs_scripts) > 0

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".tlg", id="正常系: TLG画像を検出"),
            pytest.param(".bmp", id="正常系: BMP画像を検出"),
        ],
    )
    def test_detect_image_files(self, kirikiri2_game_dir: Path, extension: str) -> None:
        """画像ファイルを検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        matching_images = [i for i in result.images if i.endswith(extension)]
        assert len(matching_images) > 0

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".ogg", id="正常系: OGG音声を検出"),
            pytest.param(".wav", id="正常系: WAV音声を検出"),
        ],
    )
    def test_detect_audio_files(self, kirikiri2_game_dir: Path, extension: str) -> None:
        """音声ファイルを検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        matching_audio = [a for a in result.audio if a.endswith(extension)]
        assert len(matching_audio) > 0

    def test_detect_video_files(self, kirikiri2_game_dir: Path) -> None:
        """動画ファイル(.mpg, .wmv)を検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        # .mpgまたは.wmvファイルが動画として検出される
        video_extensions = (".mpg", ".wmv")
        matching_videos = [v for v in result.video if v.endswith(video_extensions)]
        assert len(matching_videos) > 0

    def test_detect_plugin_files(self, kirikiri2_game_dir: Path) -> None:
        """プラグイン(.dll)を検出できる"""
        detector = GameDetector(kirikiri2_game_dir)
        result = detector.detect()

        # .dllファイルがプラグインとして検出される
        dll_plugins = [p for p in result.plugins if p.endswith(".dll")]
        assert len(dll_plugins) > 0

    def test_detect_empty_directory_error(self, empty_game_dir: Path) -> None:
        """空のディレクトリでエラーが発生する"""
        detector = GameDetector(empty_game_dir)
        with pytest.raises(ValueError):
            detector.detect()

    def test_detect_non_game_directory(self, non_game_dir: Path) -> None:
        """ゲームでないディレクトリでUNKNOWN判定"""
        detector = GameDetector(non_game_dir)
        result = detector.detect()

        # ゲームエンジンが特定できない場合はUNKNOWN
        assert result.engine == EngineType.UNKNOWN

class TestGameDetectorGetSummary:
    """GameDetector.get_summaryメソッドのテスト"""

    @pytest.fixture
    def kirikiri2_game_dir(self) -> Path:
        """吉里吉里2ゲームのテストディレクトリ"""
        return FIXTURES_DIR / "kirikiri2_game"

    def test_get_summary_returns_string(self, kirikiri2_game_dir: Path) -> None:
        """get_summaryが文字列を返す"""
        detector = GameDetector(kirikiri2_game_dir)
        summary = detector.get_summary()

        assert isinstance(summary, str)
        assert len(summary) > 0

    def test_get_summary_contains_engine_info(self, kirikiri2_game_dir: Path) -> None:
        """サマリーにエンジン情報が含まれる"""
        detector = GameDetector(kirikiri2_game_dir)
        summary = detector.get_summary()

        # エンジン名が含まれることを確認
        assert "kirikiri" in summary.lower() or "吉里吉里" in summary

    def test_get_summary_contains_file_counts(self, kirikiri2_game_dir: Path) -> None:
        """サマリーにファイル数情報が含まれる"""
        detector = GameDetector(kirikiri2_game_dir)
        summary = detector.get_summary()

        # ファイル数に関する情報が含まれることを確認
        # 数字が含まれているかをチェック
        assert any(c.isdigit() for c in summary)

    @pytest.mark.parametrize(
        "resource_type",
        [
            pytest.param("script", id="正常系: スクリプト情報を含む"),
            pytest.param("image", id="正常系: 画像情報を含む"),
            pytest.param("audio", id="正常系: 音声情報を含む"),
        ],
    )
    def test_get_summary_contains_resource_info(
        self, kirikiri2_game_dir: Path, resource_type: str
    ) -> None:
        """サマリーに各リソース種別の情報が含まれる"""
        detector = GameDetector(kirikiri2_game_dir)
        summary = detector.get_summary()

        # リソース種別に関する記述があることを確認
        # 日本語または英語でのキーワードをチェック
        keywords = {
            "script": ["script", "スクリプト", ".ks", ".tjs"],
            "image": ["image", "画像", "イメージ"],
            "audio": ["audio", "音声", "サウンド", "bgm", "se"],
        }
        assert any(kw.lower() in summary.lower() for kw in keywords[resource_type])

class TestGameDetectorIntegration:
    """GameDetectorの統合テスト"""

    @pytest.fixture
    def kirikiri2_game_dir(self) -> Path:
        """吉里吉里2ゲームのテストディレクトリ"""
        return FIXTURES_DIR / "kirikiri2_game"

    def test_full_detection_workflow(self, kirikiri2_game_dir: Path) -> None:
        """検出からサマリー取得までの一連のワークフロー"""
        # 1. 検出器を初期化
        detector = GameDetector(kirikiri2_game_dir)

        # 2. ゲーム構成を検出
        structure = detector.detect()

        # 3. 検出結果を検証
        assert structure.engine in (EngineType.KIRIKIRI2, EngineType.KIRIKIRI2_KAG3)
        assert len(structure.scripts) > 0
        assert len(structure.images) > 0
        assert len(structure.audio) > 0

        # 4. サマリーを取得
        summary = detector.get_summary()
        assert len(summary) > 0
