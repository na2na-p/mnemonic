"""ゲーム情報解析モジュール"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

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

def analyze_game(path: Path) -> GameInfo:
    """ゲームを解析する（未実装）"""
    raise NotImplementedError("F-07-03で実装予定")

def detect_engine(path: Path) -> str:
    """エンジンを検出する（未実装）"""
    raise NotImplementedError("F-07-03で実装予定")

def collect_file_stats(path: Path, extensions: list[str]) -> FileStats:
    """ファイル統計を収集する（未実装）"""
    raise NotImplementedError("F-07-03で実装予定")
