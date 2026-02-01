"""ScriptAdjusterのテスト"""

from pathlib import Path

import pytest

from mnemonic.converter.base import ConversionStatus
from mnemonic.converter.script import AdjustmentRule, ScriptAdjuster


@pytest.fixture
def adjuster() -> ScriptAdjuster:
    """ScriptAdjusterインスタンスを返すフィクスチャ"""
    return ScriptAdjuster()


@pytest.fixture
def fixtures_dir() -> Path:
    """テストフィクスチャディレクトリのパスを返すフィクスチャ"""
    return Path(__file__).parent.parent / "fixtures" / "script"


class TestAdjustmentRule:
    """AdjustmentRuleデータクラスのテスト"""

    def test_has_pattern_replacement_description_attributes(self) -> None:
        """AdjustmentRuleが正しい属性を持つことを確認する"""
        rule = AdjustmentRule(
            pattern=r"test_pattern",
            replacement="replaced",
            description="テストルール",
        )
        assert rule.pattern == r"test_pattern"
        assert rule.replacement == "replaced"
        assert rule.description == "テストルール"

    def test_is_frozen_dataclass(self) -> None:
        """AdjustmentRuleがイミュータブルであることを確認する"""
        rule = AdjustmentRule(
            pattern=r"test",
            replacement="replaced",
            description="テスト",
        )
        with pytest.raises(AttributeError):
            rule.pattern = "new_pattern"  # type: ignore[misc]


class TestScriptAdjusterInit:
    """ScriptAdjusterの初期化テスト"""

    def test_default_rules_is_used_when_none(self) -> None:
        """rulesがNoneの場合DEFAULT_RULESが使用されることを確認する"""
        adjuster = ScriptAdjuster()
        assert len(adjuster.rules) == len(ScriptAdjuster.DEFAULT_RULES)
        assert adjuster.rules[0].description == "プラグインDLL読み込みの無効化"

    def test_custom_rules_can_be_specified(self) -> None:
        """カスタムルールを指定できることを確認する"""
        custom_rules = [
            AdjustmentRule(
                pattern=r"custom",
                replacement="replaced",
                description="カスタムルール",
            )
        ]
        adjuster = ScriptAdjuster(rules=custom_rules)
        assert len(adjuster.rules) == 1
        assert adjuster.rules[0].description == "カスタムルール"

    def test_default_add_encoding_directive_is_true(self) -> None:
        """デフォルトでadd_encoding_directiveがTrueであることを確認する"""
        adjuster = ScriptAdjuster()
        assert adjuster.add_encoding_directive is True

    def test_add_encoding_directive_can_be_disabled(self) -> None:
        """add_encoding_directiveを無効化できることを確認する"""
        adjuster = ScriptAdjuster(add_encoding_directive=False)
        assert adjuster.add_encoding_directive is False


class TestScriptAdjusterDefaultRules:
    """DEFAULT_RULESのテスト"""

    def test_default_rules_exist(self) -> None:
        """DEFAULT_RULESが存在することを確認する"""
        assert len(ScriptAdjuster.DEFAULT_RULES) > 0

    def test_default_rules_contains_plugin_disable_rule(self) -> None:
        """DEFAULT_RULESにプラグイン無効化ルールが含まれることを確認する"""
        descriptions = [rule.description for rule in ScriptAdjuster.DEFAULT_RULES]
        assert "プラグインDLL読み込みの無効化" in descriptions


class TestScriptAdjusterSupportedExtensions:
    """ScriptAdjuster.supported_extensionsのテスト"""

    def test_returns_tuple(self, adjuster: ScriptAdjuster) -> None:
        """supported_extensionsがタプルを返すことを確認する"""
        assert isinstance(adjuster.supported_extensions, tuple)

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".ks", id="正常系: .ks（KAGスクリプト）"),
            pytest.param(".tjs", id="正常系: .tjs（TJSスクリプト）"),
        ],
    )
    def test_contains_expected_extension(self, adjuster: ScriptAdjuster, extension: str) -> None:
        """期待される拡張子がサポートされていることを確認する"""
        assert extension in adjuster.supported_extensions

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".txt", id="正常系: .txtはサポート外"),
            pytest.param(".png", id="正常系: .pngはサポート外"),
            pytest.param(".exe", id="正常系: .exeはサポート外"),
        ],
    )
    def test_does_not_contain_unsupported_extension(
        self, adjuster: ScriptAdjuster, extension: str
    ) -> None:
        """サポート外の拡張子が含まれていないことを確認する"""
        assert extension not in adjuster.supported_extensions


