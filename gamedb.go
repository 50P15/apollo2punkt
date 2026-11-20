package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

const (
	gameDBBase = "https://raw.githubusercontent.com/LizardByte/GameDB/gh-pages"
	igdbImages = "https://images.igdb.com/igdb/image/upload"
)

var imageIDRe = regexp.MustCompile(`/([A-Za-z0-9_-]+)\.(?:jpg|jpeg|png|webp)$`)

type gameDBImage struct {
	URL string `json:"url"`
}

type gameDBGame struct {
	Name        string        `json:"name"`
	Cover       *gameDBImage  `json:"cover"`
	Artworks    []gameDBImage `json:"artworks"`
	Screenshots []gameDBImage `json:"screenshots"`
}

func imageIDOf(u string) string {
	if m := imageIDRe.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// artFor is the cover and a wide image for the IGDB id the user already picked in Vibepollo.
func artFor(client *http.Client, id string) (Artwork, string, error) {
	res, err := client.Get(fmt.Sprintf("%s/games/%s.json", gameDBBase, id))
	if err != nil {
		return Artwork{}, "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Artwork{}, "", fmt.Errorf("IGDB %s no está en GameDB", id)
	}
	if res.StatusCode != http.StatusOK {
		return Artwork{}, "", fmt.Errorf("GameDB respondió HTTP %d", res.StatusCode)
	}
	var g gameDBGame
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&g); err != nil {
		return Artwork{}, "", err
	}
	var art Artwork
	if g.Cover != nil {
		if c := imageIDOf(g.Cover.URL); c != "" {
			art.Portrait = fmt.Sprintf("%s/t_cover_big_2x/%s.jpg", igdbImages, c)
		}
	}
	for _, list := range [][]gameDBImage{g.Artworks, g.Screenshots} {
		if art.Hero != "" {
			break
		}
		for _, im := range list {
			if h := imageIDOf(im.URL); h != "" {
				art.Hero = fmt.Sprintf("%s/t_1080p/%s.jpg", igdbImages, h)
				break
			}
		}
	}
	return art, g.Name, nil
}

func newWebClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }
