"""設定ファイル読み込みのテスト"""

from pathlib import Path

import pytest

from mnemonic.config import (
    ConversionRule,
    EncodingConfig,
    ImageConfig,
    MnemonicConfig,
    TimeoutConfig,
    VideoConfig,
    get_default_config,
    load_config,
)

class TestDefaultConfig:
    """デフォルト設定のテスト"""

    def test_get_default_config_returns_mnemonic_config(self) -> None:
        """デフォルト設定がMnemonicConfigを返す"""
        config = get_default_config()
        assert isinstance(config, MnemonicConfig)

    def test_default_image_config(self) -> None:
        """画像設定のデフォルト値が正しい"""
        config = get_default_config()
        assert config.image.format == "webp"
        assert config.image.quality == "high"
        assert config.image.lossless_alpha is True

    def test_default_video_config(self) -> None:
        """動画設定のデフォルト値が正しい"""
        config = get_default_config()
        assert config.video.codec == "h264"
        assert config.video.profile == "baseline"
        assert config.video.audio_codec == "aac"

    def test_default_encoding_config(self) -> None:
        """エンコーディング設定のデフォルト値が正しい"""
        config = get_default_config()
        assert config.encoding.source is None  # 自動検出
        assert config.encoding.target == "utf-8"

    def test_default_timeout_config(self) -> None:
        """タイムアウト設定のデフォルト値が正しい"""
        config = get_default_config()
        assert config.timeouts.ffmpeg == 300
        assert config.timeouts.gradle == 1800

class TestLoadConfig:
    """設定読み込みのテスト"""

    def test_load_config_not_implemented(self, tmp_path: Path) -> None:
        """load_configは未実装でNotImplementedErrorを投げる"""
        config_file = tmp_path / "mnemonic.yml"
        config_file.write_text("package_name: test")

        with pytest.raises(NotImplementedError):
            load_config(config_file)

class TestConfigImmutability:
    """設定のイミュータビリティテスト"""

    def test_mnemonic_config_is_frozen(self) -> None:
        """MnemonicConfigは変更不可"""
        config = get_default_config()
        with pytest.raises(AttributeError):
            config.package_name = "changed"  # type: ignore[misc]

    def test_image_config_is_frozen(self) -> None:
        """ImageConfigは変更不可"""
        config = ImageConfig()
        with pytest.raises(AttributeError):
            config.format = "png"  # type: ignore[misc]

class TestConfigDataclasses:
    """設定データクラスのテスト"""

    def test_conversion_rule_creation(self) -> None:
        """ConversionRuleが正しく作成できる"""
        rule = ConversionRule(pattern="*.png", converter="image")
        assert rule.pattern == "*.png"
        assert rule.converter == "image"

    def test_video_config_creation(self) -> None:
        """VideoConfigが正しく作成できる"""
        config = VideoConfig(codec="h265", profile="main", audio_codec="opus")
        assert config.codec == "h265"
        assert config.profile == "main"
        assert config.audio_codec == "opus"

    def test_encoding_config_creation(self) -> None:
        """EncodingConfigが正しく作成できる"""
        config = EncodingConfig(source="shift_jis", target="utf-8")
        assert config.source == "shift_jis"
        assert config.target == "utf-8"

    def test_timeout_config_creation(self) -> None:
        """TimeoutConfigが正しく作成できる"""
        config = TimeoutConfig(ffmpeg=600, gradle=3600)
        assert config.ffmpeg == 600
        assert config.gradle == 3600
