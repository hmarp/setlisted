package setlistfm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hmarp/setlisted/internal/setlist"
)

func TestSetlistShow(t *testing.T) {
	const body = `{
	  "id": "63de4613", "eventDate": "14-03-2026", "url": "https://www.setlist.fm/setlist/gojira/2026/x.html",
	  "artist": {"mbid": "a", "name": "Gojira"},
	  "venue": {"name": "Haus Auensee", "city": {"name": "Leipzig"}},
	  "tour": {"name": "Mea Culpa Tour"},
	  "sets": {"set": [
	    {"song": [
	      {"name": "Intro", "tape": true},
	      {"name": "Born for One Thing", "info": "snippet"},
	      {"name": ""},
	      {"name": "Ace of Spades", "cover": {"mbid": "m", "name": "Motörhead"}},
	      {"name": "Drum Solo"}
	    ]},
	    {"encore": 1, "song": [{"name": "A / B"}, {"name": "Outro", "tape": true, "cover": {"name": "Ennio Morricone"}}]},
	    {"name": "Acoustic", "song": [{"name": "Born for One Thing"}]}
	  ]}
	}`
	var sl Setlist
	if err := json.Unmarshal([]byte(body), &sl); err != nil {
		t.Fatal(err)
	}
	got := sl.Show()
	want := setlist.Show{
		Header: setlist.Header{
			ID: "63de4613", Band: "Gojira", Date: "14 Mar 2026", Venue: "Haus Auensee", City: "Leipzig",
			Tour: "Mea Culpa Tour", URL: "https://www.setlist.fm/setlist/gojira/2026/x.html",
		},
		Sets: []setlist.Set{
			{Heading: "", Entries: []setlist.Entry{
				{Position: 1, Name: "Intro", IsTape: true, Kind: setlist.KindSong, ExpectedArtists: []string{"Gojira"}},
				{Position: 2, Name: "Born for One Thing", Note: "snippet", Kind: setlist.KindSong, ExpectedArtists: []string{"Gojira"}},
				{Position: 0, Name: "", Kind: setlist.KindSkipped},
				{Position: 3, Name: "Ace of Spades", OriginalArtist: "Motörhead", Kind: setlist.KindSong, ExpectedArtists: []string{"Gojira", "Motörhead"}},
				{Position: 0, Name: "Drum Solo", Kind: setlist.KindSkipped},
			}},
			{Heading: "Encore 1", Entries: []setlist.Entry{
				{Position: 4, Name: "A / B", Kind: setlist.KindMedley},
				{Position: 5, Name: "Outro", IsTape: true, OriginalArtist: "Ennio Morricone", Kind: setlist.KindSong, ExpectedArtists: []string{"Gojira", "Ennio Morricone"}},
			}},
			{Heading: "Acoustic", Entries: []setlist.Entry{
				{Position: 6, Name: "Born for One Thing", Kind: setlist.KindSong, ExpectedArtists: []string{"Gojira"}},
			}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Show() =\n%+v\nwant\n%+v", got, want)
	}
	if n := got.Matchable(); n != 5 {
		t.Errorf("Matchable() = %d, want 5", n)
	}
}

func TestSetlistShow_NoTourNoSets(t *testing.T) {
	sl := Setlist{ID: "x", EventDate: "bad", Artist: Artist{Name: "Gojira"}}
	got := sl.Show()
	if got.Tour != "" || got.Date != "bad" || len(got.Sets) != 0 {
		t.Errorf("Show() = %+v, want no tour, raw date and no sets", got)
	}
}
