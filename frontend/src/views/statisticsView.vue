<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Statistics</div>
        <div class="text-caption text-medium-emphasis">Risk adjusted metrics and benchmark comparison</div>
      </div>
      <v-btn-toggle v-model="period" density="compact" variant="outlined" divided mandatory @update:model-value="loadRisk">
        <v-btn v-for="option in periods" :key="option.value" :value="option.value" size="small">{{ option.label }}</v-btn>
      </v-btn-toggle>
    </div>

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <div class="metric-grid mb-2">
      <div v-for="metric in metrics" :key="metric.label" class="panel pa-3 metric-card">
        <div class="metric-label">{{ metric.label }}</div>
        <div class="metric-value" :class="metric.color">{{ metric.value }}</div>
        <div class="text-caption text-medium-emphasis metric-hint">{{ metric.hint || '' }}</div>
      </div>
    </div>

    <v-row dense class="mt-1">
      <v-col cols="12" lg="6">
        <div class="panel pa-3">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="text-body-2 font-weight-medium">Drawdown</span>
            <span class="text-caption text-medium-emphasis">{{ risk.start }} to {{ risk.end }}</span>
          </div>
          <line-chart
            v-if="drawdownPoints.length"
            :points="drawdownPoints"
            kind="area"
            color="#ef5350"
            suffix="%"
            :height="200"
          />
          <div v-else class="text-caption text-medium-emphasis py-6 text-center">Not enough history.</div>
        </div>
      </v-col>
      <v-col cols="12" lg="6">
        <div class="panel pa-3">
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="text-body-2 font-weight-medium">Monthly returns</span>
            <span class="text-caption text-medium-emphasis">{{ risk.monthly?.length || 0 }} months</span>
          </div>
          <line-chart
            v-if="monthlyPoints.length"
            :points="monthlyPoints"
            kind="histogram"
            color="#26a69a"
            negative-color="#ef5350"
            suffix="%"
            :height="200"
          />
          <div v-else class="text-caption text-medium-emphasis py-6 text-center">Not enough history.</div>
        </div>
      </v-col>
    </v-row>

    <div class="panel pa-4 mt-2">
      <div class="d-flex align-center flex-wrap ga-2 mb-3">
        <span class="text-body-2 font-weight-medium mr-2">Backtest</span>
        <v-text-field v-model="startDate" type="date" density="compact" variant="outlined" hide-details label="Start" style="max-width: 165px"></v-text-field>
        <v-text-field v-model="endDate" type="date" density="compact" variant="outlined" hide-details label="End" style="max-width: 165px"></v-text-field>
        <v-text-field v-model="benchmark" density="compact" variant="outlined" hide-details label="Benchmark" style="max-width: 120px"></v-text-field>
        <v-btn size="small" variant="flat" color="primary" :loading="backtestLoading" @click="runBacktest">Run</v-btn>
      </div>

      <v-row v-if="backtest" dense class="mb-2">
        <v-col v-for="entry in backtestMetrics" :key="entry.label" cols="6" md="3" lg="2">
          <div class="text-caption text-medium-emphasis">{{ entry.label }}</div>
          <div class="text-body-1 font-weight-medium" :class="entry.color">{{ entry.value }}</div>
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
      <div v-else-if="!backtestLoading" class="text-caption text-medium-emphasis py-6 text-center">
        Run a backtest to compare the portfolio against a benchmark.
      </div>
    </div>

    <v-row dense class="mt-1">
      <v-col cols="12" lg="8">
        <div class="panel pa-4 h-100">
          <div class="metric-label mb-3">Exposure and contribution</div>
          <div class="exposure-head text-caption text-medium-emphasis">
            <span>Position</span><span>Weight</span><span>Return</span><span>Contribution</span>
          </div>
          <div v-for="row in risk.exposure || []" :key="row.ticker" class="exposure-row">
            <span class="text-body-2 text-truncate">
              <span class="font-weight-medium ticker-link" @click="$emit('open-ticker', row.ticker)">{{ row.ticker }}</span>
              <span class="text-medium-emphasis ml-2 d-none d-sm-inline">{{ row.name }}</span>
            </span>
            <span class="text-body-2">{{ (row.weight || 0).toFixed(1) }}%</span>
            <span class="text-body-2" :class="`text-${changeColor(row.return)}`">{{ percent(row.return) }}</span>
            <span class="text-body-2" :class="`text-${changeColor(row.contribution)}`">{{ percent(row.contribution) }}</span>
          </div>
          <div v-if="!(risk.exposure || []).length" class="text-caption text-medium-emphasis">No positions.</div>
        </div>
      </v-col>

      <v-col cols="12" lg="4">
        <div class="panel pa-4 h-100">
          <div class="metric-label mb-3">Signal per holding</div>
          <div v-for="row in holdingReturns" :key="row.ticker" class="d-flex align-center py-1">
            <span
              class="text-body-2 ticker-link"
              style="width: 84px"
              @click="$emit('open-ticker', row.ticker)"
            >{{ row.ticker }}</span>
            <span class="text-caption text-medium-emphasis flex-grow-1">{{ row.signal }}</span>
            <span class="text-body-2" :class="`text-${changeColor(row.total_return)}`">{{ percent(row.total_return) }}</span>
          </div>
          <div v-if="!holdingReturns.length" class="text-caption text-medium-emphasis">
            Run a backtest to see per holding signals.
          </div>
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import BacktestChart from '../components/backtestChart.vue'
import LineChart from '../components/lineChart.vue'
import { apiGet, changeColor, formatCurrency, formatPercent } from '../utils/api'

