package osrs

import "testing"

func TestMakeableFollowsTheChain(t *testing.T) {
	recipes := seekingArrowRecipes()
	have := map[string]int{
		"Amethyst arrowtips":       20,
		"Amethyst":                 2, // 30 more tips
		"Seeking headless arrow":   15,
		"Seeking arrow shaft":      10,
		"Feather":                  1000,
		"Redwood logs":             3,
		"Bucket of bloodwood sap":  1, // so only one redwood log can be treated: 105 shafts
		"Sap-treated redwood logs": 0,
	}
	// Tips: 20 + 30 = 50 → 3 batches of 15. Headless: 15 + (10+105 shafts →
	// 7 batches) = 15 + 105 = 120 → plenty. So tips limit: 3 batches = 45.
	if got := makeable("Seeking amethyst arrow", have, recipes, 0); got != 45 {
		t.Fatalf("makeable = %d, want 45", got)
	}
	parts := partsOf("Seeking amethyst arrow", have, recipes)
	if len(parts) != 8 || parts[0].Name != "Amethyst arrowtips" || parts[0].Have != 20 {
		t.Fatalf("parts %+v", parts)
	}
	// With no sap, the shafts come only from the bank.
	have["Bucket of bloodwood sap"] = 0
	have["Seeking headless arrow"] = 0
	if got := makeable("Seeking amethyst arrow", have, recipes, 0); got != 0 {
		t.Fatalf("10 shafts can't make a batch of 15: %d", got)
	}
}

func TestStocksCountWhatCanBeMade(t *testing.T) {
	it := StockItem{Key: "darts", Names: []string{"Amethyst dart"}, Sufficient: 100, Target: 1000, Recipes: dartRecipes()}
	have := map[string]int{"Amethyst dart": 50, "Amethyst dart tip": 30, "Amethyst": 10, "Feather": 500}
	s := Stocks([]StockItem{it}, have)[0]
	// 30 tips + 10 amethyst × 8 = 110 tips, 500 feathers: 110 more darts.
	if s.Have != 50 || s.Makeable != 110 || s.Status != "ok" {
		t.Fatalf("%+v", s)
	}
}
