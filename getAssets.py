from typing import List, Optional, Tuple, Dict, Any, Callable
import yfinance as yf
import requests
from bs4 import BeautifulSoup
import re
from functools import wraps
import time
from datetime import datetime, timedelta
import os


def ttl_cache(maxsize: int = 128, ttl_hours: int = 4):
    def decorator(func: Callable) -> Callable:
        cache: Dict[str, Tuple[Any, datetime]] = {}
        
        @wraps(func)
        def wrapper(*args, **kwargs):
            key = str(args) + str(kwargs)
            now = datetime.now()
            
            if key in cache:
                result, timestamp = cache[key]
                if now - timestamp < timedelta(hours=ttl_hours):
                    return result
                else:
                    del cache[key]
            
            result = func(*args, **kwargs)
            cache[key] = (result, now)
            
            if len(cache) > maxsize:
                oldest_key = min(cache.keys(), key=lambda k: cache[k][1])
                del cache[oldest_key]
            
            return result
        
        return wrapper
    return decorator

HoldingInfo = Tuple[str, str, str, str, str, str, float]

class ETFData:
    def __init__(self):
        self.holdings: List[HoldingInfo] = []
        self.sectors: Dict[str, float] = {}
        self.regions: Dict[str, float] = {}
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            'holdings': [
                {
                    'name': h[0],
                    'ticker': h[1],
                    'isin': h[2],
                    'exchange': h[3],
                    'sector': h[4],
                    'region': h[5],
                    'percentage': h[6]
                }
                for h in self.holdings
            ],
            'sectors': self.sectors,
            'regions': self.regions
        }

@ttl_cache(maxsize=2048, ttl_hours=4)
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

@ttl_cache(maxsize=2048, ttl_hours=4)
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

@ttl_cache(maxsize=2048, ttl_hours=24)
def get_ticker_from_isin(isin: str) -> Optional[str]:
    """Fetch ticker from ISIN using OpenFIGI API"""
    if not isin or isin == 'N/A':
        return None
    
    try:
        url = 'https://api.openfigi.com/v3/mapping'
        headers = {
            'Content-Type': 'application/json'
        }
        payload = [{"idType": "ID_ISIN", "idValue": isin}]
        
        response = requests.post(url, json=payload, headers=headers, timeout=5)
        
        if response.status_code == 200:
            data = response.json()
            if data and len(data) > 0 and 'data' in data[0]:
                results = data[0]['data']
                if results and len(results) > 0:
                    ticker = results[0].get('ticker')
                    if ticker:
                        return ticker
    except Exception as e:
        print(f"  OpenFIGI lookup failed for {isin}: {e}")
    
    return None

def enrich_holdings_with_details(holdings: List[Tuple[str, str, float]]) -> List[HoldingInfo]:
    """Enrich holdings with ticker and details - holdings format: (name, isin, percentage)"""
    enriched = []
    
    for name, isin, percentage in holdings:
        ticker = 'N/A'
        exchange = 'Unknown'
        sector = 'Unknown'
        region = 'Unknown'
        
        if isin and isin != 'N/A':
            ticker = get_ticker_from_isin(isin)
            
            if ticker and ticker != 'N/A':
                try:
                    details = get_ticker_details(ticker)
                    exchange = details.get('exchange', 'Unknown')
                    sector = details.get('sector', 'Unknown')
                    region = details.get('region', 'Unknown')
                except Exception as e:
                    print(f"  Failed to get details for {ticker}: {e}")
        
        enriched.append((
            name,
            ticker if ticker else 'N/A',
            isin,
            exchange,
            sector,
            region,
            percentage
        ))
    
    return enriched

