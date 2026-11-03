//go:build live

package main

import "testing"

func TestSetlistID(t *testing.T) {
	ok := map[string]string{
		"63ab5e1f": "63ab5e1f",
		"https://www.setlist.fm/setlist/metallica/2024/olympiastadion-munich-germany-13a9e2e3.html": "13a9e2e3",
		"https://www.setlist.fm/setlist/iron-maiden/2025/o2-arena-london-england-5bb4d7f8.html?x=1": "5bb4d7f8",
	}
	for in, want := range ok {
		if got, err := setlistID(in); err != nil || got != want {
			t.Errorf("setlistID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// An artist's page isn't a setlist, though its name ends in an ID too.
	for _, in := range []string{"", "not an id", "https://example.com/setlist/x-13a9e2e3.html", "https://www.setlist.fm/setlists/metallica-3bd6bc5c.html"} {
		if got, err := setlistID(in); err == nil {
			t.Errorf("setlistID(%q) = %q, want an error", in, got)
		}
	}
}
