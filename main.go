package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/AmpyFin/yfinance-go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

const (
	SALT       = "7a726befdfc6eff42209898e532ac1a71fc7f20290d5c1f7dd6298e5ccab4ab1"
	JWT_SECRET = "5ce80dd0f1070f65168ad0593f1669a000adfbc54c9977331de0434d3c9319c9"
)

var db *DB
var yf = yfinance.NewClient()

type User struct {
	userName string
	Email    string
	Password string
	Id       string
}

type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Holding struct {
	IdHolding     string
	Name          string
	Ticker        string
	ISIN          string
	Exchange      string
	Etf           bool
	Quantity      float64
	PurchasePrice float64
	TER           float64
	Policy        string
	userID        string
	currency      string
}

type Region struct {
	Name      string
	IdHolding string
}

type Sector struct {
	Name      string
	IdHolding string
}

type Asset struct {
	IdAsset   string
	Name      string
	Ticker    string
	ISIN      string
	Exchange  string
	Sector    string
	Region    string
	idHolding string
	currency  string
}

type News struct {
	IdNews      string
	Title       string
	Link        string
	PublishedAt string
	Summary     string
	Text        string
	Sentiment   float64
	idAsset     string
	idHolding   string
}

type Price struct {
	IdPrice   string
	Date      string
	Open      float64
	Close     float64
	High      float64
	Low       float64
	Volume    int64
	idAsset   string
	idHolding string
}