@ttl_cache(maxsize=2048, ttl_hours=4)
def get_holdings_from_justetf(isin: str, html_file: Optional[str] = None) -> Optional[List[Tuple[str, str, float]]]:
    holdings = []
    
    try:
        if not isin or isin == 'N/A':
            return None
        
        if html_file and os.path.exists(html_file):
            print(f"  Loading from HTML file: {html_file}")
            with open(html_file, 'r', encoding='utf-8') as f:
                html_content = f.read()
            soup = BeautifulSoup(html_content, 'html.parser')
        else:
            url = f"https://www.justetf.com/en/etf-profile.html?isin={isin}"
            print(f"  Fetching live data from JustETF...")
            
            try:
                from playwright.sync_api import sync_playwright
                
                with sync_playwright() as p:
                    browser = p.chromium.launch(headless=True)
                    page = browser.new_page()
                    page.goto(url, wait_until="networkidle", timeout=30000)
                    page.wait_for_selector("table[data-testid='etf-holdings_top-holdings_table']", timeout=15000)
                    time.sleep(2)
                    
                    try:
                        holdings_btn = page.locator("a[data-testid='etf-holdings_top-holdings_load-more_link']")
                        if holdings_btn.is_visible(timeout=3000):
                            holdings_btn.scroll_into_view_if_needed()
                            time.sleep(0.5)
                            holdings_btn.click()
                            print("  Clicked 'Show more' for holdings")
                            time.sleep(2)
                    except:
                        pass
                    
                    html_content = page.content()
                    browser.close()
                
                soup = BeautifulSoup(html_content, 'html.parser')
                
            except Exception as e:
                print(f"  Playwright failed ({e}), falling back to requests")
                headers = {
                    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                }
                response = requests.get(url, headers=headers, timeout=15)
                soup = BeautifulSoup(response.content, 'html.parser')
        
        holdings_table = soup.find('table', {'data-testid': 'etf-holdings_top-holdings_table'})
        if holdings_table:
            tbody = holdings_table.find('tbody')
            if tbody:
                rows = tbody.find_all('tr', {'data-testid': 'etf-holdings_top-holdings_row'})
                
                for row in rows:
                    tds = row.find_all('td')
                    if len(tds) >= 2:
                        link = tds[0].find('a', {'data-testid': 'tl_etf-holdings_top-holdings_link_name'})
                        if link:
                            company_name = link.find('span').get_text(strip=True)
                            href = link.get('href', '')
                            
                            isin_match = re.search(r'/stock-profiles/([A-Z0-9]+)', href)
                            holding_isin = isin_match.group(1) if isin_match else 'N/A'
                            
                            pct_span = tds[1].find('span', {'data-testid': 'tl_etf-holdings_top-holdings_value_percentage'})
                            if pct_span:
                                pct_text = pct_span.get_text(strip=True)
                                pct_match = re.search(r'(\d+\.?\d*)', pct_text)
                                
                                if pct_match and company_name:
                                    percentage = float(pct_match.group(1))
                                    holdings.append((company_name, holding_isin, percentage))
        
        print(f"  Found {len(holdings)} holdings from JustETF")
        return holdings if holdings else None
        
    except Exception as e:
        print(f"JustETF holdings scraping failed: {e}")
        import traceback
        traceback.print_exc()
        return None

@ttl_cache(maxsize=2048, ttl_hours=4)
def get_sectors_and_regions_from_justetf(isin: str, html_file: Optional[str] = None) -> Tuple[Dict[str, float], Dict[str, float]]:
    sectors = {}
    regions = {}
    
    try:
        if not isin or isin == 'N/A':
            return sectors, regions
        
        if html_file and os.path.exists(html_file):
            print(f"  Loading from HTML file: {html_file}")
            with open(html_file, 'r', encoding='utf-8') as f:
                html_content = f.read()
            soup = BeautifulSoup(html_content, 'html.parser')
        else:
            url = f"https://www.justetf.com/en/etf-profile.html?isin={isin}"
            print(f"  Fetching live data from JustETF...")
            
            try:
                from playwright.sync_api import sync_playwright
                
                with sync_playwright() as p:
                    browser = p.chromium.launch(headless=True)
                    page = browser.new_page()
                    page.goto(url, wait_until="networkidle", timeout=30000)
                    page.wait_for_selector("table[data-testid='etf-holdings_countries_table']", timeout=15000)
                    time.sleep(2)
                    
                    try:
                        countries_btn = page.locator("a[data-testid='etf-holdings_countries_load-more_link']")
                        if countries_btn.is_visible(timeout=3000):
                            countries_btn.scroll_into_view_if_needed()
                            time.sleep(0.5)
                            countries_btn.click()
                            print("  Clicked 'Show more' for countries")
                            time.sleep(2)
                    except:
                        pass
                    
                    try:
                        sectors_btn = page.locator("a[data-testid='etf-holdings_sectors_load-more_link']")
                        if sectors_btn.is_visible(timeout=3000):
                            sectors_btn.scroll_into_view_if_needed()
                            time.sleep(0.5)
                            sectors_btn.click()
                            print("  Clicked 'Show more' for sectors")
                            time.sleep(2)
                    except:
                        pass
                    
                    html_content = page.content()
                    browser.close()
                
                soup = BeautifulSoup(html_content, 'html.parser')
                
            except Exception as e:
                print(f"  Playwright failed ({e}), falling back to requests")
                headers = {
                    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                }
                response = requests.get(url, headers=headers, timeout=15)
                soup = BeautifulSoup(response.content, 'html.parser')
        
        countries_table = soup.find('table', {'data-testid': 'etf-holdings_countries_table'})
        if countries_table:
            tbody = countries_table.find('tbody')
            if tbody:
                all_rows = tbody.find_all('tr', {'data-testid': 'etf-holdings_countries_row'})
                for row in all_rows:
                    country_td = row.find('td', {'data-testid': 'tl_etf-holdings_countries_value_name'})
                    if not country_td:
                        tds = row.find_all('td')
                        country_td = tds[0] if len(tds) >= 2 else None
                    
                    if country_td:
                        country_name = country_td.get_text(strip=True)
                        
                        pct_span = row.find('span', {'data-testid': 'tl_etf-holdings_countries_value_percentage'})
                        if pct_span:
                            pct_text = pct_span.get_text(strip=True)
                            pct_match = re.search(r'(\d+\.?\d*)', pct_text)
                            
                            if pct_match and country_name and country_name.lower() != 'other':
                                regions[country_name] = float(pct_match.group(1))
        
        sectors_table = soup.find('table', {'data-testid': 'etf-holdings_sectors_table'})
        if sectors_table:
            tbody = sectors_table.find('tbody')
            if tbody:
                all_rows = tbody.find_all('tr', {'data-testid': 'etf-holdings_sectors_row'})
                for row in all_rows:
                    sector_td = row.find('td', {'data-testid': 'tl_etf-holdings_sectors_value_name'})
                    if not sector_td:
                        tds = row.find_all('td')
                        sector_td = tds[0] if len(tds) >= 2 else None
                    
                    if sector_td:
                        sector_name = sector_td.get_text(strip=True)
                        
                        pct_span = row.find('span', {'data-testid': 'tl_etf-holdings_sectors_value_percentage'})
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

