"""アセットファイル一覧機能のテスト

TDDワークフローのStep 3として、実装後のテストを検証する。
"""

from pathlib import Path

import pytest

from mnemonic.parser import (
    AssetFile,
    AssetManifest,
    AssetScanner,
    AssetType,
    ConversionAction,
)


class TestAssetType:
    """AssetType 列挙型のテスト"""

    @pytest.mark.parametrize(
        "asset_type,expected_value",
        [
            pytest.param(AssetType.SCRIPT, "script", id="正常系: スクリプト種別"),
            pytest.param(AssetType.IMAGE, "image", id="正常系: 画像種別"),
            pytest.param(AssetType.AUDIO, "audio", id="正常系: 音声種別"),
            pytest.param(AssetType.VIDEO, "video", id="正常系: 動画種別"),
            pytest.param(AssetType.OTHER, "other", id="正常系: その他種別"),
        ],
    )
    def test_asset_type_values(
        self,
        asset_type: AssetType,
        expected_value: str,
    ) -> None:
        """AssetTypeの値が正しく定義されている"""
        assert asset_type.value == expected_value

    def test_asset_type_count(self) -> None:
        """AssetTypeは5種類存在する"""
        assert len(AssetType) == 5


class TestConversionAction:
    """ConversionAction 列挙型のテスト"""

    @pytest.mark.parametrize(
        "action,expected_value",
        [
            pytest.param(
                ConversionAction.ENCODE_UTF8,
                "encode_utf8",
                id="正常系: UTF-8エンコード",
            ),
            pytest.param(
                ConversionAction.CONVERT_WEBP,
                "convert_webp",
                id="正常系: WebP変換",
            ),
            pytest.param(
                ConversionAction.CONVERT_OGG,
                "convert_ogg",
                id="正常系: OGG変換",
            ),
            pytest.param(
                ConversionAction.CONVERT_MP4,
                "convert_mp4",
                id="正常系: MP4変換",
            ),
            pytest.param(
                ConversionAction.COPY,
                "copy",
                id="正常系: コピー",
            ),
            pytest.param(
                ConversionAction.SKIP,
                "skip",
                id="正常系: スキップ",
            ),
        ],
    )
    def test_conversion_action_values(
        self,
        action: ConversionAction,
        expected_value: str,
    ) -> None:
        """ConversionActionの値が正しく定義されている"""
        assert action.value == expected_value

    def test_conversion_action_count(self) -> None:
        """ConversionActionは6種類存在する"""
        assert len(ConversionAction) == 6


class TestAssetFile:
    """AssetFile データクラスのテスト"""

    def test_asset_file_creation(self) -> None:
        """AssetFileが正しく生成される"""
        asset = AssetFile(
            path=Path("data/scenario/first.ks"),
            asset_type=AssetType.SCRIPT,
            action=ConversionAction.ENCODE_UTF8,
            source_format=".ks",
            target_format=None,
        )

        assert asset.path == Path("data/scenario/first.ks")
        assert asset.asset_type == AssetType.SCRIPT
        assert asset.action == ConversionAction.ENCODE_UTF8
        assert asset.source_format == ".ks"
        assert asset.target_format is None

    def test_asset_file_with_target_format(self) -> None:
        """変換後形式を持つAssetFileが正しく生成される"""
        asset = AssetFile(
            path=Path("image/bg01.tlg"),
            asset_type=AssetType.IMAGE,
            action=ConversionAction.CONVERT_WEBP,
            source_format=".tlg",
            target_format=".webp",
        )

        assert asset.target_format == ".webp"

    def test_asset_file_is_immutable(self) -> None:
        """AssetFileはイミュータブル（frozen=True）"""
        asset = AssetFile(
            path=Path("test.ks"),
            asset_type=AssetType.SCRIPT,
            action=ConversionAction.ENCODE_UTF8,
            source_format=".ks",
            target_format=None,
        )

        with pytest.raises(AttributeError):
            asset.path = Path("modified.ks")  # type: ignore[misc]


