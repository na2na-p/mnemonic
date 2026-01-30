"""E2Eテスト用フィクスチャ"""

from pathlib import Path

import pytest

@pytest.fixture(scope="session")
def fixtures_dir() -> Path:
    """E2Eテストフィクスチャのディレクトリ"""
    return Path(__file__).parent.parent / "fixtures" / "e2e"

@pytest.fixture(scope="session")
def minimal_game_fixture(fixtures_dir: Path) -> Path:
    """最小構成ゲームのXP3パス"""
    return fixtures_dir / "minimal_game" / "data.xp3"

@pytest.fixture(scope="session")
def convert_game_fixture(fixtures_dir: Path) -> Path:
    """変換テスト用ゲームのXP3パス"""
    return fixtures_dir / "convert_game" / "data.xp3"

@pytest.fixture(scope="session")
def custom_game_fixture(fixtures_dir: Path) -> Path:
    """カスタム設定ゲームのXP3パス"""
    return fixtures_dir / "custom_game" / "data.xp3"

@pytest.fixture(scope="session")
def encrypted_xp3_fixture(fixtures_dir: Path) -> Path:
    """暗号化されたXP3パス"""
    return fixtures_dir / "encrypted.xp3"
