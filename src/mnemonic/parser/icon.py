"""EXEファイルからアイコンを抽出する機能

このモジュールはWindows EXEファイルから埋め込みアイコンを抽出し、
PNG形式で保存する機能を提供します。
"""

from __future__ import annotations

import tempfile
from pathlib import Path
from typing import Protocol

from icoextract import IconExtractor as IcoExtractor
from icoextract import NoIconsAvailableError
from PIL import Image


class IconExtractorProtocol(Protocol):
    """アイコン抽出のプロトコル

    EXEファイルからアイコンを抽出するためのインターフェースを定義します。
    """

    def extract(self, exe_path: Path, output_path: Path) -> Path | None:
        """EXEからアイコンを抽出してPNGとして保存

        Args:
            exe_path: EXEファイルのパス
            output_path: 出力先ディレクトリ

        Returns:
            抽出されたアイコンのパス。抽出失敗時はNone。
        """
        ...


class ExeIconExtractor:
    """EXEファイルからアイコンを抽出するクラス

    icoextractライブラリを使用してEXEファイルから
    アイコンを抽出し、PNG形式で保存します。
    """

    def extract(self, exe_path: Path, output_path: Path) -> Path | None:
        """EXEからアイコンを抽出してPNGとして保存

        Args:
            exe_path: EXEファイルのパス
            output_path: 出力先ディレクトリ

        Returns:
            抽出されたアイコンのパス。抽出失敗時はNone。
        """
        if not exe_path.exists():
            return None

        try:
            extractor = IcoExtractor(str(exe_path))

            # ICO形式でアイコンを一時ファイルに抽出
            # icoextract は BytesIO をサポートしていないため一時ファイルを使用
            with tempfile.NamedTemporaryFile(suffix=".ico", delete=False) as tmp:
                tmp_ico_path = Path(tmp.name)

            try:
                extractor.export_icon(str(tmp_ico_path))

                # ICOを開いて最大サイズのアイコンを取得
                with Image.open(tmp_ico_path) as img:
                    # ICOファイルには複数サイズが含まれている場合がある
                    # 最大サイズを選択
                    if hasattr(img, "n_frames") and img.n_frames > 1:
                        max_size = 0
                        best_frame = 0
                        for frame in range(img.n_frames):
                            img.seek(frame)
                            size = img.size[0] * img.size[1]
                            if size > max_size:
                                max_size = size
                                best_frame = frame
                        img.seek(best_frame)

                    # 出力ディレクトリを作成
                    output_path.mkdir(parents=True, exist_ok=True)

                    # PNG形式で保存
                    icon_output = output_path / "icon.png"
                    img.save(str(icon_output), "PNG")

                    return icon_output
            finally:
                # 一時ファイルを削除
                if tmp_ico_path.exists():
                    tmp_ico_path.unlink()

        except NoIconsAvailableError:
            # アイコンが見つからない
            return None
        except Exception:
            # その他のエラー（破損したEXE等）
            return None
