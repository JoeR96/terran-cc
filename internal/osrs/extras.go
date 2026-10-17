package osrs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"terrancc/internal/model"
)

// Sources the extras are read from; tests point these at fakes.
var (
	templeURL   = "https://templeosrs.com/api/collection-log/player_collection_log.php?categories=all&onlyitems=1&player="
	mappingURL  = "https://prices.runescape.wiki/api/v1/osrs/mapping"
	wikisyncURL = "https://sync.runescape.wiki/runelite/player/%s/STANDARD"
)

// extrasEvery is how often the collection log and diaries are read.
const extrasEvery = 30 * time.Minute

func getJSON(ctx context.Context, hc *http.Client, u, version string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "terran-cc/"+version)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

// fetchObtained reads the collection log from TempleOSRS and names its item
// ids with the OSRS Wiki's item list. names caches that list between calls.
func fetchObtained(ctx context.Context, hc *http.Client, player, version string, names map[int]string) (map[string]bool, map[int]string, error) {
	if names == nil {
		var mapping []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := getJSON(ctx, hc, mappingURL, version, &mapping); err != nil {
			return nil, nil, err
		}
		names = map[int]string{}
		for _, m := range mapping {
			names[m.ID] = m.Name
		}
	}
	var clog struct {
		Data struct {
			Items []struct {
				ID int `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := getJSON(ctx, hc, templeURL+url.QueryEscape(player), version, &clog); err != nil {
		return nil, names, err
	}
	got := map[string]bool{}
	for _, it := range clog.Data.Items {
		if n, ok := names[it.ID]; ok {
			got[strings.ToLower(n)] = true
		}
	}
	return got, names, nil
}

// fetchDiaries reads elite diary progress from WikiSync.
func fetchDiaries(ctx context.Context, hc *http.Client, player, version string) (map[string]Diary, error) {
	var body struct {
		Diaries map[string]map[string]struct {
			Complete bool   `json:"complete"`
			Tasks    []bool `json:"tasks"`
		} `json:"achievement_diaries"`
	}
	if err := getJSON(ctx, hc, fmt.Sprintf(wikisyncURL, url.PathEscape(player)), version, &body); err != nil {
		return nil, err
	}
	out := map[string]Diary{}
	for area, tiers := range body.Diaries {
		e, ok := tiers["Elite"]
		if !ok {
			continue
		}
		d := Diary{Total: len(e.Tasks), Complete: e.Complete}
		for _, t := range e.Tasks {
			if t {
				d.Done++
			}
		}
		out[area] = d
	}
	return out, nil
}

// boneXP is the Prayer XP for burying each bone; altars multiply it.
var boneXP = map[string]float64{
	"Bones": 4.5, "Bat bones": 5.3, "Big bones": 15, "Jogre bones": 15, "Zogre bones": 22.5,
	"Babydragon bones": 30, "Wyrm bones": 50, "Dragon bones": 72, "Wyvern bones": 72, "Drake bones": 80,
	"Fayrg bones": 84, "Lava dragon bones": 85, "Raurg bones": 96, "Hydra bones": 110, "Dagannoth bones": 125,
	"Ourg bones": 140, "Superior dragon bones": 150,
}

// altarMultiplier is a gilded altar with both burners lit.
const altarMultiplier = 3.5

// Bones is what the banked bones are worth in Prayer XP.
type Bones struct {
	Count int `json:"count"`
	XP    int `json:"xp"` // at a gilded altar (3.5x)
}

func bonesIn(have map[string]int) Bones {
	var b Bones
	var xp float64
	for name, per := range boneXP {
		q := have[name] + have[name+" (noted)"]
		b.Count += q
		xp += float64(q) * per * altarMultiplier
	}
	b.XP = int(xp)
	return b
}

// Piece is a material to-do's progress: how many the bank holds of how many.
type Piece struct {
	Have int `json:"have"`
	Need int `json:"need"`
}

// Materials measures every open OSRS to-do of the form "716 adamantite
// nails" against the bank.
func Materials(items []model.Item, have map[string]int) map[int64]Piece {
	out := map[int64]Piece{}
	for _, it := range items {
		if it.Context != model.ContextOSRS || it.ParentID == nil || !material.MatchString(it.Title) {
			continue
		}
		m := material.FindStringSubmatch(it.Title)
		if isSkill(strings.TrimSpace(m[2])) {
			continue
		}
		need, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if q, ok := bankCount(have, m[2]); ok {
			out[it.ID] = Piece{Have: q, Need: need}
		}
	}
	return out
}
