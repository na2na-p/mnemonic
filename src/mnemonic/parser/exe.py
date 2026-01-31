"""EXEファイルからの埋め込みXP3抽出機能

Windows EXE形式のゲームファイルには、XP3アーカイブが埋め込まれていることがある。
このモジュールは、EXEファイル内のXP3オフセットを検出し、抽出する機能を提供する。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

# XP3ファイルのマジックバイト（11バイト）
XP3_MAGIC = b"XP3\x0d\x0a\x20\x0a\x1a\x8b\x67\x01"


@dataclass(frozen=True)
class EmbeddedXP3Info:
    """EXE内埋め込みXP3の情報

    Attributes:
        offset: EXE内でのXP3開始オフセット
        estimated_size: 推定サイズ（EXE終端までのバイト数）
    """

    offset: int
    estimated_size: int


class EmbeddedXP3Extractor:
    """EXEファイルから埋め込みXP3を抽出するクラス

    使用例:
        >>> extractor = EmbeddedXP3Extractor(Path("game.exe"))
        >>> xp3_list = extractor.find_embedded_xp3()
        >>> extracted = extractor.extract_all(Path("/tmp/extracted"))
    """

    def __init__(self, exe_path: Path) -> None:
        """初期化

        Args:
            exe_path: 対象EXEファイルのパス

        Raises:
            FileNotFoundError: EXEファイルが存在しない場合
        """
        if not exe_path.exists():
            raise FileNotFoundError(f"EXEファイルが見つかりません: {exe_path}")
        self._exe_path = exe_path

    @property
    def exe_path(self) -> Path:
        """対象EXEファイルのパス"""
        return self._exe_path

    def find_embedded_xp3(self) -> list[EmbeddedXP3Info]:
        """EXE内の埋め込みXP3を検索する

        EXEファイルをスキャンし、XP3マジックバイトを検出する。
        複数のXP3が埋め込まれている場合、すべてを返す。

        Returns:
            検出されたXP3情報のリスト（オフセット順）
        """
        with open(self._exe_path, "rb") as f:
            content = f.read()

        file_size = len(content)
        offsets: list[EmbeddedXP3Info] = []

        # XP3マジックバイトをスキャンしてオフセットを記録
        pos = 0
        while True:
            offset = content.find(XP3_MAGIC, pos)
            if offset == -1:
                break
            offsets.append(EmbeddedXP3Info(offset=offset, estimated_size=0))
            pos = offset + 1

        # 推定サイズを計算（次のXP3までまたはファイル終端まで）
        result: list[EmbeddedXP3Info] = []
        for i, info in enumerate(offsets):
            if i + 1 < len(offsets):
                estimated_size = offsets[i + 1].offset - info.offset
            else:
                estimated_size = file_size - info.offset
            result.append(EmbeddedXP3Info(offset=info.offset, estimated_size=estimated_size))

        return result

    def extract_all(self, output_dir: Path) -> list[Path]:
        """検出したすべてのXP3を抽出する

        Args:
            output_dir: 抽出先ディレクトリ

        Returns:
            抽出されたXP3ファイルパスのリスト

        Raises:
            FileNotFoundError: EXEファイルが存在しない場合
        """
        output_dir.mkdir(parents=True, exist_ok=True)

        xp3_list = self.find_embedded_xp3()
        if not xp3_list:
            return []

        with open(self._exe_path, "rb") as f:
            content = f.read()

        extracted_files: list[Path] = []
        base_name = self._exe_path.stem

        for i, info in enumerate(xp3_list):
            xp3_data = content[info.offset : info.offset + info.estimated_size]
            output_file = output_dir / f"{base_name}_{i}.xp3"
            output_file.write_bytes(xp3_data)
            extracted_files.append(output_file)

        return extracted_files
