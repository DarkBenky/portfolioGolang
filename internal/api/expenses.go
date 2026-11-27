package api

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/bills"
)

func (s *Server) getExpenses(c echo.Context) error {
	expenses, err := bills.GetExpensesByUserID(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get expenses"})
	}
	if expenses == nil {
		expenses = []bills.Expense{}
	}
	return c.JSON(http.StatusOK, expenses)
}

func (s *Server) addExpense(c echo.Context) error {
	var expense bills.Expense
	if err := c.Bind(&expense); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	expense.UserID = s.userID(c)
	if expense.Description == "" || expense.Amount <= 0 || expense.Category == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid required fields"})
	}
	if err := bills.AddExpense(expense); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to add expense"})
	}
	return c.JSON(http.StatusCreated, map[string]string{"message": "Expense added successfully"})
}

func (s *Server) updateExpense(c echo.Context) error {
	var expense bills.Expense
	if err := c.Bind(&expense); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	expense.UserID = s.userID(c)
	if expense.ID == 0 || expense.Description == "" || expense.Amount <= 0 || expense.Category == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid required fields"})
	}
	updated, err := bills.UpdateExpense(expense)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update expense"})
	}
	return c.JSON(http.StatusOK, map[string]any{"message": "Expense updated successfully", "updated": updated})
}

func (s *Server) deleteExpense(c echo.Context) error {
	id, err := strconv.Atoi(c.QueryParam("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid expense ID"})
	}
	if err := bills.DeleteExpense(id, s.userID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete expense"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Expense deleted successfully"})
}

