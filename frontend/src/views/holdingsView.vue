<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Holdings</div>
        <div class="text-caption text-medium-emphasis">Positions, composition and per asset detail</div>
      </div>
      <div class="d-flex align-center ga-2">
        <v-text-field
          v-model="query"
          density="compact"
          variant="outlined"
          hide-details
          clearable
          placeholder="Search ticker, name or ISIN"
          prepend-inner-icon="mdi-magnify"
          class="holdings-search"
        />
        <v-btn size="small" variant="outlined" prepend-icon="mdi-plus" @click="showAdd = !showAdd">
          {{ showAdd ? 'Close' : 'Add holding' }}
        </v-btn>
      </div>
    </div>

    <add-holding-panel v-if="showAdd" @created="onChanged" @cancel="showAdd = false" />

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <div v-if="holdings.length" class="panel table-panel">
      <table class="holdings-table">
        <thead>
          <tr>
            <th></th>
            <th v-for="column in columns" :key="column.key" class="sortable"
              :class="column.align" @click="sortBy(column.key)">
              {{ column.label }}
              <v-icon v-if="sortKey === column.key" size="14" class="ml-1">
                {{ sortDir === 1 ? 'mdi-arrow-up' : 'mdi-arrow-down' }}
              </v-icon>
              <v-icon v-else size="14" class="ml-1 sort-hint">mdi-unfold-more-horizontal</v-icon>
            </th>
            <th></th>
          </tr>
        </thead>
        <holding-view
          v-for="holding in visibleHoldings"
          :key="holding.id_holding"
          :holding="holding"
          :auth-token="token"
          :home-currency="currency"
          :focus="focusNonceFor(holding.ticker)"
          @open-ticker="$emit('open-ticker', { ticker: $event, target: 'trading' })"
          @day-change="onDayChange"
          @holding-updated="load"
          @holding-removed="load"
        />
      </table>
      <div v-if="!visibleHoldings.length" class="text-caption text-medium-emphasis pa-4 text-center">
        Nothing matches "{{ query }}".
      </div>
    </div>
    <v-alert v-else-if="!loading" type="info" variant="tonal" density="compact">
      No holdings yet. Use "Add holding" to search for a ticker or ISIN.
    </v-alert>
  </div>
</template>

<script>
import HoldingView from '../components/holdingView.vue'
import AddHoldingPanel from '../components/addHoldingPanel.vue'
import { apiGet, authToken } from '../utils/api'

export default {
  name: 'HoldingsView',
  components: { HoldingView, AddHoldingPanel },
  props: {
    currency: { type: String, default: 'EUR' },
    focusTicker: { type: String, default: '' },
    focusNonce: { type: Number, default: 0 },
  },
  emits: ['open-ticker'],
  data() {
    return {
      token: authToken(),
      holdings: [],
      dayChanges: {},
      query: '',
      sortKey: 'value',
      sortDir: -1,
      loading: false,
      error: '',
      showAdd: false,
      columns: [
        { key: 'ticker', label: 'Ticker' },
        { key: 'price', label: 'Price', align: 'text-right' },
        { key: 'day', label: 'Day %', align: 'text-right' },
        { key: 'quantity', label: 'Quantity', align: 'text-right' },
        { key: 'value', label: 'Value', align: 'text-right' },
        { key: 'gain', label: 'Gain %', align: 'text-right' },
      ],
    }
  },
  computed: {
    visibleHoldings() {
      const query = this.query.trim().toLowerCase()
      const filtered = query
        ? this.holdings.filter((holding) =>
            `${holding.ticker} ${holding.name} ${holding.isin}`.toLowerCase().includes(query))
        : this.holdings
      const value = (holding) => (holding.quantity ? holding.value / holding.quantity : 0)
      const pick = {
        ticker: (holding) => holding.ticker || '',
        price: value,
        day: (holding) => this.dayChanges[holding.id_holding] || 0,
        quantity: (holding) => holding.quantity || 0,
        value: (holding) => holding.value || 0,
        gain: (holding) => holding.gain_pct || 0,
      }[this.sortKey]
      return [...filtered].sort((a, b) => {
        const left = pick(a)
        const right = pick(b)
        const order = typeof left === 'string' ? left.localeCompare(right) : left - right
        return order * this.sortDir
      })
    },
  },
  watch: {
    focusNonce() {
      this.scrollToFocus()
    },
  },
  mounted() {
    this.load().then(() => this.scrollToFocus())
  },
  methods: {
    sortBy(key) {
      if (this.sortKey === key) {
        this.sortDir = -this.sortDir
        return
      }
      this.sortKey = key
      this.sortDir = key === 'ticker' ? 1 : -1
    },
    onDayChange(id, percent) {
      this.dayChanges = { ...this.dayChanges, [id]: percent }
    },
    onChanged() {
      this.showAdd = false
      this.load()
    },
    focusNonceFor(ticker) {
      return this.focusTicker && ticker === this.focusTicker ? this.focusNonce : 0
    },
    scrollToFocus() {
      if (!this.focusTicker) return
      this.$nextTick(() => {
        const row = document.querySelector(`[data-ticker="${CSS.escape(this.focusTicker)}"]`)
        row?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      })
    },
    async load() {
      this.loading = true
      this.error = ''
      try {
        this.holdings = await apiGet('/api/portfolio/holdings')
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
.table-panel {
  overflow-x: auto;
}

.holdings-search {
  max-width: 280px;
}

.sortable {
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
}

.sortable:hover {
  color: rgba(255, 255, 255, 0.9);
}

.sort-hint {
  opacity: 0.2;
}

.sortable:hover .sort-hint {
  opacity: 0.6;
}

@media (max-width: 600px) {
  .holdings-search {
    max-width: none;
    width: 100%;
  }
}

.holdings-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.holdings-table th {
  text-align: left;
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.55);
  padding: 10px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
</style>
