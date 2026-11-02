package matching

import (
	"errors"
	"reflect"
	"testing"

	"github.com/hmarp/setlisted/internal/setlist"
)

// track is a studio album candidate by Metallica, to be adjusted per test.
func track(id, title string) Candidate {
	return Candidate{
		ID:                   id,
		Title:                title,
		Artists:              []string{"Metallica"},
		Album:                "Some Album",
		AlbumType:            AlbumTypeAlbum,
		ReleaseDate:          "1986-03-03",
		ReleaseDatePrecision: PrecisionDay,
	}
}

func with(c Candidate, f func(*Candidate)) Candidate {
	f(&c)
	return c
}

// chosen runs Choose and returns the chosen ID ("" when not found).
func chosen(t *testing.T, entry, artist string, cands ...Candidate) (string, bool) {
	t.Helper()
	c, live, ok := Choose(entry, artist, cands)
	if !ok {
		if c.ID != "" || live {
			t.Errorf("not found but returned %+v, live=%v", c, live)
		}
		return "", false
	}
	return c.ID, live
}

func TestMatch_MATCH3_Artist(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		artists  []string
		want     bool
	}{
		{"exact", "Metallica", []string{"Metallica"}, true},
		{"band among several credited artists", "Metallica", []string{"Lou Reed", "Metallica"}, true},
		{"different artist, same title", "Metallica", []string{"Apocalyptica"}, false},
		{"accent and case", "Motorhead", []string{"Motörhead"}, true},
		{"accent and case, other way", "MOTÖRHEAD", []string{"motorhead"}, true},
		{"surrounding spaces", " Metallica ", []string{"Metallica  "}, true},
		{"The Sword is not Sword", "The Sword", []string{"Sword"}, false},
		{"Sword is not The Sword", "Sword", []string{"The Sword"}, false},
		{"punctuation counts in artist names", "Ghost", []string{"Ghost B.C."}, false},
		{"no artists", "Metallica", nil, false},
		{"empty expected artist", "", []string{""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := with(track("1", "Battery"), func(c *Candidate) { c.Artists = tt.artists })
			if got := IsCandidate("Battery", tt.expected, c); got != tt.want {
				t.Errorf("IsCandidate(artist %q vs %q) = %v, want %v", tt.expected, tt.artists, got, tt.want)
			}
			_, _, ok := Choose("Battery", tt.expected, []Candidate{c})
			if ok != tt.want {
				t.Errorf("Choose found = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestMatch_MATCH4_Title(t *testing.T) {
	tests := []struct {
		entry, title string
		want         bool
	}{
		{"Seek & Destroy", "Seek and Destroy", true},
		{"Seek and Destroy", "Seek & Destroy", true},
		{"Seek&Destroy", "Seek and Destroy", true},
		{"Blackened", "Blackened - Remastered 2021", true},
		{"Battery", "Battery (Live at Wembley)", true},
		{"Fade to Black", "Fade to Black (2016 Version)", true},
		{"Fade to Black", "Fade to Black (Remastered)", true},
		{"Master of Puppets", "Master of Puppets [Remastered]", true},
		{"Enter Sandman", "Enter Sandman - 2015 Remaster", true},
		{"Enter Sandman", "Enter Sandman (Single Version)", true},
		{"Enter Sandman", "Enter Sandman (Explicit)", true},
		{"Enter Sandman", "Enter Sandman - Mono", true},
		{"Enter Sandman", "Enter Sandman (Stereo)", true},
		{"Enter Sandman", "Enter Sandman (Deluxe Edition)", true},
		{"Enter Sandman", "Enter Sandman - Radio Edit", true},
		{"Enter Sandman", "Enter Sandman (Edit)", true},
		{"Enter Sandman", "Enter Sandman – Remastered", true},
		{"Enter Sandman", "Enter Sandman — Live", true},
		{"Battery", "Battery - Remastered (Live)", true},
		{"Battery", "Battery (Remastered) [Live]", true},
		{"Battery", "Battery - Live (Remastered 2016)", true},
		{"Battery", "Battery (Live) - Remastered", true},
		{"One", "One More Time", false},
		{"One More Time", "One", false},
		{"Live Wire", "Live Wire", true},
		{"Wire", "Live Wire", false},
		{"Part 1 - The Beginning", "Part 1 - The Beginning", true},
		{"Part 1", "Part 1 - The Beginning", false},
		{"Part 1 - The Beginning", "Part 1 - The Beginning - Remastered", true},
		{"Don't Tread on Me", "Dont Tread On Me", true},
		{"Don’t Tread on Me", "Don't Tread on Me", true},
		{"Whiplash!", "Whiplash", true},
		{"Hell-Bent", "Hell Bent", true},
		{"T.N.T.", "TNT", true},
		{"Für Elise", "Fur Elise", true},
		{"Battery", "Battery (feat. Someone)", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.entry+" vs "+tt.title, func(t *testing.T) {
			c := track("1", tt.title)
			if got := IsCandidate(tt.entry, "Metallica", c); got != tt.want {
				t.Errorf("IsCandidate(%q, title %q) = %v, want %v (Title: %q vs %q)",
					tt.entry, tt.title, got, tt.want, Title(tt.entry), Title(tt.title))
			}
		})
	}
}

func TestMatch_MATCH4_TitleNormalisation(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Seek & Destroy", "seek and destroy"},
		{"Blackened - Remastered 2021", "blackened"},
		{"Battery (Live at Wembley)", "battery"},
		{"Fade to Black (2016 Version)", "fade to black"},
		{"Live Wire", "live wire"},
		{"Part 1 - The Beginning", "part 1 the beginning"},
		{"  Don't  Stop ", "dont stop"},
		// A segment that would leave nothing isn't stripped.
		{"(Live)", "live"},
		{"Live - Remastered", "live"},
		// Brackets that aren't the end of the title stay.
		{"(Anesthesia) Pulling Teeth", "anesthesia pulling teeth"},
		// An unbalanced bracket isn't a suffix.
		{"Battery Live)", "battery live"},
		// A dash without spaces isn't a separator.
		{"Battery-Live", "battery live"},
		// A year without "version" isn't a marker; a non-year is neither.
		{"Song (2016)", "song 2016"},
		{"Song (Take 2 Version)", "song take 2 version"},
		// Exclusion markers are stripped too, so the version is recognised.
		{"Battery (Demo)", "battery"},
		{"Battery - Instrumental", "battery"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := Title(tt.in); got != tt.want {
				t.Errorf("Title(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMatch_MATCH5_Excluded(t *testing.T) {
	tests := []struct {
		name, title, album string
	}{
		{"demo suffix", "Battery (Demo)", "Some Album"},
		{"demo version suffix", "Battery - Demo Version", "Some Album"},
		{"remix suffix", "Battery (2020 Remix)", "Some Album"},
		{"instrumental suffix", "Battery - Instrumental", "Some Album"},
		{"acoustic suffix", "Battery (Acoustic Version)", "Some Album"},
		{"karaoke suffix", "Battery [Karaoke]", "Some Album"},
		{"tribute suffix", "Battery - Tribute to Cliff", "Some Album"},
		{"demo album", "Battery", "No Life 'til Leather (Demo)"},
		{"demos album", "Battery", "The Demos"},
		{"remixes album", "Battery", "Remixes"},
		{"instrumental album", "Battery", "Instrumental Hits"},
		{"acoustic album", "Battery", "Acoustic Sessions"},
		{"karaoke album", "Battery", "Karaoke Metal Vol. 3"},
		{"tribute album", "Battery", "A Tribute to Metallica"},
		{"excluded beats live", "Battery (Live Acoustic)", "Some Album"},
		{"marker with remaster", "Battery - Remastered (Demo)", "Some Album"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := with(track("x", tt.title), func(c *Candidate) { c.Album = tt.album })
			if got := Classify(c); got != VersionExcluded {
				t.Errorf("Classify(%q on %q) = %q, want excluded", tt.title, tt.album, got)
			}
			// An excluded-only result is not found.
			if id, _ := chosen(t, "Battery", "Metallica", c); id != "" {
				t.Errorf("chose %q, want not found", id)
			}
			// It loses to any other version, even a live compilation track.
			other := with(track("ok", "Battery - Live"), func(c *Candidate) { c.AlbumType = AlbumTypeCompilation })
			if id, _ := chosen(t, "Battery", "Metallica", c, other); id != "ok" {
				t.Errorf("chose %q, want ok", id)
			}
		})
	}

	t.Run("whole words only", func(t *testing.T) {
		for _, album := range []string{"Demolition", "Remixing History", "Tributary"} {
			c := with(track("x", "Battery"), func(c *Candidate) { c.Album = album })
			if got := Classify(c); got != VersionStudio {
				t.Errorf("Classify(album %q) = %q, want studio", album, got)
			}
		}
	})
	t.Run("core title is not a marker", func(t *testing.T) {
		c := track("x", "Acoustic Demo")
		if got := Classify(c); got != VersionStudio {
			t.Errorf("Classify(%q) = %q, want studio", c.Title, got)
		}
	})
}

func TestMatch_MATCH6_StudioBeatsLive(t *testing.T) {
	studio := with(track("studio", "Battery"), func(c *Candidate) {
		// Even a later compilation track beats a live album track.
		c.AlbumType = AlbumTypeCompilation
		c.ReleaseDate = "2010"
	})
	live := with(track("live", "Battery - Live"), func(c *Candidate) { c.ReleaseDate = "1980" })
	id, isLive := chosen(t, "Battery", "Metallica", live, studio)
	if id != "studio" || isLive {
		t.Errorf("got %q live=%v, want studio, not live", id, isLive)
	}
}

func TestMatch_MATCH6_OnlyLive(t *testing.T) {
	live := track("live", "Battery (Live at Wembley)")
	id, isLive := chosen(t, "Battery", "Metallica", live)
	if id != "live" || !isLive {
		t.Errorf("got %q live=%v, want live, flagged", id, isLive)
	}
}

func TestMatch_MATCH6_LiveDetection(t *testing.T) {
	tests := []struct {
		name, title, album string
		want               Version
	}{
		{"live suffix", "Battery - Live", "Some Album", VersionLive},
		{"live at suffix", "Battery (Live at Wembley)", "Some Album", VersionLive},
		{"live in brackets", "Battery [Live]", "Some Album", VersionLive},
		{"live with remaster", "Battery - Live / Remastered", "Some Album", VersionLive},
		{"album Live After Death", "Battery", "Live After Death", VersionLive},
		{"album live! punctuation", "Battery", "Live! In Paris", VersionLive},
		{"album Alive in Athens", "Battery", "Alive in Athens", VersionStudio},
		{"album Long Live Rock 'n' Roll (documented limitation)", "Battery", "Long Live Rock 'n' Roll", VersionLive},
		{"Live Wire stays studio", "Live Wire", "Too Fast for Love", VersionStudio},
		{"Live Wire remastered stays studio", "Live Wire - Remastered", "Too Fast for Love", VersionStudio},
		{"live in the core title", "Live to Win", "Some Album", VersionStudio},
		{"plain studio", "Battery", "Master of Puppets", VersionStudio},
		{"remaster is studio", "Battery - Remastered", "Master of Puppets (Deluxe Box Set)", VersionStudio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := with(track("x", tt.title), func(c *Candidate) { c.Album = tt.album })
			if got := Classify(c); got != tt.want {
				t.Errorf("Classify(%q on %q) = %q, want %q", tt.title, tt.album, got, tt.want)
			}
		})
	}
}

func TestMatch_MATCH7_AlbumType(t *testing.T) {
	album := with(track("album", "Battery"), func(c *Candidate) { c.AlbumType = AlbumTypeAlbum; c.ReleaseDate = "1990" })
	single := with(track("single", "Battery"), func(c *Candidate) { c.AlbumType = AlbumTypeSingle; c.ReleaseDate = "1985" })
	comp := with(track("comp", "Battery"), func(c *Candidate) { c.AlbumType = AlbumTypeCompilation; c.ReleaseDate = "1980" })
	unknown := with(track("unknown", "Battery"), func(c *Candidate) { c.AlbumType = ""; c.ReleaseDate = "1970" })
	upper := with(track("upper", "Battery"), func(c *Candidate) { c.AlbumType = "ALBUM"; c.ReleaseDate = "1995" })

	tests := []struct {
		name  string
		cands []Candidate
		want  string
	}{
		{"album beats single", []Candidate{single, album}, "album"},
		{"single beats compilation", []Candidate{comp, single}, "single"},
		{"album beats compilation", []Candidate{comp, album}, "album"},
		{"compilation beats unknown type", []Candidate{unknown, comp}, "comp"},
		{"type is case-insensitive", []Candidate{single, upper}, "upper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id, _ := chosen(t, "Battery", "Metallica", tt.cands...); id != tt.want {
				t.Errorf("chose %q, want %q", id, tt.want)
			}
		})
	}
}

func TestMatch_MATCH8_EarliestRelease(t *testing.T) {
	at := func(id, date, precision string) Candidate {
		return with(track(id, "Battery"), func(c *Candidate) { c.ReleaseDate = date; c.ReleaseDatePrecision = precision })
	}
	original := at("original", "1986-03-03", PrecisionDay)
	rerecord := with(at("rerecord", "2024-05-01", PrecisionDay), func(c *Candidate) { c.Title = "Battery (2024 Version)" })

	tests := []struct {
		name  string
		cands []Candidate
		want  string
	}{
		{"earlier wins", []Candidate{at("late", "1999-01-01", PrecisionDay), at("early", "1986-01-01", PrecisionDay)}, "early"},
		{"re-recording loses to original", []Candidate{rerecord, original}, "original"},
		{"re-recording used if only one", []Candidate{rerecord}, "rerecord"},
		{"year precision is the start of the year", []Candidate{at("day", "1986-01-02", PrecisionDay), at("year", "1986", PrecisionYear)}, "year"},
		{"day before a later year", []Candidate{at("year", "1987", PrecisionYear), at("day", "1986-12-31", PrecisionDay)}, "day"},
		{"month precision is the start of the month", []Candidate{at("day", "1986-03-02", PrecisionDay), at("month", "1986-03", PrecisionMonth)}, "month"},
		{"month before a later day", []Candidate{at("day", "1986-02-28", PrecisionDay), at("month", "1986-03", PrecisionMonth)}, "day"},
		{"year vs month", []Candidate{at("month", "1986-02", PrecisionMonth), at("year", "1986", PrecisionYear)}, "year"},
		{"unparseable date loses", []Candidate{at("bad", "unknown", ""), at("good", "2020", PrecisionYear)}, "good"},
		{"empty date loses", []Candidate{at("empty", "", ""), at("good", "2020", PrecisionYear)}, "good"},
		{"tie keeps input order", []Candidate{at("first", "1986", PrecisionYear), at("second", "1986-01-01", PrecisionDay)}, "first"},
		{"tie keeps input order, swapped", []Candidate{at("second", "1986-01-01", PrecisionDay), at("first", "1986", PrecisionYear)}, "second"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id, _ := chosen(t, "Battery", "Metallica", tt.cands...); id != tt.want {
				t.Errorf("chose %q, want %q", id, tt.want)
			}
		})
	}
}

func TestMatch_MATCH9_NoGuesses(t *testing.T) {
	other := with(track("other", "Battery"), func(c *Candidate) { c.Artists = []string{"Apocalyptica"} })
	close := track("close", "Battery Park")
	tests := []struct {
		name  string
		cands []Candidate
	}{
		{"no candidates", nil},
		{"only other artists", []Candidate{other}},
		{"only near titles", []Candidate{close, track("x", "Batteries")}},
		{"only excluded", []Candidate{track("demo", "Battery (Demo)")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id, _ := chosen(t, "Battery", "Metallica", tt.cands...); id != "" {
				t.Errorf("chose %q, want not found", id)
			}
		})
	}
}

// fakeSearch records the artists it's asked for and returns canned results.
type fakeSearch struct {
	results map[string][]Candidate
	errs    map[string]error
	calls   []string
}

func (f *fakeSearch) candidates(artist string) ([]Candidate, error) {
	f.calls = append(f.calls, artist)
	return f.results[artist], f.errs[artist]
}

func song(name string, artists ...string) setlist.Entry {
	e := setlist.Entry{Position: 1, Name: name, Kind: setlist.KindSong, ExpectedArtists: artists}
	if len(artists) > 1 {
		e.OriginalArtist = artists[1]
	}
	return e
}

func by(id, title, artist string) Candidate {
	return with(track(id, title), func(c *Candidate) { c.Artists = []string{artist} })
}

func TestMatch_MATCH1_NormalSongUsesBandOnly(t *testing.T) {
	f := &fakeSearch{results: map[string][]Candidate{
		"Metallica": {by("m", "Battery", "Metallica")},
	}}
	got, err := Resolve(song("Battery", "Metallica"), f.candidates)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Found: true, Track: by("m", "Battery", "Metallica"), Artist: "Metallica"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(f.calls, []string{"Metallica"}) {
		t.Errorf("searched %v, want only the band", f.calls)
	}

	t.Run("not found is not retried elsewhere", func(t *testing.T) {
		f := &fakeSearch{results: map[string][]Candidate{
			"Metallica": {by("other", "Battery", "Apocalyptica")},
		}}
		got, err := Resolve(song("Battery", "Metallica"), f.candidates)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, Result{}) {
			t.Errorf("got %+v, want not found", got)
		}
		if !reflect.DeepEqual(f.calls, []string{"Metallica"}) {
			t.Errorf("searched %v, want only the band", f.calls)
		}
	})
}

func TestMatch_MATCH2_CoverTwoPasses(t *testing.T) {
	const band, original = "Metallica", "Diamond Head"
	bandTrack := by("band", "Am I Evil?", band)
	origTrack := by("orig", "Am I Evil?", original)
	origLive := by("origlive", "Am I Evil? - Live", original)

	tests := []struct {
		name      string
		results   map[string][]Candidate
		want      Result
		wantCalls []string
	}{
		{
			name:      "band has a recording",
			results:   map[string][]Candidate{band: {bandTrack}, original: {origTrack}},
			want:      Result{Found: true, Track: bandTrack, Artist: band},
			wantCalls: []string{band},
		},
		{
			name:      "no band recording: original artist, cover fallback",
			results:   map[string][]Candidate{band: {origTrack}, original: {origTrack}},
			want:      Result{Found: true, Track: origTrack, Artist: original, CoverFallback: true},
			wantCalls: []string{band, original},
		},
		{
			name:      "band has only excluded versions: fallback",
			results:   map[string][]Candidate{band: {by("demo", "Am I Evil? (Demo)", band)}, original: {origTrack}},
			want:      Result{Found: true, Track: origTrack, Artist: original, CoverFallback: true},
			wantCalls: []string{band, original},
		},
		{
			name:      "band live version beats original studio",
			results:   map[string][]Candidate{band: {by("bandlive", "Am I Evil? - Live", band)}, original: {origTrack}},
			want:      Result{Found: true, Track: by("bandlive", "Am I Evil? - Live", band), Artist: band, Live: true},
			wantCalls: []string{band},
		},
		{
			name:      "cover fallback and live",
			results:   map[string][]Candidate{original: {origLive}},
			want:      Result{Found: true, Track: origLive, Artist: original, CoverFallback: true, Live: true},
			wantCalls: []string{band, original},
		},
		{
			name:      "neither",
			results:   map[string][]Candidate{band: {by("x", "Am I Evil?", "Someone Else")}},
			want:      Result{},
			wantCalls: []string{band, original},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{results: tt.results}
			got, err := Resolve(song("Am I Evil?", band, original), f.candidates)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(f.calls, tt.wantCalls) {
				t.Errorf("searched %v, want %v", f.calls, tt.wantCalls)
			}
		})
	}
}

