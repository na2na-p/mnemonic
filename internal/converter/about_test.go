package converter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// gameAboutScenario は実ゲームのscenario/about.ks(Shift_JIS)をUTF-8へ変換した
// 内容そのもの。CRLF改行と行末の"\"もそのまま残している。
const gameAboutScenario = "*set\r\n" +
	"[title name=\"バージョン情報\"]\\\r\n" +
	"[image storage=\"ヘルプ\" layer=base page=fore]\\\r\n" +
	"[position left=0 top=0 width=320 height=200 page=fore opacity=0 marginl=10 margint=1 marginr=10 marginb=10 color=0xffffffff]\\\r\n" +
	"[nowait]\\\r\n" +
	"[font size=20 shadow=false face=\"ＭＳ 明朝\" color=0x98a1c9]\\\r\n" +
	"[style align=right]\\\r\n" +
	";バージョンアップの際はここをいじりましょう--------------------------------\r\n" +
	"Ver.1.05\r\n" +
	";--------------------------------------------------------------------------\r\n" +
	"[resetstyle]\\\r\n" +
	"[resetfont]\\\r\n" +
	"[endnowait]\\\r\n" +
	"[s]"

func TestExtractAboutDialog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scenario string
		want     converter.AboutDialog
	}{
		{
			name:     "実ゲームのabout.ksから本文とキャプションを抜き出す",
			scenario: gameAboutScenario,
			want:     converter.AboutDialog{Text: "Ver.1.05", Caption: "バージョン情報"},
		},
		{
			name:     "BOM付きでも先頭行のラベルを読み飛ばす",
			scenario: "\uFEFF*start\nabc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "コメント行とラベル行を読み飛ばす",
			scenario: ";comment\n*label|page\nabc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "行末は改行になる",
			scenario: "line1\nline2\n",
			want:     converter.AboutDialog{Text: "line1\nline2"},
		},
		{
			name:     "空行も改行として残す",
			scenario: "line1\n\nline2\n",
			want:     converter.AboutDialog{Text: "line1\n\nline2"},
		},
		{
			name:     "行末の\\は改行を打ち消し自身も表示しない",
			scenario: "line1\\\nline2\n",
			want:     converter.AboutDialog{Text: "line1line2"},
		},
		{
			name:     "行中の\\はそのまま表示する",
			scenario: "C:\\game\n",
			want:     converter.AboutDialog{Text: "C:\\game"},
		},
		{
			name:     "[p]で終わる行は改行しない",
			scenario: "page1[p]\npage2\n",
			want:     converter.AboutDialog{Text: "page1page2"},
		},
		{
			name:     "CRだけの改行も行の区切りとして扱う",
			scenario: "line1\\\rline2\rline3",
			want:     converter.AboutDialog{Text: "line1line2\nline3"},
		},
		{
			name:     "実行時に評価するembとマクロ呼び出しは表示しない",
			scenario: "a[emb exp=\"f.version\"]b[mymacro]c\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "ifとignoreの条件は評価せずどの分岐の文字も残す",
			scenario: "[if exp=\"f.a\"]A[else]B[endif][ignore exp=\"true\"]C[endignore]\n",
			want:     converter.AboutDialog{Text: "ABC"},
		},
		{
			name:     "macro定義の中の文字もそのまま残す",
			scenario: "[macro name=m]M[endmacro]x\n",
			want:     converter.AboutDialog{Text: "Mx"},
		},
		{
			name:     "[r]タグは改行になる",
			scenario: "a[r]b\n",
			want:     converter.AboutDialog{Text: "a\nb"},
		},
		{
			name:     "[[は[として表示する",
			scenario: "[[注]本文\n",
			want:     converter.AboutDialog{Text: "[注]本文"},
		},
		{
			name:     "タグは表示しない",
			scenario: "a[font size=20]b[l]c\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "引用符内の]やスペースを含むタグ全体を読み飛ばす",
			scenario: "a[font face=\"x ]y\" size=2]b\n",
			want:     converter.AboutDialog{Text: "ab"},
		},
		{
			name:     "閉じていないタグは行末まで捨てる",
			scenario: "a[font size=2\nb\n",
			want:     converter.AboutDialog{Text: "a\nb"},
		},
		{
			name:     "@で始まる行はタグとして扱い改行もしない",
			scenario: "@font size=20\nabc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "行頭のタブと行中のタブを表示しない",
			scenario: "\t\t;comment\n\t*label\n\ta\tb\n",
			want:     converter.AboutDialog{Text: "ab"},
		},
		{
			name:     "iscriptからendscriptまでを読み飛ばす",
			scenario: "a\n[iscript]\nvar x = \"not shown\";\n[endscript]\n@iscript\ny\n@endscript\n[iscript]\\\nz\n[endscript]\\\nb\n",
			want:     converter.AboutDialog{Text: "a\nb"},
		},
		{
			name:     "[s]より後ろは表示しない",
			scenario: "shown\n[s]\nhidden\n",
			want:     converter.AboutDialog{Text: "shown"},
		},
		{
			name:     "@sより後ろは表示しない",
			scenario: "shown\n@s\nhidden\n",
			want:     converter.AboutDialog{Text: "shown"},
		},
		{
			name:     "@titleからもキャプションを得る",
			scenario: "@title name=\"About\"\nabc\n",
			want:     converter.AboutDialog{Text: "abc", Caption: "About"},
		},
		{
			name:     "単一引用符と引用符なしのtitleも読む",
			scenario: "[title name='A b'][title name=Final]abc\n",
			want:     converter.AboutDialog{Text: "abc", Caption: "Final"},
		},
		{
			name:     "&で始まるtitleは式のため評価できずキャプションを空にする",
			scenario: "[title name=&f.title]abc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "引用符で囲んでも&で始まるtitleは式として扱う",
			scenario: "[title name=\"&f.title\"]abc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "%で始まるtitleはマクロ引数のためキャプションを空にする",
			scenario: "[title name=%title]abc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "属性値の`は次の1文字をそのまま含める",
			scenario: "[title name=\"say `\"hi`\"\"]abc\n",
			want:     converter.AboutDialog{Text: "abc", Caption: "say \"hi\""},
		},
		{
			name:     "引用符の無い属性値中の'は区切りにならない",
			scenario: "[title name=it's]abc\n",
			want:     converter.AboutDialog{Text: "abc", Caption: "it's"},
		},
		{
			name:     "値を省略した属性があってもtitleを読む",
			scenario: "[title cond name=\"About\"]abc\n",
			want:     converter.AboutDialog{Text: "abc", Caption: "About"},
		},
		{
			name:     "titleタグが無ければキャプションは空",
			scenario: "abc\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
		{
			name:     "表示する文字が無ければ本文は空",
			scenario: "*start\n[title name=\"About\"]\\\n[s]\n",
			want:     converter.AboutDialog{Caption: "About"},
		},
		{
			name:     "前後の空白と改行を取り除く",
			scenario: "\n  abc  \n\n",
			want:     converter.AboutDialog{Text: "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, converter.ExtractAboutDialog(tt.scenario))
		})
	}
}

