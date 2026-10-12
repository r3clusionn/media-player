package stremio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// TitleRef is what the library remembers about a title that was played, so
// Continue Watching and Next Up can be shown without asking the add-ons.
type TitleRef struct {
	MediaID      int       `json:"mediaId"` // SyntheticID(ImdbID)
	Type         string    `json:"type"`
	ImdbID       string    `json:"imdbId"`
	Name         string    `json:"name"`
	Poster       string    `json:"poster,omitempty"`
	Background   string    `json:"background,omitempty"`
	VideoID      string    `json:"videoId"` // last played: "tt..:s:e" or the movie's ID
	Season       int       `json:"season,omitempty"`
	Episode      int       `json:"episode,omitempty"`
	EpisodeTitle string    `json:"episodeTitle,omitempty"`
	Thumbnail    string    `json:"thumbnail,omitempty"`
	BingeGroup   string    `json:"bingeGroup,omitempty"` // release of the last stream, for the next episode
	Quality      string    `json:"quality,omitempty"`
	Source       string    `json:"source,omitempty"` // add-on of the last stream: next episodes ask it first
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Library keeps TitleRefs in a JSON file.
type Library struct {
	path   string
	mu     sync.Mutex
	titles map[int]*TitleRef
}

func NewLibrary(dataDir string) *Library {
	l := &Library{path: filepath.Join(dataDir, "stremio", "library.json"), titles: map[int]*TitleRef{}}
	if b, err := os.ReadFile(l.path); err == nil {
		_ = json.Unmarshal(b, &l.titles)
	}
	return l
}

func (l *Library) save() {
	_ = os.MkdirAll(filepath.Dir(l.path), 0o755)
	b, err := json.Marshal(l.titles)
	if err != nil {
		return
	}
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}

func (l *Library) Remember(t *TitleRef) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t.UpdatedAt = time.Now()
	cp := *t
	l.titles[t.MediaID] = &cp
	l.save()
}

func (l *Library) Get(mediaID int) (*TitleRef, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t, ok := l.titles[mediaID]
	if !ok {
		return nil, false
	}
	cp := *t
	return &cp, true
}

func (l *Library) Forget(mediaID int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.titles, mediaID)
	l.save()
}

// All returns the remembered titles, most recent first.
func (l *Library) All() []*TitleRef {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*TitleRef, 0, len(l.titles))
	for _, t := range l.titles {
		cp := *t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
