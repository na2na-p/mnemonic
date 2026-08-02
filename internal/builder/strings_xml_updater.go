package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// xmlAttrEscaper はXML属性値向けのエスケープ処理を行う。
//
// why not: 標準ライブラリのhtml.EscapeStringは"を&#34;、'を&#39;という
// 異なる数値文字参照でエスケープする。"を&quot;、'を&#x27;にエスケープする
// 独自の変換表を持つreplacerを自前で用意する。
var xmlAttrEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)

var stringsXMLAppNamePattern = regexp.MustCompile(`(<string name="app_name">)[^<]*(</string>)`)

// stringsXMLUpdater はres/values/strings.xmlを作成/更新する。
type stringsXMLUpdater struct {
	projectDir string
}

// newStringsXMLUpdater はstringsXMLUpdaterを初期化する。
func newStringsXMLUpdater(projectDir string) *stringsXMLUpdater {
	return &stringsXMLUpdater{projectDir: projectDir}
}

// Update はres/values/strings.xmlを作成/更新する。
func (u *stringsXMLUpdater) Update(appName string) error {
	valuesDir := filepath.Join(u.projectDir, "app", "src", "main", "res", "values")
	if err := os.MkdirAll(valuesDir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	stringsXML := filepath.Join(valuesDir, "strings.xml")
	escapedAppName := xmlAttrEscaper.Replace(appName)

	if content, err := os.ReadFile(stringsXML); err == nil { //nolint:gosec // projectDir配下の固定相対パスを読む用途のため妥当
		text := stringsXMLAppNamePattern.ReplaceAllStringFunc(string(content), func(string) string {
			return "<string name=\"app_name\">" + escapedAppName + "</string>"
		})
		if err := os.WriteFile(stringsXML, []byte(text), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		return nil
	}

	content := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">%s</string>
</resources>
`, escapedAppName)

	if err := os.WriteFile(stringsXML, []byte(content), 0o600); err != nil { //nolint:gosec // projectDir配下の固定相対パスへ書き込む用途のため妥当
		return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
	}

	return nil
}
