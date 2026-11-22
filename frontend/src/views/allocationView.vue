<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Allocation</div>
        <div class="text-caption text-medium-emphasis">Weighted by current position value</div>
      </div>
      <div class="text-caption text-medium-emphasis">
        {{ formatCurrency(allocation.total_value, currency) }} invested across {{ holdings.length }} positions
      </div>
    </div>

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <v-row dense>
      <v-col v-for="section in sections" :key="section.key" cols="12" md="4" class="d-flex">
        <div class="panel pa-4 flex-grow-1 d-flex flex-column">
          <div class="d-flex align-center justify-space-between mb-3">
            <span class="metric-label">{{ section.title }}</span>
            <span class="text-caption text-medium-emphasis">{{ section.items.length }} groups</span>
          </div>
          <donut-chart
            v-if="section.items.length"
            :items="section.items"
            :size="132"
            :thickness="18"
            :max-slices="7"
            :center-label="section.short"
            :center-value="section.top.toFixed(0) + '%'"
          />
          <div v-else class="text-caption text-medium-emphasis py-6 text-center">No data.</div>
        </div>
      </v-col>
    </v-row>

    <v-row dense class="mt-1">
      <v-col cols="12" lg="7">
        <div class="panel pa-4 h-100">
          <div class="metric-label mb-3">By holding</div>
          <div v-for="row in byHolding" :key="row.id_holding" class="mb-3">
            <div class="d-flex justify-space-between text-body-2 mb-1">
              <span class="text-truncate mr-2">
                <span class="font-weight-medium ticker-link" @click="$emit('open-ticker', row.ticker)">{{ row.ticker }}</span>
                <span class="text-medium-emphasis ml-2">{{ row.name }}</span>
              </span>
              <span class="text-medium-emphasis text-no-wrap">
                {{ formatCurrency(row.value, currency) }} · {{ row.weight.toFixed(1) }}%
              </span>
            </div>
            <v-progress-linear :model-value="row.weight" color="primary" height="5" rounded />
            <div class="d-flex justify-space-between mt-1">
              <span class="text-caption text-medium-emphasis">{{ row.etf ? 'ETF' : 'Stock' }} · {{ row.currency }}</span>
              <span class="text-caption" :class="`text-${changeColor(row.gain_pct)}`">{{ percent(row.gain_pct) }}</span>
            </div>
          </div>
          <div v-if="!byHolding.length" class="text-caption text-medium-emphasis">No holdings yet.</div>
        </div>
      </v-col>

      <v-col cols="12" lg="5">
        <div class="panel pa-4 mb-3">
          <div class="metric-label mb-3">Concentration</div>
          <div v-for="entry in concentration" :key="entry.label" class="d-flex justify-space-between py-2">
            <span class="text-body-2 text-medium-emphasis">{{ entry.label }}</span>
            <span class="text-body-2" :class="entry.color">{{ entry.value }}</span>
          </div>
        </div>
        <div class="panel pa-4">
          <div class="metric-label mb-3">Asset type</div>
          <donut-chart
            :items="typeSplit"
            :size="130"
            :thickness="18"
            center-label="ETF"
            :center-value="etfShare + '%'"
          />
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import DonutChart from '../components/donutChart.vue'
import { apiGet, changeColor, formatCurrency, formatPercent } from '../utils/api'

export default {
  name: 'AllocationView',
  components: { DonutChart },
  props: {
    currency: { type: String, default: 'EUR' },
  },
  emits: ['open-ticker'],
  data() {
    return {
      allocation: { sectors: [], regions: [], companies: [], total_value: 0 },
      holdings: [],
      loading: false,
      error: '',
    }
  },
  computed: {
    sections() {
      const section = (key, title, short, items) => ({
        key,
        title,
        short,
        items,
        top: items.reduce((best, item) => Math.max(best, Number(item.percentage) || 0), 0),
      })
      return [
        section('sectors', 'Sectors', 'Top sector', this.allocation.sectors || []),
        section('regions', 'Regions', 'Top region', this.allocation.regions || []),
        section('companies', 'Top companies', 'Top company', this.allocation.companies || []),
      ]
    },
    byHolding() {
      return [...this.holdings].sort((a, b) => b.weight - a.weight)
    },
    concentration() {
      const sectors = this.allocation.sectors || []
      const companies = this.allocation.companies || []
      const topSector = sectors[0]
      const topCompany = companies[0]
      const top3 = companies.slice(0, 3).reduce((sum, item) => sum + item.percentage, 0)
      return [
        { label: 'Largest sector', value: topSector ? `${topSector.name} ${topSector.percentage.toFixed(1)}%` : '-' },
        { label: 'Largest company', value: topCompany ? `${topCompany.name} ${topCompany.percentage.toFixed(1)}%` : '-' },
        { label: 'Top 3 companies', value: `${top3.toFixed(1)}%`, color: top3 > 40 ? 'text-warning' : '' },
        { label: 'Number of positions', value: String(this.holdings.length) },
      ]
    },
    etfShare() {
      const total = this.holdings.reduce((sum, holding) => sum + holding.value, 0)
      if (!total) return 0
      const etfs = this.holdings.filter((holding) => holding.etf).reduce((sum, holding) => sum + holding.value, 0)
      return Math.round((etfs / total) * 100)
    },
    typeSplit() {
      return [
        { name: 'ETFs', percentage: this.etfShare },
        { name: 'Stocks', percentage: 100 - this.etfShare },
      ]
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
    percent: formatPercent,
    formatCurrency,
    async load() {
      this.loading = true
      this.error = ''
      try {
        const [allocation, holdings] = await Promise.all([
          apiGet('/api/portfolio/allocation'),
          apiGet(`/api/portfolio/holdings?currency=${this.currency}`),
        ])
        this.allocation = allocation
        this.holdings = holdings || []
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
  },
}
</script>
