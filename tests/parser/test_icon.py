"""EXEアイコン抽出機能のテスト"""

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.parser.icon import ExeIconExtractor


class TestExeIconExtractor:
    """ExeIconExtractorクラスのテスト"""

    @pytest.fixture
    def extractor(self) -> ExeIconExtractor:
        """ExeIconExtractorのインスタンスを作成"""
        return ExeIconExtractor()

    @pytest.fixture
    def temp_output_dir(self, tmp_path: Path) -> Path:
        """一時出力ディレクトリを作成"""
        output_dir = tmp_path / "output"
        return output_dir

    def test_extract_returns_none_when_exe_not_exists(
        self, extractor: ExeIconExtractor, temp_output_dir: Path
    ) -> None:
        """正常系: EXEファイルが存在しない場合にNoneを返す"""
        non_existent = Path("/nonexistent/game.exe")

        result = extractor.extract(non_existent, temp_output_dir)

        assert result is None

    @patch("mnemonic.parser.icon.IcoExtractor")
    @patch("mnemonic.parser.icon.Image")
    def test_extract_saves_png_when_icon_found(
        self,
        mock_image: MagicMock,
        mock_ico_extractor: MagicMock,
        extractor: ExeIconExtractor,
        temp_output_dir: Path,
        tmp_path: Path,
    ) -> None:
        """正常系: アイコンが見つかった場合にPNGとして保存する"""
        # EXEファイルを作成
        exe_path = tmp_path / "game.exe"
        exe_path.write_bytes(b"MZ" + b"\x00" * 100)

        # モックの設定
        mock_extractor_instance = MagicMock()
        mock_ico_extractor.return_value = mock_extractor_instance

        mock_img = MagicMock()
        mock_img.n_frames = 1
        mock_img.size = (256, 256)
        mock_img.__enter__ = MagicMock(return_value=mock_img)
        mock_img.__exit__ = MagicMock(return_value=None)
        mock_image.open.return_value = mock_img

        result = extractor.extract(exe_path, temp_output_dir)

        assert result is not None
        assert result == temp_output_dir / "icon.png"
        mock_img.save.assert_called_once()

    @patch("mnemonic.parser.icon.IcoExtractor")
    def test_extract_returns_none_when_no_icons_available(
        self,
        mock_ico_extractor: MagicMock,
        extractor: ExeIconExtractor,
        temp_output_dir: Path,
        tmp_path: Path,
    ) -> None:
        """異常系: EXEにアイコンが含まれない場合にNoneを返す"""
        from mnemonic.parser.icon import NoIconsAvailableError

        exe_path = tmp_path / "no_icon.exe"
        exe_path.write_bytes(b"MZ" + b"\x00" * 100)

        mock_extractor_instance = MagicMock()
        mock_extractor_instance.export_icon.side_effect = NoIconsAvailableError("No icons found")
        mock_ico_extractor.return_value = mock_extractor_instance

        result = extractor.extract(exe_path, temp_output_dir)

        assert result is None

    @patch("mnemonic.parser.icon.IcoExtractor")
    def test_extract_returns_none_on_corrupted_exe(
        self,
        mock_ico_extractor: MagicMock,
        extractor: ExeIconExtractor,
        temp_output_dir: Path,
        tmp_path: Path,
    ) -> None:
        """異常系: 破損したEXEファイルの場合にNoneを返す"""
        exe_path = tmp_path / "corrupted.exe"
        exe_path.write_bytes(b"INVALID")

        mock_ico_extractor.side_effect = Exception("Invalid PE file")

        result = extractor.extract(exe_path, temp_output_dir)

        assert result is None

    @patch("mnemonic.parser.icon.IcoExtractor")
    @patch("mnemonic.parser.icon.Image")
    def test_extract_selects_largest_icon_when_multiple_frames(
        self,
        mock_image: MagicMock,
        mock_ico_extractor: MagicMock,
        extractor: ExeIconExtractor,
        temp_output_dir: Path,
        tmp_path: Path,
    ) -> None:
        """正常系: 複数サイズのアイコンがある場合、最大サイズを選択する"""
        exe_path = tmp_path / "game.exe"
        exe_path.write_bytes(b"MZ" + b"\x00" * 100)

        mock_extractor_instance = MagicMock()
        mock_ico_extractor.return_value = mock_extractor_instance

        # 複数フレームを持つICO
        mock_img = MagicMock()
        mock_img.n_frames = 3
        # フレームごとにサイズを変える
        sizes = [(16, 16), (256, 256), (48, 48)]
        frame_index = [0]

        def seek(idx: int) -> None:
            frame_index[0] = idx

        type(mock_img).size = property(lambda _: sizes[frame_index[0]])
        mock_img.seek = seek
        mock_img.__enter__ = MagicMock(return_value=mock_img)
        mock_img.__exit__ = MagicMock(return_value=None)
        mock_image.open.return_value = mock_img

        result = extractor.extract(exe_path, temp_output_dir)

        assert result is not None
        # 最大サイズ（256x256）のフレーム1が選択されていることを確認
        assert frame_index[0] == 1
        mock_img.save.assert_called_once()

    @patch("mnemonic.parser.icon.IcoExtractor")
    @patch("mnemonic.parser.icon.Image")
    def test_extract_creates_output_directory(
        self,
        mock_image: MagicMock,
        mock_ico_extractor: MagicMock,
        extractor: ExeIconExtractor,
        tmp_path: Path,
    ) -> None:
        """正常系: 出力ディレクトリが存在しない場合に作成する"""
        exe_path = tmp_path / "game.exe"
        exe_path.write_bytes(b"MZ" + b"\x00" * 100)

        output_dir = tmp_path / "deep" / "nested" / "output"
        assert not output_dir.exists()

        mock_extractor_instance = MagicMock()
        mock_ico_extractor.return_value = mock_extractor_instance

        mock_img = MagicMock()
        mock_img.n_frames = 1
        mock_img.size = (256, 256)
        mock_img.__enter__ = MagicMock(return_value=mock_img)
        mock_img.__exit__ = MagicMock(return_value=None)
        mock_image.open.return_value = mock_img

        result = extractor.extract(exe_path, output_dir)

        assert result is not None
        assert output_dir.exists()
