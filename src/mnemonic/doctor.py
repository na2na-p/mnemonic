"""依存ツールチェッカー"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

@dataclass(frozen=True)
class CheckResult:
    """チェック結果"""

    name: str
    required: bool
    found: bool
    version: str | None
    message: str | None

@dataclass(frozen=True)
class DependencyInfo:
    """依存ツール情報"""

    name: str
    command: str
    version_flag: str
    required: bool
    min_version: str | None = None

DEPENDENCIES: list[DependencyInfo] = [
    DependencyInfo(
        name="Python",
        command="python",
        version_flag="--version",
        required=True,
        min_version="3.12",
    ),
    DependencyInfo(
        name="Java JDK",
        command="java",
        version_flag="-version",
        required=True,
        min_version="17",
    ),
    DependencyInfo(
        name="Android SDK",
        command="sdkmanager",
        version_flag="--version",
        required=True,
    ),
    DependencyInfo(
        name="Android NDK",
        command="ndk-build",
        version_flag="--version",
        required=True,
    ),
    DependencyInfo(
        name="FFmpeg",
        command="ffmpeg",
        version_flag="-version",
        required=True,
    ),
    DependencyInfo(
        name="Gradle",
        command="gradle",
        version_flag="--version",
        required=True,
    ),
]

class DependencyChecker(Protocol):
    """依存ツールチェッカーインターフェース"""

    def check_all(self) -> list[CheckResult]:
        """全ての依存ツールをチェックする"""
        ...

    def check_one(self, info: DependencyInfo) -> CheckResult:
        """単一の依存ツールをチェックする"""
        ...

def check_all_dependencies() -> list[CheckResult]:
    """全ての依存ツールをチェックする（未実装）"""
    raise NotImplementedError("F-05-03で実装予定")

def check_dependency(info: DependencyInfo) -> CheckResult:
    """単一の依存ツールをチェックする（未実装）"""
    raise NotImplementedError("F-05-03で実装予定")
