<template>
  <div>
    <div class="mb-4">
      <div class="text-h6">Statistics</div>
      <div class="text-caption text-medium-emphasis">Risk adjusted metrics and benchmark comparison</div>
    </div>

    <v-row dense class="mb-1">
      <v-col v-for="metric in metrics" :key="metric.label" cols="6" md="2">
        <div class="panel pa-4">
          <div class="metric-label">{{ metric.label }}</div>
          <div class="metric-value" :class="metric.color">{{ metric.value }}</div>
        </div>
      </v-col>
    </v-row>

    <div class="panel pa-4 mt-2">
      <div class="d-flex align-center flex-wrap ga-2 mb-3">
        <v-text-field v-model="startDate" type="date" density="compact" variant="outlined" hide-details label="Start"></v-text-field>
        <v-text-field v-model="endDate" type="date" density="compact" variant="outlined" hide-details label="End"></v-text-field>
        <v-text-field v-model="benchmark" density="compact" variant="outlined" hide-details label="Benchmark" style="max-width: 120px"></v-text-field>
        <v-btn size="small" variant="flat" color="primary" :loading="loading" @click="runBacktest">Run</v-btn>
      </div>

      <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

      <v-row v-if="backtest" dense class="mb-2">
        <v-col v-for="entry in backtestMetrics" :key="entry.label" cols="6" md="3">
          <div class="text-caption text-medium-emphasis">{{ entry.label }}</div>
          <div class="text-body-1 font-weight-medium">{{ entry.value }}</div>
        </v-col>
      </v-row>

      <backtest-chart
        v-if="backtest && backtest.timestamps && backtest.timestamps.length"
        :portfolio-data="backtest.portfolio_values"
        :benchmark-data="backtest.benchmark_values"
        :timestamps="backtest.timestamps"
        :benchmark-name="benchmark"
        :height="320"
      />
    </div>

    <div v-if="Object.keys(holdingReturns).length" class="panel pa-4 mt-3">
      <div class="metric-label mb-3">Per holding</div>
      <div v-for="row in holdingReturns" :key="row.ticker" class="d-flex align-center py-2 holding-row">
        <span class="text-body-2" style="width: 90px">{{ row.ticker }}</span>
        <span class="text-body-2 flex-grow-1 text-medium-emphasis">{{ row.signal }}</span>
        <span class="text-body-2" :class="`text-${changeColor(row.total_return)}`">{{ percent(row.total_return) }}</span>
      </div>
    </div>
  </div>
</template>

<script>
import BacktestChart from '../components/backtestChart.vue'
import { apiGet, changeColor, formatCurrency, formatPercent } from '../utils/api'

export default {
  name: 'StatisticsView',
  components: { BacktestChart },
  props: {
    currency: { type: String, default: 'EUR' },
  },
  data() {
    const now = new Date()
    const yearAgo = new Date(now.getTime() - 365 * 24 * 3600 * 1000)
    return {
      stats: {},
      backtest: null,
      benchmark: 'SPY',
      startDate: yearAgo.toISOString().slice(0, 10),
      endDate: now.toISOString().slice(0, 10),
      loading: false,
      error: '',
    }
  },
  computed: {
    metrics() {
      return [
        { label: 'Total value', value: formatCurrency(this.stats.total_value || 0, this.currency) },
        { label: 'Gain', value: formatPercent(this.stats.total_gain_loss_pct || 0), color: `text-${changeColor(this.stats.total_gain_loss_pct)}` },
        { label: 'YoY return', value: formatPercent(this.stats.yoy_return || 0), color: `text-${changeColor(this.stats.yoy_return)}` },
        { label: 'Max drawdown', value: `${(this.stats.max_drawdown || 0).toFixed(2)}%` },
        { label: 'Sortino', value: (this.stats.sortino_ratio || 0).toFixed(2) },
        { label: 'TER', value: `${((this.stats.aggregated_ter || 0) * 100).toFixed(2)}%` },
      ]
    },
    backtestMetrics() {
      if (!this.backtest) return []
      return [
        { label: 'CAGR portfolio', value: formatPercent(this.backtest.cagr_portfolio) },
        { label: `CAGR ${this.benchmark}`, value: formatPercent(this.backtest.cagr_benchmark) },
        { label: 'Sharpe', value: (this.backtest.sharpe_portfolio || 0).toFixed(2) },
        { label: 'Alpha / Beta', value: `${(this.backtest.alpha || 0).toFixed(2)} / ${(this.backtest.beta || 0).toFixed(2)}` },
      ]
    },
    holdingReturns() {
      if (!this.backtest || !this.backtest.holding_returns) return []
      return Object.values(this.backtest.holding_returns)
    },
  },
  mounted() {
    this.loadStats()
  },
  methods: {
    changeColor,
    percent: formatPercent,
    async loadStats() {
      try {
        this.stats = await apiGet(`/api/portfolio/stats?currency=${this.currency}`)
      } catch (err) {
        this.error = err.message
      }
    },
    async runBacktest() {
      this.loading = true
      this.error = ''
      try {
        this.backtest = await apiGet(
          `/api/portfolio/backtest?start_date=${this.startDate}&end_date=${this.endDate}&benchmark=${this.benchmark}`,
        )
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
.holding-row {
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.holding-row:last-child {
  border-bottom: none;
}
</style>
