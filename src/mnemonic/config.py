"""Configuration module for Mnemonic."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Protocol

import yaml

class ConfigError(Exception):
    """設定ファイル読み込みエラー"""

    pass

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
    """設定ファイルを読み込む

    Args:
        path: 設定ファイルパス

    Returns:
        MnemonicConfig: 読み込んだ設定（デフォルトとマージ済み）

    Raises:
        ConfigError: ファイル読み込みまたはパースエラー
    """
    if not path.exists():
        raise ConfigError(f"設定ファイルが見つかりません: {path}")

    try:
        with path.open(encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
    except yaml.YAMLError as e:
        raise ConfigError(f"YAML解析エラー: {e}") from e

    if not isinstance(data, dict):
        raise ConfigError("設定ファイルはYAMLのマッピング形式である必要があります")

    default = get_default_config()

    return MnemonicConfig(
        package_name=data.get("package_name", default.package_name),
        app_name=data.get("app_name", default.app_name),
        version_code=data.get("version_code", default.version_code),
        version_name=data.get("version_name", default.version_name),
        image=_merge_image_config(data.get("image", {}), default.image),
        video=_merge_video_config(data.get("video", {}), default.video),
        encoding=_merge_encoding_config(data.get("encoding", {}), default.encoding),
        conversion_rules=_parse_conversion_rules(data.get("conversion_rules", [])),
        exclude=data.get("exclude", default.exclude),
        timeouts=_merge_timeout_config(data.get("timeouts", {}), default.timeouts),
    )

def get_default_config() -> MnemonicConfig:
    """デフォルト設定を取得する"""
    return MnemonicConfig()

def _merge_image_config(data: dict[str, Any], default: ImageConfig) -> ImageConfig:
    """画像設定をマージする"""
    if not isinstance(data, dict):
        return default
    return ImageConfig(
        format=data.get("format", default.format),
        quality=data.get("quality", default.quality),
        lossless_alpha=data.get("lossless_alpha", default.lossless_alpha),
    )

def _merge_video_config(data: dict[str, Any], default: VideoConfig) -> VideoConfig:
    """動画設定をマージする"""
    if not isinstance(data, dict):
        return default
    return VideoConfig(
        codec=data.get("codec", default.codec),
        profile=data.get("profile", default.profile),
        audio_codec=data.get("audio_codec", default.audio_codec),
    )

def _merge_encoding_config(data: dict[str, Any], default: EncodingConfig) -> EncodingConfig:
    """エンコーディング設定をマージする"""
    if not isinstance(data, dict):
        return default
    return EncodingConfig(
        source=data.get("source", default.source),
        target=data.get("target", default.target),
    )

def _merge_timeout_config(data: dict[str, Any], default: TimeoutConfig) -> TimeoutConfig:
    """タイムアウト設定をマージする"""
    if not isinstance(data, dict):
        return default
    return TimeoutConfig(
        ffmpeg=data.get("ffmpeg", default.ffmpeg),
        gradle=data.get("gradle", default.gradle),
    )

def _parse_conversion_rules(data: list[dict[str, Any]]) -> list[ConversionRule]:
    """変換ルールをパースする"""
    if not isinstance(data, list):
        return []
    rules: list[ConversionRule] = []
    for item in data:
        if isinstance(item, dict) and "pattern" in item and "converter" in item:
            rules.append(ConversionRule(pattern=item["pattern"], converter=item["converter"]))
    return rules
