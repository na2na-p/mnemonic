package fsutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/fsutil"
)

func TestFormatSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		want string
	}{
		{"0バイトはBで表す", 0, "0 B"},
		{"1KiB未満はBで表す", 1023, "1023 B"},
		{"1KiBちょうどはKBで表す", 1 << 10, "1.0 KB"},
		{"1MiB未満はKBで小数1桁まで表す", 1536, "1.5 KB"},
		{"1MiBちょうどはMBで表す", 1 << 20, "1.0 MB"},
		{"1GiBちょうどはGBで表す", 1 << 30, "1.0 GB"},
		{"1GiB以上はGBのまま表す", 3 << 40, "3072.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, fsutil.FormatSize(tt.size))
		})
	}
}
