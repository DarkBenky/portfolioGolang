package bills

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func InitBillDB(database *sql.DB) error {
	db = database
	return createTables()
}

type Expense struct {
	ID          int     `json:"id"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Date        string  `json:"date"`
	UserID      string  `json:"user_id"`
}

type User struct {
	UserName string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Id       string `json:"id"`
}

func createTables() error {
	createExpenseTable := `
	CREATE TABLE IF NOT EXISTS expenses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		description TEXT NOT NULL,
		amount REAL NOT NULL,
		category TEXT NOT NULL,
		date TEXT NOT NULL,
		user_id TEXT NOT NULL
	);`

	_, err := db.Exec(createExpenseTable)
	if err != nil {
		return fmt.Errorf("error creating expenses table: %v", err)
	}

	createBankTxTable := `
	CREATE TABLE IF NOT EXISTS bank_transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ntry_ref TEXT NOT NULL,
		acct_svcr_ref TEXT,
		amount REAL NOT NULL,
		currency TEXT NOT NULL DEFAULT 'EUR',
		direction TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'BOOK',
		booking_date TEXT NOT NULL,
		value_date TEXT NOT NULL,
		description TEXT,
		category TEXT NOT NULL DEFAULT 'Other',
		is_savings_roundup INTEGER NOT NULL DEFAULT 0,
		user_id TEXT NOT NULL,
		UNIQUE(ntry_ref, user_id)
	);`

	_, err = db.Exec(createBankTxTable)
	if err != nil {
		return fmt.Errorf("error creating bank_transactions table: %v", err)
	}

	createReportTable := `
	CREATE TABLE IF NOT EXISTS expense_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		period TEXT NOT NULL,
		period_start TEXT NOT NULL,
		period_end TEXT NOT NULL,
		summary TEXT NOT NULL,
		created_at TEXT NOT NULL,
		user_id TEXT NOT NULL
	);`

	_, err = db.Exec(createReportTable)
	if err != nil {
		return fmt.Errorf("error creating expense_reports table: %v", err)
	}

	return nil
}

func GetExpensesByUserID(userID string) ([]Expense, error) {
	query := `SELECT id, description, amount, category, date, user_id FROM expenses WHERE user_id = ? ORDER BY date DESC`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		var exp Expense
		if err := rows.Scan(&exp.ID, &exp.Description, &exp.Amount, &exp.Category, &exp.Date, &exp.UserID); err != nil {
			return nil, err
		}
		expenses = append(expenses, exp)
	}
	return expenses, nil
}

func AddExpense(expense Expense) error {
	if expense.Date == "" {
		currentTime := time.Now()
		expense.Date = currentTime.Format("2006-01-02")
	}
	query := `INSERT INTO expenses (description, amount, category, date, user_id) VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(query, expense.Description, expense.Amount, expense.Category, expense.Date, expense.UserID)
	return err
}

func DeleteExpense(expenseID int, userID string) error {
	query := `DELETE FROM expenses WHERE id = ? AND user_id = ?`
	_, err := db.Exec(query, expenseID, userID)
	return err
}

func UpdateExpense(expense Expense) error {
	if expense.Date == "" {
		currentTime := time.Now()
		expense.Date = currentTime.Format("2006-01-02")
	}
	query := `UPDATE expenses SET description = ?, amount = ?, category = ?, date = ? WHERE id = ? AND user_id = ?`
	_, err := db.Exec(query, expense.Description, expense.Amount, expense.Category, expense.Date, expense.ID, expense.UserID)
	return err
}

func GroupExpensesByCategory(userID string) (map[string]float64, error) {
	query := `SELECT category, SUM(amount) FROM expenses WHERE user_id = ? GROUP BY category`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categoryTotals := make(map[string]float64)
	for rows.Next() {
		var category string
		var total float64
		if err := rows.Scan(&category, &total); err != nil {
			return nil, err
		}
		categoryTotals[category] = total
	}
	return categoryTotals, nil
}

func GroupExpensesByMonth(userID string) (map[string]float64, error) {
	query := `SELECT strftime('%Y-%m', date) AS month, SUM(amount) FROM expenses WHERE user_id = ? GROUP BY month ORDER BY month`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monthTotals := make(map[string]float64)
	for rows.Next() {
		var month string
		var total float64
		if err := rows.Scan(&month, &total); err != nil {
			return nil, err
		}
		monthTotals[month] = total
	}
	return monthTotals, nil
}

