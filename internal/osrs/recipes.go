package osrs

import "strings"

// Recipe makes Qty of Makes from Needs. Recipes chain: a need can itself
// be made by another recipe, down to base resources in the bank.
type Recipe struct {
	Makes string `json:"makes"`
	Qty   int    `json:"qty"`
	Needs []Need `json:"needs"`
}

type Need struct {
	Item string `json:"item"`
	Qty  int    `json:"qty"`
}

// Part is one item in a recipe chain with how many the bank holds.
type Part struct {
	Name string `json:"name"`
	Have int    `json:"have"`
}

// maxDepth guards against a recipe list that loops.
const maxDepth = 8

// makeable is how many of name the bank's materials could make, on top of
// any already made. Materials shared between chains are counted for each,
// so two items using the same amethyst both see all of it.
func makeable(name string, have map[string]int, recipes []Recipe, depth int) int {
	if depth > maxDepth {
		return 0
	}
	total := 0
	for _, r := range recipes {
		if r.Makes != name || r.Qty <= 0 || len(r.Needs) == 0 {
			continue
		}
		batches := -1
		for _, n := range r.Needs {
			if n.Qty <= 0 {
				continue
			}
			avail := have[n.Item] + makeable(n.Item, have, recipes, depth+1)
			if b := avail / n.Qty; batches < 0 || b < batches {
				batches = b
			}
		}
		total += max(batches, 0) * r.Qty
	}
	return total
}

// partsOf lists every item the chain for name uses, in recipe order.
func partsOf(name string, have map[string]int, recipes []Recipe) []Part {
	var out []Part
	seen := map[string]bool{name: true}
	var walk func(string, int)
	walk = func(n string, depth int) {
		if depth > maxDepth {
			return
		}
		for _, r := range recipes {
			if r.Makes != n {
				continue
			}
			for _, need := range r.Needs {
				if !seen[need.Item] {
					seen[need.Item] = true
					out = append(out, Part{need.Item, have[need.Item]})
					walk(need.Item, depth+1)
				}
			}
		}
	}
	walk(name, 0)
	return out
}

// seekingArrowRecipes is the best chain for seeking amethyst arrows (OSRS
// Wiki, October 2026): redwood logs treated with bloodwood sap give 105
// shafts each.
func seekingArrowRecipes() []Recipe {
	return []Recipe{
		{"Seeking amethyst arrow", 15, []Need{{"Amethyst arrowtips", 15}, {"Seeking headless arrow", 15}}},
		{"Amethyst arrowtips", 15, []Need{{"Amethyst", 1}}},
		{"Seeking headless arrow", 15, []Need{{"Seeking arrow shaft", 15}, {"Feather", 15}}},
		{"Seeking arrow shaft", 105, []Need{{"Sap-treated redwood logs", 1}}},
		{"Sap-treated redwood logs", 1, []Need{{"Redwood logs", 1}, {"Bucket of bloodwood sap", 1}}},
	}
}

// dartRecipes: a dart tip and a feather make a dart; an amethyst cuts into
// 8 dart tips.
func dartRecipes() []Recipe {
	return []Recipe{
		{"Amethyst dart", 1, []Need{{"Amethyst dart tip", 1}, {"Feather", 1}}},
		{"Amethyst dart tip", 8, []Need{{"Amethyst", 1}}},
	}
}

// doseUnit names the pool of doses for a potion, e.g. "Prayer potion
// (dose)": recipes count potions by the dose, whatever size they're in.
func doseUnit(base string) string { return base + " (dose)" }

// withDoses adds a "(dose)" pool for every potion in the bank, so recipes
// can ask for doses: 288 Prayer potion(4) + 226 (3) is 1,830 doses.
func withDoses(have map[string]int) map[string]int {
	out := make(map[string]int, len(have))
	for k, v := range have {
		out[k] = v
	}
	for item, q := range have {
		if m := doseRe.FindStringSubmatch(item); m != nil {
			d := int(m[2][0] - '0')
			out[doseUnit(strings.TrimSpace(m[1]))] += q * d
		}
	}
	return out
}

