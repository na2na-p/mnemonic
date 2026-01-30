"""Configuration module for Mnemonic."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Protocol

@dataclass(frozen=True)
class ImageConfig:
    """画像変換設定"""

    format: str = "webp"
    quality: int | str = "high"
    lossless_alpha: bool = True

@dataclass(frozen=True)
class VideoConfig:
    """動画変換設定"""

    codec: str = "h264"
    profile: str = "baseline"
    audio_codec: str = "aac"

@dataclass(frozen=True)
class EncodingConfig:
    """文字コード設定"""

    source: str | None = None
    target: str = "utf-8"

@dataclass(frozen=True)
class ConversionRule:
    """カスタム変換ルール"""

    pattern: str
    converter: str

@dataclass(frozen=True)
class TimeoutConfig:
    """タイムアウト設定"""

    ffmpeg: int = 300
    gradle: int = 1800

@dataclass(frozen=True)
class MnemonicConfig:
    """ルート設定"""

    package_name: str | None = None
    app_name: str | None = None
    version_code: int = 1
    version_name: str = "1.0.0"
    image: ImageConfig = field(default_factory=ImageConfig)
    video: VideoConfig = field(default_factory=VideoConfig)
    encoding: EncodingConfig = field(default_factory=EncodingConfig)
    conversion_rules: list[ConversionRule] = field(default_factory=list)
    exclude: list[str] = field(default_factory=list)
    timeouts: TimeoutConfig = field(default_factory=TimeoutConfig)

class ConfigLoader(Protocol):
    """設定読み込みインターフェース"""

    def load_config(self, path: Path) -> MnemonicConfig:
        """設定ファイルを読み込む"""
        ...

    def get_default_config(self) -> MnemonicConfig:
        """デフォルト設定を取得する"""
        ...

def load_config(path: Path) -> MnemonicConfig:
    """設定ファイルを読み込む（未実装）"""
    raise NotImplementedError("F-03-03で実装予定")

def get_default_config() -> MnemonicConfig:
    """デフォルト設定を取得する"""
    return MnemonicConfig()
