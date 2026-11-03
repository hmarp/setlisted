package quality_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/hmarp/setlisted/internal/quality"
)

// fixturesRoot holds the recorded setlists (step 09).
const fixturesRoot = "../../testdata/quality"

// MinRatio is spec §9 success criterion 3's found / matchable threshold.
const MinRatio = 0.90

var draftDir = flag.String("quality.draft", "",
	"write each setlist's actual results as a draft expected.json into this directory (for re-recording); the test still runs")

// TestMatchingQuality runs the pipeline offline on every recorded setlist and
// fails on a wrong-artist match, any difference from expected.json, or an
// overall found / matchable below MinRatio (ADR-006, spec §9 criterion 3).
func TestMatchingQuality(t *testing.T) {
	dirs, err := quality.Dirs(fixturesRoot)
	if err != nil {
		t.Fatalf("listing fixtures: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatalf("no fixtures under %s", fixturesRoot)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var reports []quality.Report
	for _, dir := range dirs {
		f, err := quality.Load(dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		r, err := quality.Evaluate(ctx, f)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(dir), err)
			continue
		}
		reports = append(reports, r)
		for _, m := range r.WrongArtist {
			t.Errorf("%s: WRONG ARTIST %s", r.SetlistID, m)
		}
		for _, m := range r.Mismatches {
			t.Errorf("%s: %s", r.SetlistID, m)
		}
		if *draftDir != "" {
			if err := writeDraft(*draftDir, r); err != nil {
				t.Errorf("writing draft: %v", err)
			}
		}
	}

	t.Log("\n" + table(reports))
	found, matchable := totals(reports)
	if matchable == 0 {
		t.Fatal("no matchable entries")
	}
	if ratio := float64(found) / float64(matchable); ratio < MinRatio {
		t.Errorf("found / matchable = %d / %d = %.1f%%, below %.0f%%", found, matchable, 100*ratio, 100*MinRatio)
	}
}

func totals(rs []quality.Report) (found, matchable int) {
	for _, r := range rs {
		found += r.Found
		matchable += r.Matchable
	}
	return found, matchable
}

func table(rs []quality.Report) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "setlist\tband\tdate\tfound\tmatchable\t%\twrong artist\tmismatches")
	var wrong, mism int
	for _, r := range rs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%.1f\t%d\t%d\n", r.SetlistID, r.Band, r.Date,
			r.Found, r.Matchable, 100*r.Ratio(), len(r.WrongArtist), len(r.Mismatches))
		wrong += len(r.WrongArtist)
		mism += len(r.Mismatches)
	}
	found, matchable := totals(rs)
	pct := 100.0
	if matchable > 0 {
		pct = 100 * float64(found) / float64(matchable)
	}
	fmt.Fprintf(w, "overall\t\t\t%d\t%d\t%.1f\t%d\t%d\n", found, matchable, pct, wrong, mism)
	w.Flush()
	return b.String()
}

// writeDraft writes r's actual results as an expected.json to review. It's
// never written into the fixtures: a person copies it after checking it.
func writeDraft(dir string, r quality.Report) error {
	exp := quality.Expected{SetlistID: r.SetlistID, Comment: "DRAFT from the pipeline's own output: review every entry"}
	for _, res := range r.Results {
		e := quality.Entry{Position: res.Position, Name: res.Name, Status: res.Status, Live: res.Live, Tape: res.IsTape}
		if res.Track != nil {
			e.TrackID = res.Track.ID
			e.Comment = fmt.Sprintf("%q by %s on %q", res.Track.Title, strings.Join(res.Track.Artists, ", "), res.Track.Album)
		}
		exp.Entries = append(exp.Entries, e)
	}
	if err := os.MkdirAll(filepath.Join(dir, r.SetlistID), 0o755); err != nil {
		return err
	}
	return quality.WriteJSON(filepath.Join(dir, r.SetlistID, quality.ExpectedFile), exp)
}
