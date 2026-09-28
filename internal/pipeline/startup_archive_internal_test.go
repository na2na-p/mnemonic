package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveStartupArchive(t *testing.T) {
	t.Parallel()

	archive := storedXP3Bytes("startup.tjs", []byte("// data"))
	embeddedExe := append([]byte("MZ-stub-16-bytes"), storedXP3Bytes("startup.tjs", []byte("// embedded"))...)
	plainExe := []byte("MZ-stub-16-bytes-without-archive")

	tests := []struct {
		name         string
		input        string
		files        map[string][]byte
		dirs         []string
		wantPath     string
		wantEmbedded bool
		wantAdjacent bool
		wantUnused   bool
		wantWindows  string
		wantIgnored  []string
		wantErr      string
	}{
		{
			name:     "正常系: XP3の入力はそのまま展開する",
			input:    "data.xp3",
			files:    map[string][]byte{"data.xp3": archive},
			wantPath: "data.xp3",
		},
		{
			name:  "正常系: XP3の入力と同じフォルダの他の.xp3ファイルだけを名前順に読まないものとして返す",
			input: "data.xp3",
			files: map[string][]byte{
				"data.xp3":   archive,
				"voice.XP3":  archive,
				"patch.xp3":  archive,
				"readme.txt": []byte("readme"),
			},
			dirs:        []string{"bgm.xp3"},
			wantPath:    "data.xp3",
			wantIgnored: []string{"patch.xp3", "voice.XP3"},
		},
		{
			name:         "正常系: data.xp3が無ければEXEの埋め込みXP3を展開する",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe},
			wantPath:     "game.exe",
			wantEmbedded: true,
		},
		{
			name:         "正常系: XP3を埋め込んでいないEXEは同じフォルダのdata.xp3を展開する",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": plainExe, "data.xp3": archive},
			wantPath:     "data.xp3",
			wantAdjacent: true,
		},
		{
			name:         "正常系: EXEがXP3を埋め込んでいても同じフォルダのdata.xp3を優先する",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "data.xp3": archive},
			wantPath:     "data.xp3",
			wantAdjacent: true,
			wantUnused:   true,
		},
		{
			name:         "正常系: 拡張子が大文字のEXEでもdata.xp3を探す",
			input:        "GAME.EXE",
			files:        map[string][]byte{"GAME.EXE": plainExe, "data.xp3": archive},
			wantPath:     "data.xp3",
			wantAdjacent: true,
		},
		{
			name:         "正常系: data.xp3の名前は大文字小文字を区別せずに探す",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": plainExe, "Data.XP3": archive},
			wantPath:     "Data.XP3",
			wantAdjacent: true,
		},
		{
			name:         "正常系: data.xp3の中身がXP3でなくても埋め込みXP3に戻らずdata.xp3を選ぶ",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "data.xp3": []byte("not an archive")},
			wantPath:     "data.xp3",
			wantAdjacent: true,
			wantUnused:   true,
		},
		{
			name:         "正常系: data.xp3という名前のフォルダは選ばず、読まないものにも数えない",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe},
			dirs:         []string{"data.xp3"},
			wantPath:     "game.exe",
			wantEmbedded: true,
		},
		{
			name:  "正常系: data.xp3を選んだ場合はそれ以外の.xp3ファイルを読まないものとして返す",
			input: "game.exe",
			files: map[string][]byte{
				"game.exe":  plainExe,
				"data.xp3":  archive,
				"voice.xp3": archive,
				"patch.xp3": archive,
			},
			wantPath:     "data.xp3",
			wantAdjacent: true,
			wantIgnored:  []string{"patch.xp3", "voice.xp3"},
		},
		{
			name:         "正常系: 埋め込みXP3を選んだ場合は同じフォルダの.xp3ファイルをすべて読まないものとして返す",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "patch.xp3": archive},
			wantPath:     "game.exe",
			wantEmbedded: true,
			wantIgnored:  []string{"patch.xp3"},
		},
		{
			name:         "正常系: data.xp3を選んでもcontent-dataフォルダがあればWindows版はそちらを読み込む",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": plainExe, "data.xp3": archive},
			dirs:         []string{"content-data"},
			wantPath:     "data.xp3",
			wantAdjacent: true,
			wantWindows:  "content-dataフォルダ",
		},
		{
			name:         "正常系: data.xp3を選んだ場合、data.exeとdataフォルダはWindows版でもdata.xp3より後なので挙げない",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": plainExe, "data.xp3": archive, "data.exe": embeddedExe},
			dirs:         []string{"data"},
			wantPath:     "data.xp3",
			wantAdjacent: true,
		},
		{
			name:         "正常系: 埋め込みXP3を選んでもdata.exeがあればWindows版はそちらを読み込む",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "data.exe": embeddedExe},
			wantPath:     "game.exe",
			wantEmbedded: true,
			wantWindows:  "data.exe",
		},
		{
			name:         "正常系: data.exeの名前は大文字小文字を区別せずに探す",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "DATA.EXE": embeddedExe},
			wantPath:     "game.exe",
			wantEmbedded: true,
			wantWindows:  "DATA.EXE",
		},
		{
			name:         "正常系: content-dataフォルダとdata.exeが両方あればWindows版が先に確かめるcontent-dataフォルダを挙げる",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "data.exe": embeddedExe},
			dirs:         []string{"Content-Data"},
			wantPath:     "game.exe",
			wantEmbedded: true,
			wantWindows:  "Content-Dataフォルダ",
		},
		{
			name:         "正常系: content-dataという名前のファイルとdata.exeという名前のフォルダは挙げない",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe, "content-data": []byte("file")},
			dirs:         []string{"data.exe"},
			wantPath:     "game.exe",
			wantEmbedded: true,
		},
		{
			name:         "正常系: 埋め込みXP3を選んだ場合、dataフォルダはWindows版でも埋め込みXP3より後なので挙げない",
			input:        "game.exe",
			files:        map[string][]byte{"game.exe": embeddedExe},
			dirs:         []string{"data"},
			wantPath:     "game.exe",
			wantEmbedded: true,
		},
		{
			name:         "正常系: 入力のEXE自身がdata.exeなら挙げない",
			input:        "data.exe",
			files:        map[string][]byte{"data.exe": embeddedExe},
			wantPath:     "data.exe",
			wantEmbedded: true,
		},
		{
			name:     "正常系: XP3の入力ではcontent-dataフォルダがあっても挙げない",
			input:    "data.xp3",
			files:    map[string][]byte{"data.xp3": archive},
			dirs:     []string{"content-data"},
			wantPath: "data.xp3",
		},
		{
			name:    "異常系: XP3を埋め込んでおらずdata.xp3も無いEXEはエラーを返す",
			input:   "game.exe",
			files:   map[string][]byte{"game.exe": plainExe, "patch.xp3": archive},
			wantErr: "EXEファイル内にXP3アーカイブが見つかりません: {input}",
		},
		{
			name:    "異常系: dataフォルダしか無いEXEはWindows版が読み込むものをエラーに含める",
			input:   "game.exe",
			files:   map[string][]byte{"game.exe": plainExe},
			dirs:    []string{"data"},
			wantErr: "EXEファイル内にXP3アーカイブが見つかりません: {input}。Windows版は同じフォルダのdataフォルダを読み込みますが、mnemonicは対応していません",
		},
		{
			name:    "異常系: data.exeがあるEXEはdataフォルダより先にdata.exeをエラーに含める",
			input:   "game.exe",
			files:   map[string][]byte{"game.exe": plainExe, "Data.exe": plainExe},
			dirs:    []string{"data"},
			wantErr: "EXEファイル内にXP3アーカイブが見つかりません: {input}。Windows版は同じフォルダのData.exeを読み込みますが、mnemonicは対応していません",
		},
		{
			name:    "異常系: content-dataフォルダがあるEXEはそれをエラーに含める",
			input:   "game.exe",
			files:   map[string][]byte{"game.exe": plainExe, "data.exe": plainExe},
			dirs:    []string{"content-data"},
			wantErr: "EXEファイル内にXP3アーカイブが見つかりません: {input}。Windows版は同じフォルダのcontent-dataフォルダを読み込みますが、mnemonicは対応していません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, data := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
			}
			for _, name := range tt.dirs {
				require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o750))
			}
			input := filepath.Join(dir, tt.input)

			got, err := resolveStartupArchive(input)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, strings.ReplaceAll(tt.wantErr, "{input}", input), err.Error())

				return
			}
			require.NoError(t, err)
			assert.Equal(t, startupArchive{
				path:           filepath.Join(dir, tt.wantPath),
				adjacent:       tt.wantAdjacent,
				embedded:       tt.wantEmbedded,
				embeddedUnused: tt.wantUnused,
				windowsLoads:   tt.wantWindows,
				ignored:        tt.wantIgnored,
			}, got)
		})
	}
}