func GroupExpensesBiweekly(userID string) (map[string]float64, error) {
	query := `
	SELECT
		CASE
			WHEN CAST(strftime('%d', date) AS INTEGER) <= 15 THEN strftime('%Y-%m-01', date)
			ELSE strftime('%Y-%m-16', date)
		END AS biweek,
		SUM(amount)
	FROM expenses
	WHERE user_id = ?
	GROUP BY biweek
	ORDER BY biweek
	`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	biweekTotals := make(map[string]float64)
	for rows.Next() {
		var biweek string
		var total float64
		if err := rows.Scan(&biweek, &total); err != nil {
			return nil, err
		}
		biweekTotals[biweek] = total
	}
	return biweekTotals, nil
}

type BankTransaction struct {
	ID               int     `json:"id"`
	NtryRef          string  `json:"ntry_ref"`
	AcctSvcrRef      string  `json:"acct_svcr_ref"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	Direction        string  `json:"direction"`
	Status           string  `json:"status"`
	BookingDate      string  `json:"booking_date"`
	ValueDate        string  `json:"value_date"`
	Description      string  `json:"description"`
	Category         string  `json:"category"`
	IsSavingsRoundup bool    `json:"is_savings_roundup"`
	UserID           string  `json:"user_id"`
}

func CategorizeBankTransaction(tx BankTransaction) string {
	if tx.IsSavingsRoundup {
		return "Savings"
	}
	if tx.Direction == "CRDT" {
		return "Income"
	}

	desc := strings.ToLower(tx.Description)

	if looksLikeInvestment(desc) {
		return "Investments"
	}

	if looksLikeSubscription(desc) {
		return "Subscriptions"
	}

	categoryKeywords := map[string][]string{
		"Groceries":     {"tesco", "lidl", "billa", "kaufland", "albert", "coop", "kraj", "merkury", "fresh", "jednota", "dm drogerie", "ter"},
		"Dining":        {"chilantro", "unas food", "restaurant", "bistro", "pizza", "burger", "sushi", "kebab", "food truck", "foodtruck", "cafe", "coffee", "starbucks", "mc donald", "mcdonald", "kfc", "bageta", "obcerstvenie"},
		"Transport":     {"bolt.eu", "uber", "taxi", "fuel", "benzin", "nafta", "orlen", "shell", "esso", "omv", "slovnaft", "train", "bus", "mhd", "parking", "dialnica", "highway"},
		"Entertainment": {"cinemacity", "cinema", "bowlicheck", "bowling", "theatre", "concert", "festival", "sport", "fitnes", "gym", "netflix", "hbo", "spotify", "steam", "playstation", "ticket"},
		"Shopping":      {"zara", "h&m", "primark", "mall", "ikea", "decathlon", "alza", "nay", "notino", "aboutyou", "zalando", "amazon"},
		"Utilities":     {"telekom", "orange", "o2", "electric", "gas", "water", "internet", "spp", "zsd", "zse", "vse", "teplo"},
		"Healthcare":    {"pharmacy", "lekaren", "doctor", "hospital", "nemocnica", "poliklinika", "zubar", "dentist", "benu", "dr max"},
		"Insurance":     {"poistovna", "insurance", "poistenie", "allianz", "kooperativa", "union", "general"},
		"Housing":       {"najom", "rent", "hypotek", "mortgage", "fond oprav", "sprava"},
	}

	for category, keywords := range categoryKeywords {
		for _, kw := range keywords {
			if strings.Contains(desc, kw) {
				return category
			}
		}
	}

	if strings.Contains(desc, "platba kartou") {
		return categorizeCardPayment(tx)
	}

	return "Other"
}

func looksLikeInvestment(desc string) bool {
	investmentPatterns := []string{
		"ishares", "vanguard", "spdr", "xtrackers", "lyxor", "amundi",
		"invesco", "wisdomtree", "etf", "acc etf", "dist etf",
		"world acc", "msci world", "s&p 500", "nasdaq", "emerging markets",
		"core msci", "msci acwi", "ftse all-world",
	}
	for _, p := range investmentPatterns {
		if strings.Contains(desc, p) {
			return true
		}
	}
	descUpper := strings.ToUpper(desc)
	for i := 0; i < len(descUpper)-11; i++ {
		sub := descUpper[i : i+12]
		if len(sub) >= 12 {
			c0 := sub[0]
			c1 := sub[1]
			if (c0 >= 'A' && c0 <= 'Z') && (c1 >= 'A' && c1 <= 'Z') {
				rest := sub[2:]
				allAlnum := true
				for _, c := range []byte(rest) {
					if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
						allAlnum = false
						break
					}
				}
				if allAlnum {
					return true
				}
			}
		}
	}
	return false
}

func looksLikeSubscription(desc string) bool {
	subKeywords := []string{
		"subscription", "recurring", "monthly", "annual",
		"netflix", "spotify", "hbo", "disney", "prime video",
		"google one", "icloud", "dropbox", "office 365",
		"domain", "hosting", "vps", "server",
	}
	for _, kw := range subKeywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}
	return false
}

func categorizeCardPayment(tx BankTransaction) string {
	amount := tx.Amount
	if amount < 0 {
		amount = -amount
	}
	switch {
	case amount < 3:
		return "Snacks"
	case amount < 8:
		return "Dining"
	case amount < 25:
		return "Shopping"
	case amount < 100:
		return "Services"
	default:
		return "Shopping"
	}
}

func ImportBankTransaction(tx BankTransaction) (bool, error) {
	query := `
	INSERT OR IGNORE INTO bank_transactions
		(ntry_ref, acct_svcr_ref, amount, currency, direction, status, booking_date, value_date,
		 description, category, is_savings_roundup, user_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	savingsInt := 0
	if tx.IsSavingsRoundup {
		savingsInt = 1
	}

	result, err := db.Exec(query,
		tx.NtryRef, tx.AcctSvcrRef, tx.Amount, tx.Currency, tx.Direction, tx.Status,
		tx.BookingDate, tx.ValueDate, tx.Description, tx.Category, savingsInt, tx.UserID,
	)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func GetBankTransactions(userID string) ([]BankTransaction, error) {
	query := `
	SELECT id, ntry_ref, acct_svcr_ref, amount, currency, direction, status,
	       booking_date, value_date, description, category, is_savings_roundup, user_id
	FROM bank_transactions
	WHERE user_id = ?
	ORDER BY booking_date DESC, id DESC`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []BankTransaction
	for rows.Next() {
		var tx BankTransaction
		var savingsInt int
		if err := rows.Scan(
			&tx.ID, &tx.NtryRef, &tx.AcctSvcrRef, &tx.Amount, &tx.Currency,
			&tx.Direction, &tx.Status, &tx.BookingDate, &tx.ValueDate,
			&tx.Description, &tx.Category, &savingsInt, &tx.UserID,
		); err != nil {
			return nil, err
		}
		tx.IsSavingsRoundup = savingsInt == 1
		txs = append(txs, tx)
	}
	return txs, nil
}

func GetSavingsTransactions(userID string) ([]BankTransaction, error) {
	query := `
	SELECT id, ntry_ref, acct_svcr_ref, amount, currency, direction, status,
	       booking_date, value_date, description, category, is_savings_roundup, user_id
	FROM bank_transactions
	WHERE user_id = ? AND is_savings_roundup = 1
	ORDER BY booking_date ASC, id ASC`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []BankTransaction
	for rows.Next() {
		var tx BankTransaction
		var savingsInt int
		if err := rows.Scan(
			&tx.ID, &tx.NtryRef, &tx.AcctSvcrRef, &tx.Amount, &tx.Currency,
			&tx.Direction, &tx.Status, &tx.BookingDate, &tx.ValueDate,
			&tx.Description, &tx.Category, &savingsInt, &tx.UserID,
		); err != nil {
			return nil, err
		}
		tx.IsSavingsRoundup = savingsInt == 1
		txs = append(txs, tx)
	}
	return txs, nil
}

func GetBankTransactionStats(userID string) (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	var totalIn, totalOut, totalSavings float64
	err := db.QueryRow(
		`SELECT COALESCE(SUM(amount),0) FROM bank_transactions WHERE user_id = ? AND direction = 'CRDT' AND is_savings_roundup = 0`,
		userID,
	).Scan(&totalIn)
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(
		`SELECT COALESCE(SUM(amount),0) FROM bank_transactions WHERE user_id = ? AND direction = 'DBIT' AND is_savings_roundup = 0`,
		userID,
	).Scan(&totalOut)
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(
		`SELECT COALESCE(SUM(amount),0) FROM bank_transactions WHERE user_id = ? AND is_savings_roundup = 1`,
		userID,
	).Scan(&totalSavings)
	if err != nil {
		return nil, err
	}

	stats["total_in"] = totalIn
	stats["total_out"] = totalOut
	stats["net"] = totalIn - totalOut
	stats["total_savings"] = totalSavings

	rows, err := db.Query(
		`SELECT category, SUM(amount) FROM bank_transactions WHERE user_id = ? AND direction = 'DBIT' AND is_savings_roundup = 0 GROUP BY category`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byCategory := map[string]float64{}
	for rows.Next() {
		var cat string
		var total float64
		if err := rows.Scan(&cat, &total); err != nil {
			return nil, err
		}
		byCategory[cat] = total
	}
	stats["by_category"] = byCategory

	monthRows, err := db.Query(
		`SELECT strftime('%Y-%m', booking_date) AS month, SUM(amount)
		 FROM bank_transactions
		 WHERE user_id = ? AND direction = 'DBIT' AND is_savings_roundup = 0
		 GROUP BY month ORDER BY month`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer monthRows.Close()

	byMonth := map[string]float64{}
	for monthRows.Next() {
		var month string
		var total float64
		if err := monthRows.Scan(&month, &total); err != nil {
			return nil, err
		}
		byMonth[month] = total
	}
	stats["by_month"] = byMonth

	return stats, nil
}

func UpdateBankTransaction(tx BankTransaction) error {
	query := `UPDATE bank_transactions SET description = ?, category = ?, is_savings_roundup = ? WHERE id = ? AND user_id = ?`
	savingsInt := 0
	if tx.IsSavingsRoundup {
		savingsInt = 1
	}
	_, err := db.Exec(query, tx.Description, tx.Category, savingsInt, tx.ID, tx.UserID)
	return err
}

func DeleteBankTransaction(id int, userID string) error {
	query := `DELETE FROM bank_transactions WHERE id = ? AND user_id = ?`
	_, err := db.Exec(query, id, userID)
	return err
}

type ExpenseReport struct {
	ID          int    `json:"id"`
	Period      string `json:"period"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Summary     string `json:"summary"`
	CreatedAt   string `json:"created_at"`
	UserID      string `json:"user_id"`
}

func SaveExpenseReport(report ExpenseReport) error {
	query := `INSERT INTO expense_reports (period, period_start, period_end, summary, created_at, user_id) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, report.Period, report.PeriodStart, report.PeriodEnd, report.Summary, report.CreatedAt, report.UserID)
	return err
}

func GetExpenseReports(userID string) ([]ExpenseReport, error) {
	query := `SELECT id, period, period_start, period_end, summary, created_at, user_id FROM expense_reports WHERE user_id = ? ORDER BY created_at DESC`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ExpenseReport
	for rows.Next() {
		var r ExpenseReport
		if err := rows.Scan(&r.ID, &r.Period, &r.PeriodStart, &r.PeriodEnd, &r.Summary, &r.CreatedAt, &r.UserID); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}

func GetExpenseReportByID(reportID int, userID string) (*ExpenseReport, error) {
	query := `SELECT id, period, period_start, period_end, summary, created_at, user_id FROM expense_reports WHERE id = ? AND user_id = ?`
	var r ExpenseReport
	err := db.QueryRow(query, reportID, userID).Scan(&r.ID, &r.Period, &r.PeriodStart, &r.PeriodEnd, &r.Summary, &r.CreatedAt, &r.UserID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func ReportExistsForPeriod(userID, period, periodStart, periodEnd string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM expense_reports WHERE user_id = ? AND period = ? AND period_start = ? AND period_end = ?`
	err := db.QueryRow(query, userID, period, periodStart, periodEnd).Scan(&count)
	return count > 0, err
}

func GetAllUserIDs() ([]string, error) {
	query := `SELECT DISTINCT user_id FROM expenses UNION SELECT DISTINCT user_id FROM bank_transactions`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func UpdateBankTransactionCategory(txID int, userID string, category string) error {
	query := `UPDATE bank_transactions SET category = ? WHERE id = ? AND user_id = ?`
	result, err := db.Exec(query, category, txID, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("transaction not found or not owned by user")
	}
	return nil
}