@ttl_cache(maxsize=2048, ttl_hours=4)
def fetch_complete_etf_data_playwright(isin: str) -> Tuple[Optional[List[Tuple[str, str, float]]], Dict[str, float], Dict[str, float]]:
    holdings = []
    sectors = {}
    regions = {}
    
    try:
        from playwright.sync_api import sync_playwright
        
        url = f"https://www.justetf.com/en/etf-profile.html?isin={isin}"
        print(f"  Fetching from JustETF with Playwright...")
        
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
            page = context.new_page()
            
            page.goto(url, wait_until="load", timeout=30000)
            
            try:
                page.wait_for_selector("#CybotCookiebotDialog", timeout=3000)
                page.evaluate("document.getElementById('CybotCookiebotDialog').remove()")
                print("  Removed cookie dialog")
            except:
                pass
            
            try:
                page.wait_for_selector("table[data-testid='etf-holdings_top-holdings_table']", timeout=10000)
            except Exception as e:
                print(f"  ETF data not available on JustETF for this ISIN")
                browser.close()
                return (None, {}, {})
            
            time.sleep(1)
            
            try:
                holdings_btn = page.locator("a[data-testid='etf-holdings_top-holdings_load-more_link']")
                if holdings_btn.count() > 0:
                    holdings_btn.click(timeout=5000)
                    print("  Clicked 'Show more' for holdings")
                    time.sleep(2)
            except Exception as e:
                pass
            
            try:
                countries_btn = page.locator("a[data-testid='etf-holdings_countries_load-more_link']")
                if countries_btn.count() > 0:
                    countries_btn.scroll_into_view_if_needed()
                    time.sleep(0.5)
                    countries_btn.click(timeout=5000)
                    print("  Clicked 'Show more' for countries")
                    time.sleep(2)
            except Exception as e:
                print(f"  Could not click countries button: {str(e)[:100]}")
            
            try:
                sectors_btn = page.locator("a[data-testid='etf-holdings_sectors_load-more_link']")
                if sectors_btn.count() > 0:
                    sectors_btn.scroll_into_view_if_needed()
                    time.sleep(0.5)
                    sectors_btn.click(timeout=5000)
                    print("  Clicked 'Show more' for sectors")
                    time.sleep(2)
            except Exception as e:
                print(f"  Could not click sectors button: {str(e)[:100]}")
            
            html_content = page.content()
            browser.close()
            
            soup = BeautifulSoup(html_content, 'html.parser')
            
            holdings_table = soup.find('table', {'data-testid': 'etf-holdings_top-holdings_table'})
            if holdings_table:
                tbody = holdings_table.find('tbody')
                if tbody:
                    rows = tbody.find_all('tr', {'data-testid': 'etf-holdings_top-holdings_row'})
                    for row in rows:
                        tds = row.find_all('td')
                        if len(tds) >= 2:
                            link = tds[0].find('a', {'data-testid': 'tl_etf-holdings_top-holdings_link_name'})
                            if link:
                                company_name = link.find('span').get_text(strip=True)
                                href = link.get('href', '')
                                isin_match = re.search(r'/stock-profiles/([A-Z0-9]+)', href)
                                holding_isin = isin_match.group(1) if isin_match else 'N/A'
                                pct_span = tds[1].find('span', {'data-testid': 'tl_etf-holdings_top-holdings_value_percentage'})
                                if pct_span:
                                    pct_text = pct_span.get_text(strip=True)
                                    pct_match = re.search(r'(\d+\.?\d*)', pct_text)
                                    if pct_match and company_name:
                                        holdings.append((company_name, holding_isin, float(pct_match.group(1))))
            
            countries_table = soup.find('table', {'data-testid': 'etf-holdings_countries_table'})
            if countries_table:
                tbody = countries_table.find('tbody')
                if tbody:
                    for row in tbody.find_all('tr', {'data-testid': 'etf-holdings_countries_row'}):
                        country_td = row.find('td', {'data-testid': 'tl_etf-holdings_countries_value_name'})
                        if country_td:
                            country_name = country_td.get_text(strip=True)
                            pct_span = row.find('span', {'data-testid': 'tl_etf-holdings_countries_value_percentage'})
                            if pct_span:
                                pct_match = re.search(r'(\d+\.?\d*)', pct_span.get_text(strip=True))
                                if pct_match and country_name.lower() != 'other':
                                    regions[country_name] = float(pct_match.group(1))
            
            sectors_table = soup.find('table', {'data-testid': 'etf-holdings_sectors_table'})
            if sectors_table:
                tbody = sectors_table.find('tbody')
                if tbody:
                    for row in tbody.find_all('tr', {'data-testid': 'etf-holdings_sectors_row'}):
                        sector_td = row.find('td', {'data-testid': 'tl_etf-holdings_sectors_value_name'})
                        if sector_td:
                            sector_name = sector_td.get_text(strip=True)
                            pct_span = row.find('span', {'data-testid': 'tl_etf-holdings_sectors_value_percentage'})
                            if pct_span:
                                pct_match = re.search(r'(\d+\.?\d*)', pct_span.get_text(strip=True))
                                if pct_match and sector_name.lower() != 'other':
                                    sectors[sector_name] = float(pct_match.group(1))
    
    except Exception as e:
        print(f"  Playwright failed: {e}")
        import traceback
        traceback.print_exc()
    
    return (holdings if holdings else None, sectors, regions)

