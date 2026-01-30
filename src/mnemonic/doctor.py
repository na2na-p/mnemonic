"""依存ツールチェッカー"""

from __future__ import annotations

import re
import subprocess
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
]


class DependencyChecker(Protocol):
    """依存ツールチェッカーインターフェース"""

    def check_all(self) -> list[CheckResult]:
        """全ての依存ツールをチェックする"""
        ...

    def check_one(self, info: DependencyInfo) -> CheckResult:
        """単一の依存ツールをチェックする"""
        ...


def _extract_version(output: str) -> str | None:
    """コマンド出力からバージョン番号を抽出する"""
    patterns = [
        r"(\d+\.\d+\.\d+)",
        r"(\d+\.\d+)",
        r"version\s+(\d+)",
        r"(\d+)",
    ]
    for pattern in patterns:
        match = re.search(pattern, output, re.IGNORECASE)
        if match:
            return match.group(1)
    return None


def check_dependency(info: DependencyInfo) -> CheckResult:
    """単一の依存ツールをチェックする"""
    try:
        result = subprocess.run(
            [info.command, info.version_flag],
            capture_output=True,
            text=True,
            timeout=10,
        )
        output = result.stdout + result.stderr
        version = _extract_version(output)

        return CheckResult(
            name=info.name,
            required=info.required,
            found=True,
            version=version,
            message=None,
        )
    except FileNotFoundError:
        return CheckResult(
            name=info.name,
            required=info.required,
            found=False,
            version=None,
            message=f"コマンド '{info.command}' が見つかりません",
        )
    except subprocess.TimeoutExpired:
        return CheckResult(
            name=info.name,
            required=info.required,
            found=False,
            version=None,
            message=f"コマンド '{info.command}' がタイムアウトしました",
        )
    except OSError as e:
        return CheckResult(
            name=info.name,
            required=info.required,
            found=False,
            version=None,
            message=f"コマンド実行エラー: {e}",
        )


def check_all_dependencies() -> list[CheckResult]:
    """全ての依存ツールをチェックする"""
    return [check_dependency(info) for info in DEPENDENCIES]
