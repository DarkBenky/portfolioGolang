package bills

import (
	"database/sql"
	"fmt"
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
