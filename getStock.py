import requests
from bs4 import BeautifulSoup
from dataclasses import dataclass
from typing import Optional

@dataclass
class StockMetrics:
    market_cap: Optional[str] = None
    market_cap_eur: Optional[str] = None
    country: Optional[str] = None
    sector: Optional[str] = None
    dividend_yield: Optional[float] = None
    eps: Optional[float] = None
    pb_ratio: Optional[float] = None
    pe_ratio: Optional[float] = None

@dataclass
class FinancialMetrics:
    revenue: Optional[str] = None
    net_income: Optional[str] = None
    profit_margin: Optional[float] = None

@dataclass
class StockData:
    metrics: StockMetrics
    financials: FinancialMetrics

def parse_value(value_str: str) -> tuple:
    """
    Parse a value string and return tuple (numeric_value, unit)
    Examples: "1,039,685 m" -> (1039685, "m"), "23.9" -> (23.9, ""), "1.23%" -> (1.23, "%")
    """
    value_str = value_str.strip()
    
    if '%' in value_str:
        return float(value_str.replace('%', '').strip()), '%'
    
    if value_str[-1].isalpha():
        parts = value_str.rsplit(' ', 1)
        if len(parts) == 2:
            num_str, unit = parts
            num_str = num_str.replace(',', '')
            try:
                return float(num_str), unit
            except ValueError:
                return None, None
    else:
        num_str = value_str.replace(',', '')
        try:
            return float(num_str), ''
        except ValueError:
            return None, None

def get_stock_data(isin: str) -> Optional[StockData]:
    try:
        url = f"https://www.justetf.com/en/stock-profiles/{isin}#overview"
        headers = {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }
        response = requests.get(url, headers=headers, timeout=10)
        response.raise_for_status()
        
        soup = BeautifulSoup(response.content, 'html.parser')
        
        stock_metrics = StockMetrics()
        financial_metrics = FinancialMetrics()
        
        data_overview = soup.find('div', class_='data-overview')
        if data_overview:
            overview_items = data_overview.find_all('div', class_='d-flex')
            
            for item in overview_items:
                label_elem = item.find('div', class_='vallabel')
                val_elem = item.find('div', class_='val')
                
                if label_elem and val_elem:
                    label = label_elem.get_text(strip=True).lower()
                    value = val_elem.get_text(strip=True)
                    
                    if 'market cap' in label and '(in EUR)' in label_elem.get_text():
                        stock_metrics.market_cap_eur = value
                    elif 'market cap' in label:
                        stock_metrics.market_cap = value
                    elif 'country' in label:
                        stock_metrics.country = value
                    elif 'sector' in label:
                        stock_metrics.sector = value
                    elif 'dividend yield' in label:
                        num_val, _ = parse_value(value)
                        stock_metrics.dividend_yield = num_val
        
        tables = soup.find_all('table', class_='table etf-data-table')
        
        if len(tables) >= 1:
            for table_idx, table in enumerate(tables[:2]):
                rows = table.find_all('tr')
                
                for row in rows:
                    cells = row.find_all('td')
                    if len(cells) != 2:
                        continue
                    
                    label = cells[0].get_text(strip=True).lower()
                    value = cells[1].get_text(strip=True)
                    
                    if 'market capitalisation' in label and not stock_metrics.market_cap:
                        stock_metrics.market_cap = value
                    elif 'eps' in label:
                        num_val, _ = parse_value(value)
                        stock_metrics.eps = num_val
                    elif 'p/b ratio' in label:
                        num_val, _ = parse_value(value)
                        stock_metrics.pb_ratio = num_val
                    elif 'p/e ratio' in label:
                        num_val, _ = parse_value(value)
                        stock_metrics.pe_ratio = num_val
                    elif 'dividend yield' in label and not stock_metrics.dividend_yield:
                        num_val, _ = parse_value(value)
                        stock_metrics.dividend_yield = num_val
                    elif 'revenue' in label:
                        financial_metrics.revenue = value
                    elif 'net income' in label:
                        financial_metrics.net_income = value
                    elif 'profit margin' in label:
                        num_val, _ = parse_value(value)
                        financial_metrics.profit_margin = num_val
        
        return StockData(metrics=stock_metrics, financials=financial_metrics)
    
    except requests.exceptions.RequestException as e:
        print(f"Error fetching data for {isin}: {e}")
        return None
    except Exception as e:
        print(f"Error parsing data for {isin}: {e}")
        return None

if __name__ == "__main__":
    isin = 'TW0002330008'
    stock_data = get_stock_data(isin)
    
    if stock_data:
        print("=" * 60)
        print("STOCK METRICS")
        print("=" * 60)
        print(f"Market Cap: {stock_data.metrics.market_cap}")
        print(f"Market Cap (EUR): {stock_data.metrics.market_cap_eur}")
        print(f"Country: {stock_data.metrics.country}")
        print(f"Sector: {stock_data.metrics.sector}")
        print(f"Dividend Yield: {stock_data.metrics.dividend_yield}%")
        print(f"EPS: {stock_data.metrics.eps}")
        print(f"P/B Ratio: {stock_data.metrics.pb_ratio}")
        print(f"P/E Ratio: {stock_data.metrics.pe_ratio}")
        
        print("\n" + "=" * 60)
        print("FINANCIAL METRICS")
        print("=" * 60)
        print(f"Revenue: {stock_data.financials.revenue}")
        print(f"Net Income: {stock_data.financials.net_income}")
        print(f"Profit Margin: {stock_data.financials.profit_margin}%")
        
        print("\n" + "=" * 60)
        print("RAW DATA STRUCTURE")
        print("=" * 60)
        print(stock_data)
    else:
        print("Failed to fetch stock data")
