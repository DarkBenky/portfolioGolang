<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-3 flex-wrap ga-2">
      <div>
        <div class="text-h6">Portfolio Overview</div>
        <div class="text-caption text-medium-emphasis">
          How the portfolio and your spending stand right now
        </div>
      </div>
      <v-btn-toggle v-model="interval" density="compact" variant="outlined" divided mandatory @update:model-value="loadChart">
        <v-btn v-for="option in intervals" :key="option" :value="option" size="x-small">{{ option }}</v-btn>
      </v-btn-toggle>
    </div>

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <div class="metric-grid mb-1">
      <div v-for="item in metrics" :key="item.label" class="panel pa-3 metric-cell">
        <div class="metric-label">{{ item.label }}</div>
        <div class="metric-value" :class="item.color">{{ item.value }}</div>
        <div class="text-caption" :class="item.hintColor || 'text-medium-emphasis'">{{ item.hint }}</div>
      </div>
    </div>
    <div class="d-flex flex-wrap ga-4 text-caption text-medium-emphasis mb-2 px-1">
      <span>Cost basis <span class="text-high-emphasis">{{ money(stats.total_cost) }}</span></span>
      <span>Positions <span class="text-high-emphasis">{{ holdings.length }}</span></span>
      <span>Volatility <span class="text-high-emphasis">{{ (risk.volatility || 0).toFixed(1) }}%</span></span>
      <span>Sharpe <span class="text-high-emphasis">{{ (risk.sharpe || 0).toFixed(2) }}</span></span>
      <span>
        vs {{ risk.benchmark || 'SPY' }}
        <span :class="`text-${changeColor(risk.benchmark_return)}`">{{ percent(risk.benchmark_return) }}</span>
      </span>
    </div>

    <div class="panel pa-3 mb-2">
      <div class="d-flex align-center justify-space-between mb-2 flex-wrap ga-2">
        <span class="text-body-2 font-weight-medium">Portfolio value</span>
        <div class="d-flex align-center ga-3">
          <span v-if="sentimentCount" class="text-caption" :class="`text-${sentimentColor}`">
            <v-icon size="14" class="mr-1">mdi-chart-timeline-variant</v-icon>
            Sentiment {{ sentiment.toFixed(2) }}
          </span>
          <span class="text-caption text-medium-emphasis">
            {{ candles.length }} points · break-even {{ money(value.total_invested) }}
          </span>
        </div>
      </div>
      <candle-chart
        v-if="candles.length"
        ref="portfolioChart"
        :data="candles"
        :height="chartHeight"
        :show-volume="false"
        :price-decimals="2"
        @chart-ready="applyBreakEven"
      />
      <div v-else-if="!loading" class="text-caption text-medium-emphasis py-10 text-center">
        No price history yet. Add a holding to start.
      </div>
    </div>

    <v-row dense class="mb-2">
      <v-col v-for="group in allocationGroups" :key="group.key" cols="12" md="4">
        <div class="panel pa-3 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">{{ group.title }}</span>
            <span class="text-caption text-medium-emphasis">{{ group.items.length }} groups</span>
          </div>
          <donut-chart
            v-if="group.items.length"
            :items="group.items"
            :size="120"
            :thickness="16"
            :legend-max-height="200"
            :center-label="group.short"
            :center-value="group.top.toFixed(0) + '%'"
          />
          <div v-else class="text-caption text-medium-emphasis py-6 text-center">No breakdown stored.</div>
          <v-btn size="x-small" variant="text" class="px-0 mt-1" @click="$emit('navigate', 'allocation')">
            Full allocation
          </v-btn>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mb-2">
      <v-col cols="12" md="4">
        <div class="panel pa-3 h-100">
          <div class="d-flex align-center mb-2">
            <v-icon size="16" color="success" class="mr-2">mdi-trending-up</v-icon>
            <span class="metric-label">Top gainers</span>
          </div>
          <div
            v-for="mover in movers.gainers"
            :key="'g-' + mover.ticker"
            class="mover-row"
            @click="$emit('open-ticker', mover.ticker)"
          >
            <div class="mover-name">
              <span class="ticker-link">{{ mover.ticker }}</span>
              <span class="text-caption text-medium-emphasis text-truncate">{{ mover.name }}</span>
            </div>
            <span v-if="mover.news" class="text-caption text-medium-emphasis text-no-wrap mr-2">
              <v-icon size="12">mdi-newspaper</v-icon> {{ mover.news }}
            </span>
            <span class="text-body-2 text-success text-no-wrap">{{ percent(mover.change) }}</span>
          </div>
          <div v-if="!movers.gainers.length" class="text-caption text-medium-emphasis py-4 text-center">
            No gainers in the last session.
          </div>
        </div>
      </v-col>

      <v-col cols="12" md="4">
        <div class="panel pa-3 h-100">
          <div class="d-flex align-center mb-2">
            <v-icon size="16" color="error" class="mr-2">mdi-trending-down</v-icon>
            <span class="metric-label">Top losers</span>
          </div>
          <div
            v-for="mover in movers.losers"
            :key="'l-' + mover.ticker"
            class="mover-row"
            @click="$emit('open-ticker', mover.ticker)"
          >
            <div class="mover-name">
              <span class="ticker-link">{{ mover.ticker }}</span>
              <span class="text-caption text-medium-emphasis text-truncate">{{ mover.name }}</span>
            </div>
            <span v-if="mover.news" class="text-caption text-medium-emphasis text-no-wrap mr-2">
              <v-icon size="12">mdi-newspaper</v-icon> {{ mover.news }}
            </span>
            <span class="text-body-2 text-error text-no-wrap">{{ percent(mover.change) }}</span>
          </div>
          <div v-if="!movers.losers.length" class="text-caption text-medium-emphasis py-4 text-center">
            No losers in the last session.
          </div>
        </div>
      </v-col>

      <v-col cols="12" md="4">
        <div class="panel pa-3 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Latest news</span>
            <v-btn size="x-small" variant="text" class="px-1" @click="$emit('navigate', 'news')">ALL</v-btn>
          </div>
          <div
            v-for="item in news"
            :key="item.id_news"
            class="mover-row news-row"
            @click="$emit('open-ticker', item.ticker)"
          >
            <div class="news-body">
              <div class="news-title">{{ item.title }}</div>
              <div class="text-caption text-medium-emphasis">
                {{ item.ticker }} · {{ ago(item.published_at) }}
              </div>
            </div>
          </div>
          <div v-if="!news.length" class="text-caption text-medium-emphasis py-4 text-center">
            No articles for your holdings yet.
          </div>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mb-2">
      <v-col cols="12" md="6">
        <div class="panel pa-4 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Portfolio sentiment</span>
            <v-btn
              size="x-small"
              variant="text"
              prepend-icon="mdi-magic"
              :loading="summaryGenerating"
              @click="generateSummary"
            >{{ summary ? 'Regenerate' : 'Generate' }}</v-btn>
          </div>
          <div class="d-flex align-center ga-2 mb-2">
            <v-chip size="small" variant="tonal" :color="sentimentColor">{{ sentimentLabel }}</v-chip>
            <span v-if="summary?.date" class="text-caption text-medium-emphasis">{{ summary.date }}</span>
          </div>
          <div v-if="summary?.summary" class="markdown-content summary-text" v-html="formatMarkdown(summary.summary)"></div>
          <div v-else class="text-caption text-medium-emphasis">
            No AI summary for today yet. Generate one to see the drivers behind the sentiment.
          </div>
        </div>
      </v-col>

      <v-col cols="12" md="6">
        <div class="panel pa-4 h-100">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Financial health</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'expenses')">expenses</v-btn>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Money in</span>
            <span class="text-body-2 text-success">{{ money(bank.total_in) }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Money out</span>
            <span class="text-body-2 text-error">{{ money(bank.total_out) }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Net</span>
            <span class="text-body-2" :class="`text-${changeColor(bank.net)}`">{{ money(bank.net) }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">{{ monthSpendLabel }}</span>
            <span class="text-body-2">{{ money(monthSpend) }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Savings roundups</span>
            <span class="text-body-2">{{ money(bank.total_savings) }}</span>
          </div>
          <div class="snapshot-row">
            <span class="text-caption text-medium-emphasis">Savings rate</span>
            <span class="text-body-2" :class="`text-${changeColor(savingsRate)}`">{{ savingsRate.toFixed(0) }}%</span>
          </div>
        </div>
      </v-col>
    </v-row>

    <div v-if="storage" class="panel pa-3 mb-2">
      <div class="d-flex align-center justify-space-between mb-2 flex-wrap ga-2">
        <span class="metric-label">Machine storage</span>
        <span class="text-caption" :class="storageTextColor">
          {{ bytes(storage.disk.free_bytes) }} free of {{ bytes(storage.disk.total_bytes) }}
        </span>
      </div>
      <v-progress-linear :model-value="storage.disk.used_percent" :color="storageBarColor" height="8" rounded />
      <div class="d-flex flex-wrap ga-4 mt-2 text-caption text-medium-emphasis">
        <span>{{ storage.directory }}</span>
        <span>used {{ (storage.disk.used_percent || 0).toFixed(1) }}%</span>
        <span>databases {{ bytes(storage.database_bytes) }}</span>
        <span v-for="file in storageFiles" :key="file.name">
          {{ file.name }} {{ bytes(file.bytes) }}
        </span>
        <span v-if="storage.disk.error" class="text-error">{{ storage.disk.error }}</span>
      </div>
    </div>

    <div class="panel pa-4">
      <div class="d-flex align-center justify-space-between mb-2">
        <span class="metric-label">Running summary</span>
        <v-btn size="x-small" variant="text" @click="$emit('navigate', 'summary')">open report</v-btn>
      </div>
      <div v-if="summary?.running" class="markdown-content summary-text" v-html="formatMarkdown(summary.running)"></div>
      <div v-else class="text-caption text-medium-emphasis">
        No running summary stored yet.
      </div>
    </div>
  </div>
</template>

<script>
import CandleChart from '../components/candleChart.vue'
import DonutChart from '../components/donutChart.vue'
import { apiGet, apiSend, changeColor, formatCurrency, formatPercent } from '../utils/api'
import { formatMarkdown } from '../utils/markdown'

const INTERVALS = ['5m', '15m', '1h', '4h', '1d', '1w', '1M']

export default {
  name: 'DashboardView',
  components: { CandleChart, DonutChart },
  props: {
    currency: { type: String, default: 'EUR' },
  },
  emits: ['navigate', 'open-ticker'],
  data() {
    return {
      interval: '1d',
      intervals: INTERVALS,
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
      stats: {},
      bank: {},
      storage: null,
      summary: null,
      loading: false,
      summaryGenerating: false,
      error: '',
      breakEvenLine: null,
    }
  },
  computed: {
    chartHeight() {
      return this.$vuetify.display.mobile ? 260 : 360
    },
    metrics() {
      const stats = this.stats
      const risk = this.risk
      return [
        { label: 'Total value', value: this.money(stats.total_value),
          hint: `${this.money(stats.total_gain_loss)} total` },
        { label: 'Gain', value: this.percent(stats.total_gain_loss_pct),
          color: `text-${changeColor(stats.total_gain_loss_pct)}`, hint: 'since purchase' },
        { label: 'YoY return', value: this.percent(stats.yoy_return),
          color: `text-${changeColor(stats.yoy_return)}`, hint: 'annual performance' },
        { label: 'Max drawdown', value: `${(stats.max_drawdown || 0).toFixed(1)}%`,
          hint: 'worst peak to trough' },
        { label: 'Sortino', value: (stats.sortino_ratio || 0).toFixed(2),
          hint: `sharpe ${(risk.sharpe || 0).toFixed(2)}` },
        { label: 'TER', value: `${((stats.aggregated_ter || 0) * 100).toFixed(2)}%`,
          hint: 'weighted fund cost' },
      ]
    },
    allocationGroups() {
      const allocation = this.allocation
      const group = (key, title, short, items) => ({
        key,
        title,
        short,
        items,
        top: items.reduce((best, item) => Math.max(best, Number(item.percentage) || 0), 0),
      })
      return [
        group('sectors', 'Sector allocation', 'Top sector', allocation.sectors || []),
        group('regions', 'Region allocation', 'Top region', allocation.regions || []),
        group('companies', 'Top companies', 'Top company', allocation.companies || []),
      ]
    },
    movers() {
      const asRow = (entry) => ({
        ticker: entry.holding?.ticker || '',
        name: entry.holding?.name || '',
        change: entry.price_change_pct || 0,
        news: (entry.related_news || []).length,
      })
      const gainers = (this.gainers || []).map(asRow).filter((row) => row.ticker && row.change > 0)
      const losers = (this.losers || []).map(asRow).filter((row) => row.ticker && row.change < 0)
      return { gainers, losers }
    },
    sentimentScore() {
      // The daily summary carries its own score; prefer it over the news counter,
      // which is zero on days without fresh articles.
      if (this.summary && typeof this.summary.sentiment === 'number') return this.summary.sentiment
      return this.sentiment
    },
    sentimentLabel() {
      if (!this.sentimentCount && !this.summary) return 'No data'
      if (this.sentimentScore > 0.2) return 'Bullish'
      if (this.sentimentScore < -0.2) return 'Bearish'
      return 'Neutral'
    },
    sentimentColor() {
      if (!this.sentimentCount && !this.summary) return 'grey'
      if (this.sentimentScore > 0.2) return 'success'
      if (this.sentimentScore < -0.2) return 'error'
      return 'grey'
    },
    monthSpend() {
      const months = this.bank.by_month || {}
      const keys = Object.keys(months).sort()
      if (!keys.length) return 0
      const current = new Date().toISOString().slice(0, 7)
      // Statements are imported at the turn of the month, so the current month is
      // usually still empty. Fall back to the newest imported month.
      const key = months[current] !== undefined ? current : keys[keys.length - 1]
      return Number(months[key]) || 0
    },
    monthSpendLabel() {
      const months = this.bank.by_month || {}
      const keys = Object.keys(months).sort()
      const current = new Date().toISOString().slice(0, 7)
      if (!keys.length || months[current] !== undefined) return 'Spent this month'
      const [year, month] = keys[keys.length - 1].split('-')
      const name = new Date(Number(year), Number(month) - 1, 1)
        .toLocaleString('en', { month: 'short' })
      return `Spent in ${name} ${year}`
    },
    savingsRate() {
      const income = Number(this.bank.total_in) || 0
      const out = Number(this.bank.total_out) || 0
      if (income <= 0) return 0
      return Math.max(-100, Math.min(100, (income - out) / income * 100))
    },
    storageFiles() {
      // The WAL and shared-memory files are noise in the card when empty.
      return (this.storage?.files || []).filter((file) => file.bytes > 0)
    },
    storageBarColor() {
      const used = this.storage?.disk?.used_percent || 0
      if (used >= 92) return 'error'
      if (used >= 80) return 'warning'
      return 'primary'
    },
    storageTextColor() {
      const used = this.storage?.disk?.used_percent || 0
      if (used >= 92) return 'text-error'
      if (used >= 80) return 'text-warning'
      return 'text-medium-emphasis'
    },
  },
  watch: {
    currency() {
      this.loadAll()
    },
  },
  mounted() {
    this.loadAll()
  },
  methods: {
    changeColor,
    formatMarkdown,
    money(value) {
      return formatCurrency(value, this.currency)
    },
    percent(value) {
      return formatPercent(value)
    },
    bytes(value) {
      const size = Number(value) || 0
      if (size < 1024) return `${size} B`
      const units = ['KB', 'MB', 'GB', 'TB']
      let scaled = size / 1024
      let unit = 0
      while (scaled >= 1024 && unit < units.length - 1) {
        scaled /= 1024
        unit++
      }
      return `${scaled.toFixed(scaled >= 100 ? 0 : 1)} ${units[unit]}`
    },
    ago(timestamp) {
      const seconds = parseInt(timestamp, 10)
      if (!seconds) return ''
      const minutes = Math.floor((Date.now() / 1000 - seconds) / 60)
      if (minutes < 60) return `${Math.max(minutes, 1)}m ago`
      const hours = Math.floor(minutes / 60)
      if (hours < 24) return `${hours}h ago`
      return `${Math.floor(hours / 24)}d ago`
    },
    applyBreakEven() {
      const series = this.$refs.portfolioChart?.getCandleSeries?.()
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
    async loadChart() {
      try {
        this.candles = await apiGet(`/api/portfolio/history?interval=${this.interval}`)
        this.$nextTick(this.applyBreakEven)
      } catch (err) {
        this.error = err.message
      }
    },
    async loadAll() {
      this.loading = true
      this.error = ''
      const query = `?currency=${this.currency}`
      try {
        const [value, holdings, sentiment, gainers, losers, allocation, risk, stats, news, bank,
          overview, storage] = await Promise.all([
          apiGet(`/api/portfolio/change${query}`),
          apiGet(`/api/portfolio/holdings${query}`),
          apiGet('/api/portfolio/sentiment'),
          apiGet('/api/portfolio/top_gainers'),
          apiGet('/api/portfolio/top_losers'),
          apiGet('/api/portfolio/allocation'),
          apiGet('/api/portfolio/risk?period=1y'),
          apiGet(`/api/portfolio/stats${query}`),
          apiGet('/api/portfolio/news?limit=5').catch(() => []),
          apiGet('/api/bank/stats').catch(() => ({})),
          apiGet('/api/portfolio/daily_sentiment?date=' + new Date().toISOString().slice(0, 10))
            .catch(() => null),
          apiGet('/api/system/storage').catch(() => null),
        ])
        this.value = value || {}
        this.holdings = holdings || []
        this.sentiment = sentiment.average_sentiment || 0
        this.sentimentCount = sentiment.news_count || 0
        this.gainers = gainers || []
        this.losers = losers || []
        this.allocation = allocation || { sectors: [], regions: [], companies: [] }
        this.risk = risk || {}
        this.stats = stats || {}
        this.news = news || []
        this.bank = bank || {}
        this.storage = storage || null
        this.summary = { ...(overview || {}), running: (await this.runningSummary()) }
        await this.loadChart()
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
    async runningSummary() {
      try {
        const data = await apiGet('/api/running-summary')
        return data?.summary || ''
      } catch {
        return ''
      }
    },
    async generateSummary() {
      this.summaryGenerating = true
      this.error = ''
      try {
        const result = await apiSend('/api/news/generate-summary', 'POST', {})
        this.summary = { ...(this.summary || {}), ...(result || {}) }
      } catch (err) {
        this.error = err.message
      } finally {
        this.summaryGenerating = false
      }
    },
  },
}
</script>

<style scoped>
.metric-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(6, minmax(0, 1fr));
}

.metric-cell {
  min-width: 0;
}

.metric-value {
  font-size: 20px;
  font-weight: 600;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mover-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  cursor: pointer;
}

.mover-row:last-child {
  border-bottom: none;
}

.mover-row:hover {
  background: rgba(255, 255, 255, 0.03);
}

.mover-name {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.news-row {
  align-items: flex-start;
}

.news-body {
  min-width: 0;
}

.news-title {
  font-size: 13px;
  line-height: 1.25;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
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
  max-height: 190px;
  overflow-y: auto;
}

@media (max-width: 900px) {
  .metric-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 600px) {
  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .metric-value {
    font-size: 17px;
  }
}
</style>