export default {
  name: 'StatisticsView',
  components: { BacktestChart, LineChart },
  props: {
    currency: { type: String, default: 'EUR' },
  },
  emits: ['open-ticker'],
  data() {
    const now = new Date()
    const yearAgo = new Date(now.getTime() - 365 * 24 * 3600 * 1000)
    return {
      stats: {},
      risk: {},
      period: '1y',
      periods: [
        { value: '1m', label: '1M' },
        { value: '3m', label: '3M' },
        { value: '6m', label: '6M' },
        { value: '1y', label: '1Y' },
        { value: '3y', label: '3Y' },
        { value: 'max', label: 'MAX' },
      ],
      backtest: null,
      benchmark: 'SPY',
      startDate: yearAgo.toISOString().slice(0, 10),
      endDate: now.toISOString().slice(0, 10),
      loading: false,
      backtestLoading: false,
      error: '',
    }
  },
  computed: {
    metrics() {
      const risk = this.risk
      const stats = this.stats
      return [
        { label: 'Total value', value: formatCurrency(stats.total_value || 0, this.currency) },
        { label: 'Total gain', value: formatPercent(stats.total_gain_loss_pct || 0), color: `text-${changeColor(stats.total_gain_loss_pct)}` },
        { label: 'Period return', value: formatPercent(risk.total_return || 0), color: `text-${changeColor(risk.total_return)}`, hint: risk.period || '' },
        { label: 'CAGR', value: formatPercent(risk.cagr || 0), color: `text-${changeColor(risk.cagr)}` },
        { label: 'Volatility', value: `${(risk.volatility || 0).toFixed(1)}%`, hint: 'annualized' },
        { label: 'Sharpe', value: (risk.sharpe || 0).toFixed(2), hint: `sortino ${(risk.sortino || 0).toFixed(2)}` },
        { label: 'Calmar', value: (risk.calmar || 0).toFixed(2), hint: 'return per drawdown' },
        { label: 'Max drawdown', value: `${(risk.max_drawdown || 0).toFixed(1)}%`, color: 'text-error', hint: `now ${(risk.current_drawdown || 0).toFixed(1)}%` },
        { label: 'VaR 95', value: `${(risk.var_95 || 0).toFixed(2)}%`, hint: `worst day ${(risk.worst_day || 0).toFixed(1)}%` },
        { label: 'Win rate', value: `${(risk.win_rate || 0).toFixed(0)}%`, hint: `${risk.observations || 0} days` },
        { label: 'Beta', value: (risk.beta || 0).toFixed(2), hint: risk.benchmark || '' },
        { label: 'Alpha', value: formatPercent(risk.alpha || 0), color: `text-${changeColor(risk.alpha)}`, hint: 'annualized' },
        { label: 'TER', value: `${((stats.aggregated_ter || 0) * 100).toFixed(2)}%`, hint: 'weighted' },
      ]
    },
    backtestMetrics() {
      if (!this.backtest) return []
      return [
        { label: 'CAGR portfolio', value: formatPercent(this.backtest.cagr_portfolio), color: `text-${changeColor(this.backtest.cagr_portfolio)}` },
        { label: `CAGR ${this.benchmark}`, value: formatPercent(this.backtest.cagr_benchmark) },
        { label: 'Alpha / Beta', value: `${(this.backtest.alpha || 0).toFixed(2)} / ${(this.backtest.beta || 0).toFixed(2)}` },
        { label: 'Sharpe', value: (this.backtest.sharpe_portfolio || 0).toFixed(2) },
        { label: 'Win rate', value: `${(this.backtest.win_rate || 0).toFixed(0)}%` },
        { label: 'Best / worst month', value: `${(this.backtest.best_month || 0).toFixed(1)}% / ${(this.backtest.worst_month || 0).toFixed(1)}%` },
      ]
    },
    drawdownPoints() {
      return (this.risk.drawdown_series || []).map((point) => ({ time: point.time, value: point.drawdown }))
    },
    monthlyPoints() {
      return (this.risk.monthly || []).map((item) => ({
        time: Math.floor(new Date(`${item.month}-01T00:00:00Z`).getTime() / 1000),
        value: item.return,
      }))
    },
    holdingReturns() {
      if (!this.backtest || !this.backtest.holding_returns) return []
      return Object.values(this.backtest.holding_returns)
    },
  },
  watch: {
    currency() {
      this.loadStats()
    },
  },
  mounted() {
    this.loadStats()
    this.loadRisk()
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
    async loadRisk() {
      this.loading = true
      this.error = ''
      try {
        this.risk = await apiGet(`/api/portfolio/risk?period=${this.period}&benchmark=${this.benchmark}`)
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
    async runBacktest() {
      this.backtestLoading = true
      this.error = ''
      try {
        this.backtest = await apiGet(
          `/api/portfolio/backtest?start_date=${this.startDate}&end_date=${this.endDate}&benchmark=${this.benchmark}`,
        )
      } catch (err) {
        this.error = err.message
      } finally {
        this.backtestLoading = false
      }
    },
  },
}
</script>

<style scoped>
.metric-grid {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}

.metric-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 84px;
}

.metric-hint {
  min-height: 16px;
  margin-top: 2px;
}

.exposure-head,
.exposure-row {
  display: grid;
  grid-template-columns: 1fr 70px 80px 90px;
  gap: 8px;
  align-items: center;
}

.exposure-head {
  padding-bottom: 6px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.exposure-row {
  padding: 6px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}

@media (max-width: 600px) {
  .exposure-head,
  .exposure-row {
    grid-template-columns: 1fr 54px 68px 76px;
    font-size: 12px;
  }
}
</style>
