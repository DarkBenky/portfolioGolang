package inmem

import (
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type UserTable struct {
	Users   map[string]User
	counter int
	mutex   sync.RWMutex
}

type User struct {
	userName string
	Email    string
	Password string
	Id       string
}

type HoldingsTable struct {
	Holdings map[string]Holding
	counter  int
	mutex    sync.RWMutex
}

type Holding struct {
	IdHolding     string
	Name          string
	Ticker        string
	ISIN          string
	Exchange      string
	Policy        string
	UserID        string
	Currency      string
	Quantity      float64
	PurchasePrice float64
	TER           float64
	Etf           bool
}

type RegionsTable struct {
	Regions map[string]Region
	counter int
	mutex   sync.RWMutex
}

type Region struct {
	Name       string
	Percentage float64
	Ticker     string
}

type SectorsTable struct {
	Sectors map[string]Sector
	counter int
	mutex   sync.RWMutex
}

type Sector struct {
	Name       string
	Percentage float64
	Ticker     string
}

type DailySentimentsTable struct {
	DailySentiments map[string]DailySentiment
	counter         int
	mutex           sync.RWMutex
}

type DailySentiment struct {
	IdSentiment string  `json:"id_sentiment"`
	Ticker      string  `json:"ticker"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
}

type PortfolioDailySentimentsTable struct {
	PortfolioDailySentiments map[string]PortfolioDailySentiment
	counter                  int
	mutex                    sync.RWMutex
}

type PortfolioDailySentiment struct {
	IdSentiment string  `json:"id_sentiment"`
	UserID      string  `json:"user_id"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
}

type AssetsTable struct {
	Assets  map[string]Asset
	counter int
	mutex   sync.RWMutex
}

type Asset struct {
	IdAsset      string
	Name         string
	Ticker       string
	ISIN         string
	Exchange     string
	Sector       string
	Region       string
	TickerParent string
	Currency     string
}

type AssetsDetailsTable struct {
	AssetsDetails map[string]AssetDetails
	counter       int
	mutex         sync.RWMutex
}

type AssetDetails struct {
	Ticker        string `json:"ticker"`
	ISIN          string `json:"isin"`
	MarketCap     string `json:"market_cap"`
	MarketCapEur  string `json:"market_cap_eur"`
	Country       string `json:"country"`
	Sector        string `json:"sector"`
	Eps           string `json:"eps"`
	PbRatio       string `json:"pb_ratio"`
	PeRatio       string `json:"pe_ratio"`
	DividendYield string `json:"dividend_yield"`
	Revenue       string `json:"revenue"`
	NetIncome     string `json:"net_income"`
	ProfitMargin  string `json:"profit_margin"`
	Hash          string `json:"hash"`
	Date          string `json:"date"`
}

type NewsTable struct {
	News    map[string]News
	counter int
	mutex   sync.RWMutex
}

type News struct {
	IdNews      string `json:"id_news"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	PublishedAt string `json:"published_at"`
	Summary     string `json:"summary"`
	Text        string `json:"text"`
	Author      string `json:"author"`
	idAsset     string
	idHolding   string
	Ticker      string  `json:"ticker"`
	Sentiment   float64 `json:"sentiment"`
}

type PricesTable struct {
	Prices  map[string]Prices
	counter int
	mutex   sync.RWMutex
}

type Prices struct {
	IdPrice string
	Ticker  string
	Date    string
	Open    float64
	Close   float64
	High    float64
	Low     float64
	Volume  int64
}

type InMemDB struct {
	userTable                     UserTable
	holdingsTable                 HoldingsTable
	regionsTable                  RegionsTable
	sectorsTable                  SectorsTable
	dailySentimentsTable          DailySentimentsTable
	portfolioDailySentimentsTable PortfolioDailySentimentsTable
	assetsTable                   AssetsTable
	assetsDetailsTable            AssetsDetailsTable
	newsTable                     NewsTable
	pricesTable                   PricesTable
	snapshotMutex                 sync.RWMutex
	snapshotPeriod                time.Duration
	lastSnapShotTime              time.Time
	snapShotPath                  string
}

func createInMemDB(snapshotPeriod time.Duration, snapShotPath string) *InMemDB {
	return &InMemDB{
		userTable: UserTable{
			Users: make(map[string]User),
		},
		holdingsTable: HoldingsTable{
			Holdings: make(map[string]Holding),
		},
		regionsTable: RegionsTable{
			Regions: make(map[string]Region),
		},
		sectorsTable: SectorsTable{
			Sectors: make(map[string]Sector),
		},
		dailySentimentsTable: DailySentimentsTable{
			DailySentiments: make(map[string]DailySentiment),
		},
		portfolioDailySentimentsTable: PortfolioDailySentimentsTable{
			PortfolioDailySentiments: make(map[string]PortfolioDailySentiment),
		},
		assetsTable: AssetsTable{
			Assets: make(map[string]Asset),
		},
		assetsDetailsTable: AssetsDetailsTable{
			AssetsDetails: make(map[string]AssetDetails),
		},
		newsTable: NewsTable{
			News: make(map[string]News),
		},
		pricesTable: PricesTable{
			Prices: make(map[string]Prices),
		},
		snapshotPeriod: snapshotPeriod,
		snapShotPath:   snapShotPath,
	}
}

func NewInMemDB(snapshotPeriod time.Duration, snapShotPath string, new bool) *InMemDB {
	db := createInMemDB(snapshotPeriod, snapShotPath)
	if !new {
		err := db.LoadSnapshot()
		if err != nil {
			log.Printf("Failed to load snapshot: %v, starting with empty database", err)
		}
	}
	db.StartSnapshotRoutine()
	return db
}

func NewInMemDBFromSQL(snapshotPeriod time.Duration, snapShotPath string, sqlDBPath string) (*InMemDB, error) {
	db := createInMemDB(snapshotPeriod, snapShotPath)

	err := db.LoadFromSQL(sqlDBPath)
	if err != nil {
		return nil, err
	}

	db.StartSnapshotRoutine()
	return db, nil
}

func (db *InMemDB) StartSnapshotRoutine() {
	ticker := time.NewTicker(db.snapshotPeriod)
	go func() {
		for range ticker.C {
			err := db.SaveSnapshot()
			if err != nil {
				log.Printf("Failed to save snapshot: %v", err)
			}
		}
	}()
}

type Snapshot struct {
	Users                    map[string]User
	Holdings                 map[string]Holding
	Regions                  map[string]Region
	Sectors                  map[string]Sector
	DailySentiments          map[string]DailySentiment
	PortfolioDailySentiments map[string]PortfolioDailySentiment
	Assets                   map[string]Asset
	AssetsDetails            map[string]AssetDetails
	News                     map[string]News
	Prices                   map[string]Prices
	Timestamp                time.Time
}

const DEBUG = true

func (db *InMemDB) SaveSnapshot() error {
	db.snapshotMutex.Lock()
	defer db.snapshotMutex.Unlock()

	db.userTable.mutex.RLock()
	db.holdingsTable.mutex.RLock()
	db.regionsTable.mutex.RLock()
	db.sectorsTable.mutex.RLock()
	db.dailySentimentsTable.mutex.RLock()
	db.portfolioDailySentimentsTable.mutex.RLock()
	db.assetsTable.mutex.RLock()
	db.assetsDetailsTable.mutex.RLock()
	db.newsTable.mutex.RLock()
	db.pricesTable.mutex.RLock()

	snapshot := Snapshot{
		Users:                    make(map[string]User),
		Holdings:                 make(map[string]Holding),
		Regions:                  make(map[string]Region),
		Sectors:                  make(map[string]Sector),
		DailySentiments:          make(map[string]DailySentiment),
		PortfolioDailySentiments: make(map[string]PortfolioDailySentiment),
		Assets:                   make(map[string]Asset),
		AssetsDetails:            make(map[string]AssetDetails),
		News:                     make(map[string]News),
		Prices:                   make(map[string]Prices),
		Timestamp:                time.Now(),
	}

	for k, v := range db.userTable.Users {
		snapshot.Users[k] = v
	}
	for k, v := range db.holdingsTable.Holdings {
		snapshot.Holdings[k] = v
	}
	for k, v := range db.regionsTable.Regions {
		snapshot.Regions[k] = v
	}
	for k, v := range db.sectorsTable.Sectors {
		snapshot.Sectors[k] = v
	}
	for k, v := range db.dailySentimentsTable.DailySentiments {
		snapshot.DailySentiments[k] = v
	}
	for k, v := range db.portfolioDailySentimentsTable.PortfolioDailySentiments {
		snapshot.PortfolioDailySentiments[k] = v
	}
	for k, v := range db.assetsTable.Assets {
		snapshot.Assets[k] = v
	}
	for k, v := range db.assetsDetailsTable.AssetsDetails {
		snapshot.AssetsDetails[k] = v
	}
	for k, v := range db.newsTable.News {
		snapshot.News[k] = v
	}
	for k, v := range db.pricesTable.Prices {
		snapshot.Prices[k] = v
	}

	db.pricesTable.mutex.RUnlock()
	db.newsTable.mutex.RUnlock()
	db.assetsDetailsTable.mutex.RUnlock()
	db.assetsTable.mutex.RUnlock()
	db.portfolioDailySentimentsTable.mutex.RUnlock()
	db.dailySentimentsTable.mutex.RUnlock()
	db.sectorsTable.mutex.RUnlock()
	db.regionsTable.mutex.RUnlock()
	db.holdingsTable.mutex.RUnlock()
	db.userTable.mutex.RUnlock()

	file, err := os.Create(db.snapShotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(snapshot)
	if err != nil {
		return err
	}

	if DEBUG {
		jsonFile, err := os.Create(db.snapShotPath + ".json")
		if err != nil {
			return err
		}
		defer jsonFile.Close()

		jsonEncoder := json.NewEncoder(jsonFile)
		jsonEncoder.SetIndent("", "  ")
		err = jsonEncoder.Encode(snapshot)
		if err != nil {
			return err
		}
	}

	db.lastSnapShotTime = snapshot.Timestamp
	log.Printf("Snapshot saved successfully at %s", db.snapShotPath)
	return nil
}

func (db *InMemDB) LoadSnapshot() error {
	file, err := os.Open(db.snapShotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var snapshot Snapshot
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&snapshot)
	if err != nil {
		return err
	}

	db.snapshotMutex.Lock()
	defer db.snapshotMutex.Unlock()

	db.userTable.mutex.Lock()
	db.holdingsTable.mutex.Lock()
	db.regionsTable.mutex.Lock()
	db.sectorsTable.mutex.Lock()
	db.dailySentimentsTable.mutex.Lock()
	db.portfolioDailySentimentsTable.mutex.Lock()
	db.assetsTable.mutex.Lock()
	db.assetsDetailsTable.mutex.Lock()
	db.newsTable.mutex.Lock()
	db.pricesTable.mutex.Lock()

	db.userTable.Users = snapshot.Users
	db.userTable.counter = len(snapshot.Users)
	db.holdingsTable.Holdings = snapshot.Holdings
	db.holdingsTable.counter = len(snapshot.Holdings)
	db.regionsTable.Regions = snapshot.Regions
	db.regionsTable.counter = len(snapshot.Regions)
	db.sectorsTable.Sectors = snapshot.Sectors
	db.sectorsTable.counter = len(snapshot.Sectors)
	db.dailySentimentsTable.DailySentiments = snapshot.DailySentiments
	db.dailySentimentsTable.counter = len(snapshot.DailySentiments)
	db.portfolioDailySentimentsTable.PortfolioDailySentiments = snapshot.PortfolioDailySentiments
	db.portfolioDailySentimentsTable.counter = len(snapshot.PortfolioDailySentiments)
	db.assetsTable.Assets = snapshot.Assets
	db.assetsTable.counter = len(snapshot.Assets)
	db.assetsDetailsTable.AssetsDetails = snapshot.AssetsDetails
	db.assetsDetailsTable.counter = len(snapshot.AssetsDetails)
	db.newsTable.News = snapshot.News
	db.newsTable.counter = len(snapshot.News)
	db.pricesTable.Prices = snapshot.Prices
	db.pricesTable.counter = len(snapshot.Prices)
	db.lastSnapShotTime = snapshot.Timestamp

	db.pricesTable.mutex.Unlock()
	db.newsTable.mutex.Unlock()
	db.assetsDetailsTable.mutex.Unlock()
	db.assetsTable.mutex.Unlock()
	db.portfolioDailySentimentsTable.mutex.Unlock()
	db.dailySentimentsTable.mutex.Unlock()
	db.sectorsTable.mutex.Unlock()
	db.regionsTable.mutex.Unlock()
	db.holdingsTable.mutex.Unlock()
	db.userTable.mutex.Unlock()

	log.Printf("Snapshot loaded successfully from %s (timestamp: %s)", db.snapShotPath, snapshot.Timestamp)
	return nil
}

func (db *InMemDB) LoadFromSQL(sqlDBPath string) error {
	sqlDB, err := sql.Open("sqlite3", sqlDBPath)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	log.Printf("Loading data from SQL database: %s", sqlDBPath)

	if err := db.loadHoldings(sqlDB); err != nil {
		return err
	}
	if err := db.loadRegions(sqlDB); err != nil {
		return err
	}
	if err := db.loadSectors(sqlDB); err != nil {
		return err
	}
	if err := db.loadAssets(sqlDB); err != nil {
		return err
	}
	if err := db.loadPrices(sqlDB); err != nil {
		return err
	}
	if err := db.loadNews(sqlDB); err != nil {
		return err
	}
	if err := db.loadAssetDetails(sqlDB); err != nil {
		return err
	}

	log.Printf("Data loaded successfully from SQL database")
	return nil
}

func (db *InMemDB) loadHoldings(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT id_holding, name, ticker, isin, exchange, policy, currency, quantity, purchase_price, ter, etf
		FROM holdings
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var h Holding
		err := rows.Scan(&h.IdHolding, &h.Name, &h.Ticker, &h.ISIN, &h.Exchange, &h.Policy, &h.Currency, &h.Quantity, &h.PurchasePrice, &h.TER, &h.Etf)
		if err != nil {
			log.Printf("Error scanning holding: %v", err)
			continue
		}
		db.holdingsTable.Holdings[h.IdHolding] = h
		count++
	}
	db.holdingsTable.counter = count
	log.Printf("Loaded %d holdings", count)
	return nil
}

func (db *InMemDB) loadRegions(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT r.name, r.percentage, r.id_holding, h.ticker
		FROM regions r
		JOIN holdings h ON r.id_holding = h.id_holding
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name string
		var percentage float64
		var idHolding, ticker string
		err := rows.Scan(&name, &percentage, &idHolding, &ticker)
		if err != nil {
			log.Printf("Error scanning region: %v", err)
			continue
		}
		region := Region{
			Name:       name,
			Percentage: percentage,
			Ticker:     ticker,
		}
		key := ticker + "_" + name
		db.regionsTable.Regions[key] = region
		count++
	}
	db.regionsTable.counter = count
	log.Printf("Loaded %d regions", count)
	return nil
}

func (db *InMemDB) loadSectors(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT s.name, s.percentage, s.id_holding, h.ticker
		FROM sectors s
		JOIN holdings h ON s.id_holding = h.id_holding
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name string
		var percentage float64
		var idHolding, ticker string
		err := rows.Scan(&name, &percentage, &idHolding, &ticker)
		if err != nil {
			log.Printf("Error scanning sector: %v", err)
			continue
		}
		sector := Sector{
			Name:       name,
			Percentage: percentage,
			Ticker:     ticker,
		}
		key := ticker + "_" + name
		db.sectorsTable.Sectors[key] = sector
		count++
	}
	db.sectorsTable.counter = count
	log.Printf("Loaded %d sectors", count)
	return nil
}

func (db *InMemDB) loadAssets(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT a.id_asset, a.name, a.ticker, a.isin, a.exchange, a.sector, a.region, a.id_holding, a.currency, h.ticker
		FROM assets a
		JOIN holdings h ON a.id_holding = h.id_holding
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var a Asset
		var idHolding, tickerParent string
		err := rows.Scan(&a.IdAsset, &a.Name, &a.Ticker, &a.ISIN, &a.Exchange, &a.Sector, &a.Region, &idHolding, &a.Currency, &tickerParent)
		if err != nil {
			log.Printf("Error scanning asset: %v", err)
			continue
		}
		a.TickerParent = tickerParent
		db.assetsTable.Assets[a.IdAsset] = a
		count++
	}
	db.assetsTable.counter = count
	log.Printf("Loaded %d assets", count)
	return nil
}

func (db *InMemDB) loadPrices(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT ticker, date, open, close, high, low, volume
		FROM prices
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var p Prices
		err := rows.Scan(&p.Ticker, &p.Date, &p.Open, &p.Close, &p.High, &p.Low, &p.Volume)
		if err != nil {
			log.Printf("Error scanning price: %v", err)
			continue
		}
		p.IdPrice = p.Ticker + "_" + p.Date
		key := p.Ticker + "_" + p.Date
		db.pricesTable.Prices[key] = p
		count++
	}
	db.pricesTable.counter = count
	log.Printf("Loaded %d prices", count)
	return nil
}

func (db *InMemDB) loadNews(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT id_news, title, link, published_at, summary, text, author, ticker, sentiment
		FROM news
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var n News
		err := rows.Scan(&n.IdNews, &n.Title, &n.Link, &n.PublishedAt, &n.Summary, &n.Text, &n.Author, &n.Ticker, &n.Sentiment)
		if err != nil {
			log.Printf("Error scanning news: %v", err)
			continue
		}
		db.newsTable.News[n.IdNews] = n
		count++
	}
	db.newsTable.counter = count
	log.Printf("Loaded %d news articles", count)
	return nil
}

func (db *InMemDB) loadAssetDetails(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
		SELECT ticker, isin, market_cap, market_cap_eur, country, sector, eps, pb_ratio, pe_ratio, dividend_yield, revenue, net_income, profit_margin, hash, date
		FROM asset_details
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var ad AssetDetails
		err := rows.Scan(&ad.Ticker, &ad.ISIN, &ad.MarketCap, &ad.MarketCapEur, &ad.Country, &ad.Sector, &ad.Eps, &ad.PbRatio, &ad.PeRatio, &ad.DividendYield, &ad.Revenue, &ad.NetIncome, &ad.ProfitMargin, &ad.Hash, &ad.Date)
		if err != nil {
			log.Printf("Error scanning asset details: %v", err)
			continue
		}
		key := ad.Ticker + "_" + ad.Date
		db.assetsDetailsTable.AssetsDetails[key] = ad
		count++
	}
	db.assetsDetailsTable.counter = count
	log.Printf("Loaded %d asset details", count)
	return nil
}

func (db *InMemDB) AddUser(user User) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.Lock()
	defer db.userTable.mutex.Unlock()

	db.userTable.Users[user.Id] = user
	db.userTable.counter++
}

func (db *InMemDB) GetUser(id string) (User, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.RLock()
	defer db.userTable.mutex.RUnlock()

	user, exists := db.userTable.Users[id]
	return user, exists
}

func (db *InMemDB) GetUserByEmail(email string) (User, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.RLock()
	defer db.userTable.mutex.RUnlock()

	for _, user := range db.userTable.Users {
		if user.Email == email {
			return user, true
		}
	}
	return User{}, false
}

func (db *InMemDB) GetAllUsers() []User {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.RLock()
	defer db.userTable.mutex.RUnlock()

	users := make([]User, 0, db.userTable.counter)
	for _, user := range db.userTable.Users {
		users = append(users, user)
	}
	return users
}

func (db *InMemDB) DeleteUser(id string) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.Lock()
	defer db.userTable.mutex.Unlock()

	delete(db.userTable.Users, id)
	db.userTable.counter--
}

func (db *InMemDB) UpdateUser(user User) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.userTable.mutex.Lock()
	defer db.userTable.mutex.Unlock()

	db.userTable.Users[user.Id] = user
}

func (db *InMemDB) AddHolding(holding Holding) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.Lock()
	defer db.holdingsTable.mutex.Unlock()

	db.holdingsTable.Holdings[holding.IdHolding] = holding
	db.holdingsTable.counter++
}

func (db *InMemDB) GetHolding(id string) (Holding, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.RLock()
	defer db.holdingsTable.mutex.RUnlock()

	holding, exists := db.holdingsTable.Holdings[id]
	return holding, exists
}

func (db *InMemDB) GetHoldingsByUser(userID string) []Holding {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.RLock()
	defer db.holdingsTable.mutex.RUnlock()

	holdings := make([]Holding, 0, db.holdingsTable.counter)
	for _, holding := range db.holdingsTable.Holdings {
		if holding.UserID == userID {
			holdings = append(holdings, holding)
		}
	}
	return holdings
}

func (db *InMemDB) GetAllHoldings() []Holding {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.RLock()
	defer db.holdingsTable.mutex.RUnlock()

	holdings := make([]Holding, 0, db.holdingsTable.counter)
	for _, holding := range db.holdingsTable.Holdings {
		holdings = append(holdings, holding)
	}
	return holdings
}

func (db *InMemDB) UpdateHolding(holding Holding) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.Lock()
	defer db.holdingsTable.mutex.Unlock()

	db.holdingsTable.Holdings[holding.IdHolding] = holding
}

func (db *InMemDB) DeleteHolding(id string) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.holdingsTable.mutex.Lock()
	defer db.holdingsTable.mutex.Unlock()

	delete(db.holdingsTable.Holdings, id)
	db.holdingsTable.counter--
}

func (db *InMemDB) AddRegion(region Region) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.regionsTable.mutex.Lock()
	defer db.regionsTable.mutex.Unlock()

	key := region.Ticker + "_" + region.Name
	db.regionsTable.Regions[key] = region
	db.regionsTable.counter++
}

func (db *InMemDB) GetRegionsByHolding(holdingID string) []Region {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.regionsTable.mutex.RLock()
	defer db.regionsTable.mutex.RUnlock()

	regions := make([]Region, 0, db.regionsTable.counter)
	for _, region := range db.regionsTable.Regions {
		if region.Ticker == holdingID {
			regions = append(regions, region)
		}
	}
	return regions
}

func (db *InMemDB) GetRegionsByTicker(ticker string) []Region {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.regionsTable.mutex.RLock()
	defer db.regionsTable.mutex.RUnlock()

	regions := make([]Region, 0, db.regionsTable.counter)
	for _, region := range db.regionsTable.Regions {
		if region.Ticker == ticker {
			regions = append(regions, region)
		}
	}
	return regions
}

func (db *InMemDB) GetAllRegions() []Region {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.regionsTable.mutex.RLock()
	defer db.regionsTable.mutex.RUnlock()

	regions := make([]Region, 0, db.regionsTable.counter)
	for _, region := range db.regionsTable.Regions {
		regions = append(regions, region)
	}
	return regions
}

func (db *InMemDB) DeleteRegionsByHolding(holdingID string) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.regionsTable.mutex.Lock()
	defer db.regionsTable.mutex.Unlock()

	for key, region := range db.regionsTable.Regions {
		if region.Ticker == holdingID {
			delete(db.regionsTable.Regions, key)
			db.regionsTable.counter--
		}
	}
}

func (db *InMemDB) AddSector(sector Sector) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.sectorsTable.mutex.Lock()
	defer db.sectorsTable.mutex.Unlock()

	key := sector.Ticker + "_" + sector.Name
	db.sectorsTable.Sectors[key] = sector
	db.sectorsTable.counter++
}

func (db *InMemDB) GetSectorsByHolding(holdingID string) []Sector {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.sectorsTable.mutex.RLock()
	defer db.sectorsTable.mutex.RUnlock()

	sectors := make([]Sector, 0, db.sectorsTable.counter)
	for _, sector := range db.sectorsTable.Sectors {
		if sector.Ticker == holdingID {
			sectors = append(sectors, sector)
		}
	}
	return sectors
}

func (db *InMemDB) GetSectorsByTicker(ticker string) []Sector {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.sectorsTable.mutex.RLock()
	defer db.sectorsTable.mutex.RUnlock()

	sectors := make([]Sector, 0, db.sectorsTable.counter)
	for _, sector := range db.sectorsTable.Sectors {
		if sector.Ticker == ticker {
			sectors = append(sectors, sector)
		}
	}
	return sectors
}

func (db *InMemDB) GetAllSectors() []Sector {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.sectorsTable.mutex.RLock()
	defer db.sectorsTable.mutex.RUnlock()

	sectors := make([]Sector, 0, db.sectorsTable.counter)
	for _, sector := range db.sectorsTable.Sectors {
		sectors = append(sectors, sector)
	}
	return sectors
}

func (db *InMemDB) DeleteSectorsByHolding(holdingID string) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.sectorsTable.mutex.Lock()
	defer db.sectorsTable.mutex.Unlock()

	for key, sector := range db.sectorsTable.Sectors {
		if sector.Ticker == holdingID {
			delete(db.sectorsTable.Sectors, key)
			db.sectorsTable.counter--
		}
	}
}

func (db *InMemDB) AddAsset(asset Asset) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.Lock()
	defer db.assetsTable.mutex.Unlock()

	db.assetsTable.Assets[asset.IdAsset] = asset
	db.assetsTable.counter++
}

func (db *InMemDB) GetAsset(id string) (Asset, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.RLock()
	defer db.assetsTable.mutex.RUnlock()

	asset, exists := db.assetsTable.Assets[id]
	return asset, exists
}

func (db *InMemDB) GetAssetsByHolding(holdingID string) []Asset {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.RLock()
	defer db.assetsTable.mutex.RUnlock()

	assets := make([]Asset, 0)
	for _, asset := range db.assetsTable.Assets {
		if asset.TickerParent == holdingID {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (db *InMemDB) GetAssetsByTicker(ticker string) []Asset {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.RLock()
	defer db.assetsTable.mutex.RUnlock()

	assets := make([]Asset, 0)
	for _, asset := range db.assetsTable.Assets {
		if asset.TickerParent == ticker {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (db *InMemDB) GetAssetsByTickerOrIsin(ticker string) []Asset {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.RLock()
	defer db.assetsTable.mutex.RUnlock()

	assets := make([]Asset, 0, db.assetsTable.counter)
	for _, asset := range db.assetsTable.Assets {
		if asset.Ticker == ticker || asset.ISIN == ticker {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (db *InMemDB) GetAllAssets() []Asset {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.RLock()
	defer db.assetsTable.mutex.RUnlock()

	assets := make([]Asset, 0, db.assetsTable.counter)
	for _, asset := range db.assetsTable.Assets {
		assets = append(assets, asset)
	}
	return assets
}

func (db *InMemDB) DeleteAssetsByHolding(holdingID string) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsTable.mutex.Lock()
	defer db.assetsTable.mutex.Unlock()

	for key, asset := range db.assetsTable.Assets {
		if asset.TickerParent == holdingID {
			delete(db.assetsTable.Assets, key)
			db.assetsTable.counter--
		}
	}
}

func (db *InMemDB) AddAssetDetails(details AssetDetails) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsDetailsTable.mutex.Lock()
	defer db.assetsDetailsTable.mutex.Unlock()

	key := details.Ticker + "_" + details.Date
	db.assetsDetailsTable.AssetsDetails[key] = details
	db.assetsDetailsTable.counter++
}

func (db *InMemDB) GetLatestAssetDetails(ticker string) (AssetDetails, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsDetailsTable.mutex.RLock()
	defer db.assetsDetailsTable.mutex.RUnlock()

	var latest AssetDetails
	var found bool
	var latestDate string

	for _, details := range db.assetsDetailsTable.AssetsDetails {
		if details.Ticker == ticker && details.Date > latestDate {
			latest = details
			latestDate = details.Date
			found = true
		}
	}
	return latest, found
}

func (db *InMemDB) GetAllAssetDetails() []AssetDetails {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.assetsDetailsTable.mutex.RLock()
	defer db.assetsDetailsTable.mutex.RUnlock()

	details := make([]AssetDetails, 0, db.assetsDetailsTable.counter)
	for _, detail := range db.assetsDetailsTable.AssetsDetails {
		details = append(details, detail)
	}
	return details
}

func (db *InMemDB) AddNews(news News) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.newsTable.mutex.Lock()
	defer db.newsTable.mutex.Unlock()

	db.newsTable.News[news.IdNews] = news
	db.newsTable.counter++
}

func (db *InMemDB) GetNews(id string) (News, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.newsTable.mutex.RLock()
	defer db.newsTable.mutex.RUnlock()

	news, exists := db.newsTable.News[id]
	return news, exists
}

func (db *InMemDB) GetNewsByTicker(ticker string) []News {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.newsTable.mutex.RLock()
	defer db.newsTable.mutex.RUnlock()

	newsList := make([]News, 0, db.newsTable.counter)
	for _, news := range db.newsTable.News {
		if news.Ticker == ticker {
			newsList = append(newsList, news)
		}
	}
	return newsList
}

func (db *InMemDB) GetAllNews() []News {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.newsTable.mutex.RLock()
	defer db.newsTable.mutex.RUnlock()

	newsList := make([]News, 0, db.newsTable.counter)
	for _, news := range db.newsTable.News {
		newsList = append(newsList, news)
	}
	return newsList
}

func (db *InMemDB) AddPrice(price Prices) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.Lock()
	defer db.pricesTable.mutex.Unlock()

	key := price.Ticker + "_" + price.Date
	db.pricesTable.Prices[key] = price
	db.pricesTable.counter++
}

func (db *InMemDB) GetPrice(ticker, date string) (Prices, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.RLock()
	defer db.pricesTable.mutex.RUnlock()

	key := ticker + "_" + date
	price, exists := db.pricesTable.Prices[key]
	return price, exists
}

func (db *InMemDB) GetLatestPrice(ticker string) (Prices, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.RLock()
	defer db.pricesTable.mutex.RUnlock()

	var latest Prices
	var found bool
	var latestDate string

	for _, price := range db.pricesTable.Prices {
		if price.Ticker == ticker && price.Date > latestDate {
			latest = price
			latestDate = price.Date
			found = true
		}
	}
	return latest, found
}

func (db *InMemDB) GetPricesByTickerRange(ticker string, start time.Time, end time.Time) []Prices {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.RLock()
	defer db.pricesTable.mutex.RUnlock()

	prices := make([]Prices, 0, db.pricesTable.counter)
	for _, price := range db.pricesTable.Prices {
		if price.Ticker == ticker && price.Date >= start.Format("2006-01-02") && price.Date <= end.Format("2006-01-02") {
			prices = append(prices, price)
		}
	}
	return prices
}

func (db *InMemDB) GetPricesByTicker(ticker string) []Prices {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.RLock()
	defer db.pricesTable.mutex.RUnlock()

	prices := make([]Prices, 0, db.pricesTable.counter)
	for _, price := range db.pricesTable.Prices {
		if price.Ticker == ticker {
			prices = append(prices, price)
		}
	}
	return prices
}

func (db *InMemDB) GetAllPrices() []Prices {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.pricesTable.mutex.RLock()
	defer db.pricesTable.mutex.RUnlock()

	prices := make([]Prices, 0, db.pricesTable.counter)
	for _, price := range db.pricesTable.Prices {
		prices = append(prices, price)
	}
	return prices
}

func (db *InMemDB) AddDailySentiment(sentiment DailySentiment) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.dailySentimentsTable.mutex.Lock()
	defer db.dailySentimentsTable.mutex.Unlock()

	key := sentiment.Ticker + "_" + sentiment.Date
	db.dailySentimentsTable.DailySentiments[key] = sentiment
	db.dailySentimentsTable.counter++
}

func (db *InMemDB) GetDailySentiment(ticker, date string) (DailySentiment, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.dailySentimentsTable.mutex.RLock()
	defer db.dailySentimentsTable.mutex.RUnlock()

	key := ticker + "_" + date
	sentiment, exists := db.dailySentimentsTable.DailySentiments[key]
	return sentiment, exists
}

func (db *InMemDB) GetAllDailySentiments() []DailySentiment {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.dailySentimentsTable.mutex.RLock()
	defer db.dailySentimentsTable.mutex.RUnlock()

	sentiments := make([]DailySentiment, 0, db.dailySentimentsTable.counter)
	for _, sentiment := range db.dailySentimentsTable.DailySentiments {
		sentiments = append(sentiments, sentiment)
	}
	return sentiments
}

func (db *InMemDB) AddPortfolioDailySentiment(sentiment PortfolioDailySentiment) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.portfolioDailySentimentsTable.mutex.Lock()
	defer db.portfolioDailySentimentsTable.mutex.Unlock()

	key := sentiment.UserID + "_" + sentiment.Date
	db.portfolioDailySentimentsTable.PortfolioDailySentiments[key] = sentiment
	db.portfolioDailySentimentsTable.counter++
}

func (db *InMemDB) GetPortfolioDailySentiment(userID, date string) (PortfolioDailySentiment, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.portfolioDailySentimentsTable.mutex.RLock()
	defer db.portfolioDailySentimentsTable.mutex.RUnlock()

	key := userID + "_" + date
	sentiment, exists := db.portfolioDailySentimentsTable.PortfolioDailySentiments[key]
	return sentiment, exists
}

func (db *InMemDB) GetAllPortfolioDailySentiments() []PortfolioDailySentiment {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	db.portfolioDailySentimentsTable.mutex.RLock()
	defer db.portfolioDailySentimentsTable.mutex.RUnlock()

	sentiments := make([]PortfolioDailySentiment, 0, db.portfolioDailySentimentsTable.counter)
	for _, sentiment := range db.portfolioDailySentimentsTable.PortfolioDailySentiments {
		sentiments = append(sentiments, sentiment)
	}
	return sentiments
}

type PortfolioAllocation struct {
	TotalValue float64
	BySector   map[string]float64
	ByRegion   map[string]float64
	ByCompany  map[string]float64
}

func (db *InMemDB) GetPortfolioAllocation(userID string) PortfolioAllocation {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userID)

	allocation := PortfolioAllocation{
		BySector:  make(map[string]float64),
		ByRegion:  make(map[string]float64),
		ByCompany: make(map[string]float64),
	}

	if len(holdings) == 0 {
		return allocation
	}

	holdingValues := make(map[string]float64)

	for _, holding := range holdings {
		currentPrice, exists := db.GetLatestPrice(holding.Ticker)
		if !exists {
			currentPrice.Close = holding.PurchasePrice
		}

		holdingValue := currentPrice.Close * holding.Quantity
		holdingValues[holding.IdHolding] = holdingValue
		allocation.TotalValue += holdingValue
	}

	for _, holding := range holdings {
		holdingWeight := 0.0
		if allocation.TotalValue > 0 {
			holdingWeight = holdingValues[holding.IdHolding] / allocation.TotalValue
		}

		sectors := db.GetSectorsByTicker(holding.Ticker)
		for _, sector := range sectors {
			allocation.BySector[sector.Name] += sector.Percentage * holdingWeight
		}

		regions := db.GetRegionsByTicker(holding.Ticker)
		for _, region := range regions {
			allocation.ByRegion[region.Name] += region.Percentage * holdingWeight
		}

		if !holding.Etf {
			assetDetails, exists := db.GetLatestAssetDetails(holding.Ticker)
			if exists && assetDetails.Sector != "" {
				allocation.BySector[assetDetails.Sector] += holdingWeight * 100
			} else {
				allocation.BySector["Unknown"] += holdingWeight * 100
			}
		}

		if holding.Etf {
			assets := db.GetAssetsByTicker(holding.Ticker)
			top10Count := len(assets)
			if top10Count > 10 {
				top10Count = 10
			}

			if top10Count > 0 {
				decay := 0.9
				totalWeight := 0.0
				for i := 0; i < top10Count; i++ {
					totalWeight += 1.0
					if i > 0 {
						totalWeight *= decay
					}
				}

				top10Allocation := 0.0
				for i := 0; i < top10Count; i++ {
					weight := 1.0
					if i > 0 {
						for j := 0; j < i; j++ {
							weight *= decay
						}
					}
					assetPercentage := (weight / totalWeight) * 100
					allocation.ByCompany[assets[i].Name] += assetPercentage * holdingWeight
					top10Allocation += assetPercentage
				}

				if top10Allocation < 100 {
					otherPercentage := 100 - top10Allocation
					allocation.ByCompany["Other"] += otherPercentage * holdingWeight
				}
			}
		} else {
			allocation.ByCompany[holding.Name] += holdingWeight * 100
		}
	}

	return allocation
}

type SectorData struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
}
type RegionData struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
}

type AssetData struct {
	IdAsset  string `json:"id_asset"`
	Name     string `json:"name"`
	Ticker   string `json:"ticker"`
	ISIN     string `json:"isin"`
	Exchange string `json:"exchange"`
	Sector   string `json:"sector"`
	Region   string `json:"region"`
}

type HoldingWithDetails struct {
	IdHolding     string       `json:"id_holding"`
	Name          string       `json:"name"`
	Ticker        string       `json:"ticker"`
	ISIN          string       `json:"isin"`
	Exchange      string       `json:"exchange"`
	Policy        string       `json:"policy"`
	Currency      string       `json:"currency"`
	Quantity      float64      `json:"quantity"`
	PurchasePrice float64      `json:"purchase_price"`
	TER           float64      `json:"ter"`
	Etf           bool         `json:"etf"`
	Sectors       []SectorData `json:"sectors"`
	Regions       []RegionData `json:"regions"`
	Assets        []AssetData  `json:"assets,omitempty"`
}

func (db *InMemDB) GetHoldingsWithDetails(userID string) []HoldingWithDetails {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userID)

	result := make([]HoldingWithDetails, 0, len(holdings))

	if len(holdings) == 0 {
		return result
	}

	for _, holding := range holdings {
		sectors := db.GetSectorsByTicker(holding.Ticker)
		sectorData := make([]SectorData, 0, len(sectors))
		for _, sector := range sectors {
			sectorData = append(sectorData, SectorData{
				Name:       sector.Name,
				Percentage: sector.Percentage,
			})
		}

		regions := db.GetRegionsByTicker(holding.Ticker)
		regionData := make([]RegionData, 0, len(regions))
		for _, region := range regions {
			regionData = append(regionData, RegionData{
				Name:       region.Name,
				Percentage: region.Percentage,
			})
		}

		assets := db.GetAssetsByTicker(holding.Ticker)
		assetData := make([]AssetData, 0, len(assets))
		for _, asset := range assets {
			assetData = append(assetData, AssetData{
				IdAsset:  asset.IdAsset,
				Name:     asset.Name,
				Ticker:   asset.Ticker,
				ISIN:     asset.ISIN,
				Exchange: asset.Exchange,
				Sector:   asset.Sector,
				Region:   asset.Region,
			})
		}

		result = append(result, HoldingWithDetails{
			IdHolding:     holding.IdHolding,
			Name:          holding.Name,
			Ticker:        holding.Ticker,
			ISIN:          holding.ISIN,
			Exchange:      holding.Exchange,
			Policy:        holding.Policy,
			Currency:      holding.Currency,
			Quantity:      holding.Quantity,
			PurchasePrice: holding.PurchasePrice,
			TER:           holding.TER,
			Etf:           holding.Etf,
			Sectors:       sectorData,
			Regions:       regionData,
			Assets:        assetData,
		})
	}

	return result
}

func (db *InMemDB) GetPortfolioValue(userId string) float64 {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userId)
	totalValue := 0.0

	for _, holding := range holdings {
		currentPrice, exists := db.GetLatestPrice(holding.Ticker)
		if !exists {
			currentPrice.Close = holding.PurchasePrice
		}
		holdingValue := currentPrice.Close * holding.Quantity
		totalValue += holdingValue
	}
	return totalValue
}

type PortfolioCandle struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

func (db *InMemDB) GetPortfolioValueHistory(userId string, start time.Time, end time.Time, intervalSeconds int64) []PortfolioCandle {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userId)

	if len(holdings) == 0 {
		return []PortfolioCandle{}
	}

	tickerQuantities := make(map[string]float64)
	for _, holding := range holdings {
		tickerQuantities[holding.Ticker] += holding.Quantity
	}

	type PriceData struct {
		Open   float64
		High   float64
		Low    float64
		Close  float64
		Volume int64
	}

	bucketData := make(map[int64]map[string]*PriceData)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	for ticker := range tickerQuantities {
		prices := db.GetPricesByTickerRange(ticker, start, end)

		for _, price := range prices {
			timestamp, err := strconv.ParseInt(price.Date, 10, 64)
			if err != nil || timestamp < startTimestamp || timestamp > endTimestamp {
				continue
			}

			bucket := (timestamp / intervalSeconds) * intervalSeconds

			if bucketData[bucket] == nil {
				bucketData[bucket] = make(map[string]*PriceData)
			}

			if bucketData[bucket][ticker] == nil {
				bucketData[bucket][ticker] = &PriceData{
					Open:   price.Open,
					High:   price.High,
					Low:    price.Low,
					Close:  price.Close,
					Volume: price.Volume,
				}
			} else {
				pd := bucketData[bucket][ticker]
				if price.High > pd.High {
					pd.High = price.High
				}
				if price.Low < pd.Low {
					pd.Low = price.Low
				}
				pd.Close = price.Close
				pd.Volume += price.Volume
			}
		}
	}

	bucketTimestamps := make([]int64, 0, len(bucketData))
	for ts := range bucketData {
		bucketTimestamps = append(bucketTimestamps, ts)
	}

	for i := 0; i < len(bucketTimestamps)-1; i++ {
		for j := i + 1; j < len(bucketTimestamps); j++ {
			if bucketTimestamps[i] > bucketTimestamps[j] {
				bucketTimestamps[i], bucketTimestamps[j] = bucketTimestamps[j], bucketTimestamps[i]
			}
		}
	}

	lastKnownPrices := make(map[string]*PriceData)
	result := make([]PortfolioCandle, 0, len(bucketTimestamps))

	for _, bucket := range bucketTimestamps {
		tickerPrices := bucketData[bucket]

		var portfolioOpen, portfolioHigh, portfolioLow, portfolioClose float64
		var portfolioVolume int64

		for ticker, quantity := range tickerQuantities {
			var pd *PriceData

			if tickerPrices[ticker] != nil {
				pd = tickerPrices[ticker]
				lastKnownPrices[ticker] = pd
			} else if lastKnownPrices[ticker] != nil {
				pd = lastKnownPrices[ticker]
			} else {
				continue
			}

			portfolioOpen += pd.Open * quantity
			portfolioHigh += pd.High * quantity
			portfolioLow += pd.Low * quantity
			portfolioClose += pd.Close * quantity
			portfolioVolume += pd.Volume
		}

		if portfolioClose > 0 {
			result = append(result, PortfolioCandle{
				Timestamp: bucket,
				Open:      portfolioOpen,
				High:      portfolioHigh,
				Low:       portfolioLow,
				Close:     portfolioClose,
				Volume:    portfolioVolume,
			})
		}
	}

	return result
}

type PortfolioChange struct {
	CurrentValue   float64 `json:"current_value"`
	DayChange      float64 `json:"day_change"`
	DayChangePct   float64 `json:"day_change_pct"`
	TotalChange    float64 `json:"total_change"`
	TotalChangePct float64 `json:"total_change_pct"`
	TotalInvested  float64 `json:"total_invested"`
}

func (db *InMemDB) GetPortfolioChange(userId string) PortfolioChange {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userId)

	result := PortfolioChange{
		CurrentValue:   0,
		DayChange:      0,
		DayChangePct:   0,
		TotalChange:    0,
		TotalChangePct: 0,
		TotalInvested:  0,
	}

	if len(holdings) == 0 {
		return result
	}

	now := time.Now().UTC()
	oneDayAgo := now.Add(-24 * time.Hour)

	var currentValue float64
	var previousDayValue float64
	var totalInvested float64

	for _, holding := range holdings {
		totalInvested += holding.PurchasePrice * holding.Quantity

		latestPrice, exists := db.GetLatestPrice(holding.Ticker)
		if !exists {
			latestPrice.Close = holding.PurchasePrice
		}
		currentValue += latestPrice.Close * holding.Quantity

		previousPrice := latestPrice.Close
		prices := db.GetPricesByTickerRange(holding.Ticker, oneDayAgo.Add(-7*24*time.Hour), oneDayAgo)

		oneDayAgoTimestamp := oneDayAgo.Unix()
		var closestPrice Prices
		var closestDiff int64 = -1

		for _, price := range prices {
			timestamp, err := strconv.ParseInt(price.Date, 10, 64)
			if err != nil {
				continue
			}

			if timestamp <= oneDayAgoTimestamp {
				diff := oneDayAgoTimestamp - timestamp
				if closestDiff == -1 || diff < closestDiff {
					closestDiff = diff
					closestPrice = price
				}
			}
		}

		if closestDiff != -1 {
			previousPrice = closestPrice.Close
		}

		previousDayValue += previousPrice * holding.Quantity
	}

	dayChange := currentValue - previousDayValue
	dayChangePercent := 0.0
	if previousDayValue > 0 {
		dayChangePercent = (dayChange / previousDayValue) * 100
	}

	totalChange := currentValue - totalInvested
	totalChangePercent := 0.0
	if totalInvested > 0 {
		totalChangePercent = (totalChange / totalInvested) * 100
	}

	result.CurrentValue = currentValue
	result.DayChange = dayChange
	result.DayChangePct = dayChangePercent
	result.TotalChange = totalChange
	result.TotalChangePct = totalChangePercent
	result.TotalInvested = totalInvested

	return result
}

func (db *InMemDB) getLatestNewsForPortfolio(userId string) []News {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	news := make([]News, 0)
	holdings := db.GetHoldingsByUser(userId)

	if len(holdings) == 0 {
		return news
	}

	for _, holding := range holdings {
		holdingNews := db.GetNewsByTicker(holding.Ticker)
		if holding.Etf {
			assets := db.GetAssetsByTicker(holding.Ticker)
			for _, asset := range assets {
				assetNews := db.GetNewsByTicker(asset.Ticker)
				holdingNews = append(holdingNews, assetNews...)
			}
		}
		news = append(news, holdingNews...)
	}
	return news
}

func (db *InMemDB) getPortfolioSentiment(userId string, date time.Time) (averageSentiment float64, articleCount int) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	news := db.getLatestNewsForPortfolio(userId)

	if len(news) == 0 {
		return 0, 0
	}

	totalSentiment := 0.0
	articleCount = 0

	dateStr := date.Format("2006-01-02")
	for _, n := range news {
		if n.PublishedAt >= dateStr && n.PublishedAt < dateStr+" 23:59:59" {
			totalSentiment += n.Sentiment
			articleCount++
		}
	}

	if articleCount > 0 {
		averageSentiment = totalSentiment / float64(articleCount)
	} else {
		averageSentiment = 0
	}

	return averageSentiment, articleCount
}

func (db *InMemDB) getPortfolioDailySummary(userId string, date time.Time) (PortfolioDailySentiment, bool) {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	key := userId + "_" + date.Format("2006-01-02")

	db.portfolioDailySentimentsTable.mutex.RLock()
	defer db.portfolioDailySentimentsTable.mutex.RUnlock()

	sentiment, exists := db.portfolioDailySentimentsTable.PortfolioDailySentiments[key]
	return sentiment, exists
}

type PortfolioStats struct {
	YoYReturn        float64 `json:"yoy_return"`
	MaxDrawdown      float64 `json:"max_drawdown"`
	AvgDrawdown      float64 `json:"avg_drawdown"`
	SortinoRatio     float64 `json:"sortino_ratio"`
	AggregatedTER    float64 `json:"aggregated_ter"`
	TotalValue       float64 `json:"total_value"`
	TotalCost        float64 `json:"total_cost"`
	TotalGainLoss    float64 `json:"total_gain_loss"`
	TotalGainLossPct float64 `json:"total_gain_loss_pct"`
}

func (db *InMemDB) GetPortfolioStats(userId string) PortfolioStats {
	db.snapshotMutex.RLock()
	defer db.snapshotMutex.RUnlock()

	holdings := db.GetHoldingsByUser(userId)

	stats := PortfolioStats{}

	if len(holdings) == 0 {
		return stats
	}

	tickerQuantities := make(map[string]float64)
	tickerHoldings := make(map[string]Holding)
	for _, holding := range holdings {
		tickerQuantities[holding.Ticker] += holding.Quantity
		tickerHoldings[holding.Ticker] = holding
	}

	now := time.Now().UTC()
	startTime := now.Add(-365 * 24 * time.Hour)

	type DayPrice struct {
		Ticker string
		Close  float64
	}
	dayPrices := make(map[int64][]DayPrice)

	for ticker := range tickerQuantities {
		prices := db.GetPricesByTickerRange(ticker, startTime, now)

		for _, price := range prices {
			timestamp, err := strconv.ParseInt(price.Date, 10, 64)
			if err != nil {
				continue
			}
			dayBucket := (timestamp / 86400) * 86400
			dayPrices[dayBucket] = append(dayPrices[dayBucket], DayPrice{ticker, price.Close})
		}
	}

	days := make([]int64, 0, len(dayPrices))
	for day := range dayPrices {
		days = append(days, day)
	}
	for i := 0; i < len(days)-1; i++ {
		for j := i + 1; j < len(days); j++ {
			if days[i] > days[j] {
				days[i], days[j] = days[j], days[i]
			}
		}
	}

	var portfolioValues []float64
	var timestamps []int64
	lastPrices := make(map[string]float64)

	for _, day := range days {
		for _, dp := range dayPrices[day] {
			lastPrices[dp.Ticker] = dp.Close
		}

		dayValue := 0.0
		allTickersHavePrice := true
		for ticker, qty := range tickerQuantities {
			if price, ok := lastPrices[ticker]; ok {
				dayValue += price * qty
			} else {
				allTickersHavePrice = false
			}
		}

		if allTickersHavePrice && dayValue > 0 {
			portfolioValues = append(portfolioValues, dayValue)
			timestamps = append(timestamps, day)
		}
	}

	var dailyReturns []float64
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			ret := (portfolioValues[i] - portfolioValues[i-1]) / portfolioValues[i-1]
			dailyReturns = append(dailyReturns, ret)
		}
	}

	yoyReturn := calculateYoYReturn(portfolioValues, timestamps)
	maxDD, avgDD := calculateDrawdowns(portfolioValues)
	sortinoRatio := calculateSortinoRatio(dailyReturns, 0.0)

	totalValue := 0.0
	totalCost := 0.0
	weightedTER := 0.0

	for ticker, qty := range tickerQuantities {
		holding := tickerHoldings[ticker]
		currentPrice := lastPrices[ticker]
		value := currentPrice * qty
		totalValue += value
		totalCost += holding.PurchasePrice * qty
		weightedTER += holding.TER * value
	}

	aggregatedTER := 0.0
	if totalValue > 0 {
		aggregatedTER = weightedTER / totalValue
	}

	totalGainLoss := totalValue - totalCost
	totalGainLossPct := 0.0
	if totalCost > 0 {
		totalGainLossPct = totalGainLoss / totalCost * 100
	}

	stats.YoYReturn = roundFloat(yoyReturn, 2)
	stats.MaxDrawdown = roundFloat(maxDD, 2)
	stats.AvgDrawdown = roundFloat(avgDD, 2)
	stats.SortinoRatio = roundFloat(sortinoRatio, 2)
	stats.AggregatedTER = roundFloat(aggregatedTER, 4)
	stats.TotalValue = roundFloat(totalValue, 2)
	stats.TotalCost = roundFloat(totalCost, 2)
	stats.TotalGainLoss = roundFloat(totalGainLoss, 2)
	stats.TotalGainLossPct = roundFloat(totalGainLossPct, 2)

	return stats
}

func calculateYoYReturn(prices []float64, timestamps []int64) float64 {
	if len(prices) < 2 {
		return 0
	}

	now := time.Now().UTC().Unix()
	oneYearAgo := now - 365*24*3600

	var startPrice, endPrice float64
	startFound := false

	for i, ts := range timestamps {
		if !startFound && ts >= oneYearAgo {
			startPrice = prices[i]
			startFound = true
		}
		endPrice = prices[i]
	}

	if !startFound || startPrice == 0 {
		startPrice = prices[0]
	}

	return (endPrice - startPrice) / startPrice * 100
}

func calculateDrawdowns(values []float64) (maxDD float64, avgDD float64) {
	if len(values) < 2 {
		return 0, 0
	}

	peak := values[0]
	var drawdowns []float64

	for _, value := range values {
		if value > peak {
			peak = value
		}
		if peak > 0 {
			drawdown := (peak - value) / peak * 100
			if drawdown > 0 {
				drawdowns = append(drawdowns, drawdown)
			}
			if drawdown > maxDD {
				maxDD = drawdown
			}
		}
	}

	if len(drawdowns) > 0 {
		sum := 0.0
		for _, dd := range drawdowns {
			sum += dd
		}
		avgDD = sum / float64(len(drawdowns))
	}

	return maxDD, avgDD
}

func calculateSortinoRatio(dailyReturns []float64, riskFreeRate float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}

	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	avgReturn := sum / float64(len(dailyReturns))

	var downsideSquares []float64
	for _, r := range dailyReturns {
		if r < riskFreeRate {
			diff := r - riskFreeRate
			downsideSquares = append(downsideSquares, diff*diff)
		}
	}

	if len(downsideSquares) == 0 {
		return 0
	}

	downsideSum := 0.0
	for _, sq := range downsideSquares {
		downsideSum += sq
	}
	downsideDeviation := 0.0
	if len(downsideSquares) > 0 {
		downsideDeviation = 1.0
		temp := downsideSum / float64(len(downsideSquares))
		for i := 0; i < 10; i++ {
			downsideDeviation = (downsideDeviation + temp/downsideDeviation) / 2
		}
	}

	if downsideDeviation == 0 {
		return 0
	}

	annualizedReturn := avgReturn * 252
	annualizedDownside := downsideDeviation * 15.8745

	return (annualizedReturn - riskFreeRate) / annualizedDownside
}

func roundFloat(val float64, precision int) float64 {
	multiplier := 1.0
	for i := 0; i < precision; i++ {
		multiplier *= 10
	}
	return float64(int(val*multiplier+0.5)) / multiplier
}
