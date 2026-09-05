package operations

import (
	"strings"
	"unicode"
)

func spreadsheetText(value string) string {
	trimmed := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if trimmed != "" && strings.ContainsRune("=+-@＝＋－＠", []rune(trimmed)[0]) {
		// Quoted tabs prevent formula evaluation in spreadsheet exports.
		return "\t" + value
	}
	return value
}
