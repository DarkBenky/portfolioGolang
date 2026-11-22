<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Holdings</div>
        <div class="text-caption text-medium-emphasis">Positions, composition and per asset detail</div>
      </div>
      <v-btn size="small" variant="outlined" prepend-icon="mdi-plus" @click="showAdd = !showAdd">
        {{ showAdd ? 'Close' : 'Add holding' }}
      </v-btn>
    </div>

    <add-holding-panel v-if="showAdd" @created="onChanged" @cancel="showAdd = false" />

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <div v-if="holdings.length" class="panel table-panel">
      <table class="holdings-table">
        <thead>
          <tr>
            <th></th>
            <th>Ticker</th>
            <th class="text-right">Price</th>
            <th class="text-right">Day</th>
            <th class="text-right">Quantity</th>
            <th class="text-right">Value</th>
            <th class="text-right">Gain</th>
          </tr>
        </thead>
        <holding-view
          v-for="holding in holdings"
          :key="holding.id_holding"
          :holding="holding"
          :auth-token="token"
          :home-currency="currency"
          @holding-updated="load"
          @holding-removed="load"
        />
      </table>
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
  },
  data() {
    return {
      token: authToken(),
      holdings: [],
      loading: false,
      error: '',
      showAdd: false,
    }
  },
  mounted() {
    this.load()
  },
  methods: {
    onChanged() {
      this.showAdd = false
      this.load()
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
