// Package testsupport supplies stable fixture builders shared by integration tests.
package testsupport

import (
	"strings"

	"github.com/google/uuid"
)

const gstinAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ValidGSTIN returns a checksum-valid GSTIN with the supplied two-digit state
// code. Test fixtures use it so migration-enforced production invariants apply
// equally to direct model setup.
func ValidGSTIN(stateCode string) string {
	if len(stateCode) != 2 {
		panic("GSTIN state code must contain two digits")
	}
	seed := strings.ReplaceAll(uuid.NewString(), "-", "")
	var builder strings.Builder
	builder.WriteString(stateCode)
	for index := 0; index < 5; index++ {
		builder.WriteByte('A' + seed[index]%26)
	}
	for index := 5; index < 9; index++ {
		builder.WriteByte('0' + seed[index]%10)
	}
	builder.WriteByte('A' + seed[9]%26)
	builder.WriteByte('1')
	builder.WriteByte('Z')
	base := builder.String()

	sum := 0
	for index, character := range base {
		value := strings.IndexRune(gstinAlphabet, character)
		if index%2 == 1 {
			value *= 2
		}
		sum += value/36 + value%36
	}
	return base + string(gstinAlphabet[(36-sum%36)%36])
}