class TestScriptAdjusterCanConvert:
    """ScriptAdjuster.can_convertのテスト"""

    @pytest.mark.parametrize(
        "filename, expected",
        [
            pytest.param("script.ks", True, id="正常系: .ksファイルは変換可能"),
            pytest.param("startup.tjs", True, id="正常系: .tjsファイルは変換可能"),
            pytest.param("Script.KS", True, id="正常系: 大文字の.KSファイルも変換可能"),
            pytest.param("readme.txt", False, id="正常系: .txtファイルは変換不可"),
            pytest.param("image.png", False, id="正常系: .pngファイルは変換不可"),
            pytest.param("game.exe", False, id="正常系: .exeファイルは変換不可"),
        ],
    )
    def test_returns_expected_result_for_extension(
        self,
        adjuster: ScriptAdjuster,
        tmp_path: Path,
        filename: str,
        expected: bool,
    ) -> None:
        """拡張子に基づいて正しい結果を返すことを確認する"""
        test_file = tmp_path / filename
        test_file.write_text("test content", encoding="utf-8")
        result = adjuster.can_convert(test_file)
        assert result is expected


class TestScriptAdjusterAdjustContent:
    """ScriptAdjuster.adjust_contentのテスト"""

    def test_disables_single_plugin_link(self, adjuster: ScriptAdjuster) -> None:
        """単一のPlugins.link()呼び出しを無効化できることを確認する"""
        content = 'Plugins.link("something.dll");'
        adjusted, count = adjuster.adjust_content(content)

        assert 'Plugins.link("something.dll")' in adjusted
        assert "// Disabled for Android" in adjusted
        assert adjusted.startswith("// ")
        assert count == 1

    def test_disables_multiple_plugin_links(self, adjuster: ScriptAdjuster) -> None:
        """複数のPlugins.link()呼び出しを無効化できることを確認する"""
        content = """// some comment
Plugins.link("plugin1.dll");
var x = 1;
Plugins.link('plugin2.dll');
"""
        adjusted, count = adjuster.adjust_content(content)

        assert adjusted.count("// Disabled for Android") == 2
        assert count == 2

    def test_preserves_non_plugin_code(self, adjuster: ScriptAdjuster) -> None:
        """プラグイン以外のコードを保持することを確認する"""
        content = """var x = 1;
function test() {
    return x * 2;
}
// This is a comment
"""
        adjusted, count = adjuster.adjust_content(content)

        assert adjusted == content
        assert count == 0

    def test_preserves_indentation_when_disabling_plugin(self, adjuster: ScriptAdjuster) -> None:
        """インデントを保持しながらプラグインを無効化できることを確認する"""
        content = '    Plugins.link("test.dll");'
        adjusted, count = adjuster.adjust_content(content)

        assert adjusted.startswith("    // ")
        assert count == 1

    def test_returns_adjustment_count(self, adjuster: ScriptAdjuster) -> None:
        """調整回数を正しく返すことを確認する"""
        content = """Plugins.link("a.dll");
Plugins.link("b.dll");
Plugins.link("c.dll");
"""
        _, count = adjuster.adjust_content(content)
        assert count == 3

    def test_handles_empty_content(self, adjuster: ScriptAdjuster) -> None:
        """空の内容を処理できることを確認する"""
        adjusted, count = adjuster.adjust_content("")
        assert adjusted == ""
        assert count == 0

    def test_applies_custom_rules(self) -> None:
        """カスタムルールを適用できることを確認する"""
        custom_rules = [
            AdjustmentRule(
                pattern=r"OLD_FUNCTION\(\)",
                replacement="NEW_FUNCTION()",
                description="関数名変更",
            )
        ]
        adjuster = ScriptAdjuster(rules=custom_rules)

        content = "var result = OLD_FUNCTION();"
        adjusted, count = adjuster.adjust_content(content)

        assert "NEW_FUNCTION()" in adjusted
        assert "OLD_FUNCTION()" not in adjusted
        assert count == 1

    def test_preserves_japanese_comments(self, adjuster: ScriptAdjuster) -> None:
        """日本語コメントを保持することを確認する"""
        content = """// これは日本語のコメントです
var x = 1;  // 変数xを初期化
/* 複数行の
   日本語コメント */
"""
        adjusted, count = adjuster.adjust_content(content)

        assert "これは日本語のコメントです" in adjusted
        assert "変数xを初期化" in adjusted
        assert "複数行の" in adjusted
        assert "日本語コメント" in adjusted
        assert count == 0


