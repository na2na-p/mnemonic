// why not: ANDROID_HOME/PATH環境変数をt.Setenvで変更するため、t.Setenvと
// 両立しないt.Parallel()は使わない。
package signer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/signer"
)

// 探索順序などの詳細はfindAndroidBuildToolのwhite-boxテストで検証しているため、
// ここでは公開関数がその探索結果をそのまま返すことだけを確認する。
func TestFindAndroidBuildTool_Delegation(t *testing.T) {
	tests := []struct {
		name      string
		placeTool bool
		wantFound bool
	}{
		{name: "正常系: ANDROID_HOMEのbuild-toolsにあれば見つかる", placeTool: true, wantFound: true},
		{name: "異常系: どこにも無ければ見つからない", placeTool: false, wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			androidHome := t.TempDir()
			versionDir := filepath.Join(androidHome, "build-tools", "34.0.0")
			require.NoError(t, os.MkdirAll(versionDir, 0o750))

			toolPath := filepath.Join(versionDir, "zipalign")
			if tt.placeTool {
				require.NoError(t, os.WriteFile(toolPath, nil, 0o600))
			}

			t.Setenv("ANDROID_HOME", androidHome)
			t.Setenv("PATH", t.TempDir())

			got, ok := signer.FindAndroidBuildTool("zipalign")

			assert.Equal(t, tt.wantFound, ok)

			if tt.wantFound {
				assert.Equal(t, toolPath, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}