func TestMatch_Resolve_Errors(t *testing.T) {
	boom := errors.New("spotify unavailable")
	t.Run("band pass error stops matching", func(t *testing.T) {
		f := &fakeSearch{errs: map[string]error{"Metallica": boom}}
		got, err := Resolve(song("Am I Evil?", "Metallica", "Diamond Head"), f.candidates)
		if !errors.Is(err, boom) || !reflect.DeepEqual(got, Result{}) {
			t.Errorf("got %+v, %v; want zero result and the error", got, err)
		}
		if !reflect.DeepEqual(f.calls, []string{"Metallica"}) {
			t.Errorf("searched %v, want only the band", f.calls)
		}
	})
	t.Run("fallback pass error", func(t *testing.T) {
		f := &fakeSearch{errs: map[string]error{"Diamond Head": boom}}
		got, err := Resolve(song("Am I Evil?", "Metallica", "Diamond Head"), f.candidates)
		if !errors.Is(err, boom) || !reflect.DeepEqual(got, Result{}) {
			t.Errorf("got %+v, %v; want zero result and the error", got, err)
		}
	})
}

func TestMatch_Resolve_OnlySongs(t *testing.T) {
	for _, kind := range []setlist.Kind{setlist.KindSkipped, setlist.KindMedley} {
		t.Run(string(kind), func(t *testing.T) {
			f := &fakeSearch{}
			e := setlist.Entry{Name: "Battery", Kind: kind, ExpectedArtists: []string{"Metallica"}}
			got, err := Resolve(e, f.candidates)
			if err != nil || !reflect.DeepEqual(got, Result{}) || len(f.calls) != 0 {
				t.Errorf("got %+v, %v after %v searches; want not found without searching", got, err, f.calls)
			}
		})
	}
	t.Run("song without expected artists", func(t *testing.T) {
		f := &fakeSearch{}
		got, err := Resolve(setlist.Entry{Name: "Battery", Kind: setlist.KindSong}, f.candidates)
		if err != nil || !reflect.DeepEqual(got, Result{}) || len(f.calls) != 0 {
			t.Errorf("got %+v, %v after %v searches; want not found without searching", got, err, f.calls)
		}
	})
}

