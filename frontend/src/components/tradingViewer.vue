<template>
  <div class="tv-root">
    <v-row class="ma-0 fill-height">
      <!-- Left control panel -->
      <v-col cols="12" md="3" lg="2" class="pa-2 tv-panel">
        <!-- Ticker search -->
        <v-card class="mb-2" variant="outlined">
          <v-card-text class="pa-2">
            <v-text-field
              v-model="searchQuery"
              label="Ticker or ISIN"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              prepend-inner-icon="mdi-magnify"
              @input="debouncedSearch"
              @click:clear="clearSearch"
            />
            <v-list
              v-if="searchResults.length > 0"
              density="compact"
              class="search-results mt-1"
            >
              <v-list-item
                v-for="result in searchResults"
                :key="result.ticker"
                @click="selectTicker(result)"
                class="px-2"
              >
                <v-list-item-title class="text-caption font-weight-bold">
                  {{ result.ticker }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-caption">
                  {{ result.name }}
                </v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <div v-if="activeTicker" class="mt-2 d-flex align-center gap-1 flex-wrap">
              <v-chip label color="primary" size="small">{{ activeTicker }}</v-chip>
              <span class="text-caption text-grey text-truncate" style="max-width: 120px;">
                {{ activeTickerName }}
              </span>
            </div>
          </v-card-text>
        </v-card>

        <!-- Interval selector -->
        <v-card class="mb-2" variant="outlined">
          <v-card-text class="pa-2">
            <div class="text-caption text-grey font-weight-medium mb-1">INTERVAL</div>
            <v-btn-toggle
              v-model="interval"
              mandatory
              density="compact"
              color="primary"
              class="d-flex flex-wrap"
              @update:model-value="onIntervalChange"
            >
              <v-btn value="1m" size="x-small">1m</v-btn>
              <v-btn value="5m" size="x-small">5m</v-btn>
              <v-btn value="15m" size="x-small">15m</v-btn>
              <v-btn value="1h" size="x-small">1h</v-btn>
              <v-btn value="1d" size="x-small">1d</v-btn>
            </v-btn-toggle>
          </v-card-text>
        </v-card>

        <!-- Indicators -->
        <v-card variant="outlined">
          <v-card-title class="text-subtitle-2 pa-2">Indicators</v-card-title>
          <v-divider />
          <v-card-text class="pa-2">
            <div v-for="ind in indicatorDefs" :key="ind.id" class="mb-3">
              <div class="d-flex align-center">
                <span
                  class="indicator-dot mr-2"
                  :style="{ backgroundColor: ind.color }"
                ></span>
                <v-switch
                  v-model="enabledIndicators[ind.id]"
                  :label="ind.label"
                  density="compact"
                  hide-details
                  :color="ind.color"
                  @update:model-value="val => onIndicatorToggle(ind.id, val)"
                  class="flex-grow-1"
                />
              </div>
              <div v-if="enabledIndicators[ind.id]" class="pl-6 mt-1">
                <v-text-field
                  v-model.number="indicatorParams[ind.id].period"
                  label="Period"
                  type="number"
                  variant="outlined"
                  density="compact"
                  hide-details
                  :min="1"
                  style="max-width: 110px;"
                  @update:model-value="() => onParamChange(ind.id)"
                />
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- Chart area -->
      <v-col cols="12" md="9" lg="10" class="pa-0 tv-chart-col">
        <div
          v-if="!activeTicker"
          class="d-flex align-center justify-center text-grey tv-placeholder"
        >
          <div class="text-center">
            <v-icon size="64" color="grey-lighten-1">mdi-chart-candlestick</v-icon>
            <p class="mt-4 text-body-2">Search and select a ticker to view its chart</p>
          </div>
        </div>

        <div
          v-else-if="chartLoading"
          class="d-flex align-center justify-center tv-placeholder"
        >
          <v-progress-circular indeterminate color="primary" />
        </div>

        <v-alert v-else-if="chartError" type="error" class="ma-4">
          {{ chartError }}
        </v-alert>

        <CandleChart
          v-else-if="chartData.length > 0"
          ref="tradeChart"
          :data="chartData"
          :height="chartHeight"
          :show-volume="true"
          @chart-ready="onChartReady"
        />
      </v-col>
    </v-row>
  </div>
</template>

<script>
import { ref, reactive, watch, onMounted, onUnmounted, nextTick } from 'vue'
import CandleChart from './candleChart.vue'
import { list, compute } from './indicators.js'
import { PYTHON_API_URL } from '../config.js'

export default {
  name: 'TradingViewer',
  components: { CandleChart },

  setup() {
    const searchQuery = ref('')
    const searchResults = ref([])
    const searchTimeout = ref(null)
    const activeTicker = ref('')
    const activeTickerName = ref('')
    const interval = ref('1d')
    const chartData = ref([])
    const chartLoading = ref(false)
    const chartError = ref('')
    const chartHeight = ref(600)
    const chartReady = ref(false)
    const tradeChart = ref(null)

    const indicatorDefs = list()

    const enabledIndicators = reactive(
      Object.fromEntries(indicatorDefs.map(d => [d.id, false]))
    )
    const indicatorParams = reactive(
      Object.fromEntries(indicatorDefs.map(d => [d.id, { ...d.defaultParams }]))
    )
    const activeSeries = reactive({})

    function updateChartHeight() {
      chartHeight.value = Math.max(400, window.innerHeight - 100)
    }

    // Search
    function debouncedSearch() {
      clearTimeout(searchTimeout.value)
      searchTimeout.value = setTimeout(doSearch, 400)
    }

    async function doSearch() {
      if (!searchQuery.value || searchQuery.value.length < 2) {
        searchResults.value = []
        return
      }
      const isIsin = /^[A-Z]{2}[A-Z0-9]{9,10}$/i.test(searchQuery.value)
      const type = isIsin ? 'isin' : 'ticker'
      try {
        const res = await fetch(
          `${PYTHON_API_URL}/api/search?identifier=${encodeURIComponent(searchQuery.value)}&search_type=${type}`
        )
        searchResults.value = res.ok ? await res.json() : []
      } catch {
        searchResults.value = []
      }
    }

    function clearSearch() {
      searchQuery.value = ''
      searchResults.value = []
    }

    async function selectTicker(result) {
      activeTicker.value = result.ticker
      activeTickerName.value = result.name
      searchResults.value = []
      searchQuery.value = ''
      await loadChart()
    }

    async function loadChart() {
      if (!activeTicker.value) return
      chartLoading.value = true
      chartError.value = ''
      chartData.value = []
      chartReady.value = false
      Object.keys(activeSeries).forEach(id => delete activeSeries[id])

      try {
        const res = await fetch(
          `${PYTHON_API_URL}/api/get_price?ticker=${encodeURIComponent(activeTicker.value)}&last_updates_unix_timestamp=0&interval=${interval.value}`
        )
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = await res.json()
        chartData.value = data || []
        if (chartData.value.length === 0) {
          chartError.value = 'No data available for this ticker.'
        }
      } catch (e) {
        chartError.value = e.message || 'Failed to load chart data'
      } finally {
        chartLoading.value = false
      }
    }

    function onIntervalChange() {
      if (activeTicker.value) loadChart()
    }

    function onChartReady() {
      chartReady.value = true
      nextTick(() => applyAllEnabledIndicators())
    }

    function applyAllEnabledIndicators() {
      for (const id of Object.keys(enabledIndicators)) {
        if (enabledIndicators[id]) applyIndicator(id)
      }
    }

    function buildCandles() {
      return chartData.value.map(d => ({
        time: d.timestamp,
        open: d.open,
        high: d.high,
        low: d.low,
        close: d.close,
        volume: d.volume
      }))
    }

    function applyIndicator(id) {
      if (!tradeChart.value || !chartData.value.length) return
      let data
      try {
        data = compute(id, buildCandles(), indicatorParams[id])
      } catch {
        return
      }
      if (!data || data.length === 0) return

      removeIndicatorSeries(id)

      const def = indicatorDefs.find(d => d.id === id)
      const series = tradeChart.value.addLineSeries({
        color: def.color,
        lineWidth: 1.5,
        priceLineVisible: false,
        lastValueVisible: true,
        title: id,
        crosshairMarkerVisible: false
      })
      if (series) {
        series.setData(data)
        activeSeries[id] = series
      }
    }

    function removeIndicatorSeries(id) {
      if (activeSeries[id] && tradeChart.value) {
        tradeChart.value.removeSeries(activeSeries[id])
        delete activeSeries[id]
      }
    }

    function onIndicatorToggle(id, val) {
      if (val) {
        nextTick(() => applyIndicator(id))
      } else {
        removeIndicatorSeries(id)
      }
    }

    function onParamChange(id) {
      if (enabledIndicators[id]) nextTick(() => applyIndicator(id))
    }

    watch(chartData, () => {
      nextTick(() => {
        if (chartReady.value) applyAllEnabledIndicators()
      })
    })

    onMounted(() => {
      updateChartHeight()
      window.addEventListener('resize', updateChartHeight)
    })

    onUnmounted(() => {
      window.removeEventListener('resize', updateChartHeight)
    })

    return {
      searchQuery,
      searchResults,
      activeTicker,
      activeTickerName,
      interval,
      chartData,
      chartLoading,
      chartError,
      chartHeight,
      tradeChart,
      indicatorDefs,
      enabledIndicators,
      indicatorParams,
      debouncedSearch,
      clearSearch,
      selectTicker,
      onIntervalChange,
      onChartReady,
      onIndicatorToggle,
      onParamChange
    }
  }
}
</script>

<style scoped>
.tv-root {
  height: calc(100vh - 64px);
  overflow: hidden;
}

.tv-panel {
  overflow-y: auto;
  height: 100%;
}

.tv-chart-col {
  height: 100%;
}

.tv-placeholder {
  height: 100%;
  min-height: 400px;
}

.search-results {
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid rgba(var(--v-border-color), 0.12);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

.search-results .v-list-item {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
  cursor: pointer;
}

.search-results .v-list-item:last-child {
  border-bottom: none;
}

.search-results .v-list-item:hover {
  background-color: rgba(var(--v-theme-primary), 0.08);
}

.indicator-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
</style>
