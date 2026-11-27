package bills

import "testing"

func TestMerchantName(t *testing.T) {
	setupTestDB(t)

	cases := map[string]string{
		"PLATBA KARTOU | TESCO EINSTEINOVA": "tesco",
		"TESCO HYBESOVA":                    "tesco",
		"Tesco Hybe\u0161ova, Bratislava":   "tesco",
		"LIDL BRATISLAVA 4":                 "lidl",
		"DR.MAX LEKARNA PRAHA 1":            "dr max",
		"":                                  "",
	}
	for input, expected := range cases {
		if got := MerchantName(input); got != expected {
			t.Errorf("MerchantName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestMerchantKey(t *testing.T) {
	setupTestDB(t)
	if got := MerchantKey("Tesco Hybe\u0161ova"); got != "tesco" {
		t.Errorf("MerchantKey = %q, want tesco", got)
	}
}

func TestResolveMerchantUsesAlias(t *testing.T) {
	setupTestDB(t)
	if err := SaveMerchantAlias("tesco", "tesco-stores", "Tesco Stores", "user", "user"); err != nil {
		t.Fatalf("save alias: %v", err)
	}
	aliases, err := LoadMerchantAliases("user")
	if err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	name, key := ResolveMerchant("TESCO HYBESOVA", aliases)
	if name != "Tesco Stores" || key != "tesco-stores" {
		t.Errorf("ResolveMerchant = %q / %q", name, key)
	}
}

func TestResolveMerchantFollowsAliasChain(t *testing.T) {
	setupTestDB(t)
	if err := SaveMerchantAlias("alfa", "beta-brand", "Beta Brand", "user", "user"); err != nil {
		t.Fatalf("save first alias: %v", err)
	}
	if err := SaveMerchantAlias("beta-brand", "gamma-brand", "Gamma Brand", "user", "user"); err != nil {
		t.Fatalf("save second alias: %v", err)
	}
	aliases, err := LoadMerchantAliases("user")
	if err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	name, key := ResolveMerchant("ALFA STORE", aliases)
	if name != "Gamma Brand" || key != "gamma-brand" {
		t.Errorf("ResolveMerchant = %q / %q", name, key)
	}
}

func TestResolveMerchantUserAliasWins(t *testing.T) {
	setupTestDB(t)
	if err := SaveMerchantAlias("alfa", "global-brand", "Global Brand", "", "seed"); err != nil {
		t.Fatalf("save global alias: %v", err)
	}
	if err := SaveMerchantAlias("alfa", "user-brand", "User Brand", "user", "user"); err != nil {
		t.Fatalf("save user alias: %v", err)
	}
	aliases, err := LoadMerchantAliases("user")
	if err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	name, key := ResolveMerchant("ALFA STORE", aliases)
	if name != "User Brand" || key != "user-brand" {
		t.Errorf("ResolveMerchant = %q / %q", name, key)
	}
}

func TestMerchantGroupingFoldsLocations(t *testing.T) {
	setupTestDB(t)
	type group struct {
		total     float64
		locations map[string]bool
	}
	groups := map[string]*group{}
	for _, description := range []string{"PLATBA KARTOU | TESCO EINSTEINOVA", "Tesco Hybe\u0161ova, Bratislava"} {
		_, key := ResolveMerchant(description, nil)
		if groups[key] == nil {
			groups[key] = &group{locations: map[string]bool{}}
		}
		groups[key].total += 10
		groups[key].locations[description] = true
	}
	if len(groups) != 1 {
		t.Fatalf("expected one merchant group, got %d", len(groups))
	}
	for key, group := range groups {
		if key != "tesco" || group.total != 20 || len(group.locations) != 2 {
			t.Errorf("group %q = total %v locations %d", key, group.total, len(group.locations))
		}
	}
}
