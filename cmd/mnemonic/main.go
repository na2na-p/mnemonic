// Command mnemonic は吉里吉里2製WindowsゲームをAndroid APKへ変換するCLIツール。
// Goリプレイス完了までは最小のエントリポイントのみを提供する。
package main

import (
	"fmt"

	"github.com/na2na-p/mnemonic/internal/version"
)

func main() {
	fmt.Printf("mnemonic %s (Go migration in progress)\n", version.String())
}
