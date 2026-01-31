"""CLI entry point for Mnemonic."""

from pathlib import Path
from typing import Annotated

import typer
from rich.console import Console
from rich.panel import Panel
from rich.table import Table

from mnemonic import __version__
from mnemonic.cache import clear_cache, get_cache_info
from mnemonic.doctor import check_all_dependencies
from mnemonic.info import analyze_game
from mnemonic.pipeline import BuildPipeline, PipelineConfig, PipelineProgress

app = typer.Typer(help="吉里吉里ゲームをAndroid APKに変換するCLIツール")
console = Console()


def _format_size(size_bytes: int) -> str:
    """バイト数を人間が読みやすい形式に変換する"""
    if size_bytes < 1024:
        return f"{size_bytes} B"
    elif size_bytes < 1024 * 1024:
        return f"{size_bytes / 1024:.1f} KB"
    elif size_bytes < 1024 * 1024 * 1024:
        return f"{size_bytes / (1024 * 1024):.1f} MB"
    else:
        return f"{size_bytes / (1024 * 1024 * 1024):.1f} GB"


@app.command()
def build(
    input_path: Annotated[Path, typer.Argument(help="入力ファイルパス（exe/xp3）")],
    output: Annotated[Path | None, typer.Option("-o", "--output", help="出力APKパス")] = None,
    package_name: Annotated[str, typer.Option(help="Androidパッケージ名")] = "",
    app_name: Annotated[str, typer.Option(help="アプリ表示名")] = "",
    keystore: Annotated[Path | None, typer.Option(help="署名用キーストア")] = None,
    skip_video: Annotated[bool, typer.Option(help="動画変換をスキップ")] = False,
    verbose: Annotated[int, typer.Option("-v", "--verbose", count=True, help="詳細ログ出力")] = 0,
    quality: Annotated[str, typer.Option(help="画像品質プリセット")] = "high",
    clean: Annotated[bool, typer.Option(help="キャッシュをクリア")] = False,
    log_file: Annotated[Path | None, typer.Option(help="ログファイル出力先")] = None,
    ffmpeg_timeout: Annotated[int, typer.Option(help="FFmpegタイムアウト（秒）")] = 300,
    gradle_timeout: Annotated[int, typer.Option(help="Gradleタイムアウト（秒）")] = 1800,
    template_version: Annotated[str | None, typer.Option(help="テンプレートバージョン固定")] = None,
    template_refresh_days: Annotated[
        int, typer.Option(help="テンプレートキャッシュ期限（日）")
    ] = 7,
    template_offline: Annotated[bool, typer.Option(help="オフラインモード")] = False,
) -> None:
    """ゲームをAndroid APKにビルドする"""
    if output is None:
        output = Path(input_path).with_suffix(".apk")

    config = PipelineConfig(
        input_path=Path(input_path),
        output_path=output,
        package_name=package_name,
        app_name=app_name,
        keystore_path=keystore,
        skip_video=skip_video,
        quality=quality,
        clean_cache=clean,
        verbose_level=verbose,
        log_file=log_file,
        ffmpeg_timeout=ffmpeg_timeout,
        gradle_timeout=gradle_timeout,
        template_version=template_version,
        template_refresh_days=template_refresh_days,
        template_offline=template_offline,
    )

    pipeline = BuildPipeline(config)

    # 検証
    errors = pipeline.validate()
    if errors:
        for error in errors:
            console.print(f"[red]Error: {error}[/red]")
        raise typer.Exit(1)

    # 進捗コールバック
    def progress_callback(progress: PipelineProgress) -> None:
        if verbose > 0:
            console.print(
                f"[blue]{progress.phase.value}[/blue]: {progress.message} "
                f"({progress.current}/{progress.total})"
            )

    result = pipeline.run(progress_callback=progress_callback if verbose > 0 else None)

    if result.success:
        console.print(f"[green]ビルド完了: {result.output_path}[/green]")
        raise typer.Exit(0)
    else:
        console.print(f"[red]ビルド失敗: {result.error_message}[/red]")
        raise typer.Exit(1)


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
    path = Path(input_path)

    if not path.exists():
        console.print(f"[red]Error: パスが見つかりません: {input_path}[/red]")
        raise typer.Exit(1)

    if not path.is_dir():
        console.print(f"[red]Error: ディレクトリを指定してください: {input_path}[/red]")
        raise typer.Exit(1)

    game_info = analyze_game(path)

    table = Table(title="Game Info")
    table.add_column("Property", style="cyan")
    table.add_column("Value", style="green")

    table.add_row("Engine", game_info.engine)
    table.add_row("Encoding", game_info.detected_encoding if game_info.detected_encoding else "N/A")

    table.add_section()
    table.add_row("Scripts", f"{game_info.scripts.count} files")
    if game_info.scripts.extensions:
        table.add_row("  Extensions", ", ".join(game_info.scripts.extensions))
    table.add_row("  Total Size", _format_size(game_info.scripts.total_size_bytes))

    table.add_section()
    table.add_row("Images", f"{game_info.images.count} files")
    if game_info.images.extensions:
        table.add_row("  Extensions", ", ".join(game_info.images.extensions))
    table.add_row("  Total Size", _format_size(game_info.images.total_size_bytes))

    table.add_section()
    table.add_row("Audio", f"{game_info.audio.count} files")
    if game_info.audio.extensions:
        table.add_row("  Extensions", ", ".join(game_info.audio.extensions))
    table.add_row("  Total Size", _format_size(game_info.audio.total_size_bytes))

    table.add_section()
    table.add_row("Video", f"{game_info.video.count} files")
    if game_info.video.extensions:
        table.add_row("  Extensions", ", ".join(game_info.video.extensions))
    table.add_row("  Total Size", _format_size(game_info.video.total_size_bytes))

    console.print(table)
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
    console = Console()

    target = "テンプレートキャッシュ" if template_only else "すべてのキャッシュ"

    if not force:
        confirmed = typer.confirm(f"{target}を削除しますか?")
        if not confirmed:
            console.print("[yellow]キャンセルしました[/yellow]")
            raise typer.Exit(0)

    clear_cache(template_only=template_only)
    console.print(f"[green]{target}を削除しました[/green]")
    raise typer.Exit(0)


@cache_app.command("info")
def cache_info() -> None:
    """キャッシュ情報を表示する"""
    console = Console()
    info = get_cache_info()

    table = Table(title="キャッシュ情報", show_header=False)
    table.add_column("項目", style="cyan")
    table.add_column("値", style="white")

    table.add_row("ディレクトリ", str(info.directory))
    table.add_row("サイズ", _format_size(info.size_bytes))

    if info.template_version:
        table.add_row("テンプレートバージョン", info.template_version)
        if info.template_expires_in_days is not None:
            if info.template_expires_in_days > 0:
                table.add_row("有効期限", f"{info.template_expires_in_days}日後")
            else:
                table.add_row("有効期限", "[red]期限切れ[/red]")
    else:
        table.add_row("テンプレート", "[dim]なし[/dim]")

    console.print(Panel(table, border_style="blue"))
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
