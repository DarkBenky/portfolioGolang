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

        <v-container v-if="activeView === 'dashboard'" fluid>
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
        </v-container>

        <v-container v-else fluid>
          <v-card class="pa-4">
            <h3>{{ activeView.charAt(0).toUpperCase() + activeView.slice(1) }} View</h3>
            <p>This section is under construction.</p>
            <btn>Add holding</btn>

            search bar (isin ticker)
            sort by

            stats
            total assets
            total value
            total P/L ( $ and % )
            daily P/L ( $ and % )

            <list>
              <li v-for="i in 5" :key="i">
                <p>Todays price chart small</p>
                <p>Ticker: XXX</p>
                <p>Current Price: $123.45</p>
                <p>Day Chang pct: 1.45%</p>
                <p>Day Change: $1.75</p>
                <p>Total P/L pct: 12.37%</p>
                <p>Total P/L: $15.30</p>
              </li>

              drop down
              chart
              recent news + todays sentiment analysis
              allocation if etf int sectors and regions

              details
              name, avg purches price, quannity, ticker, isin, exchange, ter, currency, policy

              list of top 10 holdings with small charts (chart, ticker name, weight pct, current price, day change pct, total pl pct)


            </list>
          </v-card>
        </v-container>
      </v-main>
    </template>
  </v-app>
</template>

<script setup>
import { ref, onMounted, watch, onUnmounted } from 'vue'
import LoginRegister from './components/loginRegister.vue'
import CandleChart from './components/candleChart.vue'

const isAuthenticated = ref(false)
const userEmail = ref('')

// Portfolio chart data
const portfolioData = ref([])
const portfolioLoading = ref(false)
const portfolioError = ref('')
const selectedPeriod = ref('1m')
const chartWidth = ref(800)

// Sidebar state
const drawer = ref(true)
const activeView = ref('dashboard')

// Cookie utilities
function getCookie(name) {
  const nameEQ = name + '='
  const cookies = document.cookie.split(';')
  for (let cookie of cookies) {
    cookie = cookie.trim()
    if (cookie.indexOf(nameEQ) === 0) {
      return cookie.substring(nameEQ.length)
    }
  }
  return null
}

function deleteCookie(name) {
  document.cookie = `${name}=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/;`
}

// Fetch portfolio value history
async function fetchPortfolioHistory() {
  const token = getCookie('auth_token')
  if (!token) return

  portfolioLoading.value = true
  portfolioError.value = ''

  try {
    // Determine interval based on period
    let interval = '1h'
    if (selectedPeriod.value === '1d') interval = '5m'
    else if (selectedPeriod.value === '1w') interval = '15m'
    else if (selectedPeriod.value === '1m') interval = '1h'
    else if (selectedPeriod.value === '3m') interval = '1d'
    else if (selectedPeriod.value === '1y') interval = '1d'

    const response = await fetch(
      `http://localhost:8085/api/portfolio/history?period=${selectedPeriod.value}&interval=${interval}`,
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
    portfolioData.value = data.map(item => ({
      timestamp: item.timestamp,
      open: item.open,
      high: item.high,
      low: item.low,
      close: item.close,
      volume: 0
    }))
  } catch (error) {
    console.error('Error fetching portfolio history:', error)
    portfolioError.value = error.message || 'Failed to load portfolio data'
  } finally {
    portfolioLoading.value = false
  }
}

// Update chart width on resize
function updateChartWidth() {
  const container = document.querySelector('.chart-wrapper')
  if (container) {
    chartWidth.value = container.clientWidth - 20
  }
}

function logout() {
  deleteCookie('auth_token')
  deleteCookie('user_email')
  isAuthenticated.value = false
  userEmail.value = ''
}

// Watch for period changes
watch(selectedPeriod, () => {
  fetchPortfolioHistory()
})

// Watch for authentication changes
watch(isAuthenticated, (newVal) => {
  if (newVal) {
    setTimeout(() => {
      updateChartWidth()
      fetchPortfolioHistory()
    }, 100)
  }
})

onMounted(() => {
  const token = getCookie('auth_token')
  const email = getCookie('user_email')
  if (token) {
    isAuthenticated.value = true
    userEmail.value = email || ''
    setTimeout(() => {
      updateChartWidth()
      fetchPortfolioHistory()
    }, 100)
  }
  
  window.addEventListener('resize', updateChartWidth)
})

onUnmounted(() => {
  window.removeEventListener('resize', updateChartWidth)
})
</script>

<style scoped>
.chart-wrapper {
  width: 100%;
  overflow-x: auto;
}
</style>
