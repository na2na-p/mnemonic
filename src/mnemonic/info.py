"""ゲーム情報解析モジュール"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

import chardet


@dataclass(frozen=True)
class FileStats:
    """ファイル統計"""

    count: int
    extensions: tuple[str, ...]
    total_size_bytes: int


@dataclass(frozen=True)
class GameInfo:
    """ゲーム情報"""

    engine: str
    scripts: FileStats
    images: FileStats
    audio: FileStats
    video: FileStats
    detected_encoding: str | None


class GameAnalyzer(Protocol):
    """ゲーム解析インターフェース"""

    def analyze(self, path: Path) -> GameInfo:
        """ゲームを解析する"""
        ...


def detect_engine(path: Path) -> str:
    """エンジンを検出する

    Args:
        path: 解析対象ディレクトリ

    Returns:
        検出されたエンジン名 ("kirikiri", "rpgmaker", "unknown")
    """
    if not path.is_dir():
        return "unknown"

    for file in path.iterdir():
        if file.suffix.lower() == ".xp3":
            return "kirikiri"

    for file in path.iterdir():
        suffix_lower = file.suffix.lower()
        if suffix_lower.startswith(".rgss"):
            return "rpgmaker"

    return "unknown"


def collect_file_stats(path: Path, extensions: list[str]) -> FileStats:
    """ファイル統計を収集する

    Args:
        path: 解析対象ディレクトリ
        extensions: 対象拡張子リスト (例: [".png", ".jpg"])

    Returns:
        ファイル統計情報
    """
    if not path.is_dir():
        return FileStats(count=0, extensions=(), total_size_bytes=0)

    extensions_lower = {ext.lower() for ext in extensions}
    found_extensions: set[str] = set()
    count = 0
    total_size = 0

    for file in path.rglob("*"):
        if not file.is_file():
            continue
        suffix_lower = file.suffix.lower()
        if suffix_lower in extensions_lower:
            count += 1
            total_size += file.stat().st_size
            found_extensions.add(suffix_lower)

    return FileStats(
        count=count,
        extensions=tuple(sorted(found_extensions)),
        total_size_bytes=total_size,
    )


def _detect_encoding(path: Path, script_extensions: list[str]) -> str | None:
    """スクリプトファイルのエンコーディングを検出する

    Args:
        path: 解析対象ディレクトリ
        script_extensions: スクリプトファイルの拡張子リスト

    Returns:
        検出されたエンコーディング名、またはNone
    """
    extensions_lower = {ext.lower() for ext in script_extensions}

    for file in path.rglob("*"):
        if not file.is_file():
            continue
        if file.suffix.lower() in extensions_lower:
            try:
                content = file.read_bytes()
                if content:
                    result = chardet.detect(content)
                    encoding = result.get("encoding")
                    if encoding:
                        return encoding.lower()
            except OSError:
                continue

    return None


def analyze_game(path: Path) -> GameInfo:
    """ゲームを解析する

    Args:
        path: 解析対象ディレクトリ

    Returns:
        ゲーム情報
    """
    engine = detect_engine(path)

    script_extensions = [".ks", ".tjs"]
    image_extensions = [".png", ".jpg", ".jpeg", ".bmp", ".gif"]
    audio_extensions = [".ogg", ".wav", ".mp3", ".flac", ".mid", ".midi"]
    video_extensions = [".mp4", ".avi", ".wmv", ".mkv"]

    scripts = collect_file_stats(path, script_extensions)
    images = collect_file_stats(path, image_extensions)
    audio = collect_file_stats(path, audio_extensions)
    video = collect_file_stats(path, video_extensions)

    detected_encoding = _detect_encoding(path, script_extensions)

    return GameInfo(
        engine=engine,
        scripts=scripts,
        images=images,
        audio=audio,
        video=video,
        detected_encoding=detected_encoding,
    )
