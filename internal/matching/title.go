package matching

import (
	"strings"
	"unicode"

	"github.com/hmarp/setlisted/internal/normalize"
)

// Title returns the normalised form of a song title used for MATCH-4: version
// suffixes removed, then normalize.Name (case, accents, whitespace), "&"
// treated as "and", apostrophes and full stops removed, and any other
// punctuation or symbol treated as a space.
//
// "Seek & Destroy" and "Seek and Destroy" both give "seek and destroy";
// "Blackened - Remastered 2021" gives "blackened"; "Live Wire" stays
// "live wire".
func Title(s string) string {
	core, _ := splitTitle(s)
	return words(core)
}

// splitTitle removes trailing version suffixes from a raw title, repeatedly,
// and returns what's left and the removed suffixes (outermost last), as
// written. A suffix is a trailing "(…)", "[…]" or " - …" segment ("–" and
// "—" count as dashes) that contains a version or exclusion marker. A
// segment without a marker ends the stripping, and so does a segment that
// would leave nothing.
func splitTitle(s string) (core string, suffixes []string) {
	core = strings.TrimSpace(s)
	for {
		rest, seg, ok := trailingSegment(core)
		if !ok || !isVersionSegment(seg) {
			return core, suffixes
		}
		core = rest
		suffixes = append(suffixes, seg)
	}
}

// trailingSegment splits off the last bracketed or dash-separated segment of
// a trimmed title. rest is never empty when ok is true.
func trailingSegment(s string) (rest, seg string, ok bool) {
	if s == "" {
		return "", "", false
	}
	if rest, seg, ok, balanced := trailingBracket(s); balanced {
		return rest, seg, ok
	}
	// No closing bracket at the end, or an unbalanced one: try a dash.
	best := -1
	sepLen := 0
	for _, sep := range []string{" - ", " – ", " — "} {
		if i := strings.LastIndex(s, sep); i > best {
			best, sepLen = i, len(sep)
		}
	}
	if best < 0 {
		return "", "", false
	}
	rest = trimSeparators(s[:best])
	if rest == "" {
		return "", "", false
	}
	return rest, s[best+sepLen:], true
}

// trailingBracket splits off a trailing "(…)" or "[…]". balanced is false
// when s doesn't end in a closing bracket or the bracket has no matching
// opening one, so the caller can try a dash separator instead.
func trailingBracket(s string) (rest, seg string, ok, balanced bool) {
	close := s[len(s)-1]
	if close != ')' && close != ']' {
		return "", "", false, false
	}
	open := byte('(')
	if close == ']' {
		open = '['
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case close:
			depth++
		case open:
			depth--
		}
		if depth == 0 {
			rest = trimSeparators(s[:i])
			if rest == "" {
				return "", "", false, true
			}
			return rest, s[i+1 : len(s)-1], true, true
		}
	}
	return "", "", false, false
}

// trimSeparators trims spaces and dangling dashes, e.g. what's left of
// "Song - (Live)" after removing "(Live)".
func trimSeparators(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), " -–—")
}

// isVersionSegment reports whether a title segment contains a version or
// exclusion marker.
func isVersionSegment(seg string) bool {
	ws := strings.Fields(words(seg))
	return containsAny(ws, versionMarkers) || containsAny(ws, excludedMarkers) || isYearVersion(ws)
}

// isYearVersion reports whether ws contains a four-digit year followed by
// "version", e.g. "2016 version".
func isYearVersion(ws []string) bool {
	for i := 0; i+1 < len(ws); i++ {
		if ws[i+1] == "version" && len(ws[i]) == 4 && strings.IndexFunc(ws[i], func(r rune) bool { return r < '0' || r > '9' }) < 0 {
			return true
		}
	}
	return false
}

// containsAny reports whether ws contains any of the markers as a whole word
// or a run of whole words.
func containsAny(ws []string, markers []string) bool {
	for _, m := range markers {
		if containsPhrase(ws, strings.Fields(m)) {
			return true
		}
	}
	return false
}

func containsPhrase(ws, phrase []string) bool {
	for i := 0; i+len(phrase) <= len(ws); i++ {
		match := true
		for j, p := range phrase {
			if ws[i+j] != p {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// words normalises text for title comparison, without stripping suffixes:
// normalize.Name, "&" → " and ", apostrophes and full stops removed (so
// "Don't" = "Dont" and "T.N.T." = "TNT"), any other punctuation or symbol →
// space, then whitespace collapsed.
func words(s string) string {
	n := normalize.Name(s)
	var b strings.Builder
	b.Grow(len(n))
	for _, r := range n {
		switch {
		case r == '&':
			b.WriteString(" and ")
		case r == '\'' || r == '’' || r == '‘' || r == 'ʼ' || r == '`' || r == '´' || r == '.':
			// dropped: they sit inside words
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
