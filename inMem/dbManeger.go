package inmem

import (
	"encoding/gob"
	"log"
	"os"
	"sync"
	"time"
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
	userID        string
	currency      string
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
	IdHolding  string
}

type SectorsTable struct {
	Sectors map[string]Sector
	counter int
	mutex   sync.RWMutex
}

type Sector struct {
	Name       string
	Percentage float64
	IdHolding  string
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
		if holding.userID == userID {
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

	key := region.IdHolding + "_" + region.Name
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
		if region.IdHolding == holdingID {
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
		if region.IdHolding == holdingID {
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

	key := sector.IdHolding + "_" + sector.Name
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
		if sector.IdHolding == holdingID {
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
		if sector.IdHolding == holdingID {
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

	assets := make([]Asset, 0, db.assetsTable.counter)
	for _, asset := range db.assetsTable.Assets {
		if asset.idHolding == holdingID {
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

	assets := make([]Asset, 0, db.assetsTable.counter)
	for _, asset := range db.assetsTable.Assets {
		if asset.Ticker == ticker {
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
		if asset.idHolding == holdingID {
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
