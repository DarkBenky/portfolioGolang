<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-4 flex-wrap ga-2">
      <div>
        <div class="text-h6">Articles</div>
        <div class="text-caption text-medium-emphasis">
          Stored articles, summaries and their embeddings - nothing here is ever deleted
        </div>
      </div>
      <v-btn size="small" variant="outlined" prepend-icon="mdi-refresh" :loading="loading" @click="reload">
        Refresh
      </v-btn>
    </div>

    <div class="panel pa-4 mb-2">
      <div class="d-flex align-center flex-wrap ga-3">
        <div class="flex-grow-1" style="min-width: 220px">
          <div class="metric-label">Embedding coverage</div>
          <div class="d-flex align-baseline ga-2">
            <span class="metric-value">{{ formatNumber(embedding.embedded) }}</span>
            <span class="text-caption text-medium-emphasis">of {{ formatNumber(embedding.total) }} chunks</span>
          </div>
          <v-progress-linear
            :model-value="embedding.total ? (embedding.embedded / embedding.total) * 100 : 0"
            color="teal"
            height="6"
            rounded
            class="mt-1"
          />
          <div class="text-caption text-medium-emphasis mt-1">
            {{ formatNumber(embedding.pending) }} pending
            <span v-if="embedding.wrong_size"> · {{ formatNumber(embedding.wrong_size) }} from an older size</span>
            <span v-if="embedding.model"> · {{ embedding.model }}</span>
            <span v-if="embedding.dimensions"> · {{ embedding.dimensions }} dims</span>
          </div>
        </div>
        <v-btn
          size="small"
          color="teal"
          variant="flat"
          prepend-icon="mdi-vector-polyline"
          :loading="embedding.running"
          :disabled="!embedding.pending"
          @click="embedMissing"
        >{{ embedding.running ? 'Embedding...' : 'Embed missing' }}</v-btn>
      </div>
      <v-alert v-if="embedError" type="warning" variant="tonal" density="compact" class="mt-2">{{ embedError }}</v-alert>
    </div>

    <v-row dense class="mb-1">
      <v-col v-for="card in statCards" :key="card.label" cols="6" md="3">
        <div class="panel pa-4">
          <div class="metric-label">{{ card.label }}</div>
          <div class="metric-value">{{ card.value }}</div>
          <div class="text-caption text-medium-emphasis">{{ card.hint }}</div>
        </div>
      </v-col>
    </v-row>

    <div class="panel pa-3 mt-2">
      <div class="d-flex flex-wrap ga-2 align-center">
        <v-text-field
          v-model="query"
          density="compact"
          variant="outlined"
          hide-details
          clearable
          prepend-inner-icon="mdi-magnify"
          label="Search article text"
          class="flex-grow-1"
          style="min-width: 220px"
          @update:model-value="debouncedSearch"
        ></v-text-field>
        <v-select
          v-model="source"
          :items="sourceOptions"
          density="compact"
          variant="outlined"
          hide-details
          label="Source"
          style="max-width: 220px"
          @update:model-value="reload"
        ></v-select>
        <v-select
          v-model="ticker"
          :items="tickerOptions"
          density="compact"
          variant="outlined"
          hide-details
          label="Ticker"
          style="max-width: 170px"
          @update:model-value="reload"
        ></v-select>
      </div>
      <div v-if="stats.sources?.length" class="d-flex flex-wrap ga-1 mt-3">
        <v-chip
          v-for="entry in stats.sources"
          :key="entry.source"
          size="small"
          :color="source === entry.source ? 'primary' : undefined"
          :variant="source === entry.source ? 'flat' : 'tonal'"
          @click="toggleSource(entry.source)"
        >
          {{ entry.source }} · {{ formatNumber(entry.count) }}
        </v-chip>
      </div>
    </div>

    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mt-3">{{ error }}</v-alert>
    <v-progress-linear v-if="loading && !articles.length" indeterminate color="primary" class="mt-3" />

    <div class="mt-3">
      <div v-for="article in articles" :key="article.id" class="panel pa-3 mb-2 article-card">
        <div class="d-flex align-center flex-wrap ga-2 mb-1">
          <v-chip size="x-small" color="primary" variant="tonal">{{ article.source }}</v-chip>
          <v-chip v-if="article.ticker" size="x-small" variant="outlined">{{ article.ticker }}</v-chip>
          <span v-if="article.date" class="text-caption text-medium-emphasis">{{ article.date }}</span>
          <v-chip
            size="x-small"
            :color="article.has_vector ? 'teal' : 'grey'"
            variant="tonal"
          >{{ article.has_vector ? 'embedded' : 'text only' }}</v-chip>
          <v-spacer></v-spacer>
          <span class="text-caption text-medium-emphasis">{{ relativeDate(article.created_at) }}</span>
        </div>
        <div class="article-text" :class="{ expanded: expanded[article.id] }">{{ article.content }}</div>
        <v-btn
          v-if="article.content.length > 320"
          size="x-small"
          variant="text"
          class="mt-1"
          @click="expanded[article.id] = !expanded[article.id]"
        >{{ expanded[article.id] ? 'Show less' : 'Show more' }}</v-btn>
      </div>
      <div v-if="!loading && !articles.length" class="text-caption text-medium-emphasis py-6 text-center">
        Nothing matches this filter.
      </div>
      <div class="d-flex align-center justify-space-between mt-2 flex-wrap ga-2">
        <span class="text-caption text-medium-emphasis">{{ rangeLabel }}</span>
        <v-btn
          v-if="articles.length < total"
          size="small"
          variant="outlined"
          :loading="loading"
          @click="loadMore"
        >Load more</v-btn>
      </div>
    </div>
  </div>
