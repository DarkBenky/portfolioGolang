<template>
  <div class="news-feed">
    <div v-if="loading" class="d-flex justify-center py-8">
      <v-progress-circular indeterminate color="primary" size="48"></v-progress-circular>
    </div>

    <div v-else-if="error" class="text-center py-8">
      <v-icon size="64" color="error">mdi-alert-circle</v-icon>
      <p class="text-error mt-4">{{ error }}</p>
      <v-btn color="primary" @click="$emit('retry')">Retry</v-btn>
    </div>

    <div v-else-if="newsItems.length === 0" class="text-center py-8">
      <v-icon size="64" color="grey-lighten-1">mdi-newspaper-variant-outline</v-icon>
      <p class="text-grey mt-4">No news available</p>
    </div>

    <div v-else>
      <v-card
        v-for="item in newsItems"
        :key="item.id_news"
        class="news-item mb-3"
        elevation="1"
        :class="{ 'news-item-expanded': expandedNews[item.id_news] }"
      >
        <v-card-text class="pb-2">
          <div class="d-flex align-center mb-2">
            <v-chip
              v-if="showTicker"
              size="small"
              color="primary"
              variant="tonal"
              class="mr-2"
            >
              {{ item.ticker }}
            </v-chip>
            
            <v-chip
              :color="getSentimentColor(item.sentiment)"
              size="small"
              variant="flat"
              class="mr-2"
            >
              <v-icon size="x-small" class="mr-1">
                {{ getSentimentIcon(item.sentiment) }}
              </v-icon>
              {{ getSentimentLabel(item.sentiment) }}
            </v-chip>

            <v-spacer></v-spacer>

            <span class="text-caption text-grey">
              <v-icon size="x-small" class="mr-1">mdi-clock-outline</v-icon>
              {{ formatTimeAgo(item.published_at) }}
            </span>
          </div>

          <h3 class="news-title mb-2">
            <a :href="item.link" target="_blank" rel="noopener noreferrer" class="news-link">
              {{ item.title }}
              <v-icon size="small" class="ml-1">mdi-open-in-new</v-icon>
            </a>
          </h3>

          <p class="text-body-2 text-grey-darken-1 mb-2">
            {{ item.summary }}
          </p>

          <v-expand-transition>
            <div v-if="expandedNews[item.id_news]" class="mt-3">
              <v-divider class="mb-3"></v-divider>
              <div class="news-full-text">
                <p class="text-body-2">{{ item.text }}</p>
              </div>
              <div v-if="item.author" class="text-caption text-grey mt-2">
                <v-icon size="x-small" class="mr-1">mdi-account</v-icon>
                Author: {{ item.author }}
              </div>
            </div>
          </v-expand-transition>
        </v-card-text>

        <v-card-actions>
          <v-btn
            size="small"
            variant="text"
            @click="toggleExpand(item.id_news)"
          >
            {{ expandedNews[item.id_news] ? 'Show Less' : 'Read More' }}
            <v-icon size="small" class="ml-1">
              {{ expandedNews[item.id_news] ? 'mdi-chevron-up' : 'mdi-chevron-down' }}
            </v-icon>
          </v-btn>

          <v-spacer></v-spacer>

          <v-btn
            size="small"
            variant="text"
            :href="item.link"
            target="_blank"
            rel="noopener noreferrer"
          >
            Open Source
            <v-icon size="small" class="ml-1">mdi-open-in-new</v-icon>
          </v-btn>
        </v-card-actions>
      </v-card>

      <div v-if="hasMore" class="d-flex justify-center mt-4">
        <v-btn
          color="primary"
          variant="outlined"
          :loading="loadingMore"
          @click="$emit('load-more')"
        >
          Load More News
        </v-btn>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NewsFeed',

  props: {
    newsItems: {
      type: Array,
      default: () => []
    },
    loading: {
      type: Boolean,
      default: false
    },
    loadingMore: {
      type: Boolean,
      default: false
    },
    error: {
      type: String,
      default: ''
    },
    hasMore: {
      type: Boolean,
      default: false
    },
    showTicker: {
      type: Boolean,
      default: true
    }
  },

  emits: ['load-more', 'retry'],

  data() {
    return {
      expandedNews: {}
    }
  },

  methods: {
    toggleExpand(newsId) {
      this.expandedNews[newsId] = !this.expandedNews[newsId]
    },

    getSentimentColor(sentiment) {
      if (sentiment > 0.3) return '#4CAF50'
      if (sentiment < -0.3) return '#EF5350'
      return '#9E9E9E'
    },

    getSentimentIcon(sentiment) {
      if (sentiment > 0.3) return 'mdi-thumb-up'
      if (sentiment < -0.3) return 'mdi-thumb-down'
      return 'mdi-minus'
    },

    getSentimentLabel(sentiment) {
      if (sentiment > 0.5) return 'Very Positive'
      if (sentiment > 0.3) return 'Positive'
      if (sentiment > 0.1) return 'Slightly Positive'
      if (sentiment < -0.5) return 'Very Negative'
      if (sentiment < -0.3) return 'Negative'
      if (sentiment < -0.1) return 'Slightly Negative'
      return 'Neutral'
    },

    formatTimeAgo(timestamp) {
      if (!timestamp) return ''
      
      const date = new Date(parseInt(timestamp) * 1000)
      const now = new Date()
      const diffMs = now - date
      const diffMinutes = Math.floor(diffMs / (1000 * 60))
      const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
      const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

      if (diffMinutes < 1) return 'Just now'
      if (diffMinutes < 60) return `${diffMinutes}m ago`
      if (diffHours < 24) return `${diffHours}h ago`
      if (diffDays === 1) return 'Yesterday'
      if (diffDays < 7) return `${diffDays}d ago`
      
      return date.toLocaleDateString()
    }
  }
}
</script>

<style scoped>
.news-feed {
  width: 100%;
}

.news-item {
  transition: all 0.3s ease;
  border-left: 3px solid transparent;
}

.news-item:hover {
  border-left-color: var(--v-primary-base);
  transform: translateX(4px);
}

.news-item-expanded {
  border-left-color: var(--v-primary-base);
}

.news-title {
  font-size: 1.1rem;
  font-weight: 600;
  line-height: 1.4;
}

.news-link {
  color: inherit;
  text-decoration: none;
  transition: color 0.2s;
}

.news-link:hover {
  color: var(--v-primary-base);
}

.news-full-text {
  max-height: 400px;
  overflow-y: auto;
  padding: 8px 0;
  line-height: 1.6;
}
</style>