class TestAssetManifest:
    """AssetManifest データクラスのテスト"""

    @pytest.fixture
    def sample_files(self) -> list[AssetFile]:
        """テスト用のアセットファイルリストを生成するフィクスチャ"""
        return [
            AssetFile(
                path=Path("scenario/first.ks"),
                asset_type=AssetType.SCRIPT,
                action=ConversionAction.ENCODE_UTF8,
                source_format=".ks",
                target_format=None,
            ),
            AssetFile(
                path=Path("scenario/system.tjs"),
                asset_type=AssetType.SCRIPT,
                action=ConversionAction.ENCODE_UTF8,
                source_format=".tjs",
                target_format=None,
            ),
            AssetFile(
                path=Path("image/bg01.tlg"),
                asset_type=AssetType.IMAGE,
                action=ConversionAction.CONVERT_WEBP,
                source_format=".tlg",
                target_format=".webp",
            ),
            AssetFile(
                path=Path("image/chara.bmp"),
                asset_type=AssetType.IMAGE,
                action=ConversionAction.CONVERT_WEBP,
                source_format=".bmp",
                target_format=".webp",
            ),
            AssetFile(
                path=Path("bgm/title.wav"),
                asset_type=AssetType.AUDIO,
                action=ConversionAction.CONVERT_OGG,
                source_format=".wav",
                target_format=".ogg",
            ),
            AssetFile(
                path=Path("voice/v001.ogg"),
                asset_type=AssetType.AUDIO,
                action=ConversionAction.COPY,
                source_format=".ogg",
                target_format=None,
            ),
            AssetFile(
                path=Path("movie/op.mpg"),
                asset_type=AssetType.VIDEO,
                action=ConversionAction.CONVERT_MP4,
                source_format=".mpg",
                target_format=".mp4",
            ),
            AssetFile(
                path=Path("data/config.txt"),
                asset_type=AssetType.OTHER,
                action=ConversionAction.COPY,
                source_format=".txt",
                target_format=None,
            ),
        ]

    def test_manifest_creation(self) -> None:
        """AssetManifestが正しく生成される"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=[],
        )

        assert manifest.game_dir == Path("/games/mygame")
        assert manifest.files == []

    def test_manifest_with_files(self, sample_files: list[AssetFile]) -> None:
        """ファイルリストを持つAssetManifestが正しく生成される"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        assert len(manifest.files) == 8

    def test_filter_by_type_script(self, sample_files: list[AssetFile]) -> None:
        """filter_by_type()でスクリプトファイルがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_type(AssetType.SCRIPT)

        assert len(result) == 2
        assert all(f.asset_type == AssetType.SCRIPT for f in result)

    def test_filter_by_type_image(self, sample_files: list[AssetFile]) -> None:
        """filter_by_type()で画像ファイルがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_type(AssetType.IMAGE)

        assert len(result) == 2
        assert all(f.asset_type == AssetType.IMAGE for f in result)

    def test_filter_by_type_audio(self, sample_files: list[AssetFile]) -> None:
        """filter_by_type()で音声ファイルがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_type(AssetType.AUDIO)

        assert len(result) == 2
        assert all(f.asset_type == AssetType.AUDIO for f in result)

    def test_filter_by_type_video(self, sample_files: list[AssetFile]) -> None:
        """filter_by_type()で動画ファイルがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_type(AssetType.VIDEO)

        assert len(result) == 1
        assert all(f.asset_type == AssetType.VIDEO for f in result)

    def test_filter_by_type_other(self, sample_files: list[AssetFile]) -> None:
        """filter_by_type()でその他ファイルがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_type(AssetType.OTHER)

        assert len(result) == 1
        assert all(f.asset_type == AssetType.OTHER for f in result)

    def test_filter_by_action_encode_utf8(self, sample_files: list[AssetFile]) -> None:
        """filter_by_action()でENCODE_UTF8アクションがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_action(ConversionAction.ENCODE_UTF8)

        assert len(result) == 2
        assert all(f.action == ConversionAction.ENCODE_UTF8 for f in result)

    def test_filter_by_action_convert_webp(self, sample_files: list[AssetFile]) -> None:
        """filter_by_action()でCONVERT_WEBPアクションがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_action(ConversionAction.CONVERT_WEBP)

        assert len(result) == 2
        assert all(f.action == ConversionAction.CONVERT_WEBP for f in result)

    def test_filter_by_action_convert_ogg(self, sample_files: list[AssetFile]) -> None:
        """filter_by_action()でCONVERT_OGGアクションがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_action(ConversionAction.CONVERT_OGG)

        assert len(result) == 1
        assert all(f.action == ConversionAction.CONVERT_OGG for f in result)

    def test_filter_by_action_convert_mp4(self, sample_files: list[AssetFile]) -> None:
        """filter_by_action()でCONVERT_MP4アクションがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_action(ConversionAction.CONVERT_MP4)

        assert len(result) == 1
        assert all(f.action == ConversionAction.CONVERT_MP4 for f in result)

    def test_filter_by_action_copy(self, sample_files: list[AssetFile]) -> None:
        """filter_by_action()でCOPYアクションがフィルタされる"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        result = manifest.filter_by_action(ConversionAction.COPY)

        assert len(result) == 2
        assert all(f.action == ConversionAction.COPY for f in result)

    def test_get_summary(self, sample_files: list[AssetFile]) -> None:
        """get_summary()で種別ごとのファイル数が取得される"""
        manifest = AssetManifest(
            game_dir=Path("/games/mygame"),
            files=sample_files,
        )

        summary = manifest.get_summary()

        assert summary[AssetType.SCRIPT] == 2
        assert summary[AssetType.IMAGE] == 2
        assert summary[AssetType.AUDIO] == 2
        assert summary[AssetType.VIDEO] == 1
        assert summary[AssetType.OTHER] == 1


