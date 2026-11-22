package bills

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := InitBillDB(conn); err != nil {
		t.Fatalf("init bill db: %v", err)
	}
}

func TestCategorizeBankTransaction(t *testing.T) {
	setupTestDB(t)

	cases := []struct {
		description string
		expected    string
	}{
		{"PLATBA KARTOU | WOLT", "Dining"},
		{"BOLT FOOD PRAGUE", "Dining"},
		{"Bolt.eu", "Transport"},
		{"UBER EATS", "Dining"},
		{"UBER TRIP", "Transport"},
		{"ALBERT HYPERMARKET", "Groceries"},
		{"NETFLIX.COM", "Entertainment"},
		{"OMV CERPACI STANICE", "Transport"},
		{"DR.MAX LEKARNA", "Healthcare"},
		{"Neznamy Obchod", "Other"},
	}

	for _, tc := range cases {
		got := CategorizeBankTransaction(BankTransaction{
			Description: tc.description,
			Direction:   "DBIT",
			Amount:      100,
			UserID:      "user",
		})
		if got != tc.expected {
			t.Errorf("CategorizeBankTransaction(%q) = %q, want %q", tc.description, got, tc.expected)
		}
	}
}

func TestNormalizeMerchantText(t *testing.T) {
	cases := map[string]string{
		"PLATBA KARTOU | BOLT FOOD": "platba kartou bolt food",
		"Dr.Max, Praha 1":           "dr max praha 1",
		"h&m":                       "h m",
	}
	for input, expected := range cases {
		if got := normalizeMerchantText(input); got != expected {
			t.Errorf("normalizeMerchantText(%q) = %q, want %q", input, got, expected)
		}
	}
}
