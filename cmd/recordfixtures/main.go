//go:build live

// Command recordfixtures records the matching-quality fixtures (step 09,
// ADR-006) from the live setlist.fm and Spotify APIs. It's run by hand,
// never in CI, and is only built with the live tag:
//
//	go run -tags live ./cmd/recordfixtures [-market GB] [-out testdata/quality] <setlist ID or URL>...
//
// For each setlist it writes, under <out>/<setlist-id>/:
//
//   - setlist.json: the setlist, trimmed to the fields the app decodes;
//   - searches.json: every search the pipeline can make for it, trimmed to
//     the fields the matcher reads. Both artists of every cover are searched,
//     so the fallback data exists even when the band's pass finds a match.
//
// Existing setlist.json and searches.json are replaced; expected.json is
// never touched.
//
// It reads SETLISTFM_API_KEY, SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET
// from the environment or a local .env, and gets a Spotify app token with
// the client-credentials flow (no user login). Keys and tokens are never
// printed or written.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/hmarp/setlisted/internal/quality"
	"github.com/hmarp/setlisted/internal/setlist"
	"github.com/hmarp/setlisted/internal/setlistfm"
	"github.com/hmarp/setlisted/internal/spotify"
)

const tokenURL = "https://accounts.spotify.com/api/token"

// searchPause spaces the Spotify searches; the recorder isn't in a hurry.
const searchPause = 150 * time.Millisecond

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "recordfixtures:", err)
		os.Exit(1)
	}
}

func run() error {
	market := flag.String("market", "GB", "Spotify market (ISO country code) to search in")
	out := flag.String("out", filepath.Join("testdata", "quality"), "fixtures directory")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run -tags live ./cmd/recordfixtures [-market GB] [-out dir] <setlist ID or setlist.fm URL>...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		return errors.New("no setlists given")
	}
	var ids []string
	for _, a := range flag.Args() {
		id, err := setlistID(a)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// godotenv's parse errors quote the file, which holds secrets.
		return errors.New("reading .env failed (check that every line is NAME=value)")
	}
	env := map[string]string{}
	for _, k := range []string{"SETLISTFM_API_KEY", "SPOTIFY_CLIENT_ID", "SPOTIFY_CLIENT_SECRET"} {
		if env[k] = os.Getenv(k); env[k] == "" {
			return fmt.Errorf("%s is not set", k)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	token, err := appToken(ctx, env["SPOTIFY_CLIENT_ID"], env["SPOTIFY_CLIENT_SECRET"])
	if err != nil {
		return err
	}
	sfm := setlistfm.New(setlistfm.Config{APIKey: env["SETLISTFM_API_KEY"]})
	sp := spotify.New(spotify.Config{})

	for _, id := range ids {
		if err := record(ctx, sfm, sp, token, *market, *out, id); err != nil {
			return fmt.Errorf("setlist %s: %w", id, err)
		}
	}
	return nil
}

// setlistID accepts a bare ID or a setlist.fm URL, whose page name ends in
// -<id>.html.
func setlistID(s string) (string, error) {
	hex := regexp.MustCompile(`^[0-9a-fA-F]{4,16}$`)
	if hex.MatchString(s) {
		return s, nil
	}
	if u, err := url.Parse(s); err == nil && strings.HasSuffix(u.Hostname(), "setlist.fm") && strings.HasPrefix(u.Path, "/setlist/") {
		base := strings.TrimSuffix(filepath.Base(u.Path), ".html")
		if i := strings.LastIndex(base, "-"); i >= 0 && hex.MatchString(base[i+1:]) {
			return base[i+1:], nil
		}
	}
	return "", fmt.Errorf("%q is neither a setlist ID nor a setlist.fm setlist URL", s)
}

func record(ctx context.Context, sfm *setlistfm.Client, sp *spotify.Client, token, market, out, id string) error {
	sl, err := sfm.Setlist(ctx, id)
	if err != nil {
		return err
	}
	show := sl.Show()
	s := quality.Searches{Market: market, Recorded: time.Now().UTC().Format("2006-01-02")}
	seen := map[[2]string]bool{}
	var covers, tapes, medleys, skipped int
	for _, e := range show.Entries() {
		switch {
		case e.Kind == setlist.KindSkipped:
			skipped++
		case e.Kind == setlist.KindMedley:
			medleys++
		}
		if e.IsTape {
			tapes++
		}
		if e.Kind != setlist.KindSong {
			continue
		}
		if e.IsCover() {
			covers++
		}
		for _, artist := range e.ExpectedArtists {
			k := [2]string{e.Name, artist}
			if seen[k] {
				continue // a repeated song (ENT-7) is searched once
			}
			seen[k] = true
			cands, err := sp.SearchTracks(ctx, token, e.Name, artist, market)
			if err != nil {
				return fmt.Errorf("searching %q by %q: %w", e.Name, artist, err)
			}
			q, _ := spotify.Query(e.Name, artist)
			rec := quality.Search{Track: e.Name, Artist: artist, Query: q, Results: []quality.Track{}}
			for _, c := range cands {
				rec.Results = append(rec.Results, quality.FromCandidate(c))
			}
			s.Searches = append(s.Searches, rec)
			time.Sleep(searchPause)
		}
	}

	dir := filepath.Join(out, show.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := quality.WriteJSON(filepath.Join(dir, quality.SetlistFile), sl); err != nil {
		return err
	}
	if err := quality.WriteJSON(filepath.Join(dir, quality.SearchesFile), s); err != nil {
		return err
	}
	note := ""
	if _, err := os.Stat(filepath.Join(dir, quality.ExpectedFile)); err != nil {
		note = " (no expected.json yet)"
	}
	fmt.Printf("%s  %s, %s, %s: %d matchable (%d covers, %d tapes), %d medleys, %d skipped; %d searches → %s%s\n",
		show.ID, show.Band, show.Date, show.Venue, show.Matchable(), covers, tapes, medleys, skipped, len(s.Searches), dir, note)
	return nil
}

// appToken gets a Spotify app access token (client-credentials flow). Error
// text never includes the credentials, the token or the response body.
func appToken(ctx context.Context, id, secret string) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("building the token request failed")
	}
	req.SetBasicAuth(id, secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return "", fmt.Errorf("getting a Spotify app token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("getting a Spotify app token: status %d (check SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET)", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", errors.New("getting a Spotify app token: unexpected response")
	}
	return body.AccessToken, nil
}
