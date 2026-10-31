package setlist

import (
	"reflect"
	"testing"
)

const band = "Gojira"

var header = Header{ID: "63de4613", Band: band, Date: "14 Mar 2026", Venue: "Haus Auensee", City: "Leipzig", Tour: "Mea Culpa Tour", URL: "https://www.setlist.fm/setlist/x.html"}

// one builds a show with a single main set holding the given entries.
func one(entries ...SourceEntry) Show {
	return Build(header, []SourceSet{{Entries: entries}})
}

// only builds a show from one entry and returns that entry.
func only(t *testing.T, se SourceEntry) Entry {
	t.Helper()
	es := one(se).Entries()
	if len(es) != 1 {
		t.Fatalf("got %d entries, want 1", len(es))
	}
	return es[0]
}

func TestEntry_ENT1_Skipped(t *testing.T) {
	tests := []struct {
		name string
		want Kind
	}{
		{"", KindSkipped},
		{"  ", KindSkipped},
		{"\t", KindSkipped},
		{"Drum Solo", KindSkipped},
		{"guitar solo", KindSkipped},
		{"Bass  Solo ", KindSkipped},
		{"Solo", KindSkipped},
		{"SOLO", KindSkipped},
		{"Jam", KindSkipped},
		{"Improvisation", KindSkipped},
		{"improvisation ", KindSkipped},
		// "solo" isn't the whole name or the final word.
		{"Solo Flight", KindSong},
		// The whole name must be "jam": "Jam Session" is a song name.
		{"Jam Session", KindSong},
		{"Drum Jam", KindSong},
		{"Improvisation No. 2", KindSong},
		// Only the word "solo" counts, not a word ending in "solo".
		{"Desolo", KindSong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.name); got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEntry_ENT1_SkippedEvenWhenTapeOrCover(t *testing.T) {
	e := only(t, SourceEntry{Name: "Drum Solo", Tape: true, CoverArtist: "Metallica", Note: "Mario"})
	if e.Kind != KindSkipped || len(e.ExpectedArtists) != 0 {
		t.Errorf("got kind %q, expected artists %v; want skipped with none", e.Kind, e.ExpectedArtists)
	}
}

func TestEntry_ENT2_SkippedNotNumberedOrCounted(t *testing.T) {
	show := one(
		SourceEntry{Name: "Born for One Thing"},
		SourceEntry{Name: "Drum Solo"},
		SourceEntry{Name: ""},
		SourceEntry{Name: "Backbone"},
		SourceEntry{Name: "Jam"},
	)
	var got []int
	for _, e := range show.Entries() {
		got = append(got, e.Position)
	}
	if want := []int{1, 0, 0, 2, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("positions = %v, want %v", got, want)
	}
	if got := show.Matchable(); got != 2 {
		t.Errorf("Matchable() = %d, want 2", got)
	}
}

func TestEntry_ENT3_Medley(t *testing.T) {
	tests := []struct {
		name string
		want Kind
	}{
		{"Medley: A / B", KindMedley},
		{"A / B", KindMedley},
		{"Flying Whales / The Heaviest Matter of the Universe", KindMedley},
		{"A  /  B", KindMedley}, // whitespace is collapsed first
		{"Medley", KindMedley},
		{"medley", KindMedley},
		{"Old Songs Medley", KindMedley},
		{"MEDLEY (Part 2)", KindMedley},
		// Not a word match.
		{"Remedley", KindSong},
		{"Medleys", KindSong},
		// A slash without spaces on both sides isn't a separator.
		{"AC/DC", KindSong},
		{"A/ B", KindSong},
		{"A /B", KindSong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.name); got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEntry_ENT1_SkipBeforeMedley(t *testing.T) {
	// §5 order: a name that is both is skipped, not a medley.
	if got := Classify("Medley Drum Solo"); got != KindSkipped {
		t.Errorf("Classify = %q, want skipped", got)
	}
}

func TestEntry_ENT4_MedleyNotMatchable(t *testing.T) {
	show := one(
		SourceEntry{Name: "Stranded"},
		SourceEntry{Name: "Medley: A / B", CoverArtist: "Metallica", Tape: true},
		SourceEntry{Name: "Silvera"},
	)
	es := show.Entries()
	m := es[1]
	if m.Kind != KindMedley {
		t.Fatalf("kind = %q, want medley", m.Kind)
	}
	if len(m.ExpectedArtists) != 0 {
		t.Errorf("ExpectedArtists = %v, want none", m.ExpectedArtists)
	}
	if m.Position != 2 {
		t.Errorf("medley position = %d, want 2 (medleys are shown and numbered)", m.Position)
	}
	if es[2].Position != 3 {
		t.Errorf("next position = %d, want 3", es[2].Position)
	}
	if got := show.Matchable(); got != 2 {
		t.Errorf("Matchable() = %d, want 2", got)
	}
}

func TestEntry_ENT5_TapesCoversAndNotesAreSongs(t *testing.T) {
	tests := []struct {
		label string
		in    SourceEntry
	}{
		{"tape", SourceEntry{Name: "Intro", Tape: true}},
		{"cover", SourceEntry{Name: "Ace of Spades", CoverArtist: "Motörhead"}},
		{"snippet note", SourceEntry{Name: "Toxic Garbage Island", Note: "snippet"}},
		{"partial note", SourceEntry{Name: "Toxic Garbage Island", Note: "partial"}},
		{"acoustic note", SourceEntry{Name: "Toxic Garbage Island", Note: "acoustic"}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			if e := only(t, tt.in); e.Kind != KindSong {
				t.Errorf("kind = %q, want song", e.Kind)
			}
		})
	}
}

func TestEntry_ENT6_NoteCarriedButIgnored(t *testing.T) {
	tests := []struct {
		label string
		in    SourceEntry
	}{
		{"song", SourceEntry{Name: "L'enfant sauvage"}},
		{"cover", SourceEntry{Name: "Ace of Spades", CoverArtist: "Motörhead"}},
		{"medley", SourceEntry{Name: "A / B"}},
		{"skipped", SourceEntry{Name: "Drum Solo"}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			plain := only(t, tt.in)
			withNote := tt.in
			// Notes that look like rules must not trigger them.
			withNote.Note = "with drum solo; medley / snippet; acoustic"
			noted := only(t, withNote)
			if noted.Note != withNote.Note {
				t.Errorf("Note = %q, want %q", noted.Note, withNote.Note)
			}
			if noted.Kind != plain.Kind || !reflect.DeepEqual(noted.ExpectedArtists, plain.ExpectedArtists) {
				t.Errorf("with note: kind %q, artists %v; without: kind %q, artists %v",
					noted.Kind, noted.ExpectedArtists, plain.Kind, plain.ExpectedArtists)
			}
		})
	}
}

func TestEntry_ENT7_DuplicatesKept(t *testing.T) {
	show := Build(header, []SourceSet{
		{Entries: []SourceEntry{{Name: "Stranded"}, {Name: "Silvera"}, {Name: "Stranded"}}},
		{Encore: 1, Entries: []SourceEntry{{Name: "Stranded", Note: "reprise"}}},
	})
	var got []string
	var pos []int
	for _, e := range show.Entries() {
		got = append(got, e.Name)
		pos = append(pos, e.Position)
	}
	if want := []string{"Stranded", "Silvera", "Stranded", "Stranded"}; !reflect.DeepEqual(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
	if want := []int{1, 2, 3, 4}; !reflect.DeepEqual(pos, want) {
		t.Errorf("positions = %v, want %v", pos, want)
	}
	if got := show.Matchable(); got != 4 {
		t.Errorf("Matchable() = %d, want 4", got)
	}
}

func TestEntry_MATCH1_BandIsExpectedArtist(t *testing.T) {
	tests := []struct {
		label string
		in    SourceEntry
	}{
		{"song", SourceEntry{Name: "Amazonia"}},
		{"tape without cover", SourceEntry{Name: "Intro", Tape: true}},
		// setlist.fm's cover is "if different to the performing artist";
		// the band credited as its own original artist isn't a cover.
		{"cover credited to the band", SourceEntry{Name: "Amazonia", CoverArtist: "GOJIRA "}},
		{"blank cover artist", SourceEntry{Name: "Amazonia", CoverArtist: "  "}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			e := only(t, tt.in)
			if want := []string{band}; !reflect.DeepEqual(e.ExpectedArtists, want) {
				t.Errorf("ExpectedArtists = %v, want %v", e.ExpectedArtists, want)
			}
			if e.OriginalArtist != "" || e.IsCover() {
				t.Errorf("OriginalArtist = %q, want none", e.OriginalArtist)
			}
			if e.IsTape != tt.in.Tape {
				t.Errorf("IsTape = %v, want %v", e.IsTape, tt.in.Tape)
			}
		})
	}
}

func TestEntry_MATCH2_CoverExpectsBandThenOriginal(t *testing.T) {
	tests := []struct {
		label string
		in    SourceEntry
	}{
		{"cover", SourceEntry{Name: "Ace of Spades", CoverArtist: "Motörhead"}},
		{"tape with cover", SourceEntry{Name: "The Ecstasy of Gold", Tape: true, CoverArtist: "Motörhead"}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			e := only(t, tt.in)
			if want := []string{band, "Motörhead"}; !reflect.DeepEqual(e.ExpectedArtists, want) {
				t.Errorf("ExpectedArtists = %v, want %v", e.ExpectedArtists, want)
			}
			if e.OriginalArtist != "Motörhead" || !e.IsCover() {
				t.Errorf("OriginalArtist = %q, want Motörhead", e.OriginalArtist)
			}
			if e.IsTape != tt.in.Tape {
				t.Errorf("IsTape = %v, want %v", e.IsTape, tt.in.Tape)
			}
		})
	}
}

func TestShow_PREV3_Headings(t *testing.T) {
	show := Build(header, []SourceSet{
		{Name: "Acoustic", Entries: []SourceEntry{{Name: "A"}}}, // first set: never a heading
		{Name: "Encore: Part Two", Encore: 1},                   // name wins over the number
		{Encore: 2},
		{},
		{Name: "  "}, // a blank name counts as none
		{Name: " Acoustic Set "},
	})
	var got []string
	for _, s := range show.Sets {
		got = append(got, s.Heading)
	}
	want := []string{"", "Encore: Part Two", "Encore 2", "Set 4", "Set 5", "Acoustic Set"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %q, want %q", got, want)
	}
}

func TestShow_PREV3_PositionsRunAcrossSets(t *testing.T) {
	show := Build(header, []SourceSet{
		{Entries: []SourceEntry{{Name: "Intro", Tape: true}, {Name: "Born for One Thing"}, {Name: "Drum Solo"}}},
		{Encore: 1, Entries: []SourceEntry{{Name: "A / B"}, {Name: "Amazonia"}}},
	})
	var got [][]int
	for _, s := range show.Sets {
		var ps []int
		for _, e := range s.Entries {
			ps = append(ps, e.Position)
		}
		got = append(got, ps)
	}
	if want := [][]int{{1, 2, 0}, {3, 4}}; !reflect.DeepEqual(got, want) {
		t.Errorf("positions = %v, want %v", got, want)
	}
	if show.Header != header {
		t.Errorf("Header = %+v, want %+v", show.Header, header)
	}
}

func TestShow_NoSets(t *testing.T) {
	show := Build(header, nil)
	if len(show.Sets) != 0 || len(show.Entries()) != 0 || show.Matchable() != 0 {
		t.Errorf("got %+v, want an empty show", show)
	}
}