class TestAssetScanner:
    """AssetScanner クラスのテスト"""

    def test_scanner_init(self, tmp_path: Path) -> None:
        """AssetScannerが正しく初期化される"""
        scanner = AssetScanner(game_dir=tmp_path)
        assert scanner is not None

    def test_scanner_init_with_config(self, tmp_path: Path) -> None:
        """AssetScannerが設定付きで正しく初期化される"""
        config = {"exclude": ["*.bak"]}
        scanner = AssetScanner(game_dir=tmp_path, config=config)
        assert scanner is not None


class TestAssetScannerFileClassification:
    """AssetScannerのファイル分類テスト"""

    @pytest.fixture
    def game_dir(self, tmp_path: Path) -> Path:
        """テスト用ゲームディレクトリ構造を作成するフィクスチャ"""
        # スクリプトファイル
        scenario_dir = tmp_path / "scenario"
        scenario_dir.mkdir()
        (scenario_dir / "first.ks").write_text("@bg storage=bg01", encoding="utf-8")
        (scenario_dir / "system.tjs").write_text("var x = 1;", encoding="utf-8")

        # 画像ファイル
        image_dir = tmp_path / "image"
        image_dir.mkdir()
        (image_dir / "bg01.tlg").write_bytes(b"\x00TLG5")
        (image_dir / "chara.bmp").write_bytes(b"BM")
        (image_dir / "icon.jpg").write_bytes(b"\xff\xd8\xff")
        (image_dir / "logo.png").write_bytes(b"\x89PNG")

        # 音声ファイル
        bgm_dir = tmp_path / "bgm"
        bgm_dir.mkdir()
        (bgm_dir / "title.wav").write_bytes(b"RIFF")
        (bgm_dir / "ending.ogg").write_bytes(b"OggS")

        # 動画ファイル
        movie_dir = tmp_path / "movie"
        movie_dir.mkdir()
        (movie_dir / "op.mpg").write_bytes(b"\x00\x00\x01\xba")
        (movie_dir / "ed.wmv").write_bytes(b"WMV")

        # その他ファイル
        (tmp_path / "readme.txt").write_text("readme", encoding="utf-8")
        (tmp_path / "config.ini").write_text("[config]", encoding="utf-8")

        return tmp_path

    @pytest.mark.parametrize(
        "extension,expected_type",
        [
            pytest.param(".ks", AssetType.SCRIPT, id="正常系: .ksはスクリプト"),
            pytest.param(".tjs", AssetType.SCRIPT, id="正常系: .tjsはスクリプト"),
        ],
    )
    def test_script_file_classification(
        self,
        tmp_path: Path,
        extension: str,
        expected_type: AssetType,
    ) -> None:
        """スクリプトファイルが正しく分類される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_text("test content", encoding="utf-8")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].asset_type == expected_type

    @pytest.mark.parametrize(
        "extension,expected_type",
        [
            pytest.param(".tlg", AssetType.IMAGE, id="正常系: .tlgは画像"),
            pytest.param(".bmp", AssetType.IMAGE, id="正常系: .bmpは画像"),
            pytest.param(".jpg", AssetType.IMAGE, id="正常系: .jpgは画像"),
            pytest.param(".png", AssetType.IMAGE, id="正常系: .pngは画像"),
        ],
    )
    def test_image_file_classification(
        self,
        tmp_path: Path,
        extension: str,
        expected_type: AssetType,
    ) -> None:
        """画像ファイルが正しく分類される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].asset_type == expected_type

    @pytest.mark.parametrize(
        "extension,expected_type",
        [
            pytest.param(".ogg", AssetType.AUDIO, id="正常系: .oggは音声"),
            pytest.param(".wav", AssetType.AUDIO, id="正常系: .wavは音声"),
        ],
    )
    def test_audio_file_classification(
        self,
        tmp_path: Path,
        extension: str,
        expected_type: AssetType,
    ) -> None:
        """音声ファイルが正しく分類される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].asset_type == expected_type

    @pytest.mark.parametrize(
        "extension,expected_type",
        [
            pytest.param(".mpg", AssetType.VIDEO, id="正常系: .mpgは動画"),
            pytest.param(".wmv", AssetType.VIDEO, id="正常系: .wmvは動画"),
        ],
    )
    def test_video_file_classification(
        self,
        tmp_path: Path,
        extension: str,
        expected_type: AssetType,
    ) -> None:
        """動画ファイルが正しく分類される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].asset_type == expected_type

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".txt", id="正常系: .txtはその他"),
            pytest.param(".ini", id="正常系: .iniはその他"),
            pytest.param(".dat", id="正常系: .datはその他"),
        ],
    )
    def test_other_file_classification(
        self,
        tmp_path: Path,
        extension: str,
    ) -> None:
        """その他ファイルがOTHERに分類される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].asset_type == AssetType.OTHER

    def test_full_game_directory_scan(self, game_dir: Path) -> None:
        """完全なゲームディレクトリのスキャンが正しく動作する"""
        scanner = AssetScanner(game_dir=game_dir)
        manifest = scanner.scan()

        # 合計12ファイル
        assert len(manifest.files) == 12

        # 種別ごとのファイル数確認
        summary = manifest.get_summary()
        assert summary[AssetType.SCRIPT] == 2
        assert summary[AssetType.IMAGE] == 4
        assert summary[AssetType.AUDIO] == 2
        assert summary[AssetType.VIDEO] == 2
        assert summary[AssetType.OTHER] == 2


class TestAssetScannerConversionAction:
    """AssetScannerの変換アクション設定テスト"""

    @pytest.mark.parametrize(
        "extension,expected_action",
        [
            pytest.param(
                ".ks",
                ConversionAction.ENCODE_UTF8,
                id="正常系: .ksはENCODE_UTF8",
            ),
            pytest.param(
                ".tjs",
                ConversionAction.ENCODE_UTF8,
                id="正常系: .tjsはENCODE_UTF8",
            ),
        ],
    )
    def test_script_conversion_action(
        self,
        tmp_path: Path,
        extension: str,
        expected_action: ConversionAction,
    ) -> None:
        """スクリプトファイルにENCODE_UTF8が設定される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_text("test", encoding="utf-8")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == expected_action

    @pytest.mark.parametrize(
        "extension,expected_action",
        [
            pytest.param(
                ".tlg",
                ConversionAction.CONVERT_WEBP,
                id="正常系: .tlgはCONVERT_WEBP",
            ),
            pytest.param(
                ".bmp",
                ConversionAction.CONVERT_WEBP,
                id="正常系: .bmpはCONVERT_WEBP",
            ),
            pytest.param(
                ".jpg",
                ConversionAction.CONVERT_WEBP,
                id="正常系: .jpgはCONVERT_WEBP",
            ),
        ],
    )
    def test_image_conversion_action(
        self,
        tmp_path: Path,
        extension: str,
        expected_action: ConversionAction,
    ) -> None:
        """画像ファイルにCONVERT_WEBPが設定される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == expected_action

    def test_wav_conversion_action(self, tmp_path: Path) -> None:
        """WAVファイルにCONVERT_OGGが設定される"""
        test_file = tmp_path / "test.wav"
        test_file.write_bytes(b"RIFF")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == ConversionAction.CONVERT_OGG

    def test_ogg_copy_action(self, tmp_path: Path) -> None:
        """OGGファイルにCOPYが設定される（変換不要）"""
        test_file = tmp_path / "test.ogg"
        test_file.write_bytes(b"OggS")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == ConversionAction.COPY

    @pytest.mark.parametrize(
        "extension,expected_action",
        [
            pytest.param(
                ".mpg",
                ConversionAction.CONVERT_MP4,
                id="正常系: .mpgはCONVERT_MP4",
            ),
            pytest.param(
                ".wmv",
                ConversionAction.CONVERT_MP4,
                id="正常系: .wmvはCONVERT_MP4",
            ),
        ],
    )
    def test_video_conversion_action(
        self,
        tmp_path: Path,
        extension: str,
        expected_action: ConversionAction,
    ) -> None:
        """動画ファイルにCONVERT_MP4が設定される"""
        test_file = tmp_path / f"test{extension}"
        test_file.write_bytes(b"\x00")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == expected_action


class TestAssetScannerErrorHandling:
    """AssetScannerの異常系テスト"""

    def test_nonexistent_directory_raises_error(self) -> None:
        """存在しないディレクトリでエラーが発生する"""
        nonexistent = Path("/nonexistent/game/directory")

        with pytest.raises(FileNotFoundError):
            AssetScanner(game_dir=nonexistent)

    def test_empty_directory_returns_empty_manifest(self, tmp_path: Path) -> None:
        """空のディレクトリで空のマニフェストが返される"""
        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 0
        assert manifest.game_dir == tmp_path


class TestAssetScannerWithConfig:
    """AssetScannerの設定ベーステスト"""

    def test_exclude_pattern(self, tmp_path: Path) -> None:
        """exclude設定でファイルがスキップされる"""
        (tmp_path / "test.ks").write_text("test", encoding="utf-8")
        (tmp_path / "backup.bak").write_text("backup", encoding="utf-8")

        config = {"exclude": ["*.bak"]}
        scanner = AssetScanner(game_dir=tmp_path, config=config)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].path == Path("test.ks")

    def test_conversion_rules_override(self, tmp_path: Path) -> None:
        """conversion_rules設定で変換ルールが上書きされる"""
        voice_dir = tmp_path / "voice"
        voice_dir.mkdir()
        (voice_dir / "v001.ogg").write_bytes(b"OggS")

        config = {
            "conversion_rules": [
                {"pattern": "voice/*.ogg", "converter": "skip"},
            ]
        }

        scanner = AssetScanner(game_dir=tmp_path, config=config)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].action == ConversionAction.SKIP

    def test_hidden_files_excluded(self, tmp_path: Path) -> None:
        """隠しファイルが自動的に除外される"""
        (tmp_path / "visible.ks").write_text("test", encoding="utf-8")
        (tmp_path / ".hidden.ks").write_text("hidden", encoding="utf-8")

        scanner = AssetScanner(game_dir=tmp_path)
        manifest = scanner.scan()

        assert len(manifest.files) == 1
        assert manifest.files[0].path == Path("visible.ks")
