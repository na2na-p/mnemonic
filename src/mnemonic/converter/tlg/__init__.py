"""TLG画像デコーダーパッケージ

吉里吉里2エンジンで使用されるTLG5/TLG6形式の画像をデコードする。
"""

from mnemonic.converter.tlg.lzss import LZSSDecoder
from mnemonic.converter.tlg.tlg5 import TLG5Decoder
from mnemonic.converter.tlg.tlg6 import TLG6Decoder

__all__ = [
    "LZSSDecoder",
    "TLG5Decoder",
    "TLG6Decoder",
]
