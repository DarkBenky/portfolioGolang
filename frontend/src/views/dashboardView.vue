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
      <v-col cols="12" sm="6" md="3">
        <div class="panel pa-4">
          <div class="metric-label">Portfolio value</div>
          <div class="metric-value">{{ money(value.current_value) }}</div>
          <div class="text-caption" :class="`text-${changeColor(value.total_change_percent)}`">
            {{ percent(value.total_change_percent) }} total
          </div>
        </div>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <div class="panel pa-4">
          <div class="metric-label">Today</div>
          <div class="metric-value" :class="`text-${changeColor(value.day_change)}`">{{ money(value.day_change) }}</div>
          <div class="text-caption" :class="`text-${changeColor(value.day_change_percent)}`">
            {{ percent(value.day_change_percent) }}
          </div>
        </div>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <div class="panel pa-4">
          <div class="metric-label">Invested</div>
          <div class="metric-value">{{ money(value.total_invested) }}</div>
          <div class="text-caption text-medium-emphasis">{{ holdings.length }} {{ holdings.length === 1 ? 'holding' : 'holdings' }}</div>
        </div>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <div class="panel pa-4">
          <div class="metric-label">News sentiment</div>
          <div class="metric-value" :class="`text-${sentimentColor}`">{{ sentimentLabel }}</div>
          <div class="text-caption text-medium-emphasis">{{ sentimentCount }} items today</div>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mt-1">
      <v-col cols="12" lg="8">
        <div class="panel pa-3">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="text-body-2 font-weight-medium">Portfolio value</span>
            <span class="text-caption text-medium-emphasis">{{ candles.length }} points</span>
          </div>
          <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
          <v-alert v-if="error" type="error" variant="tonal" density="compact">{{ error }}</v-alert>
          <candle-chart
            v-if="candles.length"
            :data="candles"
            :height="340"
            :show-volume="false"
            :price-decimals="2"
          />
          <v-alert v-else-if="!loading" type="info" variant="tonal" density="compact">
            No price history yet. Add a holding to start.
          </v-alert>
        </div>
      </v-col>

      <v-col cols="12" lg="4">
        <div class="panel pa-4 mb-3">
          <div class="metric-label mb-3">Movers today</div>
          <div v-for="mover in movers" :key="mover.ticker" class="d-flex justify-space-between py-1">
            <span class="text-body-2">{{ mover.ticker }}</span>
            <span class="text-body-2" :class="`text-${changeColor(mover.change)}`">{{ percent(mover.change) }}</span>
          </div>
          <div v-if="!movers.length" class="text-caption text-medium-emphasis">No intraday data.</div>
        </div>

        <div class="panel pa-4">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="metric-label">Latest news</span>
            <v-btn size="x-small" variant="text" @click="$emit('navigate', 'news')">all</v-btn>
          </div>
          <div v-for="item in news" :key="item.id_news" class="py-2 news-item">
            <a :href="item.link" target="_blank" rel="noopener" class="text-body-2 news-link">{{ item.title }}</a>
            <div class="text-caption text-medium-emphasis">
              {{ item.ticker }} · {{ relativeDate(item.published_at) }}
            </div>
          </div>
          <div v-if="!news.length" class="text-caption text-medium-emphasis">No news stored yet.</div>
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import CandleChart from '../components/candleChart.vue'
import { apiGet, changeColor, formatCurrency, formatPercent } from '../utils/api'

export default {
  name: 'DashboardView',
  components: { CandleChart },
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
      loading: false,
      error: '',
    }
  },
  computed: {
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
    async load() {
      this.loading = true
      this.error = ''
      const query = `?currency=${this.currency}`
      try {
        const [candles, value, holdings, sentiment, gainers, losers, news] = await Promise.all([
          apiGet(`/api/portfolio/history?interval=${this.interval}`),
          apiGet(`/api/portfolio/change${query}`),
          apiGet('/api/portfolio/holdings'),
          apiGet('/api/portfolio/sentiment'),
          apiGet('/api/portfolio/top_gainers'),
          apiGet('/api/portfolio/top_losers'),
          apiGet('/api/portfolio/news?limit=6'),
        ])
        this.candles = candles
        this.value = value
        this.holdings = holdings
        this.sentiment = sentiment.average_sentiment || 0
        this.sentimentCount = sentiment.news_count || 0
        this.gainers = gainers || []
        this.losers = losers || []
        this.news = news || []
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
</style>