func fetchAndStoreETFData(holdingID, ticker, isin, name string) error {
	baseURL := "http://localhost:5123/api/etf_data"
	params := url.Values{}
	params.Add("ticker", ticker)
	params.Add("isin", isin)
	params.Add("etf_name", name)

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		log.Printf("Error fetching ETF data for %s: %v", ticker, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to fetch ETF data: %s", resp.Status)
		log.Printf("Failed to fetch ETF data for %s: %s", ticker, resp.Status)
		return err
	}

	var etfData struct {
		Holdings []struct {
			Name     string `json:"name"`
			Ticker   string `json:"ticker"`
			ISIN     string `json:"isin"`
			Exchange string `json:"exchange"`
			Sector   string `json:"sector"`
			Region   string `json:"region"`
		} `json:"holdings"`
		Sectors map[string]float64 `json:"sectors"`
		Regions map[string]float64 `json:"regions"`
	}

	err = json.NewDecoder(resp.Body).Decode(&etfData)
	if err != nil {
		log.Printf("Error decoding ETF data for %s: %v", ticker, err)
		return err
	}

	log.Printf("Fetched ETF data for holding ID %s: %d holdings, %d sectors, %d regions",
		holdingID, len(etfData.Holdings), len(etfData.Sectors), len(etfData.Regions))

	// Insert holdings as assets
	for _, holding := range etfData.Holdings {
		asset := Asset{
			IdAsset:   generateID(),
			Name:      holding.Name,
			Ticker:    holding.Ticker,
			ISIN:      holding.ISIN,
			Exchange:  holding.Exchange,
			Sector:    holding.Sector,
			Region:    holding.Region,
			idHolding: holdingID,
			currency:  "", // Could be extracted from holding if available
		}

		err = db.addAsset(asset)
		if err != nil {
			log.Printf("Error adding asset %s: %v", holding.Ticker, err)
		}
	}

	// Insert sectors
	for sectorName := range etfData.Sectors {
		sector := Sector{
			Name:      sectorName,
			IdHolding: holdingID,
		}
		err = db.addSector(sector)
		if err != nil {
			log.Printf("Error adding sector %s: %v", sectorName, err)
		}
	}

	// Insert regions
	for regionName := range etfData.Regions {
		region := Region{
			Name:      regionName,
			IdHolding: holdingID,
		}
		err = db.addRegion(region)
		if err != nil {
			log.Printf("Error adding region %s: %v", regionName, err)
		}
	}

	log.Printf("Successfully processed ETF data for holding ID %s", holdingID)
	return nil
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./portfolio.db")
	if err != nil {
		return nil, err
	}

	// Create Users table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			user_name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Holdings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS holdings (
			id_holding TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticker TEXT NOT NULL,
			isin TEXT,
			exchange TEXT,
			etf BOOLEAN DEFAULT 0,
			quantity REAL NOT NULL,
			purchase_price REAL NOT NULL,
			ter REAL,
			policy TEXT,
			user_id TEXT NOT NULL,
			currency TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Assets table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS assets (
			id_asset TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticker TEXT NOT NULL,
			isin TEXT,
			exchange TEXT,
			sector TEXT,
			region TEXT,
			id_holding TEXT NOT NULL,
			currency TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create News table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS news (
			id_news TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			link TEXT,
			published_at TEXT,
			summary TEXT,
			text TEXT UNIQUE,
			sentiment REAL,
			id_asset TEXT,
			id_holding TEXT,
			FOREIGN KEY (id_asset) REFERENCES assets(id_asset) ON DELETE CASCADE,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Crete Regions table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS regions (
			name TEXT,
			id_holding TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Sectors table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sectors (
			name TEXT,
			id_holding TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Prices table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS prices (
			id_price TEXT PRIMARY KEY,
			date TEXT NOT NULL,
			open REAL NOT NULL,
			close REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			volume INTEGER NOT NULL,
			id_asset TEXT,
			id_holding TEXT,
			FOREIGN KEY (id_asset) REFERENCES assets(id_asset) ON DELETE CASCADE,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	return db, nil
}

type DB struct {
	*sql.DB
}

func (database *DB) addUser(user User) error {
	_, err := database.Exec(`
		INSERT INTO users (id, user_name, email, password)
		VALUES (?, ?, ?, ?)
	`, user.Id, user.userName, user.Email, user.Password)
	return err
}

func (database *DB) getUserByEmail(email string) (*User, error) {
	var user User
	err := database.QueryRow(`
		SELECT id, user_name, email, password FROM users WHERE email = ?
	`, email).Scan(&user.Id, &user.userName, &user.Email, &user.Password)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (database *DB) verifyUser(email string, password string) (bool, error) {
	var hashedPassword string
	err := database.QueryRow(`
		SELECT password FROM users WHERE email = ?
	`, email).Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password+SALT))
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (database *DB) addHolding(holding Holding) error {
	// Check if holding already exists for this user and ticker
	var existingHolding Holding
	err := database.QueryRow(`
		SELECT id_holding, quantity, purchase_price 
		FROM holdings 
		WHERE user_id = ? AND ticker = ? AND exchange = ?
	`, holding.userID, holding.Ticker, holding.Exchange).Scan(
		&existingHolding.IdHolding,
		&existingHolding.Quantity,
		&existingHolding.PurchasePrice,
	)

	if err == sql.ErrNoRows {
		// No existing holding, insert new one
		_, err := database.Exec(`
			INSERT INTO holdings (id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, holding.IdHolding, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Etf, holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.userID, holding.currency)
		return err
	} else if err != nil {
		return err
	}

	// Holding exists, calculate new average purchase price and total quantity
	totalCost := (existingHolding.Quantity * existingHolding.PurchasePrice) + (holding.Quantity * holding.PurchasePrice)
	newQuantity := existingHolding.Quantity + holding.Quantity
	newAvgPrice := totalCost / newQuantity

	// Update the existing holding
	_, err = database.Exec(`
		UPDATE holdings 
		SET quantity = ?, purchase_price = ?, name = ?, isin = ?, ter = ?, policy = ?, currency = ?
		WHERE id_holding = ?
	`, newQuantity, newAvgPrice, holding.Name, holding.ISIN, holding.TER, holding.Policy, holding.currency, existingHolding.IdHolding)

	return err
}

func (database *DB) removeHolding(holdingID string, userID string) error {
	result, err := database.Exec(`
		DELETE FROM holdings 
		WHERE id_holding = ? AND user_id = ?
	`, holdingID, userID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (database *DB) modifyHolding(holding Holding) error {
	result, err := database.Exec(`
		UPDATE holdings 
		SET name = ?, ticker = ?, isin = ?, exchange = ?, etf = ?, quantity = ?, purchase_price = ?, ter = ?, policy = ?, currency = ?
		WHERE id_holding = ? AND user_id = ?
	`, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Etf, holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.currency, holding.IdHolding, holding.userID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (database *DB) getHoldingsByUser(userID string) ([]Holding, error) {
	rows, err := database.Query(`
		SELECT id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency
		FROM holdings
		WHERE user_id = ?
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holdings []Holding
	for rows.Next() {
		var h Holding
		err := rows.Scan(&h.IdHolding, &h.Name, &h.Ticker, &h.ISIN, &h.Exchange, &h.Etf, &h.Quantity, &h.PurchasePrice, &h.TER, &h.Policy, &h.userID, &h.currency)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

func (database *DB) addAsset(asset Asset) error {
	_, err := database.Exec(`
		INSERT INTO assets (id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, asset.IdAsset, asset.Name, asset.Ticker, asset.ISIN, asset.Exchange, asset.Sector, asset.Region, asset.idHolding, asset.currency)
	return err
}

func (database *DB) addSector(sector Sector) error {
	_, err := database.Exec(`
		INSERT INTO sectors (name, id_holding)
		VALUES (?, ?)
	`, sector.Name, sector.IdHolding)
	return err
}

func (database *DB) addRegion(region Region) error {
	_, err := database.Exec(`
		INSERT INTO regions (name, id_holding)
		VALUES (?, ?)
	`, region.Name, region.IdHolding)
	return err
}

func generateID() string {
	return uuid.New().String()
}

func generateJWT(userID, email string) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWT_SECRET))
}

func addUser(c echo.Context) error {
	userName := c.FormValue("userName")
	email := c.FormValue("email")
	password := c.FormValue("password")

	password = password + SALT
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error hashing password")
	}

	user := User{
		Id:       generateID(),
		userName: userName,
		Email:    email,
		Password: string(hashed),
	}

	err = db.addUser(user)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error adding user to database")
	}

	return c.String(http.StatusOK, "User added successfully")
}

func login(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	valid, err := db.verifyUser(email, password)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error verifying user")
	}
	if !valid {
		return c.String(http.StatusUnauthorized, "Invalid email or password")
	}

	// Get user details
	user, err := db.getUserByEmail(email)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving user")
	}

	// Generate JWT token
	token, err := generateJWT(user.Id, user.Email)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error generating token")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"token":   token,
		"message": "Login successful",
	})
}

func getProfile(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)

	return c.JSON(http.StatusOK, map[string]string{
		"user_id": claims.UserID,
		"email":   claims.Email,
	})
}

// holding actions
func AddHolding(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	Name := c.FormValue("Name")
	Ticker := c.FormValue("Ticker")
	ISIN := c.FormValue("ISIN")
	Exchange := c.FormValue("Exchange")
	ETF := c.FormValue("ETF")
	Quantity := c.FormValue("Quantity")
	PurchasePrice := c.FormValue("PurchasePrice")
	TER := c.FormValue("TER")
	Policy := c.FormValue("Policy")
	Currency := c.FormValue("Currency")

	quantity, _ := strconv.ParseFloat(Quantity, 64)
	purchasePrice, _ := strconv.ParseFloat(PurchasePrice, 64)
	ter, _ := strconv.ParseFloat(TER, 64)

	holding := Holding{
		IdHolding:     generateID(),
		Name:          Name,
		Ticker:        Ticker,
		ISIN:          ISIN,
		Exchange:      Exchange,
		Etf:           ETF == "true",
		Quantity:      quantity,
		PurchasePrice: purchasePrice,
		TER:           ter,
		Policy:        Policy,
		userID:        userID,
		currency:      Currency,
	}

	if holding.Etf {
		// Trigger background processing in a goroutine
		go func(holdingID, ticker, isin, name string) {
			log.Printf("Starting background ETF data fetch for %s...", ticker)
			err := fetchAndStoreETFData(holdingID, ticker, isin, name)
			if err != nil {
				log.Printf("Error processing ETF data for %s: %v", ticker, err)
			} else {
				log.Printf("Successfully completed background ETF data fetch for %s", ticker)
			}
		}(holding.IdHolding, holding.Ticker, holding.ISIN, holding.Name)
	}

	err := db.addHolding(holding)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error adding holding to database")
	}

	return c.String(http.StatusOK, "Holding added successfully")
}

func RemoveHolding(c echo.Context) error {
	holdingID := c.FormValue("HoldingID")

	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}

	err := db.removeHolding(holdingID, userID)
	if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "Holding not found")
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error removing holding from database")
	}

	return c.String(http.StatusOK, "Holding removed successfully")
}

func ModifyHolding(c echo.Context) error {
	holdingID := c.FormValue("HoldingID")
	Name := c.FormValue("Name")
	Ticker := c.FormValue("Ticker")
	ISIN := c.FormValue("ISIN")
	Exchange := c.FormValue("Exchange")
	ETF := c.FormValue("ETF")
	Quantity := c.FormValue("Quantity")
	PurchasePrice := c.FormValue("PurchasePrice")
	TER := c.FormValue("TER")
	Policy := c.FormValue("Policy")
	Currency := c.FormValue("Currency")

	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}

	quantity, _ := strconv.ParseFloat(Quantity, 64)
	purchasePrice, _ := strconv.ParseFloat(PurchasePrice, 64)
	ter, _ := strconv.ParseFloat(TER, 64)

	holding := Holding{
		IdHolding:     holdingID,
		Name:          Name,
		Ticker:        Ticker,
		ISIN:          ISIN,
		Exchange:      Exchange,
		Etf:           ETF == "true",
		Quantity:      quantity,
		PurchasePrice: purchasePrice,
		TER:           ter,
		Policy:        Policy,
		userID:        userID,
		currency:      Currency,
	}

	err := db.modifyHolding(holding)
	if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "Holding not found")
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error modifying holding in database")
	}

	return c.String(http.StatusOK, "Holding modified successfully")
}

func GetHoldings(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	return c.JSON(http.StatusOK, holdings)
}

func main() {
	sqlDB, err := initDB()
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer sqlDB.Close()

	db = &DB{sqlDB}

	log.Println("Database initialized successfully")

	e := echo.New()

	e.POST("/register", addUser)
	e.POST("/login", login)

	protected := e.Group("/api")
	protected.Use(echojwt.WithConfig(echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTClaims)
		},
		SigningKey: []byte(JWT_SECRET),
	}))

	protected.GET("/profile", getProfile)

	// Holdings endpoints
	protected.POST("/holdings", AddHolding)
	protected.GET("/holdings", GetHoldings)
	protected.PUT("/holdings", ModifyHolding)
	protected.DELETE("/holdings", RemoveHolding)

	e.Logger.Fatal(e.Start(":8080"))
}
