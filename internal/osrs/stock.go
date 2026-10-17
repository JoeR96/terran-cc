package osrs

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// StockItem is one thing Joe keeps a stockpile of.
type StockItem struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Icon  string   `json:"icon"`  // OSRS Wiki file name
	Names []string `json:"names"` // bank item names that count
	// Doses counts potions by the dose: "Prayer potion" matches
	// "Prayer potion(1)" to "(4)" and adds up the doses.
	Doses      bool     `json:"doses,omitempty"`
	Raw        []string `json:"raw,omitempty"` // shown beside, not counted
	Sufficient int      `json:"sufficient"`
	Target     int      `json:"target"`
}

// Stock is a StockItem with what the bank holds now.
type Stock struct {
	StockItem
	Have   int    `json:"have"`
	RawHas int    `json:"raw_have"`
	Status string `json:"status"` // low | ok | full
}

// DefaultStock is the list agreed with Joe on 2026-10-17. Sufficient is 5%
// of the target.
func DefaultStock() []StockItem {
	item := func(key, label, icon string, names []string, doses bool, raw []string, target int) StockItem {
		return StockItem{Key: key, Label: label, Icon: icon, Names: names, Doses: doses, Raw: raw, Sufficient: target / 20, Target: target}
	}
	return []StockItem{
		item("anglerfish", "Anglerfish", "Anglerfish.png", []string{"Anglerfish"}, false, []string{"Raw anglerfish"}, 20000),
		item("amethyst-arrowtips", "Amethyst arrowtips", "Amethyst_arrowtips.png", []string{"Amethyst arrowtips"}, false, nil, 20000),
		item("amethyst-darts", "Amethyst darts", "Amethyst_dart.png", []string{"Amethyst dart"}, false, nil, 5000),
		item("house-tabs", "House teleports", "Teleport_to_house_(tablet).png", []string{"Teleport to house"}, false, nil, 1000),
		item("prayer", "Prayer potion (doses)", "Prayer_potion(4).png", []string{"Prayer potion"}, true, nil, 1200),
		item("restore", "Super restore (doses)", "Super_restore(4).png", []string{"Super restore"}, true, nil, 1200),
		item("brew", "Saradomin brew (doses)", "Saradomin_brew(4).png", []string{"Saradomin brew"}, true, nil, 1200),
		item("ranging", "Ranging potion (doses)", "Ranging_potion(4).png", []string{"Ranging potion"}, true, nil, 800),
		item("super-combat", "Super combat (doses)", "Super_combat_potion(4).png", []string{"Super combat potion"}, true, nil, 400),
		item("anti-venom", "Anti-venom (doses)", "Anti-venom(4).png", []string{"Anti-venom"}, true, nil, 400),
		item("karambwan", "Karambwan", "Cooked_karambwan.png", []string{"Cooked karambwan"}, false, []string{"Raw karambwan"}, 500),
	}
}

// DefaultAFK is Joe's fallback list of AFK methods.
var DefaultAFK = []string{"Bloodwood sap", "Anglerfish", "Amethyst", "Shark lures"}

// readContainer parses a Data Export container file: one {id, quantity,
// name} JSON object per line. It returns quantities by name.
func readContainer(r io.Reader, into map[string]int) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var it struct {
			Quantity int    `json:"quantity"`
			Name     string `json:"name"`
		}
		if json.Unmarshal(sc.Bytes(), &it) == nil && it.Name != "" {
			into[it.Name] += it.Quantity
		}
	}
}

var doseRe = regexp.MustCompile(`^(.*)\(([1-4])\)$`)

func count(have map[string]int, names []string, doses bool) int {
	n := 0
	if !doses {
		for _, name := range names {
			n += have[name]
		}
		return n
	}
	for item, q := range have {
		m := doseRe.FindStringSubmatch(item)
		if m == nil {
			continue
		}
		for _, name := range names {
			if strings.EqualFold(m[1], name) {
				d, _ := strconv.Atoi(m[2])
				n += q * d
			}
		}
	}
	return n
}

// Stocks measures each item against what the containers hold.
func Stocks(items []StockItem, have map[string]int) []Stock {
	out := make([]Stock, 0, len(items))
	for _, it := range items {
		s := Stock{StockItem: it, Have: count(have, it.Names, it.Doses), RawHas: count(have, it.Raw, false)}
		switch {
		case s.Have < it.Sufficient:
			s.Status = "low"
		case s.Have < it.Target:
			s.Status = "ok"
		default:
			s.Status = "full"
		}
		out = append(out, s)
	}
	return out
}
