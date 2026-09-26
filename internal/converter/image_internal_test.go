package converter

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTLGSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		skipAsRoot  bool
		setup       func(t *testing.T, dir string) string
		wantErr     error
		notWantErr  error
		wantOSErr   error
		wantPathErr bool
	}{
		{
			name: "異常系: 存在しないファイルはErrSourceNotFoundを返す",
			setup: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "missing.tlg")
			},
			wantErr:     ErrSourceNotFound,
			notWantErr:  ErrSourceUnreadable,
			wantOSErr:   nil,
			wantPathErr: false,
		},
		{
			name:       "異常系: 読み取り権限の無いファイルはOSのエラーを包んだErrSourceUnreadableを返す",
			skipAsRoot: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "unreadable.tlg")
				require.NoError(t, os.WriteFile(path, []byte("TLG5.0\x00raw\x1a"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))

				return path
			},
			wantErr:     ErrSourceUnreadable,
			notWantErr:  ErrSourceNotFound,
			wantOSErr:   fs.ErrPermission,
			wantPathErr: true,
		},
		{
			name: "異常系: ディレクトリはOSのエラーを包んだErrSourceUnreadableを返す",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "dir.tlg")
				require.NoError(t, os.Mkdir(path, 0o700))

				return path
			},
			wantErr:     ErrSourceUnreadable,
			notWantErr:  ErrSourceNotFound,
			wantOSErr:   nil,
			wantPathErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
			}

			path := tt.setup(t, t.TempDir())

			data, err := readTLGSource(path)

			require.Error(t, err)
			assert.Nil(t, data)
			require.ErrorIs(t, err, tt.wantErr)
			require.NotErrorIs(t, err, tt.notWantErr)
			require.ErrorContains(t, err, path)
			if tt.wantOSErr != nil {
				require.ErrorIs(t, err, tt.wantOSErr)
			}
			if tt.wantPathErr {
				_, ok := errors.AsType[*fs.PathError](err)
				assert.True(t, ok, "OSのエラー(*fs.PathError)を包んでいること")
			}
		})
	}
}

func TestImageConverter_decodeSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		skipAsRoot    bool
		setup         func(t *testing.T, dir string) string
		wantErr       error
		wantPermanent bool
	}{
		{
			name: "異常系: 存在しないTLGファイルは再試行対象のErrSourceNotFoundを返す",
			setup: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "x.tlg")
			},
			wantErr:       ErrSourceNotFound,
			wantPermanent: false,
		},
		{
			name: "異常系: 読み込めないTLGファイルは権限不足以外なら再試行対象のErrSourceUnreadableを返す",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "dir.tlg")
				require.NoError(t, os.Mkdir(path, 0o700))

				return path
			},
			wantErr:       ErrSourceUnreadable,
			wantPermanent: false,
		},
		{
			name:       "異常系: 読み取り権限の無いTLGファイルは再試行不要なErrSourceUnreadableを返す",
			skipAsRoot: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "unreadable.tlg")
				require.NoError(t, os.WriteFile(path, []byte("TLG5.0\x00raw\x1a"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))

				return path
			},
			wantErr:       ErrSourceUnreadable,
			wantPermanent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
			}

			path := tt.setup(t, t.TempDir())

			img, err := NewImageConverter().decodeSource(path)

			require.Error(t, err)
			assert.Nil(t, img)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantPermanent {
				require.ErrorIs(t, err, ErrPermanentFailure)
			} else {
				require.NotErrorIs(t, err, ErrPermanentFailure)
			}
		})
	}
}
