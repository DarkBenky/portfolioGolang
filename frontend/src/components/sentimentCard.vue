<template>
  <v-card :class="['sentiment-card', sentimentClass]" elevation="2">
    <v-card-text>
      <div class="d-flex align-center mb-2">
        <v-icon :color="sentimentColor" size="small" class="mr-2">
          {{ sentimentIcon }}
        </v-icon>
        <span class="text-caption text-grey">{{ label }}</span>
        <v-spacer></v-spacer>
        <v-chip :color="sentimentColor" size="small" variant="tonal">
          {{ sentimentLabel }}
        </v-chip>
      </div>

      <div class="sentiment-gauge mb-3">
        <v-progress-linear
          :model-value="sentimentPercentage"
          :color="sentimentColor"
          height="12"
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

      <div v-if="summary" class="summary-text markdown-content" v-html="formattedSummary"></div>

      <div v-if="showTrend && trend !== null" class="d-flex align-center mt-2">
        <v-icon :color="trendColor" size="small" class="mr-1">
          {{ trendIcon }}
        </v-icon>
        <span class="text-caption" :style="{ color: trendColor }">
          {{ trendText }}
        </span>
      </div>

      <div v-if="date" class="text-caption text-grey mt-2">
        <v-icon size="x-small" class="mr-1">mdi-calendar</v-icon>
        {{ formatDate(date) }}
      </div>
    </v-card-text>

    <v-card-actions v-if="showDetails && $slots.actions">
      <slot name="actions"></slot>
    </v-card-actions>
  </v-card>
</template>

<script>
export default {
  name: 'SentimentCard',
  
  props: {
    label: {
      type: String,
      default: 'Sentiment'
    },
    sentimentScore: {
      type: Number,
      required: true,
      validator: (value) => value >= -1 && value <= 1
    },
    summary: {
      type: String,
      default: ''
    },
    date: {
      type: String,
      default: ''
    },
    trend: {
      type: Number,
      default: null
    },
    showTrend: {
      type: Boolean,
      default: false
    },
    showDetails: {
      type: Boolean,
      default: false
    },
    compact: {
      type: Boolean,
      default: false
    }
  },

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

    sentimentIcon() {
      if (this.sentimentScore > 0.3) return 'mdi-emoticon-happy'
      if (this.sentimentScore < -0.3) return 'mdi-emoticon-sad'
      return 'mdi-emoticon-neutral'
    },

    trendColor() {
      if (this.trend > 0) return '#4CAF50'
      if (this.trend < 0) return '#EF5350'
      return '#9E9E9E'
    },

    trendIcon() {
      if (this.trend > 0) return 'mdi-trending-up'
      if (this.trend < 0) return 'mdi-trending-down'
      return 'mdi-trending-neutral'
    },

    trendText() {
      if (this.trend > 0) return `Improving (+${this.trend.toFixed(2)})`
      if (this.trend < 0) return `Declining (${this.trend.toFixed(2)})`
      return 'Stable'
    },

    formattedSummary() {
      if (!this.summary) return ''
      
      let text = this.summary
        .replace(/^Here is a sample .+?:\s*/i, '')
        .replace(/^Here is .+?:\s*/i, '')
        .replace(/###\s*/g, '')
        .replace(/##\s*/g, '')
        .replace(/#\s*/g, '')
        .replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        .replace(/^\s*-\s+/gm, '• ')
        .replace(/\n\n+/g, '<br><br>')
        .replace(/\n/g, '<br>')
      
      return text
    }
  },

  methods: {
    formatDate(dateString) {
      if (!dateString) return ''
      
      const date = new Date(parseInt(dateString) * 1000)
      const now = new Date()
      const diffMs = now - date
      const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
      const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

      if (diffHours < 1) return 'Just now'
      if (diffHours < 24) return `${diffHours}h ago`
      if (diffDays === 1) return 'Yesterday'
      if (diffDays < 7) return `${diffDays} days ago`
      
      return date.toLocaleDateString()
    }
  }
}
</script>

<style scoped>
.sentiment-card {
  transition: all 0.3s ease;
  max-height: 500px;
}

.sentiment-card:hover {
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

.sentiment-gauge {
  padding: 0 4px;
}

.summary-text {
  max-height: 450px;
  overflow-y: auto;
  line-height: 1.5;
}

.markdown-content {
  font-size: 0.875rem !important;
  line-height: 1.6;
}

.markdown-content * {
  font-size: 0.875rem !important;
}

.markdown-content strong {
  font-weight: 600;
  color: rgba(255, 255, 255, 0.95);
}

.markdown-content em {
  font-style: italic;
}

.markdown-content br {
  line-height: 1.6;
}
</style>
