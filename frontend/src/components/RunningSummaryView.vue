<template>
  <v-container fluid class="pa-2">
    <v-row dense class="mb-2">
      <v-col cols="12" class="d-flex align-center ga-2">
        <v-icon size="20" color="primary">mdi-chart-timeline-variant</v-icon>
        <span class="text-h6 font-weight-bold">Running Summary Report</span>
        <v-spacer></v-spacer>
        <v-btn-toggle v-model="windowDays" mandatory density="compact" color="primary" variant="outlined" @update:model-value="onWindowChange">
          <v-btn :value="7" size="x-small">7 Days</v-btn>
          <v-btn :value="14" size="x-small">14 Days</v-btn>
          <v-btn :value="30" size="x-small">30 Days</v-btn>
        </v-btn-toggle>
        <v-btn
          size="small"
          variant="tonal"
          color="primary"
          :loading="generating"
          prepend-icon="mdi-magic"
          @click="generateReport"
        >
          {{ hasData ? 'Regenerate' : 'Generate Report' }}
        </v-btn>
      </v-col>
    </v-row>

    <v-row v-if="error" dense>
      <v-col cols="12">
        <v-alert type="error" density="compact" closable @click:close="error = ''">
          {{ error }}
        </v-alert>
      </v-col>
    </v-row>

    <v-row v-if="loading && !hasData" dense>
      <v-col cols="12" class="d-flex justify-center py-12">
        <v-progress-circular indeterminate color="primary" size="48"></v-progress-circular>
      </v-col>
    </v-row>

    <template v-if="hasData && !loading">
      <v-row dense>
        <v-col cols="12">
          <v-card variant="outlined" :class="['overview-card', sentimentClass]">
            <v-card-text class="pa-3">
              <div class="d-flex align-center mb-2">
                <v-chip :color="sentimentColor" size="small" variant="tonal" class="mr-2">
                  {{ sentimentLabel }}
                </v-chip>
                <span class="text-caption text-grey">
                  {{ windowDays }}-day window ending {{ formatDate(data.date) }}
                </span>
              </div>
              <div class="sentiment-gauge mb-3">
                <v-progress-linear
                  :model-value="sentimentPercentage"
                  :color="sentimentColor"
                  height="12"
                  rounded
                ></v-progress-linear>
                <div class="d-flex justify-space-between mt-1">
                  <span class="text-caption text-grey">Very Negative</span>
                  <span class="text-caption text-grey">Negative</span>
                  <span class="text-caption font-weight-bold" :style="{ color: sentimentColor }">
                    {{ data.sentiment.toFixed(2) }}
                  </span>
                  <span class="text-caption text-grey">Positive</span>
                  <span class="text-caption text-grey">Very Positive</span>
                </div>
              </div>
              <div class="text-body-2 markdown-content" v-html="formatMarkdown(data.summary)"></div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-row v-if="data.theme_predictions && data.theme_predictions.length > 0" dense class="mt-2">
        <v-col cols="12">
          <div class="text-subtitle-2 font-weight-bold mb-2">
            <v-icon size="16" class="mr-1">mdi-lightbulb-group</v-icon>
            Key Themes
          </div>
        </v-col>
        <v-col v-for="(theme, i) in data.theme_predictions" :key="'theme-' + i" cols="12" md="6" lg="4">
          <v-card variant="outlined" class="theme-card">
            <v-card-text class="pa-2">
              <div class="d-flex align-center mb-1">
                <span class="font-weight-bold text-body-2">{{ theme.theme }}</span>
              </div>
              <div v-for="(sc, j) in (theme.scenarios || []).slice(0, 1)" :key="'ts-' + j">
                <p class="text-caption text-grey-darken-2 mb-0">{{ sc.description }}</p>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-row v-if="data.sector_predictions && data.sector_predictions.length > 0" dense class="mt-3">
        <v-col cols="12">
          <div class="text-subtitle-2 font-weight-bold mb-2">
            <v-icon size="16" class="mr-1">mdi-domain</v-icon>
            Sector Predictions
          </div>
        </v-col>
        <v-col v-for="(sp, i) in data.sector_predictions" :key="'sector-' + i" cols="12" md="6" lg="4">
          <v-card variant="outlined" class="prediction-card">
            <v-card-text class="pa-2">
              <div class="font-weight-bold text-body-2 mb-2">{{ sp.sector }}</div>
              <div v-for="(sc, j) in sp.scenarios" :key="'sc-' + j" class="scenario-row mb-1">
                <div class="d-flex align-center">
                  <v-chip :color="getScenarioColor(sc.label)" size="x-small" variant="flat" class="mr-1 scenario-chip">
                    {{ sc.label }}
                  </v-chip>
                  <span class="text-caption font-weight-bold" :style="{ color: getScenarioColor(sc.label) }">
                    {{ sc.probability }}%
                  </span>
                </div>
                <v-progress-linear
                  :model-value="sc.probability"
                  :color="getScenarioColor(sc.label)"
                  height="4"
                  rounded
                  class="mt-1"
                ></v-progress-linear>
                <p class="text-caption text-grey-darken-2 mt-1 mb-0 scenario-desc">{{ sc.description }}</p>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-row v-if="data.theme_predictions && data.theme_predictions.length > 0" dense class="mt-3">
        <v-col cols="12">
          <div class="text-subtitle-2 font-weight-bold mb-2">
            <v-icon size="16" class="mr-1">mdi-trending-up</v-icon>
            Theme Predictions
          </div>
        </v-col>
        <v-col v-for="(tp, i) in data.theme_predictions" :key="'tpred-' + i" cols="12" md="6" lg="4">
          <v-card variant="outlined" class="prediction-card">
            <v-card-text class="pa-2">
              <div class="font-weight-bold text-body-2 mb-2">{{ tp.theme }}</div>
              <div v-for="(sc, j) in tp.scenarios" :key="'tsc-' + j" class="scenario-row mb-1">
                <div class="d-flex align-center">
                  <v-chip :color="getScenarioColor(sc.label)" size="x-small" variant="flat" class="mr-1 scenario-chip">
                    {{ sc.label }}
                  </v-chip>
                  <span class="text-caption font-weight-bold" :style="{ color: getScenarioColor(sc.label) }">
                    {{ sc.probability }}%
                  </span>
                </div>
                <v-progress-linear
                  :model-value="sc.probability"
                  :color="getScenarioColor(sc.label)"
                  height="4"
                  rounded
                  class="mt-1"
                ></v-progress-linear>
                <p class="text-caption text-grey-darken-2 mt-1 mb-0 scenario-desc">{{ sc.description }}</p>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-row v-if="hasOutlook" dense class="mt-3">
        <v-col cols="12">
          <div class="text-subtitle-2 font-weight-bold mb-2">
            <v-icon size="16" class="mr-1">mdi-chart-bell-curve</v-icon>
            Portfolio Outlook
          </div>
        </v-col>
        <v-col cols="12">
          <v-card variant="outlined">
            <v-card-text class="pa-3">
              <div v-for="(sc, j) in outlookScenarios" :key="'po-' + j" class="outlook-row mb-3">
                <div class="d-flex align-center mb-1">
                  <v-chip :color="getScenarioColor(sc.label)" size="small" variant="flat" class="mr-2">
                    {{ sc.label }}
                  </v-chip>
                  <span class="font-weight-bold" :style="{ color: getScenarioColor(sc.label) }">
                    {{ sc.probability }}%
                  </span>
                </div>
                <v-progress-linear
                  :model-value="sc.probability"
                  :color="getScenarioColor(sc.label)"
                  height="8"
                  rounded
                  class="mb-1"
                ></v-progress-linear>
                <p class="text-caption text-grey-darken-2 mb-0">{{ sc.description }}</p>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </template>

    <v-row v-else-if="!loading && !hasData" dense>
      <v-col cols="12">
        <v-card variant="outlined" class="text-center pa-12">
          <v-icon size="64" color="grey" class="mb-3">mdi-chart-timeline-variant</v-icon>
          <div class="text-h6 text-grey mb-2">No Running Summary Yet</div>
          <div class="text-body-2 text-grey mb-4">
            Generate a running summary to see multi-scenario predictions across sectors and themes
          </div>
          <v-btn
            size="large"
            variant="tonal"
            color="primary"
            :loading="generating"
            prepend-icon="mdi-magic"
            @click="generateReport"
          >
            Generate Running Summary
          </v-btn>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script>
