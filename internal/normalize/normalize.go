// Package normalize compares names the way the spec defines a "normalised
// name" (spec §1): ignoring letter case, accents and surrounding spaces.
// Step 07 builds title normalisation on top of Name.
package normalize

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Name returns s lower-cased, without accents, trimmed, and with internal
// runs of whitespace collapsed to a single space, e.g. "  Blue Öyster  Cult"
// → "blue oyster cult".
//
// Accents are removed by decomposing (NFD) and dropping combining marks, so
// only letters that decompose lose their accent: "ö" becomes "o", but "ø",
// "ł" and "ß" are kept as they are.
func Name(s string) string {
	// A fresh transformer per call: transformers aren't safe for concurrent use.
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		// Only possible with invalid input the transformers can't handle;
		// fall back to the original text rather than losing it.
		out = s
	}
	return strings.Join(strings.Fields(strings.ToLower(out)), " ")
}

// Equal reports whether a and b have the same normalised name.
func Equal(a, b string) bool { return Name(a) == Name(b) }
