<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">News</div>
        <div class="text-caption text-medium-emphasis">Portfolio headlines and the daily summary</div>
      </div>
      <v-btn
        size="small"
        variant="outlined"
        prepend-icon="mdi-text-box-plus-outline"
        :loading="generating"
        @click="generateSummary"
      >
        Generate summary
      </v-btn>
    </div>

    <v-row dense>
      <v-col cols="12" lg="4">
        <div class="panel pa-4">
          <div class="metric-label">Today's sentiment</div>
          <div class="metric-value mb-1" :class="`text-${sentimentColor}`">{{ sentimentLabel }}</div>
          <div class="text-caption text-medium-emphasis">{{ sentimentCount }} items</div>
          <v-divider class="my-3"></v-divider>
          <div v-if="summary" class="text-body-2 summary-text">{{ summary.summary }}</div>
          <div v-else class="text-caption text-medium-emphasis">No daily summary stored yet.</div>
        </div>
      </v-col>
      <v-col cols="12" lg="8">
        <div class="panel pa-2">
          <news-feed
            :news-items="news"
            :loading="loading"
            :loading-more="loadingMore"
            :error="error"
            :has-more="hasMore"
            @load-more="loadMore"
            @retry="load"
          />
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<script>
import NewsFeed from '../components/newsFeed.vue'
import { apiGet, apiSend } from '../utils/api'

export default {
  name: 'NewsView',
  components: { NewsFeed },
  data() {
    return {
      news: [],
      sentiment: 0,
      sentimentCount: 0,
      summary: null,
      loading: false,
      loadingMore: false,
      generating: false,
      error: '',
      limit: 20,
    }
  },
  computed: {
    hasMore() {
      return this.news.length > 0 && this.news.length % this.limit === 0
    },
    sentimentLabel() {
      if (!this.sentimentCount) return 'No data'
      if (this.sentiment > 0.2) return 'Bullish'
      if (this.sentiment < -0.2) return 'Bearish'
      return 'Neutral'
    },
    sentimentColor() {
      if (this.sentiment > 0.2) return 'success'
      if (this.sentiment < -0.2) return 'error'
      return 'medium-emphasis'
    },
  },
  mounted() {
    this.load()
  },
  methods: {
    async load() {
      this.loading = true
      this.error = ''
      try {
        const [news, sentiment] = await Promise.all([
          apiGet(`/api/portfolio/news?limit=${this.limit}&offset=0`),
          apiGet('/api/portfolio/sentiment'),
          this.loadSummary(),
        ])
        this.news = news || []
        this.sentiment = sentiment.average_sentiment || 0
        this.sentimentCount = sentiment.news_count || 0
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
    async loadSummary() {
      const today = new Date().toISOString().slice(0, 10)
      try {
        this.summary = await apiGet(`/api/portfolio/daily_sentiment?date=${today}`)
      } catch {
        this.summary = null
      }
    },
    async loadMore() {
      this.loadingMore = true
      try {
        const more = await apiGet(`/api/portfolio/news?limit=${this.limit}&offset=${this.news.length}`)
        const known = new Set(this.news.map((item) => item.id_news))
        this.news.push(...(more || []).filter((item) => !known.has(item.id_news)))
      } catch (err) {
        this.error = err.message
      } finally {
        this.loadingMore = false
      }
    },
    async generateSummary() {
      this.generating = true
      this.error = ''
      try {
        await apiSend('/api/news/generate-summary', 'POST', {})
        await this.loadSummary()
      } catch (err) {
        this.error = err.message
      } finally {
        this.generating = false
      }
    },
  },
}
</script>

<style scoped>
.summary-text {
  white-space: pre-wrap;
  max-height: 420px;
  overflow-y: auto;
}
</style>
