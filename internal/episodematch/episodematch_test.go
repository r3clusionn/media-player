package episodematch

import (
	"errors"
	"fmt"
	"math/rand"
	"path"
	"testing"
)

func cands(paths ...string) []Candidate {
	out := make([]Candidate, len(paths))
	for i, p := range paths {
		out[i] = Candidate{Index: i, Path: p}
	}
	return out
}

func numbered(format string, from, to int) []string {
	var out []string
	for e := from; e <= to; e++ {
		out = append(out, fmt.Sprintf(format, e))
	}
	return out
}

// A two-season pack, as Judas and similar groups release it.
func aotPack() []string {
	r := "[Judas] Shingeki no Kyojin (Seasons 1-2) [BD 1080p][HEVC x265 10bit][Eng-Subs]"
	l := numbered(r+"/Season 1/[Judas] Shingeki no Kyojin - S01E%02d.mkv", 1, 25)
	l = append(l, numbered(r+"/Season 2/[Judas] Shingeki no Kyojin - S02E%02d.mkv", 1, 12)...)
	return append(l, r+"/Extras/[Judas] Shingeki no Kyojin - NCOP1.mkv", r+"/Extras/[Judas] Shingeki no Kyojin - NCED1.mkv")
}

// Absolute numbering for a sequel: Jujutsu Kaisen S2 episode 1 is "- 25".
func jjkAbsolute() []string {
	return numbered("[SubsPlease] Jujutsu Kaisen S2 (01-23) (1080p) [Batch]/[SubsPlease] Jujutsu Kaisen - %d (1080p) [ABCD1234].mkv", 25, 47)
}

// Openings, endings and a mini anime listed before the episodes.
func frierenExtras() []string {
	r := "[Erai-raws] Sousou no Frieren - 01 ~ 28 [1080p][Multiple Subtitle]"
	l := []string{
		r + "/Extras/[Erai-raws] Sousou no Frieren - NCOP - 01 [1080p].mkv",
		r + "/Extras/[Erai-raws] Sousou no Frieren - NCED - 01 [1080p].mkv",
		r + "/Specials/[Erai-raws] Sousou no Frieren - Mini Anime - 01 [1080p].mkv",
		r + "/Specials/[Erai-raws] Sousou no Frieren - Mini Anime - 02 [1080p].mkv",
	}
	return append(l, numbered(r+"/[Erai-raws] Sousou no Frieren - %02d [1080p][Multiple Subtitle].mkv", 1, 28)...)
}

