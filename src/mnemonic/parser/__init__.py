"""Parser module for Mnemonic.

ゲーム構成を検出し、解析するためのモジュール。
XP3アーカイブの操作と暗号化チェック機能を提供する。
EXEファイルからの埋め込みXP3抽出機能も提供する。
"""

from mnemonic.parser.assets import (
    AssetFile,
    AssetManifest,
    AssetScanner,
    AssetType,
    ConversionAction,
)
from mnemonic.parser.detector import EngineType, GameDetector, GameStructure
from mnemonic.parser.exe import XP3_MAGIC, EmbeddedXP3Extractor, EmbeddedXP3Info
from mnemonic.parser.xp3 import (
    EncryptionInfo,
    EncryptionType,
    XP3Archive,
    XP3EncryptionChecker,
    XP3EncryptionError,
)

__all__ = [
    "AssetFile",
    "AssetManifest",
    "AssetScanner",
    "AssetType",
    "ConversionAction",
    "EmbeddedXP3Extractor",
    "EmbeddedXP3Info",
    "EncryptionInfo",
    "EncryptionType",
    "EngineType",
    "GameDetector",
    "GameStructure",
    "XP3Archive",
    "XP3EncryptionChecker",
    "XP3EncryptionError",
    "XP3_MAGIC",
]
