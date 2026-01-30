"""CLI entry point for Mnemonic."""

from typing import Annotated

import typer
from rich.console import Console
from rich.table import Table

from mnemonic import __version__
from mnemonic.doctor import check_all_dependencies

app = typer.Typer(help="吉里吉里ゲームをAndroid APKに変換するCLIツール")

@app.command()
def build(
    input_path: Annotated[str, typer.Argument(help="入力ファイル（exe/xp3）")],
    output: Annotated[str | None, typer.Option("-o", "--output", help="出力APKパス")] = None,
) -> None:
    """ゲームをAPKにビルドする"""
    raise typer.Exit(0)

@app.command()
def doctor() -> None:
    """依存ツールをチェックする"""
    console = Console()
    results = check_all_dependencies()

    table = Table(title="依存ツールチェック結果")
    table.add_column("ステータス", justify="center")
    table.add_column("ツール名", justify="left")
    table.add_column("バージョン", justify="left")
    table.add_column("必須", justify="center")
    table.add_column("メッセージ", justify="left")

    has_missing_required = False

    for result in results:
        if result.found:
            status = "[green]✓[/green]"
        else:
            status = "[red]✗[/red]"
            if result.required:
                has_missing_required = True

        required_str = "[yellow]必須[/yellow]" if result.required else "オプション"
        version_str = result.version or "-"
        message_str = result.message or ""

        table.add_row(status, result.name, version_str, required_str, message_str)

    console.print(table)

    if has_missing_required:
        console.print("\n[red]エラー: 必須ツールが不足しています[/red]")
        raise typer.Exit(1)
    else:
        console.print("\n[green]すべての必須ツールが利用可能です[/green]")
        raise typer.Exit(0)

@app.command()
def info(
    input_path: Annotated[str, typer.Argument(help="解析対象パス")],
) -> None:
    """ゲーム構成を解析・表示する"""
    raise typer.Exit(0)

# cache サブコマンドグループ
cache_app = typer.Typer(help="キャッシュ管理")
app.add_typer(cache_app, name="cache")

@cache_app.command("clean")
def cache_clean(
    force: Annotated[bool, typer.Option("-f", "--force", help="確認なしで削除")] = False,
    template_only: Annotated[
        bool, typer.Option("--template-only", help="テンプレートのみ削除")
    ] = False,
) -> None:
    """キャッシュを削除する"""
    raise typer.Exit(0)

@cache_app.command("info")
def cache_info() -> None:
    """キャッシュ情報を表示する"""
    raise typer.Exit(0)

def version_callback(value: bool) -> None:
    """バージョン表示コールバック"""
    if value:
        typer.echo(f"mnemonic {__version__}")
        raise typer.Exit(0)

@app.callback()
def main(
    version: Annotated[
        bool,
        typer.Option(
            "--version",
            callback=version_callback,
            is_eager=True,
            help="バージョンを表示する",
        ),
    ] = False,
) -> None:
    """Mnemonic CLI - 吉里吉里ゲームをAndroid APKに変換"""
    pass