import { API_BASE_URL } from '../config'

export default {
  name: 'RunningSummaryView',

  props: {
    authToken: {
      type: String,
      required: true
    }
  },

  emits: ['updated'],

  data() {
    return {
      windowDays: 30,
      data: null,
      loading: false,
      generating: false,
      error: ''
    }
  },

  computed: {
    hasData() {
      return this.data !== null && this.data.summary
    },

    sentimentPercentage() {
      if (!this.data) return 50
      return ((this.data.sentiment + 1) / 2) * 100
    },

    sentimentColor() {
      if (!this.data) return '#9E9E9E'
      if (this.data.sentiment > 0.3) return '#4CAF50'
      if (this.data.sentiment < -0.3) return '#EF5350'
      return '#9E9E9E'
    },

    sentimentClass() {
      if (!this.data) return ''
      if (this.data.sentiment > 0.3) return 'sentiment-positive'
      if (this.data.sentiment < -0.3) return 'sentiment-negative'
      return 'sentiment-neutral'
    },

    sentimentLabel() {
      if (!this.data) return 'Neutral'
      const s = this.data.sentiment
      if (s > 0.5) return 'Very Positive'
      if (s > 0.3) return 'Positive'
      if (s > 0.1) return 'Slightly Positive'
      if (s < -0.5) return 'Very Negative'
      if (s < -0.3) return 'Negative'
      if (s < -0.1) return 'Slightly Negative'
      return 'Neutral'
    },

    hasOutlook() {
      if (!this.data) return false
      if (this.data.portfolio_outlook && this.data.portfolio_outlook.scenarios) return true
      const summaryText = (this.data.summary || '').toLowerCase()
      return summaryText.includes('portfolio outlook')
    },

    outlookScenarios() {
      if (this.data && this.data.portfolio_outlook && this.data.portfolio_outlook.scenarios) {
        return this.data.portfolio_outlook.scenarios
      }
      const defaults = [
        { label: 'Very Positive', probability: 15, description: 'Strong market tailwinds across all holdings' },
        { label: 'Positive', probability: 30, description: 'Favorable conditions for key positions' },
        { label: 'Neutral', probability: 30, description: 'Mixed signals with sideways movement expected' },
        { label: 'Negative', probability: 20, description: 'Headwinds from macroeconomic factors' },
        { label: 'Very Negative', probability: 5, description: 'Severe market stress scenario' }
      ]
      const summaryText = this.data?.summary || ''
      const outlookMatch = summaryText.match(/## Portfolio Outlook\s*\n([\s\S]*?)(?:\n##|\n*$)/)
      if (outlookMatch) {
        const lines = outlookMatch[1].split('\n').filter(l => l.trim().startsWith('- **'))
        if (lines.length > 0) {
          return lines.map(line => {
            const match = line.match(/\*\*(.+?)\*\*\s*\((\d+)%\):\s*(.+)/)
            if (match) {
              return { label: match[1].trim(), probability: parseInt(match[2]), description: match[3].trim() }
            }
            return null
          }).filter(Boolean)
        }
      }
      return defaults
    }
  },

  mounted() {
    this.fetchLatest()
  },

  methods: {
    async fetchLatest() {
      this.loading = true
      this.error = ''
      try {
        const resp = await fetch(
          `${API_BASE_URL}/api/running-summary?window_days=${this.windowDays}`,
          { headers: { 'Authorization': `Bearer ${this.authToken}` } }
        )
        if (resp.status === 404) {
          this.data = null
          return
        }
        if (!resp.ok) throw new Error(`Server returned ${resp.status}`)
        this.data = await resp.json()
      } catch (e) {
        this.error = `Failed to load running summary: ${e.message}`
        this.data = null
      } finally {
        this.loading = false
      }
    },

    async generateReport() {
      this.generating = true
      this.error = ''
      try {
        const resp = await fetch(
          `${API_BASE_URL}/api/running-summary/generate`,
          {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${this.authToken}`,
              'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: `window_days=${this.windowDays}`
          }
        )
        if (resp.status === 429) {
          const err = await resp.json()
          this.error = `Please wait ${err.retry_after_min} minute(s) before generating another summary`
          return
        }
        if (!resp.ok) {
          const err = await resp.json()
          throw new Error(err.error || `Server returned ${resp.status}`)
        }
        const result = await resp.json()
        this.data = result
        this.$emit('updated')
      } catch (e) {
        this.error = `Failed to generate running summary: ${e.message}`
      } finally {
        this.generating = false
      }
    },

    onWindowChange() {
      this.data = null
      this.fetchLatest()
    },

    getScenarioColor(label) {
      switch (label) {
        case 'Very Positive': return '#2E7D32'
        case 'Positive': return '#4CAF50'
        case 'Neutral': return '#9E9E9E'
        case 'Negative': return '#EF5350'
        case 'Very Negative': return '#C62828'
        default: return '#9E9E9E'
      }
    },

    getSentimentColor(score) {
      if (score > 0.3) return '#4CAF50'
      if (score < -0.3) return '#EF5350'
      return '#9E9E9E'
    },

    getSentimentLabel(score) {
      if (score > 0.5) return 'Very Positive'
      if (score > 0.3) return 'Positive'
      if (score > 0.1) return 'Slightly Positive'
      if (score < -0.5) return 'Very Negative'
      if (score < -0.3) return 'Negative'
      if (score < -0.1) return 'Slightly Negative'
      return 'Neutral'
    },

    getThemeClass(sentiment) {
      if (sentiment > 0.3) return 'theme-positive'
      if (sentiment < -0.3) return 'theme-negative'
      return 'theme-neutral'
    },

    formatDate(dateString) {
      if (!dateString) return ''
      const d = new Date(dateString + 'T00:00:00')
      return d.toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' })
    },

    formatMarkdown(text) {
      if (!text) return ''
      return text
        .replace(/###\s+(.+?)(?:\n|$)/g, '<h4 class="text-subtitle-2 font-weight-bold mt-3 mb-1">$1</h4>')
        .replace(/##\s+(.+?)(?:\n|$)/g, '<h3 class="text-h6 font-weight-bold mt-3 mb-2">$1</h3>')
        .replace(/#\s+(.+?)(?:\n|$)/g, '<h2 class="text-h5 font-weight-bold mt-4 mb-2">$1</h2>')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        .replace(/^\s*-\s+(.+)$/gm, '<li class="ml-4">$1</li>')
        .replace(/(<li[^>]*>.*<\/li>\n?)+/g, '<ul class="mb-2">$&</ul>')
        .replace(/\n\n+/g, '<br><br>')
        .replace(/\n/g, '<br>')
    }
  }
}
</script>

<style scoped>
.overview-card {
  transition: all 0.3s ease;
}

.sentiment-positive {
  border-left: 4px solid #4CAF50;
}

.sentiment-negative {
  border-left: 4px solid #EF5350;
}

.sentiment-neutral {
  border-left: 4px solid #9E9E9E;
}

.theme-card {
  transition: all 0.2s ease;
  height: 100%;
}

.theme-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1) !important;
}

.theme-positive {
  border-left: 3px solid #4CAF50;
}

.theme-negative {
  border-left: 3px solid #EF5350;
}

.theme-neutral {
  border-left: 3px solid #9E9E9E;
}

.prediction-card {
  height: 100%;
  transition: all 0.2s ease;
}

.prediction-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1) !important;
}

.scenario-row {
  padding: 2px 0;
}

.scenario-chip {
  min-width: 80px;
  justify-content: center;
}

.scenario-desc {
  line-height: 1.3;
  padding-left: 2px;
}

.outlook-row {
  padding: 4px 0;
}

.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4) {
  margin-top: 12px;
  margin-bottom: 4px;
}

.markdown-content :deep(ul) {
  padding-left: 16px;
  margin-bottom: 8px;
}

.markdown-content :deep(li) {
  margin-bottom: 2px;
}
</style>
