package bills

import (
	"sort"
	"strings"
)

var merchantBoilerplate = []string{
	"platba kartou", "card payment", "kartou", "nakup", "payment", "trx", "pos",
}

var merchantLocationWords = map[string]bool{
	"ulica": true, "namestie": true, "trieda": true, "cesta": true,
	"street": true, "road": true, "praha": true, "bratislava": true,
	"brno": true, "kosice": true,
}

// MerchantName keeps the counterparty after the last pipe, the cleanest brand
// signal in a camt.053 description.
func MerchantName(description string) string {
	if idx := strings.LastIndex(description, "|"); idx >= 0 {
		description = description[idx+1:]
	}
	if idx := strings.Index(description, ","); idx >= 0 {
		description = description[:idx]
	}
	tokens := strings.Fields(stripMerchantBoilerplate(normalizeMerchantText(description)))
	for len(tokens) > 1 && isMerchantTail(tokens[len(tokens)-1]) {
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) == 0 {
		return ""
	}
	if brand, ok := seedMerchantPrefix(strings.Join(tokens, " ")); ok {
		return brand
	}
	return tokens[0]
}

func MerchantKey(description string) string {
	return MerchantSlug(MerchantName(description))
}

func MerchantSlug(name string) string {
	return strings.ReplaceAll(normalizeMerchantText(name), " ", "-")
}

func ResolveMerchant(description string, aliases map[string]string) (string, string) {
	name := MerchantName(description)
	key := MerchantSlug(name)
	seen := map[string]bool{}
	for {
		canonical, ok := aliases[key]
		if !ok || canonical == "" || seen[key] {
			return name, key
		}
		seen[key] = true
		name = canonical
		key = MerchantSlug(canonical)
	}
}

func stripMerchantBoilerplate(value string) string {
	for {
		stripped := false
		for _, phrase := range merchantBoilerplate {
			if value == phrase {
				return ""
			}
			if strings.HasPrefix(value, phrase+" ") {
				value = strings.TrimSpace(value[len(phrase)+1:])
				stripped = true
			}
		}
		if !stripped {
			return value
		}
	}
}

func isMerchantTail(token string) bool {
	if merchantLocationWords[token] {
		return true
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func seedMerchantPrefix(value string) (string, bool) {
	loadSeedEntries()
	padded := value + " "
	for _, entry := range seedEntriesByLength {
		if strings.HasPrefix(padded, entry.keyword+" ") {
			return entry.keyword, true
		}
	}
	return "", false
}

func LoadMerchantAliases(userID string) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT alias_key, display_name FROM merchant_aliases
		WHERE user_id = ? OR user_id = ''
		ORDER BY CASE WHEN user_id = '' THEN 0 ELSE 1 END
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := map[string]string{}
	for rows.Next() {
		var aliasKey, displayName string
		if err := rows.Scan(&aliasKey, &displayName); err != nil {
			return nil, err
		}
		aliases[aliasKey] = displayName
	}
	return aliases, rows.Err()
}

func SaveMerchantAlias(aliasKey, merchantKey, displayName, userID, source string) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO merchant_aliases (alias_key, merchant_key, display_name, user_id, source)
		VALUES (?, ?, ?, ?, ?)
	`, aliasKey, merchantKey, displayName, userID, source)
	return err
}

func DeleteMerchantAlias(aliasKey, userID string) error {
	_, err := db.Exec(`DELETE FROM merchant_aliases WHERE alias_key = ? AND user_id = ?`, aliasKey, userID)
	return err
}

type MerchantSuggestion struct {
	AliasKey     string `json:"alias_key"`
	AliasName    string `json:"alias_name"`
	MerchantKey  string `json:"merchant_key"`
	MerchantName string `json:"merchant_name"`
}

func SuggestMerchantMerges(userID string) ([]MerchantSuggestion, error) {
	transactions, err := GetBankTransactions(userID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, tx := range transactions {
		if tx.Direction != "DBIT" || tx.IsSavingsRoundup || tx.MerchantKey == "" {
			continue
		}
		names[tx.MerchantKey] = tx.Merchant
	}
	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	suggestions := make([]MerchantSuggestion, 0)
	for i, left := range keys {
		for _, right := range keys[i+1:] {
			if trigramSimilarity(left, right) < 0.8 {
				continue
			}
			suggestions = append(suggestions, MerchantSuggestion{
				AliasKey: left, AliasName: names[left],
				MerchantKey: right, MerchantName: names[right],
			})
		}
	}
	return suggestions, nil
}

func trigramSimilarity(a, b string) float64 {
	left, right := trigrams(a), trigrams(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	shared := 0
	for gram := range left {
		if right[gram] {
			shared++
		}
	}
	return float64(shared) / float64(len(left)+len(right)-shared)
}

func trigrams(value string) map[string]bool {
	grams := map[string]bool{}
	for i := 0; i+3 <= len(value); i++ {
		grams[value[i:i+3]] = true
	}
	return grams
}