// herbChain is unf potion ← clean herb ← grimy herb ← seed, at the wiki's
// average of 8.8 herbs a seed (44 from 5).
func herbChain(herb, grimy, unf, seed string) []Recipe {
	return []Recipe{
		{unf, 1, []Need{{herb, 1}, {"Vial of water", 1}}},
		{herb, 1, []Need{{grimy, 1}}},
		{grimy, 44, []Need{{seed, 5}}},
	}
}

// potion is a three-dose mix of an unf potion and a secondary.
func potion(base, unf, secondary string) Recipe {
	return Recipe{doseUnit(base), 3, []Need{{unf, 1}, {secondary, 1}}}
}

func join(rs ...[]Recipe) []Recipe {
	var out []Recipe
	for _, r := range rs {
		out = append(out, r...)
	}
	return out
}

// Potion chains (OSRS Wiki, October 2026).
func prayerRecipes() []Recipe {
	return join([]Recipe{potion("Prayer potion", "Ranarr potion (unf)", "Snape grass")},
		herbChain("Ranarr weed", "Grimy ranarr weed", "Ranarr potion (unf)", "Ranarr seed"))
}

func restoreRecipes() []Recipe {
	return join([]Recipe{potion("Super restore", "Snapdragon potion (unf)", "Red spiders' eggs")},
		herbChain("Snapdragon", "Grimy snapdragon", "Snapdragon potion (unf)", "Snapdragon seed"))
}

func brewRecipes() []Recipe {
	return join([]Recipe{potion("Saradomin brew", "Toadflax potion (unf)", "Crushed nest"), {"Crushed nest", 1, []Need{{"Bird nest", 1}}}},
		herbChain("Toadflax", "Grimy toadflax", "Toadflax potion (unf)", "Toadflax seed"))
}

func rangingRecipes() []Recipe {
	return join([]Recipe{potion("Ranging potion", "Dwarf weed potion (unf)", "Wine of zamorak")},
		herbChain("Dwarf weed", "Grimy dwarf weed", "Dwarf weed potion (unf)", "Dwarf weed seed"))
}

// superCombatRecipes: a torstol and one four-dose each of super attack,
// strength and defence make a four-dose super combat.
func superCombatRecipes() []Recipe {
	return join([]Recipe{
		{doseUnit("Super combat potion"), 4, []Need{{"Torstol", 1}, {doseUnit("Super attack"), 4}, {doseUnit("Super strength"), 4}, {doseUnit("Super defence"), 4}}},
		{"Torstol", 1, []Need{{"Grimy torstol", 1}}},
		{"Grimy torstol", 44, []Need{{"Torstol seed", 5}}},
		potion("Super attack", "Irit potion (unf)", "Eye of newt"),
		potion("Super strength", "Kwuarm potion (unf)", "Limpwurt root"),
		potion("Super defence", "Cadantine potion (unf)", "White berries"),
	},
		herbChain("Irit leaf", "Grimy irit leaf", "Irit potion (unf)", "Irit seed"),
		herbChain("Kwuarm", "Grimy kwuarm", "Kwuarm potion (unf)", "Kwuarm seed"),
		herbChain("Cadantine", "Grimy cadantine", "Cadantine potion (unf)", "Cadantine seed"))
}

// divine adds one crystal dust a dose; a crystal shard grinds into 10.
func divine(name, base string, baseChain []Recipe) []Recipe {
	return join([]Recipe{
		{doseUnit(name), 1, []Need{{doseUnit(base), 1}, {"Crystal dust", 1}}},
		{"Crystal dust", 10, []Need{{"Crystal shard", 1}}},
	}, baseChain)
}

// staminaRecipes: one marlin scale a dose on a stamina potion, which is one
// amylase crystal a dose on a super energy.
func staminaRecipes() []Recipe {
	return join([]Recipe{
		{doseUnit("Extended stamina potion"), 1, []Need{{doseUnit("Stamina potion"), 1}, {"Marlin scales", 1}}},
		{doseUnit("Stamina potion"), 1, []Need{{doseUnit("Super energy"), 1}, {"Amylase crystal", 1}}},
		potion("Super energy", "Avantoe potion (unf)", "Mort myre fungus"),
	}, herbChain("Avantoe", "Grimy avantoe", "Avantoe potion (unf)", "Avantoe seed"))
}
