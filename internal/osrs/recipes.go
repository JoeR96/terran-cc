package osrs

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
