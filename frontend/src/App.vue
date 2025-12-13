<template>
  <v-app>
    <!-- Show login/register if not authenticated -->
    <LoginRegister v-if="!isAuthenticated" />

    <!-- Show main app if authenticated -->
    <template v-else>
      <v-app-bar color="primary" dark>
        <v-app-bar-title>Portfolio Tracker</v-app-bar-title>
        <v-spacer></v-spacer>
        <span class="mr-4">{{ userEmail }}</span>
        <v-btn icon @click="logout">
          <v-icon>mdi-logout</v-icon>
        </v-btn>
      </v-app-bar>

      <v-main>
        <!-- Navigation Drawer / Sidebar -->
        <v-navigation-drawer v-model="drawer" permanent>
          <v-list nav>
            <!-- Add Holding Button -->
            <v-list-item
              prepend-icon="mdi-plus-circle"
              title="Add Holding"
              :class="{ 'text-primary': showAddHolding }"
              @click="showAddHolding = !showAddHolding"
            >
              <template v-slot:append>
                <v-icon :class="{ 'text-primary': showAddHolding }">
                  {{ showAddHolding ? 'mdi-chevron-up' : 'mdi-chevron-down' }}
                </v-icon>
              </template>
            </v-list-item>

            <!-- Add Holding Expandable Form -->
            <v-expand-transition>
              <div v-if="showAddHolding" class="add-holding-form pa-3">
                <v-text-field
                  v-model="searchQuery"
                  label="Search Ticker or ISIN"
                  placeholder="e.g., AAPL or US0378331005"
                  variant="outlined"
                  density="compact"
                  hide-details
                  clearable
                  prepend-inner-icon="mdi-magnify"
                  @input="debouncedSearch"
                  @click:clear="clearSearch"
                  class="mb-2"
                ></v-text-field>

                <!-- Search Results -->
                <v-list v-if="searchResults.length > 0" density="compact" class="search-results mb-2">
                  <v-list-item
                    v-for="result in searchResults"
                    :key="result.ticker"
                    @click="selectSearchResult(result)"
                    :class="{ 'bg-primary-lighten-4': selectedResult?.ticker === result.ticker }"
                  >
                    <v-list-item-title class="text-caption font-weight-bold">
                      {{ result.ticker }}
                    </v-list-item-title>
                    <v-list-item-subtitle class="text-caption">
                      {{ result.name }}
                    </v-list-item-subtitle>
                    <template v-slot:append>
                      <div class="text-right">
                        <div class="text-caption font-weight-medium text-success">
                          {{ result.currency }} {{ result.price }}
                        </div>
                        <v-chip size="x-small" :color="result.type === 'ETF' ? 'blue' : 'grey'" class="mt-1">
                          {{ result.type }}
                        </v-chip>
                      </div>
                    </template>
                  </v-list-item>
                </v-list>

                <v-progress-linear v-if="searchLoading" indeterminate color="primary" class="mb-2"></v-progress-linear>

                <!-- Selected Asset Details -->
                <template v-if="selectedResult">
                  <v-divider class="my-2"></v-divider>
                  <div class="text-caption text-grey mb-2">Selected: <strong>{{ selectedResult.name }}</strong></div>
                  
                  <v-text-field
                    v-model.number="newHolding.quantity"
                    label="Number of Shares"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hide-details
                    min="0"
                    step="0.01"
                    class="mb-2"
                  ></v-text-field>

                  <v-text-field
                    v-model.number="newHolding.purchasePrice"
                    label="Average Purchase Price"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hide-details
                    min="0"
                    step="0.01"
                    :suffix="selectedResult.currency"
                    class="mb-2"
                  ></v-text-field>

                  <!-- Auto-filled fields (collapsible) -->
                  <v-expansion-panels variant="accordion" class="mb-2">
                    <v-expansion-panel>
                      <v-expansion-panel-title class="text-caption py-1">
                        Additional Details
                      </v-expansion-panel-title>
                      <v-expansion-panel-text>
                        <v-text-field
                          v-model="newHolding.name"
                          label="Name"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.ticker"
                          label="Ticker"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.isin"
                          label="ISIN"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.exchange"
                          label="Exchange"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.currency"
                          label="Currency"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.ter"
                          label="TER (%)"
                          type="number"
                          variant="outlined"
                          density="compact"
                          hide-details
                          step="0.01"
                          class="mb-2"
                        ></v-text-field>
                        <v-select
                          v-model="newHolding.policy"
                          label="Distribution Policy"
                          :items="['Accumulating', 'Distributing', 'N/A']"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-select>
                        <v-checkbox
                          v-model="newHolding.etf"
                          label="Is ETF"
                          density="compact"
                          hide-details
                        ></v-checkbox>
                      </v-expansion-panel-text>
                    </v-expansion-panel>
                  </v-expansion-panels>

                  <v-btn
                    color="primary"
                    block
                    :loading="addHoldingLoading"
                    :disabled="!canAddHolding"
                    @click="addHolding"
                  >
                    <v-icon left>mdi-plus</v-icon>
                    Add to Portfolio
                  </v-btn>

                  <v-alert
                    v-if="addHoldingError"
                    type="error"
                    density="compact"
                    class="mt-2"
                  >
                    {{ addHoldingError }}
                  </v-alert>

                  <v-alert
                    v-if="addHoldingSuccess"
                    type="success"
                    density="compact"
                    class="mt-2"
                  >
                    Holding added successfully!
                  </v-alert>
                </template>
              </div>
            </v-expand-transition>

            <v-divider class="my-2"></v-divider>

            <v-list-item
              prepend-icon="mdi-view-dashboard"
              title="Dashboard"
              value="dashboard"
              :active="activeView === 'dashboard'"
              @click="activeView = 'dashboard'"
            />
            <v-list-item
              prepend-icon="mdi-wallet"
              title="Holdings"
              value="holdings"
              :active="activeView === 'holdings'"
              @click="activeView = 'holdings'"
            />
            <v-list-item
              prepend-icon="mdi-newspaper"
              title="News"
              value="news"
              :active="activeView === 'news'"
              @click="activeView = 'news'"
            />
            <v-list-item
              prepend-icon="mdi-chart-pie"
              title="Allocation"
              value="allocation"
              :active="activeView === 'allocation'"
              @click="activeView = 'allocation'"
            />
            <v-list-item
              prepend-icon="mdi-chart-line"
              title="Statistics"
              value="statistics"
              :active="activeView === 'statistics'"
              @click="activeView = 'statistics'"
            />
          </v-list>
        </v-navigation-drawer>

        <v-container v-if="activeView === 'holdings'" fluid>
          <v-card>
            <v-card-title class="d-flex align-center">
              <span>My Holdings</span>
              <v-spacer></v-spacer>
              <v-text-field
                v-model="holdingsSearch"
                prepend-inner-icon="mdi-magnify"
                label="Search holdings..."
                single-line
                hide-details
                density="compact"
                variant="outlined"
                style="max-width: 250px;"
                class="mr-2"
              ></v-text-field>
              <v-btn-toggle v-model="holdingsSortBy" mandatory density="compact" color="primary">
                <v-btn value="ticker" size="small">Ticker</v-btn>
                <v-btn value="value" size="small">Value</v-btn>
                <v-btn value="change" size="small">Day %</v-btn>
              </v-btn-toggle>
            </v-card-title>

            <v-card-text class="pa-0">
              <div v-if="!portfolioHoldings || portfolioHoldings.length === 0" class="text-center py-8 text-grey">
                <v-icon size="64" color="grey-lighten-1">mdi-wallet-outline</v-icon>
                <p class="mt-4">No holdings yet. Add your first holding using the sidebar.</p>
              </div>
              
              <v-table v-else fixed-header density="comfortable" class="holdings-table">
                <thead>
                  <tr>
                    <th style="width: 90px;">Chart</th>
                    <th style="min-width: 180px;">Ticker</th>
                    <th style="width: 100px;" class="text-right">Price</th>
                    <th style="width: 100px;" class="text-right">Day %</th>
                    <th style="width: 100px;" class="text-right">Day $</th>
                    <th style="width: 100px;" class="text-right">Total %</th>
                    <th style="width: 110px;" class="text-right">Total $</th>
                    <th style="width: 120px;" class="text-right">Value</th>
                    <th style="width: 50px;"></th>
                  </tr>
                </thead>
                <HoldingView
                  v-for="holding in filteredHoldings"
                  :key="holding.id_holding"
                  :holding="holding"
                  :auth-token="getCookie('auth_token')"
                />
              </v-table>
            </v-card-text>
          </v-card>
        </v-container>

        <v-container v-else-if="activeView === 'dashboard'" fluid>
          <!-- Portfolio Value Chart -->
          <v-card class="mb-4">
            <v-card-title class="d-flex align-center">
              <span>Portfolio Value</span>
              <v-spacer></v-spacer>
              <v-btn-toggle v-model="selectedPeriod" mandatory density="compact" color="primary">
                <v-btn value="1d" size="small">1D</v-btn>
                <v-btn value="1w" size="small">1W</v-btn>
                <v-btn value="1m" size="small">1M</v-btn>
                <v-btn value="3m" size="small">3M</v-btn>
                <v-btn value="1y" size="small">1Y</v-btn>
              </v-btn-toggle>
            </v-card-title>
            <v-card-text>
              <div v-if="portfolioLoading" class="d-flex justify-center py-8">
                <v-progress-circular indeterminate color="primary"></v-progress-circular>
              </div>
              <div v-else-if="portfolioError" class="text-center py-8 text-error">
                {{ portfolioError }}
              </div>
              <div v-else-if="portfolioData.length > 0" class="chart-wrapper">
                <CandleChart
                  :data="portfolioData"
                  :height="400"
                  :price-decimals="2"
                  :show-volume="false"
                  bull-color="#26a79a"
                  bear-color="#ef5250"
                />
              </div>
              <div v-else class="text-center py-8 text-grey">
                No portfolio data available
              </div>
            </v-card-text>
          </v-card>

          <!-- Portfolio Allocation -->
          <v-row>
            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-domain</v-icon>
                  Sector Allocation
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in sectorPieSlices"
                          :key="'sector-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'sector' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'sector', index }; scrollLegendIntoView('sector', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(sector, index) in sortedSectors" 
                        :key="sector.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'sector' && hoveredSlice?.index === index }"
                        :data-type="'sector'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'sector', index }; scrollLegendIntoView('sector', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors[index % pieColors.length] }"></span>
                        <span class="legend-text">{{ sector.name }}</span>
                        <span class="legend-value">{{ sector.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-earth</v-icon>
                  Region Allocation
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in regionPieSlices"
                          :key="'region-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'region' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'region', index }; scrollLegendIntoView('region', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(region, index) in sortedRegions" 
                        :key="region.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'region' && hoveredSlice?.index === index }"
                        :data-type="'region'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'region', index }; scrollLegendIntoView('region', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors2[index % pieColors2.length] }"></span>
                        <span class="legend-text">{{ region.name }}</span>
                        <span class="legend-value">{{ region.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-chart-pie</v-icon>
                  Top Companies
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in companyPieSlices"
                          :key="'company-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'company' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'company', index }; scrollLegendIntoView('company', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(company, index) in sortedCompanies" 
                        :key="company.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'company' && hoveredSlice?.index === index }"
                        :data-type="'company'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'company', index }; scrollLegendIntoView('company', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors[index % pieColors.length] }"></span>
                        <span class="legend-text">{{ company.name }}</span>
                        <span class="legend-value">{{ company.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>
        </v-container>

        <v-container v-else-if="activeView === 'news'" fluid>
          <v-card class="pa-4">
            <h3>News View</h3>
            <p>This section is under construction.</p>
          </v-card>
        </v-container>

        <v-container v-else-if="activeView === 'allocation'" fluid>
          <v-card class="pa-4">
            <h3>Allocation View</h3>
            <p>This section is under construction.</p>
          </v-card>
        </v-container>

        <v-container v-else-if="activeView === 'statistics'" fluid>
          <v-card class="pa-4">
            <h3>Statistics View</h3>
            <p>This section is under construction.</p>
          </v-card>
        </v-container>
      </v-main>
    </template>
  </v-app>
</template>

<script>
import LoginRegister from './components/loginRegister.vue'
import CandleChart from './components/candleChart.vue'
import HoldingView from './components/holdingView.vue'

export default {
  name: 'App',

  components: {
    LoginRegister,
    CandleChart,
    HoldingView
  },

  data() {
    return {
      // Authentication
      portfolioHoldings: null,
      isAuthenticated: false,
      userEmail: '',

      // Portfolio chart data
      portfolioData: [],
      portfolioLoading: false,
      portfolioError: '',
      selectedPeriod: '1m',
      chartWidth: 800,

      // Sidebar state
      drawer: true,
      activeView: 'dashboard',

      // Add Holding state
      showAddHolding: false,
      searchQuery: '',
      searchResults: [],
      searchLoading: false,
      selectedResult: null,
      searchTimeout: null,
      addHoldingLoading: false,
      addHoldingError: '',
      addHoldingSuccess: false,
      portfolioAllocations: {},
      hoveredSlice: null,
      pieColors: ['#2196F3', '#4CAF50', '#FF9800', '#E91E63', '#9C27B0', '#00BCD4', '#795548', '#607D8B', '#FF5722', '#3F51B5'],
      pieColors2: ['#3F51B5', '#8BC34A', '#FFC107', '#F44336', '#673AB7', '#009688', '#FF5722', '#03A9F4', '#CDDC39', '#E91E63'],
      newHolding: {
        name: '',
        ticker: '',
        isin: '',
        exchange: '',
        currency: 'USD',
        quantity: null,
        purchasePrice: null,
        ter: 0,
        policy: 'N/A',
        etf: false
      },

      // Holdings view state
      holdingsSearch: '',
      holdingsSortBy: 'ticker'
    }
  },

  computed: {
    canAddHolding() {
      return this.selectedResult && 
             this.newHolding.quantity > 0 && 
             this.newHolding.purchasePrice > 0 &&
             this.newHolding.ticker
    },

    sortedSectors() {
      if (!this.portfolioAllocations.sectors) return []
      return [...this.portfolioAllocations.sectors].sort((a, b) => b.percentage - a.percentage)
    },

    sortedRegions() {
      if (!this.portfolioAllocations.regions) return []
      return [...this.portfolioAllocations.regions].sort((a, b) => b.percentage - a.percentage)
    },

    sortedCompanies() {
      if (!this.portfolioAllocations.companies) return []
      return [...this.portfolioAllocations.companies].sort((a, b) => b.percentage - a.percentage)
    },

    sectorPieSlices() {
      return this.generatePieSlices(this.sortedSectors, this.pieColors)
    },

    regionPieSlices() {
      return this.generatePieSlices(this.sortedRegions, this.pieColors2)
    },

    companyPieSlices() {
      return this.generatePieSlices(this.sortedCompanies, this.pieColors)
    },

    filteredHoldings() {
      if (!this.portfolioHoldings) return []
      
      let holdings = [...this.portfolioHoldings]
      
      // Filter by search
      if (this.holdingsSearch) {
        const search = this.holdingsSearch.toLowerCase()
        holdings = holdings.filter(h => 
          h.ticker.toLowerCase().includes(search) ||
          h.name.toLowerCase().includes(search) ||
          (h.isin && h.isin.toLowerCase().includes(search))
        )
      }
      
      // Sort
      holdings.sort((a, b) => {
        if (this.holdingsSortBy === 'ticker') {
          return a.ticker.localeCompare(b.ticker)
        } else if (this.holdingsSortBy === 'value') {
          const valueA = a.quantity * a.purchase_price
          const valueB = b.quantity * b.purchase_price
          return valueB - valueA
        }
        // For 'change', we'd need real-time data which is fetched in each row
        return 0
      })
      
      return holdings
    }
  },

  watch: {
    selectedPeriod() {
      this.fetchPortfolioHistory()
    },

    isAuthenticated(newVal) {
      if (newVal) {
        setTimeout(() => {
          this.updateChartWidth()
          this.fetchPortfolioHistory()
        }, 100)
      }
    }
  },

  mounted() {
    const token = this.getCookie('auth_token')
    const email = this.getCookie('user_email')
    
    if (token) {
      this.isAuthenticated = true
      this.userEmail = email || ''
      setTimeout(() => {
        this.updateChartWidth()
        this.fetchPortfolioHistory()
        this.fetchPortfolioHoldings()
        this.getPortfolioAllocation()
      }, 100)
    }

    window.addEventListener('resize', this.updateChartWidth)
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.updateChartWidth)
  },

  methods: {
    async getPortfolioAllocation() {
      try {
        const token = this.getCookie('auth_token')
        if (!token) return []

        const response = await fetch('http://localhost:8085/api/portfolio/allocation', {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio allocation')
        }
        this.portfolioAllocations = await response.json()
        console.log('Portfolio Allocations:', this.portfolioAllocations)
      } catch (error) {
        console.error('Error fetching portfolio allocation:', error)
      }
    },

    // Cookie utilities
    getCookie(name) {
      const nameEQ = name + '='
      const cookies = document.cookie.split(';')
      for (let cookie of cookies) {
        cookie = cookie.trim()
        if (cookie.indexOf(nameEQ) === 0) {
          return cookie.substring(nameEQ.length)
        }
      }
      return null
    },

    deleteCookie(name) {
      document.cookie = `${name}=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/;`
    },

    // Search functionality
    debouncedSearch() {
      if (this.searchTimeout) {
        clearTimeout(this.searchTimeout)
      }
      this.searchTimeout = setTimeout(() => {
        this.searchTicker()
      }, 400)
    },

    async searchTicker() {
      if (!this.searchQuery || this.searchQuery.length < 2) {
        this.searchResults = []
        return
      }

      this.searchLoading = true
      try {
        // Determine search type (ISIN if starts with 2 letters followed by numbers)
        const isIsin = /^[A-Z]{2}[A-Z0-9]{9,10}$/i.test(this.searchQuery)
        const searchType = isIsin ? 'isin' : 'ticker'
        
        const response = await fetch(
          `http://localhost:5123/api/search?identifier=${encodeURIComponent(this.searchQuery)}&search_type=${searchType}`
        )
        
        if (response.ok) {
          this.searchResults = await response.json()
        } else {
          this.searchResults = []
        }
      } catch (error) {
        console.error('Search error:', error)
        this.searchResults = []
      } finally {
        this.searchLoading = false
      }
    },

    selectSearchResult(result) {
      this.selectedResult = result
      this.searchResults = []
      
      // Auto-fill the form
      this.newHolding.name = result.name || ''
      this.newHolding.ticker = result.ticker || ''
      this.newHolding.isin = result.isin || ''
      this.newHolding.exchange = result.exchange || ''
      this.newHolding.currency = result.currency || 'USD'
      this.newHolding.etf = result.type === 'ETF'
      
      // Parse TER if available
      if (result.ter) {
        const terValue = parseFloat(result.ter.replace('%', ''))
        this.newHolding.ter = isNaN(terValue) ? 0 : terValue
      } else {
        this.newHolding.ter = 0
      }
      
      this.newHolding.policy = result.distribution_policy || 'N/A'
      
      // Set current price as default purchase price
      if (result.price && result.price !== 'N/A') {
        this.newHolding.purchasePrice = parseFloat(result.price)
      }
      
      this.addHoldingError = ''
      this.addHoldingSuccess = false
    },

    clearSearch() {
      this.searchQuery = ''
      this.searchResults = []
      this.selectedResult = null
      this.resetNewHolding()
    },

    resetNewHolding() {
      this.newHolding = {
        name: '',
        ticker: '',
        isin: '',
        exchange: '',
        currency: 'USD',
        quantity: null,
        purchasePrice: null,
        ter: 0,
        policy: 'N/A',
        etf: false
      }
      this.addHoldingError = ''
      this.addHoldingSuccess = false
    },

    async addHolding() {
      const token = this.getCookie('auth_token')
      if (!token) {
        this.addHoldingError = 'Not authenticated'
        return
      }

      this.addHoldingLoading = true
      this.addHoldingError = ''
      this.addHoldingSuccess = false

      try {
        const formData = new FormData()
        formData.append('Name', this.newHolding.name)
        formData.append('Ticker', this.newHolding.ticker)
        formData.append('ISIN', this.newHolding.isin)
        formData.append('Exchange', this.newHolding.exchange)
        formData.append('Currency', this.newHolding.currency)
        formData.append('Quantity', this.newHolding.quantity.toString())
        formData.append('PurchasePrice', this.newHolding.purchasePrice.toString())
        formData.append('TER', this.newHolding.ter.toString())
        formData.append('Policy', this.newHolding.policy)
        formData.append('ETF', this.newHolding.etf ? 'true' : 'false')

        const response = await fetch('http://localhost:8085/api/asset/holdings', {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`
          },
          body: formData
        })

        if (response.ok) {
          this.addHoldingSuccess = true
          this.clearSearch()
          // Refresh holdings
          this.fetchPortfolioHoldings()
          this.fetchPortfolioHistory()
          
          // Auto-hide success message after 3 seconds
          setTimeout(() => {
            this.addHoldingSuccess = false
          }, 3000)
        } else {
          const errorText = await response.text()
          this.addHoldingError = errorText || 'Failed to add holding'
        }
      } catch (error) {
        console.error('Add holding error:', error)
        this.addHoldingError = error.message || 'Failed to add holding'
      } finally {
        this.addHoldingLoading = false
      }
    },

    // Fetch portfolio value history
    async fetchPortfolioHistory() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.portfolioLoading = true
      this.portfolioError = ''

      try {
        // Determine interval based on period
        let interval = '1h'
        if (this.selectedPeriod === '1d') interval = '5m'
        else if (this.selectedPeriod === '1w') interval = '5m'
        else if (this.selectedPeriod === '1m') interval = '15m'
        else if (this.selectedPeriod === '3m') interval = '1h'
        else if (this.selectedPeriod === '1y') interval = '1d'

        const response = await fetch(
          `http://localhost:8085/api/portfolio/history?period=${this.selectedPeriod}&interval=${interval}`,
          {
            headers: {
              'Authorization': `Bearer ${token}`
            }
          }
        )

        if (!response.ok) {
          throw new Error('Failed to fetch portfolio history')
        }

        const data = await response.json()

        // Transform data for candle chart
        // The API returns { timestamp, open, high, low, close, value }
        this.portfolioData = data.map(item => ({
          timestamp: item.timestamp,
          open: item.open,
          high: item.high,
          low: item.low,
          close: item.close,
          volume: 0
        }))
      } catch (error) {
        console.error('Error fetching portfolio history:', error)
        this.portfolioError = error.message || 'Failed to load portfolio data'
      } finally {
        this.portfolioLoading = false
      }
    },

    async fetchPortfolioHoldings() {
      const token = this.getCookie('auth_token')
      if (!token) return

      try {
        const url = `http://localhost:8085/api/portfolio/holdings`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio holdings')
        }
        const data = await response.json()
        this.portfolioHoldings = data
        console.log('Portfolio Holdings:', data)
      } catch (error) {
        console.error('Error fetching portfolio holdings:', error)
      }
    },

    // Update chart width on resize
    updateChartWidth() {
      const container = document.querySelector('.chart-wrapper')
      if (container) {
        this.chartWidth = container.clientWidth - 20
      }
    },

    logout() {
      this.deleteCookie('auth_token')
      this.deleteCookie('user_email')
      this.isAuthenticated = false
      this.userEmail = ''
    },

    openHoldingDetail(holding) {
      console.log('Opening holding detail:', holding)
    },

    scrollLegendIntoView(type, index) {
      this.$nextTick(() => {
        const legendItem = document.querySelector(`.legend-item[data-type="${type}"][data-index="${index}"]`)
        if (legendItem) {
          legendItem.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
        }
      })
    },

    generatePieSlices(items, colors) {
      if (!items || items.length === 0) return []
      
      const total = items.reduce((sum, item) => sum + item.percentage, 0)
      if (total === 0) return []
      
      const slices = []
      let currentAngle = -90

      items.forEach((item, index) => {
        const percentage = (item.percentage / total) * 100
        const angle = (percentage / 100) * 360
        const startAngle = currentAngle
        const endAngle = currentAngle + angle

        const startRad = (startAngle * Math.PI) / 180
        const endRad = (endAngle * Math.PI) / 180

        const radius = 80
        const x1 = Math.cos(startRad) * radius
        const y1 = Math.sin(startRad) * radius
        const x2 = Math.cos(endRad) * radius
        const y2 = Math.sin(endRad) * radius

        const largeArc = angle > 180 ? 1 : 0

        const path = `M 0 0 L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${x2} ${y2} Z`

        slices.push({
          path,
          color: colors[index % colors.length],
          percentage: item.percentage,
          name: item.name
        })

        currentAngle = endAngle
      })

      return slices
    }
  }
}
</script>

<style scoped>
.chart-wrapper {
  width: 100%;
}

.add-holding-form {
  background-color: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 8px;
  margin: 0 8px;
}

.search-results {
  max-height: 200px;
  overflow-y: auto;
  background-color: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), 0.12);
  border-radius: 4px;
}

.search-results .v-list-item {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
  cursor: pointer;
}

.search-results .v-list-item:hover {
  background-color: rgba(var(--v-theme-primary), 0.08);
}

.search-results .v-list-item:last-child {
  border-bottom: none;
}

.text-primary {
  color: rgb(var(--v-theme-primary)) !important;
}

.bg-primary-lighten-4 {
  background-color: rgba(var(--v-theme-primary), 0.15) !important;
}

/* Holdings table styling */
.holdings-table {
  width: 100%;
}

.holdings-table :deep(thead th) {
  font-size: 0.75rem !important;
  font-weight: 600 !important;
  text-transform: uppercase !important;
  letter-spacing: 0.5px !important;
  color: rgba(var(--v-theme-on-surface), 0.6) !important;
  background-color: rgba(var(--v-theme-surface-variant), 0.4) !important;
  padding: 12px 8px !important;
  white-space: nowrap;
}

.holdings-table :deep(table) {
  table-layout: fixed;
  width: 100%;
}

.pie-chart-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  height: 380px;
}

.pie-chart {
  width: 160px;
  height: 160px;
  flex-shrink: 0;
}

.pie-slice {
  cursor: pointer;
  transition: opacity 0.2s ease, transform 0.2s ease, filter 0.2s ease;
  transform-origin: center;
  filter: brightness(1);
}

.pie-slice:hover,
.pie-slice-active {
  opacity: 0.9;
  transform: scale(1.05);
  filter: brightness(1.1);
}

.pie-legend {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-height: 180px;
  overflow-y: auto;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  transition: all 0.2s ease;
  cursor: pointer;
  border: 1px solid transparent;
}

.legend-item:hover {
  background-color: rgba(var(--v-theme-primary), 0.08);
  border-color: rgba(var(--v-theme-primary), 0.3);
  transform: translateX(2px);
}

.legend-item-active {
  background-color: rgba(var(--v-theme-primary), 0.15);
  border-color: rgba(var(--v-theme-primary), 0.4);
  transform: translateX(2px);
}

.legend-color {
  width: 12px;
  height: 12px;
  border-radius: 2px;
  flex-shrink: 0;
}

.legend-text {
  flex: 1;
  font-size: 0.75rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.legend-value {
  font-size: 0.8rem;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.7);
}
</style>