@ttl_cache(maxsize=2048, ttl_hours=4)
def get_etf_data(ticker: str, isin: str = None, etf_name: str = None, html_file: Optional[str] = None) -> ETFData:
    result = ETFData()
    
    print(f"Fetching ETF data for {ticker} (ISIN: {isin})...")
    
    if not isin or isin == 'N/A':
        print(f"  No ISIN provided, skipping ETF data fetch")
        return result
    
    if html_file and os.path.exists(html_file):
        print(f"  Loading from HTML file: {html_file}")
        justetf_holdings = get_holdings_from_justetf(isin, html_file)
        sectors, regions = get_sectors_and_regions_from_justetf(isin, html_file)
    else:
        justetf_holdings, sectors, regions = fetch_complete_etf_data_playwright(isin)
    
    if justetf_holdings and len(justetf_holdings) > 0:
        result.holdings = enrich_holdings_with_details(justetf_holdings)
        print(f"  Total: {len(result.holdings)} holdings")
    else:
        print(f"  No holdings data found (ETF may not be available on JustETF)")
    
    result.sectors = sectors
    result.regions = regions
    print(f"  Total: {len(sectors)} sectors and {len(regions)} regions")
    
    return result

if __name__ == "__main__":
    start = time.time()
    
    etf_data = get_etf_data(
        ticker="XMME.DE", 
        isin="IE00BTJRMP35",
        etf_name="Xtrackers MSCI Emerging Markets"
    )
    print(f"\nTime taken: {time.time() - start:.2f} seconds")
    print(f"\nAll Holdings ({len(etf_data.holdings)} total):")
    for i, h in enumerate(etf_data.holdings, 1):
        print(f"  {i}, {h}")
    print(f"\nAll Sectors ({len(etf_data.sectors)} total):")
    for sector, pct in sorted(etf_data.sectors.items(), key=lambda x: x[1], reverse=True):
        print(f"  {sector}: {pct}%")
    print(f"\nAll Regions ({len(etf_data.regions)} total):")
    for region, pct in sorted(etf_data.regions.items(), key=lambda x: x[1], reverse=True):
        print(f"  {region}: {pct}%")
    
    print("\n" + "="*50)
    print("Testing cache...")
    start = time.time()
    etf_data = get_etf_data(
        ticker="XMME.DE", 
        isin="IE00BTJRMP35",
        etf_name="Xtrackers MSCI Emerging Markets"
    )
    print(f"Time taken (CACHED): {time.time() - start:.2f} seconds")