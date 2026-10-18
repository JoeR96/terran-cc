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
	Doses bool `json:"doses,omitempty"`
	// Best counts only the highest tier held: Names run best first, and the
	// first one in the bank is the one tracked; lower tiers show beside it.
	Best bool     `json:"best,omitempty"`
	Raw  []string `json:"raw,omitempty"` // shown beside, not counted
	// Recipes say how the bank's materials turn into this item; what they
	// could make counts towards sufficient and target.
	Recipes    []Recipe `json:"recipes,omitempty"`
	Sufficient int      `json:"sufficient"`
	Target     int      `json:"target"`
}

// Stock is a StockItem with what the bank holds now.
type Stock struct {
	StockItem
	Have     int    `json:"have"`
	RawHas   int    `json:"raw_have"`
	Tier     string `json:"tier,omitempty"`  // with Best: the tier being counted
	Lower    int    `json:"lower,omitempty"` // with Best: what the lower tiers hold
	Makeable int    `json:"makeable"`        // more the materials could make
	Parts    []Part `json:"parts"`           // every material in the recipe chain
	Status   string `json:"status"`          // low | ok | full, counting what can be made
}

// DefaultStock is the list agreed with Joe on 2026-10-17. Sufficient is 5%
// of the target.
func DefaultStock() []StockItem {
	item := func(key, label, icon string, names []string, doses bool, raw []string, target int) StockItem {
		return StockItem{Key: key, Label: label, Icon: icon, Names: names, Doses: doses, Raw: raw, Sufficient: target / 20, Target: target}
	}
	return []StockItem{
		item("anglerfish", "Anglerfish", "Anglerfish.png", []string{"Anglerfish"}, false, []string{"Raw anglerfish"}, 20000),
		// Finished ammo is what counts; the tips beside it are the raw stage.
		withRecipes(item("seeking-arrows", "Seeking amethyst arrows", "Seeking_amethyst_arrow_5.png", []string{"Seeking amethyst arrow"}, false, nil, 20000), seekingArrowRecipes()),
		withRecipes(item("amethyst-darts", "Amethyst darts", "Amethyst_dart.png", []string{"Amethyst dart"}, false, nil, 5000), dartRecipes()),
		item("house-tabs", "House teleports", "Teleport_to_house_(tablet).png", []string{"Teleport to house"}, false, nil, 1000),
		item("prayer", "Prayer potion (doses)", "Prayer_potion(4).png", []string{"Prayer potion"}, true, nil, 1200),
		item("restore", "Super restore (doses)", "Super_restore(4).png", []string{"Super restore"}, true, nil, 1200),
		item("brew", "Saradomin brew (doses)", "Saradomin_brew(4).png", []string{"Saradomin brew"}, true, nil, 1200),
		item("ranging", "Ranging potion (doses)", "Ranging_potion(4).png", []string{"Ranging potion"}, true, nil, 800),
		item("super-combat", "Super combat (doses)", "Super_combat_potion(4).png", []string{"Super combat potion"}, true, nil, 400),
		best(item("anti-venom", "Anti-venom, best tier (doses)", "Extended_anti-venom+(4).png", []string{"Extended anti-venom+", "Anti-venom+", "Anti-venom"}, true, nil, 400)),
		item("divine-super-combat", "Divine super combat (doses)", "Divine_super_combat_potion(4).png", []string{"Divine super combat potion"}, true, nil, 400),
		item("divine-ranging", "Divine ranging (doses)", "Divine_ranging_potion(4).png", []string{"Divine ranging potion"}, true, nil, 400),
		item("karambwan", "Karambwan", "Cooked_karambwan.png", []string{"Cooked karambwan"}, false, []string{"Raw karambwan"}, 500),
		// Sailing-era stamina for long trips, made with trawled marlin scales.
		item("extended-stamina", "Extended stamina (doses)", "Extended_stamina_potion(4).png", []string{"Extended stamina potion"}, true, nil, 400),
	}
}

func best(it StockItem) StockItem {
	it.Best = true
	return it
}

func withRecipes(it StockItem, r []Recipe) StockItem {
	it.Recipes = r
	return it
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
		s := Stock{StockItem: it, Have: count(have, it.Names, it.Doses), RawHas: count(have, it.Raw, false), Parts: []Part{}}
		if it.Best {
			s.Have, s.Lower = 0, 0
			for _, n := range it.Names {
				c := count(have, []string{n}, it.Doses)
				if s.Tier == "" && c > 0 {
					s.Tier, s.Have = n, c
				} else {
					s.Lower += c
				}
			}
		}
		if len(it.Recipes) > 0 && len(it.Names) > 0 {
			s.Makeable = makeable(it.Names[0], have, it.Recipes, 0)
			s.Parts = partsOf(it.Names[0], have, it.Recipes)
		}
		total := s.Have + s.Makeable
		switch {
		case total < it.Sufficient:
			s.Status = "low"
		case total < it.Target:
			s.Status = "ok"
		default:
			s.Status = "full"
		}
		out = append(out, s)
	}
	return out
}
