package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (d *DB) AddUser(user User) error {
	_, err := d.Exec(`INSERT INTO users (id, user_name, email, password) VALUES (?, ?, ?, ?)`,
		user.ID, user.UserName, strings.ToLower(user.Email), user.Password)
	return err
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	var user User
	err := d.QueryRow(`SELECT id, user_name, email, password FROM users WHERE email = ?`, strings.ToLower(email)).
		Scan(&user.ID, &user.UserName, &user.Email, &user.Password)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *DB) GetAllUsers() ([]User, error) {
	rows, err := d.Query(`SELECT id, user_name, email FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.UserName, &user.Email); err == nil {
			users = append(users, user)
		}
	}
	return users, rows.Err()
}

func (d *DB) AddHolding(holding Holding) error {
	_, err := d.Exec(`INSERT INTO holdings (id_holding, name, ticker, isin, exchange, etf, quantity,
		purchase_price, ter, policy, user_id, currency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		holding.IDHolding, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, boolToInt(holding.ETF),
		holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.UserID, holding.Currency)
	return err
}

func (d *DB) RemoveHolding(id string, userID string) error {
	_, err := d.Exec(`DELETE FROM holdings WHERE id_holding = ? AND user_id = ?`, id, userID)
	return err
}

func (d *DB) ModifyHolding(holding Holding) error {
	_, err := d.Exec(`UPDATE holdings SET name = ?, ticker = ?, isin = ?, exchange = ?, quantity = ?,
		purchase_price = ?, ter = ?, policy = ?, etf = ?, currency = ? WHERE id_holding = ? AND user_id = ?`,
		holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Quantity, holding.PurchasePrice,
		holding.TER, holding.Policy, boolToInt(holding.ETF), holding.Currency, holding.IDHolding, holding.UserID)
	return err
}

const holdingColumns = `id_holding, name, ticker, COALESCE(isin,''), COALESCE(exchange,''), COALESCE(policy,''),
	user_id, COALESCE(currency,''), quantity, purchase_price, COALESCE(ter,0), COALESCE(etf,0)`

func scanHolding(scanner interface{ Scan(...any) error }) (Holding, error) {
	var holding Holding
	var etf int
	err := scanner.Scan(&holding.IDHolding, &holding.Name, &holding.Ticker, &holding.ISIN, &holding.Exchange,
		&holding.Policy, &holding.UserID, &holding.Currency, &holding.Quantity, &holding.PurchasePrice,
		&holding.TER, &etf)
	holding.ETF = etf == 1
	return holding, err
}

func (d *DB) GetHoldingsByUser(userID string) ([]Holding, error) {
	rows, err := d.Query(`SELECT `+holdingColumns+` FROM holdings WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var holdings []Holding
	for rows.Next() {
		holding, err := scanHolding(rows)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, holding)
	}
	return holdings, rows.Err()
}

func (d *DB) GetHoldingsByTicker(ticker string) ([]Holding, error) {
	rows, err := d.Query(`SELECT `+holdingColumns+` FROM holdings WHERE ticker = ?`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var holdings []Holding
	for rows.Next() {
		holding, err := scanHolding(rows)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, holding)
	}
	return holdings, rows.Err()
}

func (d *DB) GetAllETFHoldings() ([]Holding, error) {
	rows, err := d.Query(`SELECT ` + holdingColumns + ` FROM holdings WHERE etf = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var holdings []Holding
	for rows.Next() {
		holding, err := scanHolding(rows)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, holding)
	}
	return holdings, rows.Err()
}

func (d *DB) AddAsset(asset Asset) error {
	_, err := d.Exec(`INSERT INTO assets (id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		asset.IDAsset, asset.Name, asset.Ticker, asset.ISIN, asset.Exchange, asset.Sector, asset.Region,
		asset.IDHolding, asset.Currency)
	return err
}

func (d *DB) DeleteAssetsForHolding(holdingID string) error {
	_, err := d.Exec(`DELETE FROM assets WHERE id_holding = ?`, holdingID)
	return err
}

func (d *DB) DeleteBreakdownForHolding(holdingID string) error {
	if _, err := d.Exec(`DELETE FROM sectors WHERE id_holding = ?`, holdingID); err != nil {
		return err
	}
	_, err := d.Exec(`DELETE FROM regions WHERE id_holding = ?`, holdingID)
	return err
}

func (d *DB) GetAssetsByTicker(ticker string) ([]Asset, error) {
	rows, err := d.Query(`SELECT id_asset, name, ticker, COALESCE(isin,''), COALESCE(exchange,''),
		COALESCE(sector,''), COALESCE(region,''), id_holding, COALESCE(currency,'') FROM assets WHERE ticker = ?`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssets(rows)
}

func (d *DB) GetAssetsForHolding(holdingID string) ([]Asset, error) {
	rows, err := d.Query(`SELECT id_asset, name, ticker, COALESCE(isin,''), COALESCE(exchange,''),
		COALESCE(sector,''), COALESCE(region,''), id_holding, COALESCE(currency,'') FROM assets WHERE id_holding = ?`, holdingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssets(rows)
}

func scanAssets(rows *sql.Rows) ([]Asset, error) {
	var assets []Asset
	for rows.Next() {
		var asset Asset
		err := rows.Scan(&asset.IDAsset, &asset.Name, &asset.Ticker, &asset.ISIN, &asset.Exchange,
			&asset.Sector, &asset.Region, &asset.IDHolding, &asset.Currency)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func (d *DB) AddSector(sector Sector) error {
	_, err := d.Exec(`INSERT INTO sectors (name, percentage, id_holding) VALUES (?, ?, ?)`,
		sector.Name, sector.Percentage, sector.IDHolding)
	return err
}

func (d *DB) AddRegion(region Region) error {
	_, err := d.Exec(`INSERT INTO regions (name, percentage, id_holding) VALUES (?, ?, ?)`,
		region.Name, region.Percentage, region.IDHolding)
	return err
}

func (d *DB) GetBreakdownForHolding(holdingID string) ([]Sector, []Region, error) {
	sectorRows, err := d.Query(`SELECT name, percentage, id_holding FROM sectors WHERE id_holding = ?`, holdingID)
	if err != nil {
		return nil, nil, err
	}
	var sectors []Sector
	for sectorRows.Next() {
		var sector Sector
		if err := sectorRows.Scan(&sector.Name, &sector.Percentage, &sector.IDHolding); err == nil {
			sectors = append(sectors, sector)
		}
	}
	sectorRows.Close()

	regionRows, err := d.Query(`SELECT name, percentage, id_holding FROM regions WHERE id_holding = ?`, holdingID)
	if err != nil {
		return nil, nil, err
	}
	defer regionRows.Close()
	var regions []Region
	for regionRows.Next() {
		var region Region
		if err := regionRows.Scan(&region.Name, &region.Percentage, &region.IDHolding); err == nil {
			regions = append(regions, region)
		}
	}
	return sectors, regions, regionRows.Err()
}

func (d *DB) UpdateHoldingISIN(id string, isin string) error {
	_, err := d.Exec(`UPDATE holdings SET isin = ? WHERE id_holding = ?`, isin, id)
	return err
}

func (d *DB) UpdateHoldingTickerByISIN(isin string, ticker string) error {
	_, err := d.Exec(`UPDATE holdings SET ticker = ? WHERE isin = ?`, ticker, isin)
	return err
}

func (d *DB) UpdateHoldingTickerByID(id string, ticker string) error {
	_, err := d.Exec(`UPDATE holdings SET ticker = ? WHERE id_holding = ?`, ticker, id)
	return err
}

func (d *DB) AddAssetDetails(detail AssetDetails) error {
	_, err := d.Exec(`INSERT INTO asset_details (id_asset, ticker, isin, market_cap, market_cap_eur, country,
		sector, eps, pb_ratio, pe_ratio, dividend_yield, revenue, net_income, profit_margin, hash, date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		NewID(), detail.Ticker, detail.ISIN, detail.MarketCap, detail.MarketCapEur, detail.Country, detail.Sector,
		detail.Eps, detail.PbRatio, detail.PeRatio, detail.DividendYield, detail.Revenue, detail.NetIncome,
		detail.ProfitMargin, detail.Hash, detail.Date)
	return err
}

const detailColumns = `ticker, COALESCE(isin,''), COALESCE(market_cap,''), COALESCE(market_cap_eur,''),
	COALESCE(country,''), COALESCE(sector,''), COALESCE(eps,''), COALESCE(pb_ratio,''), COALESCE(pe_ratio,''),
	COALESCE(dividend_yield,''), COALESCE(revenue,''), COALESCE(net_income,''), COALESCE(profit_margin,''),
	COALESCE(hash,''), COALESCE(date,'')`

func scanDetail(scanner interface{ Scan(...any) error }) (AssetDetails, error) {
	var detail AssetDetails
	err := scanner.Scan(&detail.Ticker, &detail.ISIN, &detail.MarketCap, &detail.MarketCapEur, &detail.Country,
		&detail.Sector, &detail.Eps, &detail.PbRatio, &detail.PeRatio, &detail.DividendYield, &detail.Revenue,
		&detail.NetIncome, &detail.ProfitMargin, &detail.Hash, &detail.Date)
	return detail, err
}

func (d *DB) LatestAssetDetails(ticker string) (*AssetDetails, error) {
	detail, err := scanDetail(d.QueryRow(`SELECT `+detailColumns+` FROM asset_details
		WHERE ticker = ? OR isin = ? ORDER BY CAST(date AS INTEGER) DESC LIMIT 1`, ticker, ticker))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

func (d *DB) AssetDetailsHistory(ticker string) ([]AssetDetails, error) {
	rows, err := d.Query(`SELECT `+detailColumns+` FROM asset_details
		WHERE ticker = ? OR isin = ? ORDER BY CAST(date AS INTEGER) DESC`, ticker, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var details []AssetDetails
	for rows.Next() {
		detail, err := scanDetail(rows)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, rows.Err()
}

func (d *DB) UniqueTickers() ([]string, error) {
	rows, err := d.Query(`SELECT DISTINCT ticker FROM holdings WHERE ticker <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tickers []string
	for rows.Next() {
		var ticker string
		if err := rows.Scan(&ticker); err == nil {
			tickers = append(tickers, ticker)
		}
	}
	return tickers, rows.Err()
}

func (d *DB) UniqueISINs() ([]string, error) {
	rows, err := d.Query(`SELECT DISTINCT isin FROM holdings WHERE isin IS NOT NULL AND isin <> '' AND isin <> 'N/A'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var isins []string
	for rows.Next() {
		var isin string
		if err := rows.Scan(&isin); err == nil {
			isins = append(isins, isin)
		}
	}
	return isins, rows.Err()
}

// TickerForISIN maps an ISIN to the ticker the rest of the app uses for prices. The
// mapping is stored with the holdings and assets the user already imported, so it
// avoids the remote lookup for every request.
func (d *DB) TickerForISIN(isin string) string {
	if isin == "" {
		return ""
	}
	for _, query := range []string{
		`SELECT ticker FROM holdings WHERE isin = ? AND ticker <> '' LIMIT 1`,
		`SELECT ticker FROM assets WHERE isin = ? AND ticker <> '' LIMIT 1`,
	} {
		var ticker string
		if err := d.QueryRow(query, isin).Scan(&ticker); err == nil && ticker != "" && ticker != isin {
			return ticker
		}
	}
	return ""
}

func (d *DB) HoldingsMissingISIN() ([]Holding, error) {
	rows, err := d.Query(`SELECT ` + holdingColumns + ` FROM holdings
		WHERE (isin IS NULL OR isin = '' OR isin = 'N/A') AND ticker <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var holdings []Holding
	for rows.Next() {
		holding, err := scanHolding(rows)
		if err != nil {
			continue
		}
		holdings = append(holdings, holding)
	}
	return holdings, rows.Err()
}

func (d *DB) TickersWithDetails(today string) ([]string, error) {
	rows, err := d.Query(`SELECT DISTINCT ticker FROM holdings WHERE ticker <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tickers []string
	for rows.Next() {
		var ticker string
		if err := rows.Scan(&ticker); err == nil {
			tickers = append(tickers, ticker)
		}
	}
	return tickers, rows.Err()
}

func Today() string {
	return time.Now().UTC().Format("2006-01-02")
}