func TestPick(t *testing.T) {
	jjkRelative := numbered("[Group] Jujutsu Kaisen S2 [1080p]/[Group] Jujutsu Kaisen S2 - %02d [1080p].mkv", 1, 23)
	frierenV2 := []string{
		"[Group] Sousou no Frieren [1080p]/[Group] Sousou no Frieren - 06 [1080p].mkv",
		"[Group] Sousou no Frieren [1080p]/[Group] Sousou no Frieren - 07 [1080p].mkv",
		"[Group] Sousou no Frieren [1080p]/[Group] Sousou no Frieren - 07v2 [1080p].mkv",
		"[Group] Sousou no Frieren [1080p]/[Group] Sousou no Frieren - 08 [1080p].mkv",
	}
	tests := []struct {
		name  string
		files []string
		t     Target
		want  string // base name of the expected file
		err   error
	}{
		{"second season of a two-season pack", aotPack(), Target{Episode: 5, Season: 2, Offset: 25, EpisodeCount: 12},
			"[Judas] Shingeki no Kyojin - S02E05.mkv", nil},
		{"first season of a two-season pack", aotPack(), Target{Episode: 5, Season: 1, EpisodeCount: 25},
			"[Judas] Shingeki no Kyojin - S01E05.mkv", nil},
		{"last episode of the second season", aotPack(), Target{Episode: 12, Season: 2, Offset: 25, EpisodeCount: 12},
			"[Judas] Shingeki no Kyojin - S02E12.mkv", nil},
		{"absolute numbering, first episode", jjkAbsolute(), Target{Episode: 1, Season: 2, Offset: 24, EpisodeCount: 23},
			"[SubsPlease] Jujutsu Kaisen - 25 (1080p) [ABCD1234].mkv", nil},
		{"absolute numbering, last episode", jjkAbsolute(), Target{Episode: 23, Season: 2, Offset: 24, EpisodeCount: 23},
			"[SubsPlease] Jujutsu Kaisen - 47 (1080p) [ABCD1234].mkv", nil},
		{"relative numbering in a sequel pack", jjkRelative, Target{Episode: 1, Season: 2, Offset: 24, EpisodeCount: 23},
			"[Group] Jujutsu Kaisen S2 - 01 [1080p].mkv", nil},
		{"episode 1 next to openings and a mini anime", frierenExtras(), Target{Episode: 1, Season: 1, EpisodeCount: 28},
			"[Erai-raws] Sousou no Frieren - 01 [1080p][Multiple Subtitle].mkv", nil},
		{"episode 2 next to a mini anime 02", frierenExtras(), Target{Episode: 2, Season: 1, EpisodeCount: 28},
			"[Erai-raws] Sousou no Frieren - 02 [1080p][Multiple Subtitle].mkv", nil},
		{"a v2 beats the original", frierenV2, Target{Episode: 7, Season: 1, EpisodeCount: 28},
			"[Group] Sousou no Frieren - 07v2 [1080p].mkv", nil},
		{"the episode after a v2", frierenV2, Target{Episode: 8, Season: 1, EpisodeCount: 28},
			"[Group] Sousou no Frieren - 08 [1080p].mkv", nil},
		{"season of the entry unknown in a multi-season pack", aotPack(), Target{Episode: 5, EpisodeCount: 12},
			"", ErrUnsure},
		{"two groups' copies of the same episode", []string{
			"Pack/[A] Show - 03 [1080p].mkv", "Pack/[B] Show - 03 [1080p].mkv"}, Target{Episode: 3, Season: 1},
			"", ErrUnsure},
		{"the same episode in two sizes takes the larger", nil, Target{}, "", nil}, // see TestPickPrefersLarger
		{"an episode the torrent doesn't have", frierenExtras(), Target{Episode: 99, Season: 1, EpisodeCount: 28},
			"", ErrNotFound},
		{"a multi-episode file", []string{"Show S3/Show S03E07-E08 1080p.mkv", "Show S3/Show S03E09 1080p.mkv"},
			Target{Episode: 8, Season: 3}, "Show S03E07-E08 1080p.mkv", nil},
		{"a release season that is one AniList entry under another number", numbered("Show S2 [1080p]/Show - S02E%02d.mkv", 1, 12),
			Target{Episode: 3, Season: 3, Offset: 36, EpisodeCount: 12}, "Show - S02E03.mkv", nil},
		{"a release season longer than the entry (AniList split it)", numbered("Show S2 [1080p]/Show - S02E%02d.mkv", 1, 24),
			Target{Episode: 3, Season: 3, Offset: 36, EpisodeCount: 12}, "", ErrUnsure},
		{"subtitle files with the same number are ignored", []string{
			"Pack/[G] Show - 04 [1080p].ass", "Pack/[G] Show - 04 [1080p].mkv"}, Target{Episode: 4, Season: 1},
			"[G] Show - 04 [1080p].mkv", nil},
		{"an OVA entry keeps its Specials folder", []string{"Show/Specials/[G] Show OVA - 01 [1080p].mkv"},
			Target{Episode: 1, SpecialEntry: true}, "[G] Show OVA - 01 [1080p].mkv", nil},
		{"an episode title with the word Special is still an episode", []string{
			"Pack/[G] Show - 05 - A Special Day [1080p].mkv", "Pack/[G] Show - 06 [1080p].mkv"},
			Target{Episode: 5, Season: 1}, "[G] Show - 05 - A Special Day [1080p].mkv", nil},
	}
	for _, tc := range tests {
		if tc.files == nil {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			got, err := Pick(cands(tc.files...), tc.t)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("got %v, %v; want error %v", got, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b := path.Base(got.Path); b != tc.want {
				t.Fatalf("picked %q (%s); want %q", b, got.Reason, tc.want)
			}
		})
	}
}

func TestPickPrefersLarger(t *testing.T) {
	c := []Candidate{
		{Index: 0, Path: "Pack/[G] Show - 03 [720p].mkv", Size: 300 << 20},
		{Index: 1, Path: "Pack/[G] Show - 03 [1080p].mkv", Size: 900 << 20},
	}
	got, err := Pick(c, Target{Episode: 3, Season: 1})
	if err != nil || got.Index != 1 {
		t.Fatalf("got %+v, %v; want index 1", got, err)
	}
}

// The order of the files must never change the answer.
func TestPickIgnoresOrder(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	cases := []struct {
		files []string
		t     Target
	}{
		{aotPack(), Target{Episode: 5, Season: 2, Offset: 25, EpisodeCount: 12}},
		{jjkAbsolute(), Target{Episode: 7, Season: 2, Offset: 24, EpisodeCount: 23}},
		{frierenExtras(), Target{Episode: 1, Season: 1, EpisodeCount: 28}},
	}
	for _, tc := range cases {
		want, err := Pick(cands(tc.files...), tc.t)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 50; i++ {
			c := cands(tc.files...)
			r.Shuffle(len(c), func(a, b int) { c[a], c[b] = c[b], c[a] })
			got, err := Pick(c, tc.t)
			if err != nil || got.Path != want.Path {
				t.Fatalf("shuffle %d: got %v, %v; want %s", i, got, err, want.Path)
			}
		}
	}
}