func TestLoadAboutDialog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		want  converter.AboutDialog
	}{
		{
			name:  "about.ksが無ければゼロ値を返す",
			files: map[string]string{"scenario/first.ks": "abc\n"},
			want:  converter.AboutDialog{},
		},
		{
			name:  "サブディレクトリのabout.ksを読む",
			files: map[string]string{"scenario/about.ks": "\uFEFF[title name=\"About\"]abc\n"},
			want:  converter.AboutDialog{Text: "abc", Caption: "About"},
		},
		{
			name:  "ファイル名の大文字小文字を区別しない",
			files: map[string]string{"scenario/About.KS": "abc\n"},
			want:  converter.AboutDialog{Text: "abc"},
		},
		{
			name: "複数あっても表示内容が同じならその内容を返す",
			files: map[string]string{
				"scenario/about.ks": "abc\n",
				"others/about.ks":   ";comment\nabc\n",
			},
			want: converter.AboutDialog{Text: "abc"},
		},
		{
			name: "表示内容の異なる複数のabout.ksがあればゼロ値を返す",
			files: map[string]string{
				"scenario/about.ks": "abc\n",
				"others/about.ks":   "xyz\n",
			},
			want: converter.AboutDialog{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			for rel, content := range tt.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				mkdirAll(t, filepath.Dir(path))
				writeFile(t, path, []byte(content))
			}

			got, err := converter.LoadAboutDialog(root)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("UTF-8として読めないabout.ksはErrScriptNotUTF8を返す", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "about.ks"), []byte{0x82, 0xa0, 0xff})

		_, err := converter.LoadAboutDialog(root)
		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrScriptNotUTF8)
	})

	t.Run("存在しないディレクトリはエラーを返す", func(t *testing.T) {
		t.Parallel()

		_, err := converter.LoadAboutDialog(filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

// gameAboutHandler は実ゲームのsystem/MainWindow.tjs(Shift_JISをUTF-8へ変換)に
// あるKAG3標準の「このソフトについて」ハンドラと前後の関数そのもの。
const gameAboutHandler = "\tfunction onHelpIndexMenuItemClick(sender)\r\n" +
	"\t{\r\n" +
	"\t\t// ヘルプファイルを開く\r\n" +
	"\t\tSystem.shellExecute(Storages.getLocalName(System.exePath) + helpFile);\r\n" +
	"\t}\r\n" +
	"\r\n" +
	"\tfunction onHelpAboutMenuItemClick(sender)\r\n" +
	"\t{\r\n" +
	"\t\t// 「このソフトについて」ウィンドウを表示\r\n" +
	"\t\tvar win = new global.KAGWindow(false, aboutWidth, aboutHeight);\r\n" +
	"\t\twin.setPos(left + ((width - win.width)>>1), top + ((height - win.height)>>1));\r\n" +
	"\t\twin.process('about.ks' ,,, true); // about.ks を immediate で表示\r\n" +
	"\t\twin.showModal(); // モード付きで表示\r\n" +
	"\t\tinvalidate win;\r\n" +
	"\t}\r\n" +
	"\r\n" +
	"\tfunction onReloadScenarioMenuItemClick(sender)\r\n" +
	"\t{\r\n"

// upstreamAboutHandler は上流kag3のMainWindow.tjs(908-916行)と同じ本文。
const upstreamAboutHandler = "\tfunction onHelpAboutMenuItemClick(sender)\n" +
	"\t{\n" +
	"\t\t// 「このソフトについて」ウィンドウを表示\n" +
	"\t\tvar win = new global.KAGWindow(false, aboutWidth, aboutHeight);\n" +
	"\t\twin.setPos(left + ((width - win.width)>>1), top + ((height - win.height)>>1));\n" +
	"\t\twin.process('about.ks' ,,, true); // about.ks を immediate で表示\n" +
	"\t\twin.showModal(); // モード付きで表示\n" +
	"\t\tinvalidate win;\n" +
	"\t}\n"

func TestScriptAdjuster_ApplyMainWindowCompat_AboutHandler(t *testing.T) {
	t.Parallel()

	gameAbout := converter.AboutDialog{Text: "Ver.1.05", Caption: "バージョン情報"}

	t.Run("実ゲームのハンドラをSystem.informに置き換えCRLFと前後の関数を保つ", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(gameAbout)
		got, count := adjuster.ApplyMainWindowCompat(gameAboutHandler)

		assert.Equal(t, 1, count)
		assert.NotContains(t, got, "KAGWindow")
		assert.NotContains(t, got, "showModal")
		assert.True(t, strings.HasPrefix(got, "\tfunction onHelpIndexMenuItemClick(sender)\r\n"))
		assert.Contains(t, got, "\tfunction onHelpAboutMenuItemClick(sender)\r\n\t{\r\n")
		assert.Contains(t, got, "\t\tSystem.inform(\"Ver.1.05\", \"バージョン情報\");\r\n\r\n\r\n\r\n\t}\r\n\r\n\tfunction onReloadScenarioMenuItemClick(sender)\r\n")
		assert.NotContains(t, strings.ReplaceAll(got, "\r\n", ""), "\n")
	})

	t.Run("上流kag3のハンドラも置き換える", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(gameAbout)
		got, count := adjuster.ApplyMainWindowCompat(upstreamAboutHandler)

		assert.Equal(t, 1, count)
		assert.NotContains(t, got, "KAGWindow")
		assert.True(t, strings.HasPrefix(got, "\tfunction onHelpAboutMenuItemClick(sender)\n\t{\n"))
		assert.True(t, strings.HasSuffix(got, "\t\tSystem.inform(\"Ver.1.05\", \"バージョン情報\");\n\n\n\n\t}\n"))
		assert.NotContains(t, got, "\r")
	})

	t.Run("空白やコメントの違いは許容する", func(t *testing.T) {
		t.Parallel()

		content := "function onHelpAboutMenuItemClick( sender ) {\n" +
			"  /* 表示 */ var win=new global.KAGWindow(false,aboutWidth,aboutHeight);\n" +
			"  win.setPos(left+((width-win.width)>>1),top+((height-win.height)>>1));\n" +
			"  win.process(\"about.ks\",,,true);\n" +
			"  win.showModal();\n" +
			"  invalidate win; // 後始末\n" +
			"}\n"

		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(gameAbout)
		got, count := adjuster.ApplyMainWindowCompat(content)

		assert.Equal(t, 1, count)
		assert.Contains(t, got, "System.inform(\"Ver.1.05\", \"バージョン情報\");")
		assert.NotContains(t, got, "KAGWindow")
	})

	t.Run("本文に$や引用符があってもそのままリテラルにする", func(t *testing.T) {
		t.Parallel()

		about := converter.AboutDialog{Text: "price $1 ${2} \"x\" \\", Caption: "c$0"}
		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(about)
		got, count := adjuster.ApplyMainWindowCompat(upstreamAboutHandler)

		assert.Equal(t, 1, count)
		assert.Contains(t, got, `System.inform("price $1 ${2} \"x\" \\", "c$0");`)
	})

	t.Run("本文が複数行なら\\nで連結したリテラルにする", func(t *testing.T) {
		t.Parallel()

		about := converter.AboutDialog{Text: "a\nb", Caption: "c"}
		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(about)
		got, _ := adjuster.ApplyMainWindowCompat(upstreamAboutHandler)

		assert.Contains(t, got, `System.inform("a\nb", "c");`)
	})

	fallbacks := []struct {
		name  string
		about converter.AboutDialog
		want  string
	}{
		{
			name:  "about.ksが無ければ本文もキャプションもSystem.titleにする",
			about: converter.AboutDialog{},
			want:  "System.inform(System.title, System.title);",
		},
		{
			name:  "本文が空ならキャプションを本文にも使う",
			about: converter.AboutDialog{Caption: "About"},
			want:  `System.inform("About", "About");`,
		},
		{
			name:  "キャプションが空ならSystem.titleをキャプションにする",
			about: converter.AboutDialog{Text: "abc"},
			want:  `System.inform("abc", System.title);`,
		},
	}
	for _, tt := range fallbacks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(tt.about)
			got, count := adjuster.ApplyMainWindowCompat(upstreamAboutHandler)

			assert.Equal(t, 1, count)
			assert.Contains(t, got, tt.want)
		})
	}

	t.Run("WithAboutDialogを使わなくてもSystem.titleで置き換える", func(t *testing.T) {
		t.Parallel()

		got, count := converter.NewScriptAdjuster(nil, true).ApplyMainWindowCompat(upstreamAboutHandler)

		assert.Equal(t, 1, count)
		assert.Contains(t, got, "System.inform(System.title, System.title);")
	})

	unchanged := []struct {
		name    string
		content string
	}{
		{
			name:    "別のシナリオを開くハンドラは置き換えない",
			content: strings.Replace(upstreamAboutHandler, "'about.ks'", "'credit.ks'", 1),
		},
		{
			name:    "文が追加されたハンドラは置き換えない",
			content: strings.Replace(upstreamAboutHandler, "\t\tinvalidate win;\n", "\t\tinvalidate win;\n\t\tkag.process('title.ks');\n", 1),
		},
		{
			name:    "invalidateが無いハンドラは置き換えない",
			content: strings.Replace(upstreamAboutHandler, "\t\tinvalidate win;\n", "", 1),
		},
		{
			name:    "別名の関数は置き換えない",
			content: strings.Replace(upstreamAboutHandler, "onHelpAboutMenuItemClick", "onHelpAboutMenuItemClick2", 1),
		},
		{
			name:    "コメントアウトされたハンドラは置き換えない",
			content: "// " + strings.ReplaceAll(upstreamAboutHandler, "\n", "\n// "),
		},
	}
	for _, tt := range unchanged {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(gameAbout)
			got, count := adjuster.ApplyMainWindowCompat(tt.content)

			assert.Equal(t, 0, count)
			assert.Equal(t, tt.content, got)
		})
	}

	lineCounts := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "実ゲームのハンドラを置き換えても行数を保つ",
			content: gameAboutHandler,
			want:    "\t\tSystem.inform(\"Ver.1.05\", \"バージョン情報\");\r\n",
		},
		{
			name:    "上流kag3のハンドラを置き換えても行数を保つ",
			content: upstreamAboutHandler,
			want:    "\t\tSystem.inform(\"Ver.1.05\", \"バージョン情報\");\n",
		},
		{
			name: "1行に書かれたハンドラは1行のまま置き換える",
			content: "x;\nfunction onHelpAboutMenuItemClick(sender) { var win = new global.KAGWindow(false, aboutWidth, aboutHeight); " +
				"win.setPos(left + ((width - win.width)>>1), top + ((height - win.height)>>1)); " +
				"win.process('about.ks',,,true); win.showModal(); invalidate win; }\ny;\n",
			want: "x;\nfunction onHelpAboutMenuItemClick(sender) { System.inform(\"Ver.1.05\", \"バージョン情報\"); }\ny;\n",
		},
		{
			name: "3行に書かれたハンドラも行数を保つ",
			content: "function onHelpAboutMenuItemClick(sender) {\n" +
				"var win = new global.KAGWindow(false, aboutWidth, aboutHeight); win.setPos(left + ((width - win.width)>>1), top + ((height - win.height)>>1));\n" +
				"win.process('about.ks',,,true); win.showModal(); invalidate win;\n}\ny;\n",
			want: "System.inform(\"Ver.1.05\", \"バージョン情報\");",
		},
	}
	for _, tt := range lineCounts {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(gameAbout)
			got, count := adjuster.ApplyMainWindowCompat(tt.content)

			assert.Equal(t, 1, count)
			assert.Contains(t, got, tt.want)
			assert.Equal(t, strings.Count(tt.content, "\n"), strings.Count(got, "\n"))
			assert.NotContains(t, got, "KAGWindow")
		})
	}

	t.Run("WithAboutDialogは元のScriptAdjusterを変更しない", func(t *testing.T) {
		t.Parallel()

		base := converter.NewScriptAdjuster(nil, true)
		_ = base.WithAboutDialog(gameAbout)
		got, _ := base.ApplyMainWindowCompat(upstreamAboutHandler)

		assert.Contains(t, got, "System.inform(System.title, System.title);")
	})
}

func TestScriptAdjuster_Convert_AboutHandler(t *testing.T) {
	t.Parallel()

	t.Run("mainwindow.tjsのハンドラだけを置き換える", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mainWindow := filepath.Join(dir, "MainWindow.tjs")
		other := filepath.Join(dir, "other.tjs")
		writeFile(t, mainWindow, []byte("\uFEFF"+upstreamAboutHandler))
		writeFile(t, other, []byte("\uFEFF"+upstreamAboutHandler))

		adjuster := converter.NewScriptAdjuster(nil, true).WithAboutDialog(converter.AboutDialog{Text: "abc"})

		result, err := adjuster.Convert(mainWindow, mainWindow)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Contains(t, string(readFile(t, mainWindow)), `System.inform("abc", System.title);`)

		result, err = adjuster.Convert(other, other)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
		assert.Contains(t, string(readFile(t, other)), "KAGWindow")
	})
}
