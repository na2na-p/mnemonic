"""共通型定義"""

from dataclasses import dataclass
from enum import IntEnum


class ExitCode(IntEnum):
    """CLIの終了コード"""

    SUCCESS = 0
    ERROR = 1
    INVALID_INPUT = 2
    DEPENDENCY_ERROR = 3


@dataclass(frozen=True)
class Result:
    """操作結果を表す不変オブジェクト"""

    success: bool
    message: str
    exit_code: ExitCode = ExitCode.SUCCESS