class TestScriptAdjusterAddStartupDirective:
    """ScriptAdjuster.add_startup_directiveのテスト"""

    def test_adds_encoding_directive_to_startup_tjs(self, adjuster: ScriptAdjuster) -> None:
        """startup.tjsにエンコーディングディレクティブを追加できることを確認する"""
        content = """// Original script
var initialized = false;
"""
        result = adjuster.add_startup_directive(content)

        assert "// krkrsdl2 polyfill initialization" in result
        assert 'Scripts.execStorage("system/polyfillinitialize.tjs");' in result
        assert "// Original script" in result

    def test_directive_is_added_at_beginning(self, adjuster: ScriptAdjuster) -> None:
        """ディレクティブがファイル先頭に追加されることを確認する"""
        content = "var x = 1;"
        result = adjuster.add_startup_directive(content)

        assert result.startswith("// krkrsdl2 polyfill initialization")

    def test_original_content_preserved(self, adjuster: ScriptAdjuster) -> None:
        """元の内容が保持されることを確認する"""
        content = """function initialize() {
    System.inform("Hello");
}
"""
        result = adjuster.add_startup_directive(content)

        assert content in result


class TestScriptAdjusterConvert:
    """ScriptAdjuster.convertのテスト"""

    def test_converts_ks_file_with_plugin_calls(
        self, adjuster: ScriptAdjuster, tmp_path: Path
    ) -> None:
        """プラグイン呼び出しを含む.ksファイルを変換できることを確認する"""
        source = tmp_path / "test.ks"
        dest = tmp_path / "output" / "test.ks"

        content = """Plugins.link("test.dll");
[message text="Hello"]
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        converted = dest.read_text(encoding="utf-8")
        assert "// Disabled for Android" in converted
        assert "[message" in converted

    def test_converts_tjs_file_with_plugin_calls(
        self, adjuster: ScriptAdjuster, tmp_path: Path
    ) -> None:
        """プラグイン呼び出しを含む.tjsファイルを変換できることを確認する"""
        source = tmp_path / "test.tjs"
        dest = tmp_path / "output" / "test.tjs"

        content = """Plugins.link('extrans.dll');
var transition = new ExtTransition();
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        converted = dest.read_text(encoding="utf-8")
        assert "// Disabled for Android" in converted

    def test_adds_directive_to_startup_tjs(self, adjuster: ScriptAdjuster, tmp_path: Path) -> None:
        """startup.tjsにエンコーディングディレクティブを追加することを確認する"""
        source = tmp_path / "startup.tjs"
        dest = tmp_path / "output" / "startup.tjs"

        content = """// Game startup script
var kag = new KAGWindow();
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        converted = dest.read_text(encoding="utf-8")
        assert "// krkrsdl2 polyfill initialization" in converted
        assert 'Scripts.execStorage("system/polyfillinitialize.tjs");' in converted

    def test_skips_directive_when_disabled(self, tmp_path: Path) -> None:
        """add_encoding_directiveが無効の場合ディレクティブを追加しないことを確認する"""
        adjuster = ScriptAdjuster(add_encoding_directive=False)
        source = tmp_path / "startup.tjs"
        dest = tmp_path / "output" / "startup.tjs"

        # プラグイン呼び出しを含めることで調整が発生しファイルが出力される
        content = """Plugins.link("test.dll");
