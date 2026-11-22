<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Dashboard</div>
        <div class="text-caption text-medium-emphasis">Live portfolio overview</div>
      </div>
      <v-btn-toggle v-model="interval" density="compact" variant="outlined" divided mandatory @update:model-value="load">
        <v-btn v-for="option in intervals" :key="option" :value="option" size="small">{{ option }}</v-btn>
      </v-btn-toggle>
    </div>

    <v-row dense>
      <v-col v-for="metric in metrics" :key="metric.label" cols="6" md="3">
        <div class="panel pa-3 pa-md-4">
          <div class="metric-label">{{ metric.label }}</div>
          <div class="metric-value" :class="metric.color">{{ metric.value }}</div>
          <div class="text-caption" :class="metric.hintColor || 'text-medium-emphasis'">{{ metric.hint }}</div>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mt-1">
      <v-col cols="12" lg="8">
        <div class="panel pa-3">
          <div class="d-flex align-center justify-space-between mb-2 flex-wrap">
            <span class="text-body-2 font-weight-medium">Portfolio value</span>
            <span class="text-caption text-medium-emphasis">
              {{ candles.length }} points · break-even {{ money(value.total_invested) }}
            </span>
          </div>
          <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
          <v-alert v-if="error" type="error" variant="tonal" density="compact">{{ error }}</v-alert>
          <candle-chart
            v-if="candles.length"
            ref="portfolioChart"
            :data="candles"
            :height="340"
            :show-volume="false"
            :price-decimals="2"
            @chart-ready="applyBreakEven"
          />
          <v-alert v-else-if="!loading" type="info" variant="tonal" density="compact">
            No price history yet. Add a holding to start.
          </v-alert>
        </div>
      </v-col>

      <v-col cols="12" lg="4">
        <div class="panel pa-4 mb-3">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Movers today</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'statistics')">stats</v-btn>
          </div>
          <div v-for="mover in movers" :key="mover.ticker" class="mover-row">
            <span class="text-body-2 text-truncate mr-2">{{ mover.ticker }}</span>
            <v-progress-linear
              :model-value="Math.min(Math.abs(mover.change) * 8, 100)"
              :color="mover.change >= 0 ? 'success' : 'error'"
              height="3"
              rounded
              class="flex-grow-1 mr-2"
            />
            <span class="text-body-2 text-no-wrap" :class="`text-${changeColor(mover.change)}`">
              {{ percent(mover.change) }}
            </span>
          </div>
          <div v-if="!movers.length" class="text-caption text-medium-emphasis">No intraday data.</div>
        </div>

        <div class="panel pa-4">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Latest news</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'news')">all</v-btn>
          </div>
          <div v-for="item in news" :key="item.id_news" class="py-2 news-item">
            <a :href="item.link" target="_blank" rel="noopener noreferrer" class="text-body-2 news-link">{{ item.title }}</a>
            <div class="text-caption text-medium-emphasis">
              {{ item.ticker }} · {{ relativeDate(item.published_at) }}
            </div>
          </div>
          <div v-if="!news.length" class="text-caption text-medium-emphasis">No news stored yet.</div>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mt-1">
      <v-col cols="12" md="6" lg="3">
        <div class="panel pa-4 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Sectors</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'allocation')">details</v-btn>
          </div>
          <donut-chart
            :items="allocation.sectors || []"
            :size="128"
            :thickness="17"
            :max-slices="6"
            :center-label="topSectorName"
            :center-value="topSectorValue"
          />
        </div>
      </v-col>

      <v-col cols="12" md="6" lg="3">
        <div class="panel pa-4 h-100">
          <div class="metric-label mb-2">Risk snapshot</div>
          <div class="snapshot-row" v-for="row in riskRows" :key="row.label">
            <span class="text-caption text-medium-emphasis">{{ row.label }}</span>
            <span class="text-body-2" :class="row.color">{{ row.value }}</span>
          </div>
          <v-btn size="x-small" variant="text" class="mt-2 px-0" @click="$emit('navigate', 'statistics')">
            Open statistics
          </v-btn>
        </div>
      </v-col>

      <v-col cols="12" md="6" lg="3">
        <div class="panel pa-4 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Allocation</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'holdings')">holdings</v-btn>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Positions</span>
            <span class="text-body-2">{{ holdings.length }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Largest position</span>
            <span class="text-body-2">{{ largestHolding }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Top company</span>
            <span class="text-body-2">{{ topCompany }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Top region</span>
            <span class="text-body-2">{{ topRegion }}</span>
          </div>
        </div>
      </v-col>

      <v-col cols="12" md="6" lg="3">
        <div class="panel pa-4 h-100 d-flex flex-column">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Running summary</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'summary')">open</v-btn>
          </div>
          <div v-if="summary" class="text-body-2 summary-text">{{ summary }}</div>
          <div v-else class="text-caption text-medium-emphasis">No summary stored yet.</div>
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import CandleChart from '../components/candleChart.vue'
import DonutChart from '../components/donutChart.vue'
import { apiGet, changeColor, formatCurrency, formatPercent } from '../utils/api'

export default {
  name: 'DashboardView',
  components: { CandleChart, DonutChart },
  props: {
    currency: { type: String, default: 'EUR' },
  },
  emits: ['navigate'],
  data() {
    return {
      interval: '1d',
      intervals: ['1h', '1d', '1w', '1M'],
      candles: [],
      value: {},
      holdings: [],
      sentiment: 0,
      sentimentCount: 0,
      gainers: [],
      losers: [],
      news: [],
      allocation: { sectors: [], regions: [], companies: [] },
      risk: {},
      summary: '',
      loading: false,
      error: '',
      breakEvenLine: null,
    }
  },
  computed: {
    metrics() {
      return [
        { label: 'Portfolio value', value: this.money(this.value.current_value),
          hint: `${this.percent(this.value.total_change_percent)} total`,
          hintColor: `text-${this.changeColor(this.value.total_change_percent)}` },
        { label: 'Today', value: this.money(this.value.day_change),
          color: `text-${this.changeColor(this.value.day_change)}`,
          hint: this.percent(this.value.day_change_percent),
          hintColor: `text-${this.changeColor(this.value.day_change_percent)}` },
        { label: 'Invested', value: this.money(this.value.total_invested),
          hint: `${this.holdings.length} ${this.holdings.length === 1 ? 'holding' : 'holdings'}` },
        { label: 'News sentiment', value: this.sentimentLabel,
          color: `text-${this.sentimentColor}`, hint: `${this.sentimentCount} items today` },
      ]
    },
    riskRows() {
      const risk = this.risk
      if (!risk.observations) {
        return [{ label: 'Status', value: 'not enough data', color: 'text-medium-emphasis' }]
      }
      return [
        { label: '1y return', value: this.percent(risk.total_return), color: `text-${this.changeColor(risk.total_return)}` },
        { label: 'Volatility', value: `${(risk.volatility || 0).toFixed(1)}%` },
        { label: 'Sharpe', value: (risk.sharpe || 0).toFixed(2) },
        { label: 'Max drawdown', value: `${(risk.max_drawdown || 0).toFixed(1)}%`, color: 'text-error' },
        { label: `vs ${risk.benchmark || 'SPY'}`, value: this.percent(risk.benchmark_return), color: `text-${this.changeColor(risk.benchmark_return)}` },
      ]
    },
    sentimentLabel() {
      if (!this.sentimentCount) return 'No data'
      if (this.sentiment > 0.2) return 'Bullish'
      if (this.sentiment < -0.2) return 'Bearish'
      return 'Neutral'
    },
    sentimentColor() {
      if (this.sentiment > 0.2) return 'success'
      if (this.sentiment < -0.2) return 'error'
      return 'medium-emphasis'
    },
    movers() {
      return [...this.gainers, ...this.losers]
        .filter((item) => item.holding)
        .map((item) => ({ ticker: item.holding.ticker, change: item.price_change_pct }))
        .sort((a, b) => Math.abs(b.change) - Math.abs(a.change))
        .slice(0, 6)
    },
    topSectorName() {
      return this.allocation.sectors?.[0]?.name?.split(' ')[0] || '-'
    },
    topSectorValue() {
      const top = this.allocation.sectors?.[0]
      return top ? `${top.percentage.toFixed(0)}%` : '-'
    },
    largestHolding() {
      const sorted = [...this.holdings].sort((a, b) => (b.weight || 0) - (a.weight || 0))
      return sorted[0] ? `${sorted[0].ticker} ${(sorted[0].weight || 0).toFixed(0)}%` : '-'
    },
    topCompany() {
      const top = this.allocation.companies?.[0]
      return top ? `${top.name} ${top.percentage.toFixed(1)}%` : '-'
    },
    topRegion() {
      const top = this.allocation.regions?.[0]
      return top ? `${top.name} ${top.percentage.toFixed(1)}%` : '-'
    },
  },
  watch: {
    currency() {
      this.load()
    },
  },
  mounted() {
    this.load()
  },
  methods: {
    changeColor,
    money(value) {
      return formatCurrency(value, this.currency)
    },
    percent(value) {
      return formatPercent(value)
    },
    relativeDate(unix) {
      const timestamp = Number(unix)
      if (!timestamp) return ''
      const hours = Math.round((Date.now() / 1000 - timestamp) / 3600)
      if (hours < 1) return 'just now'
      if (hours < 24) return `${hours}h ago`
      return `${Math.round(hours / 24)}d ago`
    },
    applyBreakEven() {
      const chart = this.$refs.portfolioChart
      const series = chart?.getCandleSeries?.()
      if (!series) return
      if (this.breakEvenLine) {
        series.removePriceLine(this.breakEvenLine)
        this.breakEvenLine = null
      }
      if (this.value.total_invested > 0) {
        this.breakEvenLine = series.createPriceLine({
          price: this.value.total_invested,
          color: '#9e9e9e',
          lineWidth: 1,
          lineStyle: 2,
          axisLabelVisible: true,
          title: 'Break-even',
        })
      }
    },
    async load() {
      this.loading = true
      this.error = ''
      const query = `?currency=${this.currency}`
      try {
        const [candles, value, holdings, sentiment, gainers, losers, news, allocation, risk, summary] = await Promise.all([
          apiGet(`/api/portfolio/history?interval=${this.interval}`),
          apiGet(`/api/portfolio/change${query}`),
          apiGet(`/api/portfolio/holdings${query}`),
          apiGet('/api/portfolio/sentiment'),
          apiGet('/api/portfolio/top_gainers'),
          apiGet('/api/portfolio/top_losers'),
          apiGet('/api/portfolio/news?limit=6'),
          apiGet('/api/portfolio/allocation'),
          apiGet('/api/portfolio/risk?period=1y'),
          apiGet('/api/running-summary').catch(() => null),
        ])
        this.candles = candles
        this.value = value
        this.holdings = holdings || []
        this.sentiment = sentiment.average_sentiment || 0
        this.sentimentCount = sentiment.news_count || 0
        this.gainers = gainers || []
        this.losers = losers || []
        this.news = news || []
        this.allocation = allocation || { sectors: [], regions: [], companies: [] }
        this.risk = risk || {}
        this.summary = summary?.summary || ''
        this.$nextTick(this.applyBreakEven)
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
  },
}
</script>

<style scoped>
.news-item {
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.news-item:last-child {
  border-bottom: none;
}

.news-link {
  color: rgba(255, 255, 255, 0.88);
  text-decoration: none;
}

.news-link:hover {
  color: rgb(var(--v-theme-primary));
}

.mover-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
}

.snapshot-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 5px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.snapshot-row:last-child {
  border-bottom: none;
}

.summary-text {
  white-space: pre-wrap;
  font-size: 12px;
  line-height: 1.45;
  max-height: 150px;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.8);
}
</style>
