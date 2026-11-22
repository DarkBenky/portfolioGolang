<template>
  <v-app>
    <LoginRegister v-if="!authenticated" />

    <template v-else>
      <v-app-bar flat density="compact" class="app-bar">
        <v-app-bar-nav-icon v-if="isMobile" @click="drawer = !drawer"></v-app-bar-nav-icon>
        <v-app-bar-title class="text-body-1 font-weight-medium">
          <v-icon size="18" class="mr-2">mdi-chart-line</v-icon>Portfolio
        </v-app-bar-title>
        <v-spacer></v-spacer>
        <v-select
          v-model="homeCurrency"
          :items="currencies"
          density="compact"
          variant="plain"
          hide-details
          class="currency-select"
          @update:model-value="persistCurrency"
        ></v-select>
        <span class="text-caption text-medium-emphasis mx-3 d-none d-sm-inline">{{ email }}</span>
        <v-btn icon size="small" variant="text" @click="logout">
          <v-icon size="18">mdi-logout</v-icon>
        </v-btn>
      </v-app-bar>

      <v-navigation-drawer
        v-model="drawer"
        :rail="!isMobile"
        :temporary="isMobile"
        :rail-width="72"
        class="nav-drawer"
      >
        <v-list nav density="compact">
          <v-list-item
            v-for="item in navigation"
            :key="item.value"
            :prepend-icon="item.icon"
            :title="item.title"
            :value="item.value"
            :active="view === item.value"
            class="nav-item"
            @click="selectView(item.value)"
          ></v-list-item>
        </v-list>
      </v-navigation-drawer>

      <v-main class="app-main">
        <v-container fluid class="pa-3 pa-md-6 pb-16 pb-md-6">
          <dashboard-view
            v-if="view === 'dashboard'"
            :currency="homeCurrency"
            @navigate="selectView"
            @open-ticker="openTicker"
          />
          <holdings-view v-else-if="view === 'holdings'" :currency="homeCurrency"
            :focus-ticker="focus.ticker" :focus-nonce="focus.nonce" />
          <news-view v-else-if="view === 'news'" />
          <allocation-view v-else-if="view === 'allocation'" :currency="homeCurrency"
            @open-ticker="openTicker" />
          <statistics-view v-else-if="view === 'statistics'" :currency="homeCurrency"
            @open-ticker="openTicker" />
          <trading-viewer v-else-if="view === 'trading'"
            :focus-ticker="focus.ticker" :focus-nonce="focus.nonce" />
          <running-summary-view v-else-if="view === 'summary'" :auth-token="token" />
          <expenses-view v-else-if="view === 'expenses'" />
          <chat-view v-else-if="view === 'chat'" />
          <articles-view v-else-if="view === 'articles'" />
          <situation-view v-else-if="view === 'situations'" />
        </v-container>
      </v-main>

      <v-bottom-navigation
        v-if="isMobile"
        :model-value="view"
        grow
        color="primary"
        bg-color="#1a1a1a"
        class="bottom-nav"
        @update:model-value="onBottomNav"
      >
        <v-btn v-for="item in mobileNavigation" :key="item.value" :value="item.value" size="small">
          <v-icon size="20">{{ item.icon }}</v-icon>
        </v-btn>
        <v-btn value="more" size="small">
          <v-icon size="20">mdi-dots-grid</v-icon>
        </v-btn>
      </v-bottom-navigation>
    </template>
  </v-app>
</template>

<script>
import LoginRegister from './components/loginRegister.vue'
import ExpensesView from './components/expensesView.vue'
import TradingViewer from './components/tradingViewer.vue'
import ChatView from './components/chatView.vue'
import SituationView from './components/situationView.vue'
import RunningSummaryView from './components/RunningSummaryView.vue'
import DashboardView from './views/dashboardView.vue'
import HoldingsView from './views/holdingsView.vue'
import NewsView from './views/newsView.vue'
import AllocationView from './views/allocationView.vue'
import StatisticsView from './views/statisticsView.vue'
import ArticlesView from './views/articlesView.vue'
import { authToken, clearAuth, getCookie } from './utils/api'

