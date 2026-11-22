<template>
  <div class="panel pa-4 mb-4">
    <div class="d-flex align-center justify-space-between mb-3">
      <span class="text-body-2 font-weight-medium">Add holding</span>
      <v-btn size="x-small" variant="text" @click="$emit('cancel')">cancel</v-btn>
    </div>

    <div class="d-flex ga-2 mb-3 flex-wrap">
      <v-text-field
        v-model="query"
        density="compact"
        variant="outlined"
        hide-details
        prepend-inner-icon="mdi-magnify"
        label="Ticker, ISIN or name"
        clearable
        style="min-width: 260px; flex: 1"
        @update:model-value="onQuery"
        @keydown.enter.prevent="search"
      ></v-text-field>
      <v-btn variant="outlined" size="small" :loading="searching" @click="search">Search</v-btn>
    </div>

    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <div v-if="results.length" class="mb-4">
      <div
        v-for="result in results"
        :key="result.ticker"
        class="result-row pa-2"
        :class="{ selected: form.ticker === result.ticker }"
        @click="select(result)"
      >
        <div class="d-flex align-center justify-space-between">
          <div>
            <span class="text-body-2 font-weight-medium">{{ result.ticker }}</span>
            <span class="text-caption text-medium-emphasis ml-2">{{ result.name }}</span>
          </div>
          <v-chip size="x-small" variant="tonal">{{ result.type || 'EQUITY' }}</v-chip>
        </div>
        <div class="text-caption text-medium-emphasis">
          {{ result.exchange }}
          <span v-if="result.currency"> · {{ result.currency }}</span>
          <span v-if="result.isin && result.isin !== 'N/A'"> · {{ result.isin }}</span>
          <span v-if="result.ter"> · TER {{ result.ter }}</span>
        </div>
      </div>
    </div>

    <v-row dense>
      <v-col cols="12" sm="6" md="3">
        <v-text-field v-model="form.name" density="compact" variant="outlined" hide-details label="Name"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2">
        <v-text-field v-model="form.ticker" density="compact" variant="outlined" hide-details label="Ticker"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-text-field v-model="form.isin" density="compact" variant="outlined" hide-details label="ISIN"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2">
        <v-text-field v-model="form.currency" density="compact" variant="outlined" hide-details label="Currency"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2">
        <v-switch v-model="form.etf" density="compact" hide-details label="ETF" color="primary"></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-text-field v-model.number="form.quantity" type="number" density="compact" variant="outlined" hide-details label="Quantity"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-text-field v-model.number="form.purchasePrice" type="number" density="compact" variant="outlined" hide-details label="Purchase price"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2">
        <v-text-field v-model.number="form.ter" type="number" density="compact" variant="outlined" hide-details label="TER %"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2">
        <v-text-field v-model="form.policy" density="compact" variant="outlined" hide-details label="Policy"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="2" class="d-flex align-center">
        <v-btn block variant="flat" color="primary" :loading="saving" @click="submit">Add</v-btn>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import { apiFetch, apiGet } from '../utils/api'

export default {
  name: 'AddHoldingPanel',
  emits: ['created', 'cancel'],
  data() {
    return {
      query: '',
      results: [],
      searching: false,
      saving: false,
      error: '',
      timer: null,
      form: {
        name: '',
        ticker: '',
        isin: '',
        exchange: '',
        currency: 'USD',
        quantity: null,
        purchasePrice: null,
        ter: 0,
        policy: 'N/A',
        etf: false,
      },
    }
  },
  beforeUnmount() {
    clearTimeout(this.timer)
  },
  methods: {
    onQuery(value) {
      clearTimeout(this.timer)
      if (!value || value.length < 2) {
        this.results = []
        return
      }
      this.timer = setTimeout(this.search, 350)
    },
    async search() {
      const query = (this.query || '').trim()
      if (!query) return
      this.searching = true
      this.error = ''
      try {
        const looksLikeISIN = /^[A-Z]{2}[A-Z0-9]{10}$/i.test(query)
        this.results = await apiGet(
          `/api/search?identifier=${encodeURIComponent(query)}&search_type=${looksLikeISIN ? 'isin' : 'ticker'}`,
        )
      } catch (err) {
        this.error = err.message
      } finally {
        this.searching = false
      }
    },
    select(result) {
      this.form.name = result.name || result.ticker
      this.form.ticker = result.ticker
      this.form.isin = result.isin && result.isin !== 'N/A' ? result.isin : ''
      this.form.exchange = result.exchange || ''
      this.form.currency = result.currency || 'USD'
      this.form.etf = result.type === 'ETF' || result.type === 'MUTUALFUND'
      if (result.ter) {
        this.form.ter = Number(String(result.ter).replace('%', '')) || 0
      }
      if (result.distribution_policy) {
        this.form.policy = result.distribution_policy
      }
    },
    async submit() {
      if (!this.form.ticker || !this.form.quantity || !this.form.purchasePrice) {
        this.error = 'Ticker, quantity and purchase price are required'
        return
      }
      this.saving = true
      this.error = ''
      try {
        const body = new FormData()
        body.append('Name', this.form.name || this.form.ticker)
        body.append('Ticker', this.form.ticker.toUpperCase())
        body.append('ISIN', this.form.isin)
        body.append('Exchange', this.form.exchange)
        body.append('Quantity', String(this.form.quantity))
        body.append('PurchasePrice', String(this.form.purchasePrice))
        body.append('TER', String(this.form.ter || 0))
        body.append('Policy', this.form.policy)
        body.append('Currency', this.form.currency)
        body.append('ETF', String(this.form.etf))
        const response = await apiFetch('/api/asset/holdings', { method: 'POST', body })
        if (!response.ok) throw new Error(await response.text())
        this.$emit('created')
      } catch (err) {
        this.error = err.message
      } finally {
        this.saving = false
      }
    },
  },
}
</script>

<style scoped>
.result-row {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  margin-bottom: 6px;
  cursor: pointer;
}

.result-row:hover,
.result-row.selected {
  border-color: rgb(var(--v-theme-primary));
}
</style>