func (s *Server) getExpensesByCategory(c echo.Context) error {
	stats, err := bills.GroupExpensesByCategory(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get category stats"})
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) getExpensesByMonth(c echo.Context) error {
	stats, err := bills.GroupExpensesByMonth(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get monthly stats"})
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) getExpensesBiweekly(c echo.Context) error {
	stats, err := bills.GroupExpensesBiweekly(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get biweekly stats"})
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) getExpenseCategories(c echo.Context) error {
	return c.JSON(http.StatusOK, bills.CategoryNames())
}

func (s *Server) getMerchantRules(c echo.Context) error {
	rules, err := bills.ListMerchantRules(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error loading rules")
	}
	return c.JSON(http.StatusOK, rules)
}

func (s *Server) deleteMerchantRule(c echo.Context) error {
	key := strings.TrimSpace(c.QueryParam("key"))
	if key == "" {
		return c.String(http.StatusBadRequest, "key is required")
	}
	if err := bills.DeleteMerchantRule(s.userID(c), key); err != nil {
		return c.String(http.StatusInternalServerError, "Error deleting rule")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Rule deleted"})
}

func (s *Server) getMerchantSuggestions(c echo.Context) error {
	suggestions, err := bills.SuggestMerchantMerges(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load suggestions"})
	}
	return c.JSON(http.StatusOK, suggestions)
}

func (s *Server) mergeMerchants(c echo.Context) error {
	var body struct {
		AliasKey    string `json:"alias_key"`
		DisplayName string `json:"display_name"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	aliasKey := bills.MerchantSlug(body.AliasKey)
	displayName := strings.TrimSpace(body.DisplayName)
	merchantKey := bills.MerchantSlug(displayName)
	if aliasKey == "" || merchantKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "alias_key and display_name are required"})
	}
	if err := bills.SaveMerchantAlias(aliasKey, merchantKey, displayName, s.userID(c), "user"); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save merge"})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Merchant merged", "merchant": displayName, "merchant_key": merchantKey,
	})
}

func (s *Server) deleteMerchantMerge(c echo.Context) error {
	key := bills.MerchantSlug(c.QueryParam("key"))
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "key is required"})
	}
	if err := bills.DeleteMerchantAlias(key, s.userID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to remove merge"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Merge removed"})
}

func (s *Server) getExpenseReports(c echo.Context) error {
	reports, err := bills.GetExpenseReports(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get reports"})
	}
	if reports == nil {
		reports = []bills.ExpenseReport{}
	}
	return c.JSON(http.StatusOK, reports)
}

func (s *Server) getExpenseReport(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid report ID"})
	}
	report, err := bills.GetExpenseReportByID(id, s.userID(c))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Report not found"})
	}
	return c.JSON(http.StatusOK, report)
}

func (s *Server) generateExpenseReport(c echo.Context) error {
	period := c.QueryParam("period")
	if period != "week" && period != "month" && period != "quarter" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Period must be week, month, or quarter"})
	}
	userID := s.userID(c)
	if err := s.Jobs.AutoExpenseReport(userID, period); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	reports, err := bills.GetExpenseReports(userID)
	if err != nil || len(reports) == 0 {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Report was not generated"})
	}
	report := reports[0]
	return c.JSON(http.StatusOK, map[string]any{
		"period": report.Period, "period_start": report.PeriodStart, "period_end": report.PeriodEnd,
		"summary": report.Summary, "created_at": time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) getBankTransactions(c echo.Context) error {
	transactions, err := bills.GetBankTransactions(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get transactions"})
	}
	if transactions == nil {
		transactions = []bills.BankTransaction{}
	}
	return c.JSON(http.StatusOK, transactions)
}

func (s *Server) getSavingsTransactions(c echo.Context) error {
	transactions, err := bills.GetSavingsTransactions(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get savings"})
	}
	if transactions == nil {
		transactions = []bills.BankTransaction{}
	}
	return c.JSON(http.StatusOK, transactions)
}

func (s *Server) getBankStats(c echo.Context) error {
	stats, err := bills.GetBankTransactionStats(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get bank stats"})
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) updateBankTransaction(c echo.Context) error {
	var transaction bills.BankTransaction
	if err := c.Bind(&transaction); err != nil || transaction.ID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	transaction.UserID = s.userID(c)
	updated, err := bills.UpdateBankTransaction(transaction)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update transaction"})
	}
	return c.JSON(http.StatusOK, map[string]any{"message": "Transaction updated", "updated": updated})
}

func (s *Server) deleteBankTransaction(c echo.Context) error {
	id, err := strconv.Atoi(c.QueryParam("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid transaction ID"})
	}
	if err := bills.DeleteBankTransaction(id, s.userID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete transaction"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Transaction deleted"})
}

type camtDocument struct {
	Statements struct {
		Statement struct {
			Entries []camtEntry `xml:"Ntry"`
		} `xml:"Stmt"`
	} `xml:"BkToCstmrStmt"`
}

type camtEntry struct {
	Reference string     `xml:"NtryRef"`
	Amount    camtAmount `xml:"Amt"`
	Direction string     `xml:"CdtDbtInd"`
	Status    string     `xml:"Sts"`
	BookDate  camtDate   `xml:"BookgDt"`
	ValueDate camtDate   `xml:"ValDt"`
	Details   struct {
		Transaction struct {
			Reference string `xml:"Refs>AcctSvcrRef"`
			Parties   struct {
				DebtorAccount struct {
					IBAN string `xml:"Id>IBAN"`
					Name string `xml:"Nm"`
				} `xml:"DbtrAcct"`
				CreditorAccount struct {
					IBAN string `xml:"Id>IBAN"`
					Name string `xml:"Nm"`
				} `xml:"CdtrAcct"`
				Debtor struct {
					Name string `xml:"Nm"`
				} `xml:"Dbtr"`
				Creditor struct {
					Name string `xml:"Nm"`
				} `xml:"Cdtr"`
			} `xml:"RltdPties"`
			Remittance struct {
				Text string `xml:"Ustrd"`
			} `xml:"RmtInf"`
		} `xml:"TxDtls"`
	} `xml:"NtryDtls"`
}

type camtAmount struct {
	Value    float64 `xml:",chardata"`
	Currency string  `xml:"Ccy,attr"`
}

type camtDate struct {
	Date string `xml:"Dt"`
}

// importBankXML parses a camt.053 statement, categorises each entry with the local
// rules and only asks the model about the leftovers.
func (s *Server) importBankXML(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No file provided"})
	}
	if file.Size > 10*1024*1024 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "File too large (max 10MB)"})
	}
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open file"})
	}
	defer src.Close()
	raw, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read file"})
	}
	raw = bytes.ReplaceAll(raw, []byte(`xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02"`), nil)
	raw = bytes.ReplaceAll(raw, []byte(`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`), nil)

	var document camtDocument
	if err := xml.Unmarshal(raw, &document); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid XML format"})
	}

	userID := s.userID(c)
	savingsIBANs, _ := bills.GetKnownSavingsIBANs(userID)
	if savingsIBANs == nil {
		savingsIBANs = map[string]bool{}
	}
	imported, skipped := 0, 0
	var uncategorized []bills.BankTransaction

	for _, entry := range document.Statements.Statement.Entries {
		description := entry.Details.Transaction.Remittance.Text
		parties := entry.Details.Transaction.Parties
		counterpartyName := parties.CreditorAccount.Name
		if counterpartyName == "" {
			counterpartyName = parties.Creditor.Name
		}
		counterpartyIBAN := parties.CreditorAccount.IBAN
		if entry.Direction == "CRDT" {
			counterpartyName = parties.DebtorAccount.Name
			if counterpartyName == "" {
				counterpartyName = parties.Debtor.Name
			}
			counterpartyIBAN = parties.DebtorAccount.IBAN
		}
		if description == "" && counterpartyName != "" {
			description = counterpartyName
		} else if description != "" && counterpartyName != "" {
			description += " | " + counterpartyName
		}
		if description == "" {
			description = "No description"
		}

		isRoundup := strings.Contains(strings.ToLower(description), "drobne bokom")
		if isRoundup && counterpartyIBAN != "" {
			savingsIBANs[counterpartyIBAN] = true
			bills.SaveSavingsIBAN(userID, counterpartyIBAN)
		}
		category := bills.CategorizeBankTransaction(bills.BankTransaction{
			Description: description, Direction: entry.Direction, IsSavingsRoundup: isRoundup, UserID: userID,
		})
		if !isRoundup && counterpartyIBAN != "" && savingsIBANs[counterpartyIBAN] {
			category = "Transfer"
		}

		transaction := bills.BankTransaction{
			NtryRef: entry.Reference, AcctSvcrRef: entry.Details.Transaction.Reference,
			Amount: entry.Amount.Value, Currency: entry.Amount.Currency, Direction: entry.Direction,
			Status: entry.Status, BookingDate: entry.BookDate.Date, ValueDate: entry.ValueDate.Date,
			Description: description, Category: category, IsSavingsRoundup: isRoundup, UserID: userID,
		}
		if transaction.Category == "Other" && transaction.Direction == "DBIT" && !transaction.IsSavingsRoundup {
			uncategorized = append(uncategorized, transaction)
		}
		added, err := bills.ImportBankTransaction(transaction)
		if err != nil {
			log.Printf("api: import transaction %s: %v", transaction.NtryRef, err)
			continue
		}
		if added {
			imported++
		} else {
			skipped++
		}
	}

	if len(uncategorized) > 0 {
		go s.classifyPending(uncategorized, userID)
	}
	return c.JSON(http.StatusOK, map[string]int{"imported": imported, "skipped": skipped, "total": imported + skipped})
}

// classifyPending asks the cheap model for the leftovers, one call per unique merchant.
func (s *Server) classifyPending(transactions []bills.BankTransaction, userID string) {
	if !s.LLM.Configured() {
		return
	}
	ctx := context.Background()
	categories := bills.CategoryNames()
	done := map[string]bool{}
	for _, transaction := range transactions {
		key := bills.NormalizeMerchantKey(transaction.Description)
		if done[key] {
			continue
		}
		done[key] = true
		category := s.LLM.ClassifyExpense(ctx, userID, transaction.Description, categories)
		if category == "" {
			continue
		}
		bills.SaveMerchantCategory(key, category, userID, "llm")
		time.Sleep(100 * time.Millisecond)
	}
}