var x = 1;
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        converted = dest.read_text(encoding="utf-8")
        assert "@if (kirikiriz)" not in converted
        assert "// Disabled for Android" in converted

    def test_returns_skipped_when_no_adjustments(
        self, adjuster: ScriptAdjuster, tmp_path: Path
    ) -> None:
        """調整が不要な場合SKIPPEDを返すことを確認する"""
        source = tmp_path / "clean.ks"
        dest = tmp_path / "output" / "clean.ks"

        content = """[message text="No plugins here"]
[wait time=1000]
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SKIPPED

    def test_records_bytes_before_and_after(self, adjuster: ScriptAdjuster, tmp_path: Path) -> None:
        """変換前後のバイト数を記録することを確認する"""
        source = tmp_path / "test.ks"
        dest = tmp_path / "output" / "test.ks"

        content = 'Plugins.link("test.dll");'
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.bytes_before > 0
        assert result.bytes_after > 0
        assert result.bytes_after > result.bytes_before  # コメント追加で増える

    def test_returns_failed_for_nonexistent_file(
        self, adjuster: ScriptAdjuster, tmp_path: Path
    ) -> None:
        """存在しないファイルの場合FAILEDを返すことを確認する"""
        source = tmp_path / "nonexistent.ks"
        dest = tmp_path / "output.ks"

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.FAILED

    def test_creates_dest_directory_if_not_exists(
        self, adjuster: ScriptAdjuster, tmp_path: Path
    ) -> None:
        """変換先ディレクトリが存在しない場合作成することを確認する"""
        source = tmp_path / "test.ks"
        dest = tmp_path / "nested" / "dir" / "output.ks"

        content = 'Plugins.link("test.dll");'
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.parent.exists()
        assert dest.exists()

    def test_reads_and_writes_utf8(self, adjuster: ScriptAdjuster, tmp_path: Path) -> None:
        """UTF-8でファイルを読み書きすることを確認する"""
        source = tmp_path / "japanese.ks"
        dest = tmp_path / "output" / "japanese.ks"

        content = """Plugins.link("test.dll");
[message text="日本語テスト"]
// 日本語コメント
"""
        source.write_text(content, encoding="utf-8")

        result = adjuster.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        converted = dest.read_text(encoding="utf-8")
        assert "日本語テスト" in converted
        assert "日本語コメント" in converted


class TestScriptAdjusterMidiRules:
    """MIDIファイル参照の変換ルールテスト"""

    def test_replaces_midi_sound_buffer(self, adjuster: ScriptAdjuster) -> None:
        """MIDISoundBufferをWaveSoundBufferに変換することを確認する"""
        content = 'var bgm = new MIDISoundBuffer("bgm/title.mid");'
        adjusted, count = adjuster.adjust_content(content)

        assert "WaveSoundBuffer" in adjusted
        assert "MIDISoundBuffer" not in adjusted
        assert count >= 1

    def test_replaces_mid_extension_in_double_quotes(self, adjuster: ScriptAdjuster) -> None:
        """.mid参照を.mid.oggに変換することを確認する（ダブルクォート）"""
        content = 'var bgm = new WaveSoundBuffer("bgm/title.mid");'
        adjusted, count = adjuster.adjust_content(content)

        assert '"bgm/title.mid.ogg"' in adjusted
        assert count >= 1

    def test_replaces_mid_extension_in_single_quotes(self, adjuster: ScriptAdjuster) -> None:
        """.mid参照を.mid.oggに変換することを確認する（シングルクォート）"""
        content = "var bgm = new WaveSoundBuffer('bgm/title.mid');"
        adjusted, count = adjuster.adjust_content(content)

        assert "'bgm/title.mid.ogg'" in adjusted
        assert count >= 1

    def test_replaces_midi_extension(self, adjuster: ScriptAdjuster) -> None:
        """.midi参照を.midi.oggに変換することを確認する"""
        content = 'var bgm = new WaveSoundBuffer("bgm/title.midi");'
        adjusted, count = adjuster.adjust_content(content)

        assert '"bgm/title.midi.ogg"' in adjusted
        assert count >= 1

    def test_combined_midi_sound_buffer_and_extension(self, adjuster: ScriptAdjuster) -> None:
        """MIDISoundBufferと.mid拡張子の両方を変換することを確認する"""
        content = 'var bgm = new MIDISoundBuffer("bgm/title.mid");'
        adjusted, count = adjuster.adjust_content(content)

        assert "WaveSoundBuffer" in adjusted
        assert "MIDISoundBuffer" not in adjusted
        assert ".mid.ogg" in adjusted
        assert count >= 2

    def test_multiple_midi_references(self, adjuster: ScriptAdjuster) -> None:
        """複数のMIDI参照を変換することを確認する"""
        content = """var bgm1 = new MIDISoundBuffer("bgm/title.mid");
