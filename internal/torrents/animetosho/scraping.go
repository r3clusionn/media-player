package animetosho

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// media-player: the standard library instead of colly (upstream's built-in
// providers were removed in 3.x).

var (
	hrefRe     = regexp.MustCompile(`href="([^"]+)"`)
	httpClient = &http.Client{Timeout: 20 * time.Second}
)

func pageLinks(url string) ([]string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unexpected status " + resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range hrefRe.FindAllStringSubmatch(string(b), -1) {
		out = append(out, strings.ReplaceAll(m[1], "&amp;", "&"))
	}
	return out, nil
}

func TorrentFile(viewURL string) (string, error) {
	links, err := pageLinks(viewURL)
	if err != nil {
		return "", err
	}
	for _, l := range links {
		if strings.HasSuffix(l, ".torrent") {
			return l, nil
		}
	}
	return "", errors.New("download link not found")
}

func TorrentMagnet(viewURL string) (string, error) {
	links, err := pageLinks(viewURL)
	if err != nil {
		return "", err
	}
	for _, l := range links {
		if strings.HasPrefix(l, "magnet:?xt=") {
			return l, nil
		}
	}
	return "", errors.New("magnet link not found")
}

func TorrentHash(viewURL string) (string, error) {
	file, err := TorrentFile(viewURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Replace(file, "https://", "", 1), "/")
	if len(parts) < 4 {
		return "", errors.New("hash not found")
	}
	return parts[3], nil
}
