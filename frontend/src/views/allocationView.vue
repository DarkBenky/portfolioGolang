<template>
  <div>
    <div class="mb-4">
      <div class="text-h6">Allocation</div>
      <div class="text-caption text-medium-emphasis">Weighted by current position value</div>
    </div>

    <v-progress-linear v-if="loading" indeterminate color="primary" class="mb-2"></v-progress-linear>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <v-row dense>
      <v-col v-for="section in sections" :key="section.key" cols="12" md="4">
        <div class="panel pa-4 h-100">
          <div class="metric-label mb-3">{{ section.title }}</div>
          <div v-for="item in section.items.slice(0, 12)" :key="item.name" class="mb-2">
            <div class="d-flex justify-space-between text-body-2 mb-1">
              <span class="text-truncate mr-2">{{ item.name }}</span>
              <span class="text-medium-emphasis">{{ item.percentage.toFixed(1) }}%</span>
            </div>
            <v-progress-linear
              :model-value="item.percentage"
              :color="section.color"
              height="4"
              rounded
            ></v-progress-linear>
          </div>
          <div v-if="!section.items.length" class="text-caption text-medium-emphasis">No data.</div>
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import { apiGet } from '../utils/api'

export default {
  name: 'AllocationView',
  data() {
    return {
      allocation: { sectors: [], regions: [], companies: [], total_value: 0 },
      loading: false,
      error: '',
    }
  },
  computed: {
    sections() {
      return [
        { key: 'sectors', title: 'Sectors', items: this.allocation.sectors || [], color: 'primary' },
        { key: 'regions', title: 'Regions', items: this.allocation.regions || [], color: 'teal' },
        { key: 'companies', title: 'Top companies', items: this.allocation.companies || [], color: 'amber' },
      ]
    },
  },
  mounted() {
    this.load()
  },
  methods: {
    async load() {
      this.loading = true
      try {
        this.allocation = await apiGet('/api/portfolio/allocation')
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
  },
}
</script>