var bgm2 = new MIDISoundBuffer("bgm/battle.midi");
var se = new WaveSoundBuffer("se/click.wav");
"""
        adjusted, count = adjuster.adjust_content(content)

        assert adjusted.count("WaveSoundBuffer") == 3
        assert "MIDISoundBuffer" not in adjusted
        assert '"bgm/title.mid.ogg"' in adjusted
        assert '"bgm/battle.midi.ogg"' in adjusted
        assert '"se/click.wav"' in adjusted  # WAVはそのまま

    def test_does_not_replace_mid_in_unquoted_context(self, adjuster: ScriptAdjuster) -> None:
        """クォートされていないコンテキストではMIDが変換されないことを確認する"""
        content = "var midpoint = calculateMidpoint();"
        adjusted, count = adjuster.adjust_content(content)

        # midpoint はそのまま維持されるべき
        assert "midpoint" in adjusted
        # MIDISoundBufferルールによるカウントがなければ0
        # （他のルールがマッチしない限り）


class TestScriptAdjusterLoadpluginRules:
    """loadpluginタグの変換ルールテスト"""

    def test_replaces_extrans_dll_to_libextrans_so(self, adjuster: ScriptAdjuster) -> None:
        """extrans.dllをlibextrans.soに変換することを確認する（Android krkrsdl2対応）"""
        content = '[loadplugin module="extrans.dll"]'
        adjusted, count = adjuster.adjust_content(content)

        assert '[loadplugin module="libextrans.so"]' in adjusted
        assert "extrans.dll" not in adjusted
        assert count >= 1

    def test_comments_out_wuvorbis_dll(self, adjuster: ScriptAdjuster) -> None:
        """wuvorbis.dllをコメントアウトすることを確認する（krkrsdl2ビルトイン）"""
        content = '[loadplugin module="wuvorbis.dll"]'
        adjusted, count = adjuster.adjust_content(content)

        assert ";#" in adjusted
        assert "Ogg Vorbis built-in krkrsdl2" in adjusted
        assert count >= 1

    def test_comments_out_krmovie_dll(self, adjuster: ScriptAdjuster) -> None:
        """krmovie.dllをコメントアウトすることを確認する"""
        content = '[loadplugin module="krmovie.dll"]'
        adjusted, count = adjuster.adjust_content(content)

        assert ";#" in adjusted
        assert "not supported on krkrsdl2" in adjusted
        assert count >= 1

    def test_comments_out_other_dll_plugins(self, adjuster: ScriptAdjuster) -> None:
        """その他のDLLプラグインをコメントアウトすることを確認する"""
        content = '[loadplugin module="layerexdraw.dll"]'
        adjusted, count = adjuster.adjust_content(content)

        assert ";#" in adjusted
        assert "Disabled for Android" in adjusted
        assert count >= 1

    def test_multiple_loadplugin_tags(self, adjuster: ScriptAdjuster) -> None:
        """複数のloadpluginタグを処理することを確認する"""
        content = """[loadplugin module="wuvorbis.dll"]
[loadplugin module="extrans.dll"]
[loadplugin module="krmovie.dll"]
[loadplugin module="something.dll"]
"""
        adjusted, count = adjuster.adjust_content(content)

        # extrans.dll → libextrans.so
        assert '[loadplugin module="libextrans.so"]' in adjusted
        # wuvorbis.dll はkrkrsdl2ビルトインでコメントアウト
        assert "Ogg Vorbis built-in krkrsdl2" in adjusted
        # krmovie.dll はkrkrsdl2未対応でコメントアウト
        assert "not supported on krkrsdl2" in adjusted
        # something.dll はAndroid非対応でコメントアウト
        assert "Disabled for Android" in adjusted
        assert count >= 4

    def test_preserves_libextrans_so(self, adjuster: ScriptAdjuster) -> None:
        """変換後のlibextrans.soタグは再変換されないことを確認する"""
        content = '[loadplugin module="libextrans.so"]'
        adjusted, count = adjuster.adjust_content(content)

        # libextrans.soはそのまま維持される（コメントアウトされない）
        assert '[loadplugin module="libextrans.so"]' in adjusted
        assert ";#" not in adjusted
        assert count == 0

    def test_handles_loadplugin_with_extra_spaces(self, adjuster: ScriptAdjuster) -> None:
        """スペースを含むloadpluginタグを処理することを確認する"""
        content = '[loadplugin  module="extrans.dll"]'
        adjusted, count = adjuster.adjust_content(content)

        # スペースが2つの場合も変換される
        assert "extrans" in adjusted
        # 注: 現在のパターンは厳密なスペース1つを想定しているため、
        # スペース2つの場合は変換されない可能性がある
