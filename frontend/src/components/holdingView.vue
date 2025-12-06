<template>
  <tr class="holding-row" @click="opened = !opened">
    <!-- Mini Chart -->
    <td class="chart-cell">
      <div class="mini-chart-container">
        <svg :width="chartWidth" :height="chartHeight" class="mini-chart">
          <polyline
            :points="chartPoints"
            fill="none"
            :stroke="isPositiveDay ? '#26a79a' : '#ef5250'"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </div>
    </td>

    <!-- Ticker -->
    <td class="ticker-cell">
      <div class="d-flex align-center">
        <div>
          <div class="font-weight-bold">{{ holding.ticker }}</div>
          <div class="text-caption text-grey text-truncate" style="max-width: 120px;">
            {{ holding.name }}
          </div>
        </div>
        <v-chip v-if="holding.etf" size="x-small" color="blue" class="ml-2">ETF</v-chip>
      </div>
    </td>

    <!-- Current Price -->
    <td class="text-right">
      <div class="font-weight-medium">
        {{ formatCurrency(currentPrice, holding.currency) }}
      </div>
    </td>

    <!-- Day Change % -->
    <td class="text-right">
      <v-chip
        :color="isPositiveDay ? 'success' : 'error'"
        size="small"
        variant="tonal"
      >
        <v-icon size="x-small" class="mr-1">
          {{ isPositiveDay ? 'mdi-arrow-up' : 'mdi-arrow-down' }}
        </v-icon>
        {{ formatPercent(dayChangePercent) }}
      </v-chip>
    </td>

    <!-- Day Change $ -->
    <td class="text-right">
      <span :class="isPositiveDay ? 'text-success' : 'text-error'">
        {{ isPositiveDay ? '+' : '' }}{{ formatCurrency(dayChange, holding.currency) }}
      </span>
    </td>

    <!-- Total Change % -->
    <td class="text-right">
      <v-chip
        :color="isPositiveTotal ? 'success' : 'error'"
        size="small"
        variant="tonal"
      >
        <v-icon size="x-small" class="mr-1">
          {{ isPositiveTotal ? 'mdi-arrow-up' : 'mdi-arrow-down' }}
        </v-icon>
        {{ formatPercent(totalChangePercent) }}
      </v-chip>
    </td>

    <!-- Total Change $ -->
    <td class="text-right">
      <span :class="isPositiveTotal ? 'text-success' : 'text-error'" class="font-weight-medium">
        {{ isPositiveTotal ? '+' : '' }}{{ formatCurrency(totalChange, holding.currency) }}
      </span>
    </td>

    <!-- Current Value -->
    <td class="text-right">
      <div class="font-weight-bold">
        {{ formatCurrency(currentValue, holding.currency) }}
      </div>
      <div class="text-caption text-grey">
        {{ holding.quantity }} shares
      </div>
    </td>

    <!-- detail view -->
        <!-- price chart -->
        <!-- news list -->
        <!-- asset details -->
    <!-- detail view -->
  </tr>
</template>

<script>
export default {
  name: 'HoldingView',

  props: {
    holding: {
      type: Object,
      required: true
    },
    authToken: {
      type: String,
      required: true
    }
  },

  emits: ['click'],

  data() {
    return {
      opened : false,
      priceHistory: [],
      currentPrice: 0,
      dayChange: 0,
      dayChangePercent: 0,
      loading: false,
      chartWidth: 80,
      chartHeight: 30
    }
  },

  computed: {
    // Calculate total change since purchase
    currentValue() {
      return this.currentPrice * this.holding.quantity
    },

    costBasis() {
      return this.holding.purchase_price * this.holding.quantity
    },

    totalChange() {
      return this.currentValue - this.costBasis
    },

    totalChangePercent() {
      if (this.costBasis === 0) return 0
      return (this.totalChange / this.costBasis) * 100
    },

    isPositiveDay() {
      return this.dayChangePercent >= 0
    },

    isPositiveTotal() {
      return this.totalChangePercent >= 0
    },

    // Generate SVG points for mini chart
    chartPoints() {
      if (this.priceHistory.length < 2) {
        return `0,${this.chartHeight / 2} ${this.chartWidth},${this.chartHeight / 2}`
      }

      const prices = this.priceHistory.map(p => p.close)
      const minPrice = Math.min(...prices)
      const maxPrice = Math.max(...prices)
      const priceRange = maxPrice - minPrice || 1

      const padding = 2
      const effectiveHeight = this.chartHeight - padding * 2
      const effectiveWidth = this.chartWidth - padding * 2

      const points = prices.map((price, index) => {
        const x = padding + (index / (prices.length - 1)) * effectiveWidth
        const y = padding + effectiveHeight - ((price - minPrice) / priceRange) * effectiveHeight
        return `${x},${y}`
      })

      return points.join(' ')
    }
  },

  mounted() {
    this.fetchData()
  },

  methods: {
    async fetchData() {
      this.loading = true
      try {
        await Promise.all([
          this.fetchPriceHistory(),
          this.fetchAssetChange()
        ])
      } catch (error) {
        console.error('Error fetching holding data:', error)
      } finally {
        this.loading = false
      }
    },

    async fetchPriceHistory() {
      try {
        const response = await fetch(
          `http://localhost:8085/api/asset/history?ticker=${encodeURIComponent(this.holding.ticker)}&period=1d&interval=5m`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.priceHistory = data || []
          
          // Get current price from latest data point
          if (this.priceHistory.length > 0) {
            this.currentPrice = this.priceHistory[this.priceHistory.length - 1].close
          }
        }
      } catch (error) {
        console.error('Error fetching price history:', error)
      }
    },

    async fetchAssetChange() {
      try {
        const response = await fetch(
          `http://localhost:8085/api/asset/change?ticker=${encodeURIComponent(this.holding.ticker)}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.dayChange = data.day_change || 0
          this.dayChangePercent = data.day_change_percent || 0
          
          // Update current price from value if we have quantity
          if (this.holding.quantity > 0 && data.current_value) {
            this.currentPrice = data.current_value / this.holding.quantity
          }
        }
      } catch (error) {
        console.error('Error fetching asset change:', error)
      }
    },

    formatCurrency(value, currency = 'USD') {
      if (value === null || value === undefined || isNaN(value)) {
        return '-'
      }
      
      const currencySymbols = {
        'USD': '$',
        'EUR': '€',
        'GBP': '£',
        'CHF': 'CHF ',
        'JPY': '¥'
      }
      
      const symbol = currencySymbols[currency] || currency + ' '
      const absValue = Math.abs(value)
      
      if (absValue >= 1000000) {
        return symbol + (value / 1000000).toFixed(2) + 'M'
      } else if (absValue >= 1000) {
        return symbol + (value / 1000).toFixed(2) + 'K'
      } else {
        return symbol + value.toFixed(2)
      }
    },

    formatPercent(value) {
      if (value === null || value === undefined || isNaN(value)) {
        return '-'
      }
      return Math.abs(value).toFixed(2) + '%'
    }
  }
}
</script>

<style scoped>
.holding-row {
  cursor: pointer;
  transition: background-color 0.2s;
}

.holding-row:hover {
  background-color: rgba(var(--v-theme-primary), 0.05);
}

.chart-cell {
  width: 90px;
  padding: 8px !important;
}

.mini-chart-container {
  display: flex;
  align-items: center;
  justify-content: center;
}

.mini-chart {
  display: block;
}

.ticker-cell {
  min-width: 150px;
}

.text-truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

td {
  padding: 12px 16px !important;
  vertical-align: middle !important;
}
</style>