export default {
  name: 'App',
  components: {
    LoginRegister,
    ExpensesView,
    TradingViewer,
    ChatView,
    SituationView,
    RunningSummaryView,
    DashboardView,
    HoldingsView,
    NewsView,
    AllocationView,
    StatisticsView,
    ArticlesView,
  },
  data() {
    return {
      authenticated: Boolean(authToken()),
      token: authToken(),
      email: getCookie('user_email') || '',
      homeCurrency: localStorage.getItem('homeCurrency') || 'EUR',
      currencies: ['EUR', 'USD', 'CHF', 'GBP'],
      drawer: !this.isMobileViewport(),
      view: localStorage.getItem('lastView') || 'dashboard',
      focus: { ticker: '', nonce: 0 },
      navigation: [
        { value: 'dashboard', title: 'Dashboard', icon: 'mdi-view-dashboard-outline' },
        { value: 'holdings', title: 'Holdings', icon: 'mdi-briefcase-outline' },
        { value: 'news', title: 'News', icon: 'mdi-newspaper' },
        { value: 'allocation', title: 'Allocation', icon: 'mdi-chart-donut' },
        { value: 'statistics', title: 'Statistics', icon: 'mdi-chart-box-outline' },
        { value: 'trading', title: 'Trading View', icon: 'mdi-finance' },
        { value: 'summary', title: 'Summary', icon: 'mdi-text-box-outline' },
        { value: 'expenses', title: 'Expenses', icon: 'mdi-wallet-outline' },
        { value: 'articles', title: 'Articles', icon: 'mdi-database-search-outline' },
        { value: 'chat', title: 'Chat', icon: 'mdi-message-text-outline' },
        { value: 'situations', title: 'Situations', icon: 'mdi-earth' },
      ],
    }
  },
  computed: {
    isMobile() {
      return this.$vuetify.display.mobile
    },
    mobileNavigation() {
      const keep = ['dashboard', 'holdings', 'news', 'chat']
      return this.navigation.filter((item) => keep.includes(item.value))
    },
  },
  watch: {
    isMobile(mobile) {
      this.drawer = !mobile
    },
  },
  methods: {
    isMobileViewport() {
      return typeof window !== 'undefined' && window.innerWidth < 960
    },
    selectView(value) {
      this.view = value
      localStorage.setItem('lastView', value)
      if (this.isMobile) this.drawer = false
    },
    // Any ticker rendered by a view can be opened in the holdings table or on the
    // trading chart. The nonce makes a repeated click re-trigger the target.
    openTicker(payload) {
      const ticker = typeof payload === 'string' ? payload : payload?.ticker
      if (!ticker) return
      const target = typeof payload === 'string' ? 'trading' : payload.target || 'trading'
      this.focus = { ticker, nonce: this.focus.nonce + 1 }
      this.selectView(target)
    },
    onBottomNav(value) {
      if (value === 'more') {
        this.drawer = true
        return
      }
      this.selectView(value)
    },
    persistCurrency(value) {
      localStorage.setItem('homeCurrency', value)
    },
    logout() {
      clearAuth()
      this.authenticated = false
      this.token = ''
      window.location.href = '/'
    },
  },
}
</script>

<style>
.app-bar {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.currency-select {
  max-width: 84px;
}

.currency-select .v-field__input {
  min-height: 28px;
  padding-top: 4px;
  padding-bottom: 4px;
  font-size: 13px;
}

.nav-drawer {
  border-right: 1px solid rgba(255, 255, 255, 0.08);
}

.nav-item {
  border-radius: 8px;
  margin: 2px 8px;
}

.app-main {
  background: #121212;
  min-height: 100vh;
}

.bottom-nav {
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.metric-label {
  font-size: 11px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.55);
}

.metric-value {
  font-size: 22px;
  font-weight: 600;
  line-height: 1.2;
}

.panel {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: #1a1a1a;
}

.equal-rows {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
}

.equal-rows > * {
  height: 100%;
}

.markdown-content {
  font-size: 13px;
  line-height: 1.55;
  color: rgba(255, 255, 255, 0.85);
  overflow-wrap: anywhere;
}

.markdown-content p {
  margin: 0 0 8px;
}

.markdown-content p:last-child {
  margin-bottom: 0;
}

.markdown-content h2,
.markdown-content h3,
.markdown-content h4,
.markdown-content h5 {
  margin: 12px 0 6px;
  font-weight: 600;
  line-height: 1.3;
}

.markdown-content h2 {
  font-size: 15px;
}

.markdown-content h3 {
  font-size: 14px;
}

.markdown-content h4,
.markdown-content h5 {
  font-size: 13px;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.6);
}

.markdown-content ul,
.markdown-content ol {
  margin: 0 0 8px;
  padding-left: 18px;
}

.markdown-content li {
  margin: 2px 0;
}

.markdown-content hr {
  border: none;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  margin: 10px 0;
}

.markdown-content a {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}

.markdown-content a:hover {
  text-decoration: underline;
}

.markdown-content .inline-code {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 3px;
  padding: 0 3px;
  font-size: 12px;
}

.ticker-link {
  cursor: pointer;
  color: rgba(255, 255, 255, 0.92);
  font-weight: 600;
  text-decoration: none;
}

.ticker-link:hover {
  color: rgb(var(--v-theme-primary));
  text-decoration: underline;
}

@media (max-width: 600px) {
  .metric-value {
    font-size: 18px;
  }
}
</style>
