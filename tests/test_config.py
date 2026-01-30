"""設定ファイル読み込みのテスト"""

from pathlib import Path

import pytest

from mnemonic.config import (
    ConfigError,
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

    def test_load_config_valid_file(self, tmp_path: Path) -> None:
        """有効な設定ファイルが読み込める"""
        config_file = tmp_path / "mnemonic.yml"
        config_file.write_text("package_name: com.example.test")

        config = load_config(config_file)
        assert config.package_name == "com.example.test"

    def test_load_config_file_not_found(self, tmp_path: Path) -> None:
        """存在しないファイルでConfigError"""
        with pytest.raises(ConfigError):
            load_config(tmp_path / "nonexistent.yml")

    def test_load_config_invalid_yaml(self, tmp_path: Path) -> None:
        """無効なYAMLでConfigError"""
        config_file = tmp_path / "invalid.yml"
        config_file.write_text("this is not valid yaml: [")

        with pytest.raises(ConfigError):
            load_config(config_file)

    def test_load_config_empty_file(self, tmp_path: Path) -> None:
        """空のファイルはデフォルト設定を返す"""
        config_file = tmp_path / "empty.yml"
        config_file.write_text("")

        config = load_config(config_file)
        default = get_default_config()
        assert config == default

    def test_load_config_merges_with_defaults(self, tmp_path: Path) -> None:
        """部分的な設定がデフォルト値とマージされる"""
        config_file = tmp_path / "partial.yml"
        config_file.write_text("package_name: com.example.partial")

        config = load_config(config_file)
        assert config.package_name == "com.example.partial"
        assert config.version_code == 1
        assert config.image.format == "webp"

    def test_load_config_nested_settings(self, tmp_path: Path) -> None:
        """ネストされた設定が正しく読み込まれる"""
        config_content = """
image:
  format: png
  quality: 90
video:
  codec: h265
"""
        config_file = tmp_path / "nested.yml"
        config_file.write_text(config_content)

        config = load_config(config_file)
        assert config.image.format == "png"
        assert config.image.quality == 90
        assert config.image.lossless_alpha is True
        assert config.video.codec == "h265"
        assert config.video.profile == "baseline"

    def test_load_config_conversion_rules(self, tmp_path: Path) -> None:
        """変換ルールが正しく読み込まれる"""
        config_content = """
conversion_rules:
  - pattern: "*.png"
    converter: image
  - pattern: "*.mp4"
    converter: video
"""
        config_file = tmp_path / "rules.yml"
        config_file.write_text(config_content)

        config = load_config(config_file)
        assert len(config.conversion_rules) == 2
        assert config.conversion_rules[0].pattern == "*.png"
        assert config.conversion_rules[0].converter == "image"
        assert config.conversion_rules[1].pattern == "*.mp4"

    def test_load_config_exclude_list(self, tmp_path: Path) -> None:
        """除外リストが正しく読み込まれる"""
        config_content = """
exclude:
  - "*.bak"
  - "temp/*"
"""
        config_file = tmp_path / "exclude.yml"
        config_file.write_text(config_content)

        config = load_config(config_file)
        assert config.exclude == ["*.bak", "temp/*"]

    def test_load_config_full_settings(self, tmp_path: Path) -> None:
        """全ての設定が正しく読み込まれる"""
        config_content = """
package_name: com.example.full
app_name: Full Test App
version_code: 42
version_name: "2.0.0"
image:
  format: webp
  quality: high
  lossless_alpha: false
video:
  codec: h264
  profile: main
  audio_codec: opus
encoding:
  source: shift_jis
  target: utf-8
timeouts:
  ffmpeg: 600
  gradle: 3600
conversion_rules:
  - pattern: "*.txt"
    converter: text
exclude:
  - "debug/*"
"""
        config_file = tmp_path / "full.yml"
        config_file.write_text(config_content)

        config = load_config(config_file)
        assert config.package_name == "com.example.full"
        assert config.app_name == "Full Test App"
        assert config.version_code == 42
        assert config.version_name == "2.0.0"
        assert config.image.format == "webp"
        assert config.image.lossless_alpha is False
        assert config.video.profile == "main"
        assert config.video.audio_codec == "opus"
        assert config.encoding.source == "shift_jis"
        assert config.timeouts.ffmpeg == 600
        assert config.timeouts.gradle == 3600
        assert len(config.conversion_rules) == 1
        assert config.exclude == ["debug/*"]

    def test_load_config_non_mapping_yaml(self, tmp_path: Path) -> None:
        """マッピング形式でないYAMLでConfigError"""
        config_file = tmp_path / "list.yml"
        config_file.write_text("- item1\n- item2")

        with pytest.raises(ConfigError):
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
