package nyaa

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// media-player: the standard library instead of colly (upstream's built-in
// providers were removed in 3.x; only the magnet lookup is used).

var magnetHref = regexp.MustCompile(`href="(magnet:\?xt=[^"]+)"`)

var httpClient = &http.Client{Timeout: 20 * time.Second}

func fetchPage(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("unexpected status " + resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), err
}

// TorrentMagnet returns the magnet link on a torrent's view page.
func TorrentMagnet(viewURL string) (string, error) {
	page, err := fetchPage(viewURL)
	if err != nil {
		return "", err
	}
	m := magnetHref.FindStringSubmatch(page)
	if m == nil {
		return "", errors.New("magnet link not found")
	}
	return htmlUnescape(m[1]), nil
}

func htmlUnescape(s string) string {
	return ampRe.ReplaceAllString(s, "&")
}

var ampRe = regexp.MustCompile(`&amp;`)

var (
	titleRe     = regexp.MustCompile(`(?s)<h3 class="panel-title">\s*(.*?)\s*</h3>`)
	seedersRe   = regexp.MustCompile(`<span style="color: green;">(\d+)</span>`)
	leechersRe  = regexp.MustCompile(`<span style="color: red;">(\d+)</span>`)
	completedRe = regexp.MustCompile(`(?s)Completed:</div>\s*<div class="col-md-5">\s*(\d+)\s*</div>`)
	sizeRe      = regexp.MustCompile(`(?s)File size:</div>\s*<div class="col-md-5">\s*([^<]+?)\s*</div>`)
	hashRe      = regexp.MustCompile(`<kbd>([0-9a-fA-F]{40})</kbd>`)
)

// TorrentInfo reads a torrent's view page (used by SeaDex to fill in details).
func TorrentInfo(viewURL string) (title string, seeders int, leechers int, completed int, formattedSize string, infoHash string, magnetLink string, err error) {
	page, err := fetchPage(viewURL)
	if err != nil {
		return
	}
	first := func(re *regexp.Regexp) string {
		if m := re.FindStringSubmatch(page); m != nil {
			return m[1]
		}
		return ""
	}
	title = htmlUnescape(first(titleRe))
	seeders, _ = strconv.Atoi(first(seedersRe))
	leechers, _ = strconv.Atoi(first(leechersRe))
	completed, _ = strconv.Atoi(first(completedRe))
	formattedSize = first(sizeRe)
	infoHash = first(hashRe)
	if m := magnetHref.FindStringSubmatch(page); m != nil {
		magnetLink = htmlUnescape(m[1])
	} else {
		err = errors.New("magnet link not found")
	}
	return
}
