<template>
  <v-card :class="['running-summary-card', sentimentClass]" elevation="2">
    <v-card-text class="pa-3">
      <div class="d-flex align-center mb-2">
        <v-icon :color="sentimentColor" size="small" class="mr-2">
          mdi-chart-timeline-variant
        </v-icon>
        <span class="text-caption text-grey font-weight-medium">RUNNING SUMMARY</span>
        <v-spacer></v-spacer>
        <v-chip size="x-small" variant="outlined" class="mr-1">
          {{ windowDays }}d
        </v-chip>
        <v-chip :color="sentimentColor" size="x-small" variant="tonal">
          {{ sentimentLabel }}
        </v-chip>
      </div>

      <div class="sentiment-gauge mb-2">
        <v-progress-linear
          :model-value="sentimentPercentage"
          :color="sentimentColor"
          height="8"
          rounded
        ></v-progress-linear>
        <div class="d-flex justify-space-between mt-1">
          <span class="text-caption text-grey">Negative</span>
          <span class="text-caption font-weight-bold" :style="{ color: sentimentColor }">
            {{ sentimentScore.toFixed(2) }}
          </span>
          <span class="text-caption text-grey">Positive</span>
        </div>
      </div>

      <div v-if="summary" class="summary-preview text-caption text-grey-darken-2 mb-2">
        {{ summaryPreview }}
      </div>

      <div v-if="keyThemeChips.length > 0" class="d-flex flex-wrap ga-1 mb-2">
        <v-chip
          v-for="(theme, i) in keyThemeChips"
          :key="i"
          size="x-small"
          variant="tonal"
          :color="theme.sentiment > 0 ? 'success' : theme.sentiment < 0 ? 'error' : 'grey'"
        >
          {{ theme.name }}
        </v-chip>
      </div>

      <div v-if="date" class="d-flex align-center justify-space-between">
        <span class="text-caption text-grey">
          <v-icon size="x-small" class="mr-1">mdi-calendar</v-icon>
          {{ formatDate(date) }}
        </span>
        <v-btn
          size="x-small"
          variant="text"
          color="primary"
          @click="$emit('view-full')"
        >
          Full Report
          <v-icon size="x-small" class="ml-1">mdi-arrow-right</v-icon>
        </v-btn>
      </div>
    </v-card-text>

    <v-card-actions v-if="!hasData" class="justify-center pa-2">
      <v-btn
        size="small"
        variant="tonal"
        color="primary"
        :loading="generating"
        @click="$emit('generate')"
        prepend-icon="mdi-magic"
      >
        Generate Running Summary
      </v-btn>
    </v-card-actions>
  </v-card>
</template>

<script>
export default {
  name: 'RunningSummaryCard',

  props: {
    sentimentScore: {
      type: Number,
      default: 0
    },
    summary: {
      type: String,
      default: ''
    },
    date: {
      type: String,
      default: ''
    },
    windowDays: {
      type: Number,
      default: 30
    },
    keyThemes: {
      type: Array,
      default: () => []
    },
    generating: {
      type: Boolean,
      default: false
    },
    hasData: {
      type: Boolean,
      default: false
    }
  },

  emits: ['view-full', 'generate'],

  computed: {
    sentimentPercentage() {
      return ((this.sentimentScore + 1) / 2) * 100
    },

    sentimentColor() {
      if (this.sentimentScore > 0.3) return '#4CAF50'
      if (this.sentimentScore < -0.3) return '#EF5350'
      return '#9E9E9E'
    },

    sentimentClass() {
      if (this.sentimentScore > 0.3) return 'sentiment-positive'
      if (this.sentimentScore < -0.3) return 'sentiment-negative'
      return 'sentiment-neutral'
    },

    sentimentLabel() {
      if (this.sentimentScore > 0.5) return 'Very Positive'
      if (this.sentimentScore > 0.3) return 'Positive'
      if (this.sentimentScore > 0.1) return 'Slightly Positive'
      if (this.sentimentScore < -0.5) return 'Very Negative'
      if (this.sentimentScore < -0.3) return 'Negative'
      if (this.sentimentScore < -0.1) return 'Slightly Negative'
      return 'Neutral'
    },

    summaryPreview() {
      if (!this.summary) return ''
      const clean = this.summary.replace(/[#*]/g, '').trim()
      return clean.length > 200 ? clean.substring(0, 200) + '...' : clean
    },

    keyThemeChips() {
      if (!this.keyThemes || this.keyThemes.length === 0) {
        const themes = []
        const summaryText = this.summary || ''
        const themesSection = summaryText.split('## Sector Predictions')[0]
        const themeRegex = /###\s+(.+?)(?:\n|$)/g
        let match
        while ((match = themeRegex.exec(themesSection)) !== null) {
          const name = match[1].trim()
          if (!['Key Themes', 'Executive Summary', 'Portfolio Outlook'].includes(name)) {
            themes.push({ name, sentiment: 0 })
          }
          if (themes.length >= 3) break
        }
        return themes
      }
      return this.keyThemes.slice(0, 3).map(t => ({
        name: t.theme || t.name || '',
        sentiment: t.impact_sentiment || t.sentiment || 0
      }))
    }
  },

  methods: {
    formatDate(dateString) {
      if (!dateString) return ''
      const d = new Date(dateString + 'T00:00:00')
      const now = new Date()
      const diffDays = Math.floor((now - d) / (1000 * 60 * 60 * 24))
      if (diffDays === 0) return 'Today'
      if (diffDays === 1) return 'Yesterday'
      if (diffDays < 7) return `${diffDays} days ago`
      return d.toLocaleDateString()
    }
  }
}
</script>

<style scoped>
.running-summary-card {
  transition: all 0.3s ease;
}

.running-summary-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
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

.summary-preview {
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
