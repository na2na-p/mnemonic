package resources_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/resources"
)

func TestSystemPolyfillFS_ContainsAllEmbeddedFiles(t *testing.T) {
	t.Parallel()

	allFiles := []string{
		"PolyfillInitialize.tjs",
		"MenuItem_stub.tjs",
		"KAGParser.tjs",
		"MIDISoundBuffer_stub.tjs",
		"VideoOverlay_stub.tjs",
		"SaveDataPath_patch.tjs",
	}

	for _, name := range allFiles {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)

			require.NoError(t, err)
			assert.NotEmpty(t, data)
		})
	}
}

func TestSystemPolyfillFiles_MatchesPythonCopyList(t *testing.T) {
	t.Parallel()

	// why: Python版の_copy_polyfill_filesがコピーする5ファイルの一覧・順序を
	// ピン留めする。SaveDataPath_patch.tjsは同梱されているが
	// コピー対象には含まれない（resources.goのwhy notコメント参照）。
	want := []string{
		"PolyfillInitialize.tjs",
		"MenuItem_stub.tjs",
		"KAGParser.tjs",
		"MIDISoundBuffer_stub.tjs",
		"VideoOverlay_stub.tjs",
	}

	assert.Equal(t, want, resources.SystemPolyfillFiles)
	assert.NotContains(t, resources.SystemPolyfillFiles, "SaveDataPath_patch.tjs")
}

func TestSystemPolyfillFiles_AllReadableFromFS(t *testing.T) {
	t.Parallel()

	for _, name := range resources.SystemPolyfillFiles {
		data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)

		require.NoError(t, err)
		assert.NotEmpty(t, data)
	}
}