</template>

<script>
import { apiGet, apiSend } from '../utils/api'

const PAGE = 40

export default {
  name: 'ArticlesView',
  data() {
    return {
      articles: [],
      total: 0,
      stats: {},
      query: '',
      source: '',
      ticker: '',
      loading: false,
      error: '',
      expanded: {},
      searchTimer: null,
      embedding: { embedded: 0, total: 0, pending: 0, wrong_size: 0, running: false },
      embedError: '',
      embedTimer: null,
    }
  },
  computed: {
    statCards() {
      return [
        { label: 'Stored chunks', value: this.formatNumber(this.stats.total), hint: `${this.shortDate(this.stats.oldest_date)} to ${this.shortDate(this.stats.newest_date)}` },
        { label: 'With embeddings', value: this.formatNumber(this.embedding.embedded), hint: `${this.formatNumber(this.embedding.pending)} pending` },
        { label: 'Article text', value: this.formatBytes(this.stats.content_bytes), hint: 'original content' },
        { label: 'Embedding data', value: this.formatBytes(this.stats.vector_bytes), hint: 'never pruned' },
      ]
    },
    sourceOptions() {
      return [{ title: 'All sources', value: '' }, ...(this.stats.sources || []).map((entry) => ({
        title: `${entry.source} (${entry.count})`,
        value: entry.source,
      }))]
    },
    tickerOptions() {
      return [{ title: 'All tickers', value: '' }, ...(this.stats.tickers || []).map((entry) => ({
        title: `${entry.ticker} (${entry.count})`,
        value: entry.ticker,
      }))]
    },
    rangeLabel() {
      if (!this.articles.length) return 'No rows'
      return `Showing ${this.articles.length} of ${this.formatNumber(this.total)}`
    },
  },
  mounted() {
    this.reload()
  },
  beforeUnmount() {
    clearTimeout(this.embedTimer)
  },
  methods: {
    async reload() {
      await Promise.all([this.loadArticles(), this.loadStats()])
    },
    async loadArticles(append = false) {
      this.loading = true
      this.error = ''
      try {
        const params = new URLSearchParams({ limit: PAGE, offset: append ? this.articles.length : 0 })
        if (this.query) params.set('query', this.query)
        if (this.source) params.set('source', this.source)
        if (this.ticker) params.set('ticker', this.ticker)
        const data = await apiGet(`/api/articles?${params}`)
        this.articles = append ? [...this.articles, ...data.articles] : data.articles
        this.total = data.total
      } catch (err) {
        this.error = err.message
      } finally {
        this.loading = false
      }
    },
    async loadStats() {
      try {
        this.stats = await apiGet('/api/articles/stats')
        this.embedding = this.stats.embedding || this.embedding
        this.syncEmbedPolling()
      } catch (err) {
        this.error = err.message
      }
    },
    // Poll only while a backfill is in flight, so an idle tab makes no requests.
    syncEmbedPolling() {
      clearTimeout(this.embedTimer)
      if (!this.embedding.running) return
      this.embedTimer = setTimeout(() => this.loadStats(), 2500)
    },
    async embedMissing() {
      this.embedError = ''
      try {
        const result = await apiSend('/api/articles/embed', 'POST', {})
        this.embedding = result.embedding || this.embedding
        this.embedding.running = true
        this.syncEmbedPolling()
      } catch (err) {
        this.embedError = err.message
      }
    },
    loadMore() {
      this.loadArticles(true)
    },
    toggleSource(value) {
      this.source = this.source === value ? '' : value
      this.reload()
    },
    debouncedSearch() {
      clearTimeout(this.searchTimer)
      this.searchTimer = setTimeout(() => this.reload(), 350)
    },
    formatNumber(value) {
      return new Intl.NumberFormat('en-US').format(Number(value) || 0)
    },
    shortDate(value) {
      const raw = String(value || '')
      if (!raw) return '-'
      if (/^\d{9,}$/.test(raw)) return new Date(Number(raw) * 1000).toISOString().slice(0, 10)
      return raw.length > 10 ? raw.slice(0, 10) : raw
    },
    formatBytes(value) {
      const bytes = Number(value) || 0
      if (bytes > 1e9) return `${(bytes / 1e9).toFixed(2)} GB`
      if (bytes > 1e6) return `${(bytes / 1e6).toFixed(1)} MB`
      if (bytes > 1e3) return `${(bytes / 1e3).toFixed(0)} kB`
      return `${bytes} B`
    },
    relativeDate(unix) {
      const timestamp = Number(unix)
      if (!timestamp) return ''
      const hours = Math.round((Date.now() / 1000 - timestamp) / 3600)
      if (hours < 1) return 'just now'
      if (hours < 24) return `${hours}h ago`
      return `${Math.round(hours / 24)}d ago`
    },
  },
}
</script>

<style scoped>
.article-card {
  border-left: 2px solid rgba(var(--v-theme-primary), 0.5);
}

.article-text {
  font-size: 13px;
  line-height: 1.5;
  color: rgba(255, 255, 255, 0.82);
  white-space: pre-wrap;
  display: -webkit-box;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.article-text.expanded {
  display: block;
  -webkit-line-clamp: unset;
}
</style>
