from typing import List, Optional, Tuple, Dict, Any
import yfinance as yf
import requests
from bs4 import BeautifulSoup
import re
from functools import lru_cache
import time

# (Name, Ticker, ISIN, Exchange, Sector, Region)
HoldingInfo = Tuple[str, str, str, str, str, str]

# ETF aggregate data structure
class ETFData:
    def __init__(self):
        self.holdings: List[HoldingInfo] = []
        self.sectors: Dict[str, float] = {}  # {sector_name: percentage}
        self.regions: Dict[str, float] = {}  # {country/region: percentage}
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert ETFData to a JSON-serializable dictionary"""
        return {
            'holdings': [
                {
                    'name': h[0],
                    'ticker': h[1],
                    'isin': h[2],
                    'exchange': h[3],
                    'sector': h[4],
                    'region': h[5]
                }
                for h in self.holdings
            ],
            'sectors': self.sectors,
            'regions': self.regions
        }

@lru_cache(maxsize=1000)
def fetch_isin_from_multiple_sources(ticker: str) -> Optional[str]:
    """Try multiple methods to get ISIN"""
    try:
        stock = yf.Ticker(ticker)
        
        try:
            isin = stock.isin
            if isin and isin != '-' and isin != '':
                return isin
        except:
            pass
        
        try:
            isin = stock.get_isin()
            if isin and isin != '-' and isin != '':
                return isin
        except:
            pass
        
        info = stock.info
        isin = info.get('isin')
        if isin and isin != '-' and isin != '':
            return isin
        
        try:
            quote = stock.get_quote_table()
            if quote and 'isin' in quote:
                isin = quote['isin']
                if isin and isin != '-' and isin != '':
                    return isin
        except:
            pass
        
    except:
        pass
    
    return None

@lru_cache(maxsize=1000)
def get_ticker_details(ticker: str) -> Dict[str, str]:
    try:
        stock = yf.Ticker(ticker)
        info = stock.info
        
        isin_value = fetch_isin_from_multiple_sources(ticker)
        
        return {
            'name': info.get('shortName', info.get('longName', 'Unknown')),
            'isin': isin_value if isin_value else 'N/A',
            'exchange': info.get('exchange', 'Unknown'),
            'sector': info.get('sector', info.get('industry', 'Unknown')),
            'region': info.get('country', 'Unknown')
        }
    except Exception as e:
        print(f"Error fetching details for {ticker}: {e}")
        return {
            'name': 'Unknown',
            'isin': 'N/A',
            'exchange': 'Unknown',
            'sector': 'Unknown',
            'region': 'Unknown'
        }

def enrich_holdings_with_details(holdings: List[Tuple[str, float]]) -> List[HoldingInfo]:
    enriched = []
    
    for ticker, percentage in holdings:
        details = get_ticker_details(ticker)
        
        enriched.append((
            details['name'],
            ticker,
            details['isin'],
            details['exchange'],
            details['sector'],
            details['region']
        ))
    
    return enriched

@lru_cache(maxsize=1000)
def get_sectors_and_regions_from_justetf(isin: str) -> Tuple[Dict[str, float], Dict[str, float]]:
    """Scrape sector and country/region breakdown from JustETF with Selenium for JS expansion"""
    sectors = {}
    regions = {}
    
    try:
        if not isin or isin == 'N/A':
            return sectors, regions
        
        url = f"https://www.justetf.com/en/etf-profile.html?isin={isin}"
        
        try:
            from selenium import webdriver
            from selenium.webdriver.common.by import By
            from selenium.webdriver.support.ui import WebDriverWait
            from selenium.webdriver.support import expected_conditions as EC
            from selenium.webdriver.chrome.options import Options
            from selenium.webdriver.chrome.service import Service
            from webdriver_manager.chrome import ChromeDriverManager
            
            # Setup headless Chrome
            chrome_options = Options()
            chrome_options.add_argument("--headless")
            chrome_options.add_argument("--no-sandbox")
            chrome_options.add_argument("--disable-dev-shm-usage")
            chrome_options.add_argument("user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
            
            driver = webdriver.Chrome(service=Service(ChromeDriverManager().install()), options=chrome_options)
            driver.get(url)
            
            # Wait for the page to load
            WebDriverWait(driver, 10).until(
                EC.presence_of_element_located((By.CSS_SELECTOR, "table[data-testid='etf-holdings_countries_table']"))
            )
            
            # Click "Show more" for countries if exists
            try:
                countries_show_more = driver.find_element(By.CSS_SELECTOR, "a[data-testid='etf-holdings_countries_load-more_link']")
                driver.execute_script("arguments[0].click();", countries_show_more)
                time.sleep(1)
            except:
                pass
            
            # Click "Show more" for sectors if exists
            try:
                sectors_show_more = driver.find_element(By.CSS_SELECTOR, "a[data-testid='etf-holdings_sectors_load-more_link']")
                driver.execute_script("arguments[0].click();", sectors_show_more)
                time.sleep(1)
            except:
                pass
            
            # Now get the expanded HTML
            html_content = driver.page_source
            driver.quit()
            
            soup = BeautifulSoup(html_content, 'html.parser')
            
        except Exception as e:
            print(f"  Selenium failed ({e}), falling back to regular requests")
            headers = {
                'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
            }
            response = requests.get(url, headers=headers, timeout=15)
            soup = BeautifulSoup(response.content, 'html.parser')
        
        # Extract all country rows
        countries_table = soup.find('table', {'data-testid': 'etf-holdings_countries_table'})
        if countries_table:
            tbody = countries_table.find('tbody')
            if tbody:
                all_rows = tbody.find_all('tr')
                for row in all_rows:
                    tds = row.find_all('td')
                    if len(tds) >= 2:
                        country_name = tds[0].get_text(strip=True)
                        pct_span = tds[1].find('span')
                        if pct_span:
                            pct_text = pct_span.get_text(strip=True)
                            pct_match = re.search(r'(\d+\.?\d*)', pct_text)
                            
                            if pct_match and country_name and country_name.lower() != 'other':
                                regions[country_name] = float(pct_match.group(1))
        
        # Extract all sector rows
        sectors_table = soup.find('table', {'data-testid': 'etf-holdings_sectors_table'})
        if sectors_table:
            tbody = sectors_table.find('tbody')
            if tbody:
                all_rows = tbody.find_all('tr')
                for row in all_rows:
                    tds = row.find_all('td')
                    if len(tds) >= 2:
                        sector_name = tds[0].get_text(strip=True)
                        pct_span = tds[1].find('span')
                        if pct_span:
                            pct_text = pct_span.get_text(strip=True)
                            pct_match = re.search(r'(\d+\.?\d*)', pct_text)
                            
                            if pct_match and sector_name and sector_name.lower() != 'other':
                                sectors[sector_name] = float(pct_match.group(1))
        
        
    except Exception as e:
        print(f"JustETF sector/region scraping failed: {e}")
        import traceback
        traceback.print_exc()
    
    return sectors, regions
    
@lru_cache(maxsize=1000)
def get_top_holdings_from_yfinance(ticker: str) -> Optional[List[Tuple[str, float]]]:
    """Get top 10 holdings from Yahoo Finance"""
    try:
        etf = yf.Ticker(ticker)
        
        # Try funds_data.top_holdings first
        try:
            funds_data = etf.funds_data
            if funds_data and hasattr(funds_data, 'top_holdings'):
                top_holdings = funds_data.top_holdings
                if top_holdings is not None and not top_holdings.empty:
                    result = []
                    for idx, row in top_holdings.iterrows():
                        symbol = row.get('Symbol', idx)
                        percentage = row.get('Holding Percent', 0)
                        result.append((symbol, percentage))
                    return result
        except:
            pass
        
        # Fallback to info.holdings
        holdings = etf.info.get('holdings', [])
        if holdings and len(holdings) > 0:
            top_holdings = sorted(holdings, key=lambda x: x.get('holdingPercent', 0), reverse=True)[:10]
            return [(h.get('symbol', 'Unknown'), h.get('holdingPercent', 0)) for h in top_holdings]
        
        return None
    except Exception as e:
        print(f"yfinance holdings fetch failed for {ticker}: {e}")
        return None
    
def get_holdings_from_yfinance(ticker: str) -> Optional[List[Tuple[str, float]]]:
    try:
        etf = yf.Ticker(ticker)
        
        holdings = etf.info.get('holdings', [])
        if holdings and len(holdings) > 0:
            all_holdings = sorted(holdings, key=lambda x: x.get('holdingPercent', 0), reverse=True)
            return [(h.get('symbol', 'Unknown'), h.get('holdingPercent', 0)) for h in all_holdings]
        
        try:
            funds_data = etf.funds_data
            if funds_data and hasattr(funds_data, 'top_holdings'):
                top_holdings = funds_data.top_holdings
                if top_holdings is not None and not top_holdings.empty:
                    result = []
                    for idx, row in top_holdings.iterrows():
                        symbol = row.get('Symbol', idx)
                        percentage = row.get('Holding Percent', 0)
                        result.append((symbol, percentage))
                    return result
        except:
            pass
        
        return None
    except Exception as e:
        print(f"yfinance holdings fetch failed for {ticker}: {e}")
        return None

@lru_cache(maxsize=2048)
def get_etf_data(ticker: str, isin: str = None, etf_name: str = None) -> ETFData:
    """
    Get ETF data including:
    - Top 10 holdings with details (Name, Ticker, ISIN, Exchange, Sector, Region)
    - Overall sector breakdown
    - Overall region/country breakdown
    """
    result = ETFData()
    
    print(f"Fetching ETF data for {ticker} (ISIN: {isin})...")
    
    # Step 1: Get top holdings from yfinance
    basic_holdings = get_top_holdings_from_yfinance(ticker)
    if basic_holdings and len(basic_holdings) > 0:
        result.holdings = enrich_holdings_with_details(basic_holdings)
    else:
        # Use generic fallback
        generic_tickers = ['AAPL', 'MSFT', 'GOOGL', 'AMZN', 'NVDA', 'TSLA', 'META', 'BRK.B', 'UNH', 'JNJ']
        equal_weight = 1.0 / len(generic_tickers)
        basic_holdings = [(ticker, equal_weight) for ticker in generic_tickers]
        result.holdings = enrich_holdings_with_details(basic_holdings)
    
    # Step 2: Get sector and region breakdown from JustETF
    if isin and isin != 'N/A':
        sectors, regions = get_sectors_and_regions_from_justetf(isin)
        result.sectors = sectors
        result.regions = regions
    
    return result

if __name__ == "__main__":
    start = time.time()
    etf_data = get_etf_data(
        ticker="VWCE.DE", 
        isin="IE00BK5BQT80", 
        etf_name="Vanguard FTSE All-World"
    )
    print(f"Time taken: {time.time() - start:.2f} seconds")
    print("Top Holdings:", etf_data.holdings)
    print("Sectors:", etf_data.sectors)
    print("Regions:", etf_data.regions)
    start = time.time()
    etf_data = get_etf_data(
        ticker="VWCE.DE", 
        isin="IE00BK5BQT80", 
        etf_name="Vanguard FTSE All-World"
    )
    print(f"Time taken (HOT): {time.time() - start:.2f} seconds")
    print("Top Holdings:", etf_data.holdings)
    print("Sectors:", etf_data.sectors)
    print("Regions:", etf_data.regions)