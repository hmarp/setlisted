package setlistfm

import "github.com/hmarp/setlisted/internal/setlist"

// Show converts the setlist into the app's own model with every entry
// classified (spec §5). The conversion lives here, not in package setlist,
// so that package stays free of HTTP dependencies.
func (s Setlist) Show() setlist.Show {
	tour := ""
	if s.Tour != nil {
		tour = s.Tour.Name
	}
	h := setlist.Header{
		ID:    s.ID,
		Band:  s.Artist.Name,
		Date:  s.DisplayDate(),
		Venue: s.Venue.Name,
		City:  s.Venue.City.Name,
		Tour:  tour,
		URL:   s.URL,
	}
	sets := make([]setlist.SourceSet, 0, len(s.Sets.Set))
	for _, set := range s.Sets.Set {
		src := setlist.SourceSet{Name: set.Name, Encore: set.Encore, Entries: make([]setlist.SourceEntry, 0, len(set.Song))}
		for _, song := range set.Song {
			cover := ""
			if song.Cover != nil {
				cover = song.Cover.Name
			}
			src.Entries = append(src.Entries, setlist.SourceEntry{
				Name:        song.Name,
				Note:        song.Info,
				Tape:        song.Tape,
				CoverArtist: cover,
			})
		}
		sets = append(sets, src)
	}
	return setlist.Build(h, sets)
}