// TestMatch_ENT6_NoteIgnored checks that a note that looks like a version
// marker changes nothing.
func TestMatch_ENT6_NoteIgnored(t *testing.T) {
	results := map[string][]Candidate{"Metallica": {by("studio", "Battery", "Metallica"), by("live", "Battery - Live", "Metallica")}}
	plain := song("Battery", "Metallica")
	noted := plain
	noted.Note = "live acoustic demo; with intro"

	f1, f2 := &fakeSearch{results: results}, &fakeSearch{results: results}
	a, _ := Resolve(plain, f1.candidates)
	b, _ := Resolve(noted, f2.candidates)
	if !reflect.DeepEqual(a, b) || a.Track.ID != "studio" {
		t.Errorf("with note %+v, without %+v; want the same studio match", b, a)
	}
}

// TestMatch_EndToEnd_Show runs Resolve over a show built by step 06, the way
// step 08 will.
func TestMatch_EndToEnd_Show(t *testing.T) {
	show := setlist.Build(setlist.Header{Band: "Metallica"}, []setlist.SourceSet{{Entries: []setlist.SourceEntry{
		{Name: "The Ecstasy of Gold", Tape: true, CoverArtist: "Ennio Morricone"},
		{Name: "Creeping Death"},
		{Name: "Drum Solo"},
		{Name: "Am I Evil?", CoverArtist: "Diamond Head"},
	}}})
	results := map[string][]Candidate{
		"Metallica":       {by("cd", "Creeping Death - Remastered", "Metallica"), by("aie", "Am I Evil?", "Metallica")},
		"Ennio Morricone": {by("eog", "The Ecstasy of Gold", "Ennio Morricone")},
	}
	var got []string
	for _, e := range show.Entries() {
		f := &fakeSearch{results: results}
		r, err := Resolve(e, f.candidates)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, r.Track.ID)
		if e.Name == "The Ecstasy of Gold" && !r.CoverFallback {
			t.Errorf("tape attributed to another artist: want cover fallback, got %+v", r)
		}
	}
	want := []string{"eog", "cd", "", "aie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
