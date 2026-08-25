<template>
  <v-app>
    <!-- Show login/register if not authenticated -->
    <LoginRegister v-if="!isAuthenticated" />

    <!-- Show main app if authenticated -->
    <template v-else>
      <v-app-bar color="primary" dark>
        <v-app-bar-title>Portfolio Tracker</v-app-bar-title>
        <v-spacer></v-spacer>
        <v-select
          v-model="homeCurrency"
          :items="['EUR', 'USD', 'CHF', 'GBP']"
          label="Home Currency"
          variant="outlined"
          density="compact"
          hide-details
          style="max-width: 120px;"
          class="mr-4"
        ></v-select>
        <span class="mr-4">{{ userEmail }}</span>
        <v-btn icon @click="logout">
          <v-icon>mdi-logout</v-icon>
        </v-btn>
      </v-app-bar>

      <v-main>
        <!-- Navigation Drawer / Sidebar -->
        <v-navigation-drawer v-model="drawer" permanent>
          <v-list nav>
            <!-- Add Holding Button -->
            <v-list-item
              prepend-icon="mdi-plus-circle"
              title="Add Holding"
              :class="{ 'text-primary': showAddHolding }"
              @click="showAddHolding = !showAddHolding"
            >
              <template v-slot:append>
                <v-icon :class="{ 'text-primary': showAddHolding }">
                  {{ showAddHolding ? 'mdi-chevron-up' : 'mdi-chevron-down' }}
                </v-icon>
              </template>
            </v-list-item>

            <!-- Add Holding Expandable Form -->
            <v-expand-transition>
              <div v-if="showAddHolding" class="add-holding-form pa-3">
                <v-text-field
                  v-model="searchQuery"
                  label="Search Ticker or ISIN"
                  placeholder="e.g., AAPL or US0378331005"
                  variant="outlined"
                  density="compact"
                  hide-details
                  clearable
                  prepend-inner-icon="mdi-magnify"
                  @input="debouncedSearch"
                  @click:clear="clearSearch"
                  class="mb-2"
                ></v-text-field>

                <!-- Search Results -->
                <v-list v-if="searchResults.length > 0" density="compact" class="search-results mb-2">
                  <v-list-item
                    v-for="result in searchResults"
                    :key="result.ticker"
                    @click="selectSearchResult(result)"
                    :class="{ 'bg-primary-lighten-4': selectedResult?.ticker === result.ticker }"
                  >
                    <v-list-item-title class="text-caption font-weight-bold">
                      {{ result.ticker }}
                    </v-list-item-title>
                    <v-list-item-subtitle class="text-caption">
                      {{ result.name }}
                    </v-list-item-subtitle>
                    <template v-slot:append>
                      <div class="text-right">
                        <div class="text-caption font-weight-medium text-success">
                          {{ result.currency }} {{ result.price }}
                        </div>
                        <v-chip size="x-small" :color="result.type === 'ETF' ? 'blue' : 'grey'" class="mt-1">
                          {{ result.type }}
                        </v-chip>
                      </div>
                    </template>
                  </v-list-item>
                </v-list>

                <v-progress-linear v-if="searchLoading" indeterminate color="primary" class="mb-2"></v-progress-linear>

                <!-- Selected Asset Details -->
                <template v-if="selectedResult">
                  <v-divider class="my-2"></v-divider>
                  <div class="text-caption text-grey mb-2">Selected: <strong>{{ selectedResult.name }}</strong></div>
                  
                  <v-text-field
                    v-model.number="newHolding.quantity"
                    label="Number of Shares"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hide-details
                    min="0"
                    step="0.01"
                    class="mb-2"
                  ></v-text-field>

                  <v-text-field
                    v-model.number="newHolding.purchasePrice"
                    label="Average Purchase Price"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hide-details
                    min="0"
                    step="0.01"
                    :suffix="selectedResult.currency"
                    class="mb-2"
                  ></v-text-field>

                  <!-- Auto-filled fields (collapsible) -->
                  <v-expansion-panels variant="accordion" class="mb-2">
                    <v-expansion-panel>
                      <v-expansion-panel-title class="text-caption py-1">
                        Additional Details
                      </v-expansion-panel-title>
                      <v-expansion-panel-text>
                        <v-text-field
                          v-model="newHolding.name"
                          label="Name"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.ticker"
                          label="Ticker"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.isin"
                          label="ISIN"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.exchange"
                          label="Exchange"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.currency"
                          label="Currency"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-text-field>
                        <v-text-field
                          v-model="newHolding.ter"
                          label="TER (%)"
                          type="number"
                          variant="outlined"
                          density="compact"
                          hide-details
                          step="0.01"
                          class="mb-2"
                        ></v-text-field>
                        <v-select
                          v-model="newHolding.policy"
                          label="Distribution Policy"
                          :items="['Accumulating', 'Distributing', 'N/A']"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-2"
                        ></v-select>
                        <v-checkbox
                          v-model="newHolding.etf"
                          label="Is ETF"
                          density="compact"
                          hide-details
                        ></v-checkbox>
                      </v-expansion-panel-text>
                    </v-expansion-panel>
                  </v-expansion-panels>

                  <v-btn
                    color="primary"
                    block
                    :loading="addHoldingLoading"
                    :disabled="!canAddHolding"
                    @click="addHolding"
                  >
                    <v-icon left>mdi-plus</v-icon>
                    Add to Portfolio
                  </v-btn>

                  <v-alert
                    v-if="addHoldingError"
                    type="error"
                    density="compact"
                    class="mt-2"
                  >
                    {{ addHoldingError }}
                  </v-alert>

                  <v-alert
                    v-if="addHoldingSuccess"
                    type="success"
                    density="compact"
                    class="mt-2"
                  >
                    Holding added successfully!
                  </v-alert>
                </template>
              </div>
            </v-expand-transition>

            <v-divider class="my-2"></v-divider>

            <v-list-item
              prepend-icon="mdi-view-dashboard"
              title="Dashboard"
              value="dashboard"
              :active="activeView === 'dashboard'"
              @click="activeView = 'dashboard'"
            />
            <v-list-item
              prepend-icon="mdi-wallet"
              title="Holdings"
              value="holdings"
              :active="activeView === 'holdings'"
              @click="activeView = 'holdings'"
            />
            <v-list-item
              prepend-icon="mdi-newspaper"
              title="News"
              value="news"
              :active="activeView === 'news'"
              @click="activeView = 'news'"
            />
            <v-list-item
              prepend-icon="mdi-chart-pie"
              title="Allocation"
              value="allocation"
              :active="activeView === 'allocation'"
              @click="activeView = 'allocation'"
            />
            <v-list-item
              prepend-icon="mdi-chart-line"
              title="Statistics"
              value="statistics"
              :active="activeView === 'statistics'"
              @click="activeView = 'statistics'"
            />
            <v-list-item
              prepend-icon="mdi-trending-up"
              title="Trading View"
              value="tradingView"
              :active="activeView === 'tradingView'"
              @click="activeView = 'tradingView'"
            />
            <v-list-item
              prepend-icon="mdi-chart-timeline-variant"
              title="Running Summary"
              value="runningSummary"
              :active="activeView === 'runningSummary'"
              @click="activeView = 'runningSummary'"
            />
            <v-list-item
              prepend-icon="mdi-receipt"
              title="Expenses"
              value="expenses"
              :active="activeView === 'expenses'"
              @click="activeView = 'expenses'"
            />
            <v-list-item
              prepend-icon="mdi-chat"
              title="Chat"
              value="chat"
              :active="activeView === 'chat'"
              @click="activeView = 'chat'"
            />
            <v-list-item
              prepend-icon="mdi-radar"
              title="Situations"
              value="situations"
              :active="activeView === 'situations'"
              @click="activeView = 'situations'"
            />
          </v-list>
        </v-navigation-drawer>

        <v-container v-if="activeView === 'holdings'" fluid>
          <v-card>
            <v-card-title class="d-flex align-center">
              <span>My Holdings</span>
              <v-spacer></v-spacer>
              <v-text-field
                v-model="holdingsSearch"
                prepend-inner-icon="mdi-magnify"
                label="Search holdings..."
                single-line
                hide-details
                density="compact"
                variant="outlined"
                style="max-width: 250px;"
                class="mr-2"
              ></v-text-field>
              <v-btn-toggle v-model="holdingsSortBy" mandatory density="compact" color="primary">
                <v-btn value="ticker" size="small">Ticker</v-btn>
                <v-btn value="value" size="small">Value</v-btn>
                <v-btn value="change" size="small">Day %</v-btn>
              </v-btn-toggle>
            </v-card-title>

            <v-card-text class="pa-0">
              <div v-if="!portfolioHoldings || portfolioHoldings.length === 0" class="text-center py-8 text-grey">
                <v-icon size="64" color="grey-lighten-1">mdi-wallet-outline</v-icon>
                <p class="mt-4">No holdings yet. Add your first holding using the sidebar.</p>
              </div>
              
              <v-table v-else fixed-header density="comfortable" class="holdings-table">
                <thead>
                  <tr>
                    <th style="width: 90px;">Chart</th>
                    <th style="min-width: 180px;">Ticker</th>
                    <th style="width: 100px;" class="text-right">Price</th>
                    <th style="width: 100px;" class="text-right">Day %</th>
                    <th style="width: 100px;" class="text-right">Day $</th>
                    <th style="width: 100px;" class="text-right">Total %</th>
                    <th style="width: 110px;" class="text-right">Total $</th>
                    <th style="width: 120px;" class="text-right">Value</th>
                    <th style="width: 50px;"></th>
                  </tr>
                </thead>
                <HoldingView
                  v-for="holding in sortedHoldings"
                  :key="holding.id_holding"
                  :holding="holding"
                  :auth-token="getCookie('auth_token')"
                  :home-currency="homeCurrency"
                  @day-change="updateHoldingDayChange"
                />
              </v-table>
            </v-card-text>
          </v-card>

          <!-- Portfolio Sentiment Card -->
          <v-row v-if="portfolioSentiment && activeView == 'dashboard'" class="mt-4">
            <v-col cols="12">
              <SentimentCard
                label="Portfolio Sentiment"
                :sentiment-score="portfolioSentiment.sentiment"
                :summary="portfolioSentiment.summary"
                :date="portfolioSentiment.date"
                :show-trend="false"
              />
              <div class="d-flex justify-end mt-2">
                <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Regenerate AI Summary</v-btn>
              </div>
            </v-col>
          </v-row>
          <v-row v-else-if="activeView == 'dashboard'" class="mt-4">
            <v-col cols="12">
              <v-card variant="outlined" class="text-center pa-4">
                <v-icon size="32" color="grey" class="mb-2">mdi-robot</v-icon>
                <div class="text-caption text-grey mb-2">No AI summary yet</div>
                <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Generate AI Summary</v-btn>
              </v-card>
            </v-col>
          </v-row>
        </v-container>

        <v-container v-else-if="activeView === 'dashboard'" fluid>
          <!-- Portfolio Statistics - Compact -->
          <v-card class="mb-3">
            <v-card-text class="pa-3">
              <v-row dense>
                <v-col cols="12" sm="6" md="3">
                  <div class="stat-item">
                    <div class="text-caption text-grey">TOTAL VALUE</div>
                    <div class="text-h6 font-weight-bold" :class="statsData.total_gain_loss >= 0 ? 'text-success' : 'text-error'">
                      {{ formatCurrency(statsData.total_value) }}
                    </div>
                    <div class="text-caption" :class="statsData.total_gain_loss >= 0 ? 'text-success' : 'text-error'">
                      {{ statsData.total_gain_loss >= 0 ? '+' : '' }}{{ formatCurrency(statsData.total_gain_loss) }} ({{ statsData.total_gain_loss_pct >= 0 ? '+' : '' }}{{ statsData.total_gain_loss_pct?.toFixed(2) }}%)
                    </div>
                  </div>
                </v-col>
                <v-col cols="12" sm="6" md="3">
                  <div class="stat-item">
                    <div class="text-caption text-grey">YOY RETURN</div>
                    <div class="text-h6 font-weight-bold" :class="statsData.yoy_return >= 0 ? 'text-success' : 'text-error'">
                      {{ statsData.yoy_return >= 0 ? '+' : '' }}{{ statsData.yoy_return?.toFixed(2) }}%
                    </div>
                    <div class="text-caption text-grey">Annual performance</div>
                  </div>
                </v-col>
                <v-col cols="12" sm="6" md="3">
                  <div class="stat-item">
                    <div class="text-caption text-grey">SORTINO RATIO</div>
                    <div class="text-h6 font-weight-bold">{{ statsData.sortino_ratio?.toFixed(2) || '0.00' }}</div>
                    <div class="text-caption text-grey">Risk-adjusted return</div>
                  </div>
                </v-col>
                <v-col cols="12" sm="6" md="3">
                  <div class="stat-item">
                    <div class="text-caption text-grey">MAX DRAWDOWN</div>
                    <div class="text-h6 font-weight-bold text-error">{{ statsData.max_drawdown?.toFixed(2) || '0.00' }}%</div>
                    <div class="text-caption text-grey">Avg: {{ statsData.avg_drawdown?.toFixed(2) || '0.00' }}%</div>
                  </div>
                </v-col>
              </v-row>
              <v-divider class="my-2"></v-divider>
              <v-row dense>
                <v-col cols="12" sm="6">
                  <div class="stat-item-inline">
                    <span class="text-caption text-grey mr-2">Cost Basis:</span>
                    <span class="font-weight-bold">{{ formatCurrency(statsData.total_cost) }}</span>
                  </div>
                </v-col>
                <v-col cols="12" sm="6">
                  <div class="stat-item-inline">
                    <span class="text-caption text-grey mr-2">Aggregated TER:</span>
                    <span class="font-weight-bold">{{ (statsData.aggregated_ter * 100)?.toFixed(3) || '0.000' }}%</span>
                  </div>
                </v-col>
              </v-row>
            </v-card-text>
          </v-card>

          <!-- Portfolio Value Chart -->
          <v-card class="mb-4">
            <v-card-title class="d-flex align-center">
              <span>Portfolio Value</span>
              <v-spacer></v-spacer>
              <span v-if="sentimentHistory.length > 0" class="text-caption mr-3" :class="sentimentHistory[sentimentHistory.length - 1].sentiment >= 0 ? 'text-success' : 'text-error'">
                <v-icon size="x-small" class="mr-1">mdi-chart-timeline-variant</v-icon>Sentiment {{ sentimentHistory[sentimentHistory.length - 1].sentiment >= 0 ? '+' : '' }}{{ sentimentHistory[sentimentHistory.length - 1].sentiment.toFixed(3) }}
              </span>
              <v-btn-toggle v-model="selectedInterval" mandatory density="compact" color="primary">
                <v-btn value="5m" size="small">5m</v-btn>
                <v-btn value="15m" size="small">15m</v-btn>
                <v-btn value="1h" size="small">1h</v-btn>
                <v-btn value="4h" size="small">4h</v-btn>
                <v-btn value="1d" size="small">1d</v-btn>
                <v-btn value="1w" size="small">1w</v-btn>
                <v-btn value="1M" size="small">1M</v-btn>
              </v-btn-toggle>
            </v-card-title>
            <v-card-text>
              <div v-if="portfolioLoading" class="d-flex justify-center py-8">
                <v-progress-circular indeterminate color="primary"></v-progress-circular>
              </div>
              <div v-else-if="portfolioError" class="text-center py-8 text-error">
                {{ portfolioError }}
              </div>
              <div v-else-if="portfolioData.length > 0" class="chart-wrapper">
                <CandleChart
                  ref="portfolioChart"
                  :data="portfolioData"
                  :height="340"
                  :price-decimals="2"
                  :show-volume="false"
                  bull-color="#26a79a"
                  bear-color="#ef5250"
                  @chart-ready="onPortfolioChartReady"
                />
              </div>
              <div v-else class="text-center py-8 text-grey">
                No portfolio data available
              </div>
            </v-card-text>
          </v-card>

          <!-- Portfolio Allocation -->
          <v-row>
            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-domain</v-icon>
                  Sector Allocation
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in sectorPieSlices"
                          :key="'sector-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'sector' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'sector', index }; scrollLegendIntoView('sector', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(sector, index) in sortedSectors" 
                        :key="sector.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'sector' && hoveredSlice?.index === index }"
                        :data-type="'sector'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'sector', index }; scrollLegendIntoView('sector', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors[index % pieColors.length] }"></span>
                        <span class="legend-text">{{ sector.name }}</span>
                        <span class="legend-value">{{ sector.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-earth</v-icon>
                  Region Allocation
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in regionPieSlices"
                          :key="'region-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'region' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'region', index }; scrollLegendIntoView('region', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(region, index) in sortedRegions" 
                        :key="region.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'region' && hoveredSlice?.index === index }"
                        :data-type="'region'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'region', index }; scrollLegendIntoView('region', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors2[index % pieColors2.length] }"></span>
                        <span class="legend-text">{{ region.name }}</span>
                        <span class="legend-value">{{ region.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="4">
              <v-card>
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-chart-pie</v-icon>
                  Top Companies
                </v-card-title>
                <v-card-text>
                  <div class="pie-chart-container">
                    <svg viewBox="0 0 200 200" class="pie-chart">
                      <g transform="translate(100, 100)">
                        <path
                          v-for="(slice, index) in companyPieSlices"
                          :key="'company-' + index"
                          :d="slice.path"
                          :fill="slice.color"
                          class="pie-slice"
                          :class="{ 'pie-slice-active': hoveredSlice?.type === 'company' && hoveredSlice?.index === index }"
                          @mouseenter="hoveredSlice = { type: 'company', index }; scrollLegendIntoView('company', index)"
                          @mouseleave="hoveredSlice = null"
                        >
                          <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                        </path>
                      </g>
                    </svg>
                    <div class="pie-legend">
                      <div 
                        v-for="(company, index) in sortedCompanies" 
                        :key="company.name" 
                        class="legend-item"
                        :class="{ 'legend-item-active': hoveredSlice?.type === 'company' && hoveredSlice?.index === index }"
                        :data-type="'company'"
                        :data-index="index"
                        @mouseenter="hoveredSlice = { type: 'company', index }; scrollLegendIntoView('company', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <span class="legend-color" :style="{ backgroundColor: pieColors[index % pieColors.length] }"></span>
                        <span class="legend-text">{{ company.name }}</span>
                        <span class="legend-value">{{ company.percentage.toFixed(1) }}%</span>
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>

          <!-- Top Gainers and Losers -->
          <v-row class="mt-4">
            <v-col cols="12" md="6">
              <v-card elevation="2">
                <v-card-title class="d-flex align-center bg-success-lighten-4">
                  <v-icon size="small" class="mr-2" color="success">mdi-trending-up</v-icon>
                  <span>Top Gainers (24h)</span>
                </v-card-title>
                <v-card-text class="pa-0">
                  <div v-if="topGainersLosersLoading" class="text-center py-8">
                    <v-progress-circular indeterminate color="success"></v-progress-circular>
                  </div>
                  <div v-else-if="actualGainers.length === 0" class="text-center py-8 text-grey">
                    <v-icon size="48" color="grey-lighten-1">mdi-chart-line-variant</v-icon>
                    <p class="mt-3">No gainers in the last 24 hours</p>
                  </div>
                  <v-list v-else density="compact">
                    <v-list-item
                      v-for="(gainer, index) in actualGainers"
                      :key="gainer.Holding?.IdHolding || index"
                      class="gainer-item"
                    >
                      <template v-slot:prepend>
                        <v-avatar color="success" size="32" class="mr-2">
                          <span class="text-caption">#{{ index + 1 }}</span>
                        </v-avatar>
                      </template>
                      <v-list-item-title class="font-weight-bold">
                        {{ gainer.Holding?.Ticker || 'N/A' }}
                      </v-list-item-title>
                      <v-list-item-subtitle class="text-caption">
                        {{ gainer.Holding?.Name || 'Unknown' }}
                      </v-list-item-subtitle>
                      <template v-slot:append>
                        <div class="text-right">
                          <div class="text-success font-weight-bold">
                            +{{ gainer.PriceChangePct?.toFixed(2) }}%
                          </div>
                          <div v-if="gainer.RelatedNews?.length > 0" class="text-caption text-grey">
                            <v-icon size="x-small" class="mr-1">mdi-newspaper</v-icon>
                            {{ gainer.RelatedNews.length }} news
                          </div>
                        </div>
                      </template>
                    </v-list-item>
                  </v-list>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="6">
              <v-card elevation="2">
                <v-card-title class="d-flex align-center bg-error-lighten-4">
                  <v-icon size="small" class="mr-2" color="error">mdi-trending-down</v-icon>
                  <span>Top Losers (24h)</span>
                </v-card-title>
                <v-card-text class="pa-0">
                  <div v-if="topGainersLosersLoading" class="text-center py-8">
                    <v-progress-circular indeterminate color="error"></v-progress-circular>
                  </div>
                  <div v-else-if="actualLosers.length === 0" class="text-center py-8 text-grey">
                    <v-icon size="48" color="grey-lighten-1">mdi-chart-line-variant</v-icon>
                    <p class="mt-3">No losers in the last 24 hours</p>
                  </div>
                  <v-list v-else density="compact">
                    <v-list-item
                      v-for="(loser, index) in actualLosers"
                      :key="loser.Holding?.IdHolding || index"
                      class="loser-item"
                    >
                      <template v-slot:prepend>
                        <v-avatar color="error" size="32" class="mr-2">
                          <span class="text-caption">#{{ index + 1 }}</span>
                        </v-avatar>
                      </template>
                      <v-list-item-title class="font-weight-bold">
                        {{ loser.Holding?.Ticker || 'N/A' }}
                      </v-list-item-title>
                      <v-list-item-subtitle class="text-caption">
                        {{ loser.Holding?.Name || 'Unknown' }}
                      </v-list-item-subtitle>
                      <template v-slot:append>
                        <div class="text-right">
                          <div class="text-error font-weight-bold">
                            {{ loser.PriceChangePct?.toFixed(2) }}%
                          </div>
                          <div v-if="loser.RelatedNews?.length > 0" class="text-caption text-grey">
                            <v-icon size="x-small" class="mr-1">mdi-newspaper</v-icon>
                            {{ loser.RelatedNews.length }} news
                          </div>
                        </div>
                      </template>
                    </v-list-item>
                  </v-list>
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>

          <!-- Portfolio Sentiment Card -->
          <v-row v-if="portfolioSentiment" class="mt-4">
            <v-col cols="12">
              <SentimentCard
                label="Portfolio Sentiment"
                :sentiment-score="portfolioSentiment.sentiment"
                :summary="portfolioSentiment.summary"
                :date="portfolioSentiment.date"
                :show-trend="false"
              />
              <div class="d-flex justify-end mt-2">
                <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Regenerate AI Summary</v-btn>
              </div>
            </v-col>
          </v-row>
          <v-row v-else class="mt-4">
            <v-col cols="12">
              <v-card variant="outlined" class="text-center pa-6">
                <v-icon size="40" color="grey" class="mb-2">mdi-robot</v-icon>
                <div class="text-body-2 text-grey mb-3">No AI portfolio summary yet</div>
                <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Generate AI Summary</v-btn>
              </v-card>
            </v-col>
          </v-row>

          <v-row v-if="runningSummary || activeView === 'dashboard'" class="mt-4">
            <v-col cols="12">
              <RunningSummaryCard
                :sentiment-score="runningSummary?.sentiment || 0"
                :summary="runningSummary?.summary || ''"
                :date="runningSummary?.date || ''"
                :window-days="runningSummary?.window_days || 30"
                :key-themes="runningSummary?.theme_predictions || []"
                :generating="runningSummaryGenerating"
                :has-data="!!runningSummary"
                @view-full="activeView = 'runningSummary'"
                @generate="generateRunningSummary"
              />
            </v-col>
          </v-row>
        </v-container>

        <v-container v-else-if="activeView === 'news'" fluid>
          <v-row>
            <v-col cols="12" md="4">
              <SentimentCard
                v-if="portfolioSentiment"
                label="Portfolio Sentiment Today"
                :sentiment-score="portfolioSentiment.sentiment"
                :summary="portfolioSentiment.summary"
                :date="portfolioSentiment.date"
                :show-trend="false"
              />
              <v-card v-else class="mb-4" variant="outlined">
                <v-card-text class="text-center py-6">
                  <v-icon size="36" color="grey" class="mb-2">mdi-robot</v-icon>
                  <div class="text-body-2 text-grey mb-2">No AI summary yet</div>
                  <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Generate AI Summary</v-btn>
                </v-card-text>
              </v-card>
              <div v-if="portfolioSentiment" class="d-flex justify-end mt-2">
                <v-btn size="small" variant="tonal" color="primary" :loading="summaryGenerating" @click="generateSummary" prepend-icon="mdi-magic">Regenerate AI Summary</v-btn>
              </div>

              <v-card class="mt-4" elevation="2">
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-filter</v-icon>
                  Filters
                </v-card-title>
                <v-card-text>
                  <v-text-field
                    v-model="newsSearchQuery"
                    label="Search news..."
                    prepend-inner-icon="mdi-magnify"
                    variant="outlined"
                    density="compact"
                    hide-details
                    clearable
                    class="mb-3"
                  ></v-text-field>

                  <v-select
                    v-model="selectedTickerFilter"
                    :items="tickerFilterOptions"
                    label="Filter by holding"
                    variant="outlined"
                    density="compact"
                    hide-details
                    class="mb-3"
                  ></v-select>

                  <div class="text-caption text-grey mb-2">Filter by sentiment:</div>
                  <v-chip-group
                    v-model="newsFilter"
                    mandatory
                    column
                    selected-class="text-primary"
                  >
                    <v-chip
                      value="all"
                      size="small"
                      variant="outlined"
                      filter
                    >
                      <v-icon size="small" start>mdi-all-inclusive</v-icon>
                      All
                    </v-chip>
                    <v-chip
                      value="positive"
                      size="small"
                      variant="outlined"
                      filter
                      color="success"
                    >
                      <v-icon size="small" start>mdi-thumb-up</v-icon>
                      Positive
                    </v-chip>
                    <v-chip
                      value="neutral"
                      size="small"
                      variant="outlined"
                      filter
                    >
                      <v-icon size="small" start>mdi-minus</v-icon>
                      Neutral
                    </v-chip>
                    <v-chip
                      value="negative"
                      size="small"
                      variant="outlined"
                      filter
                      color="error"
                    >
                      <v-icon size="small" start>mdi-thumb-down</v-icon>
                      Negative
                    </v-chip>
                  </v-chip-group>
                </v-card-text>
              </v-card>

              <v-card class="mt-4" elevation="2">
                <v-card-title class="text-subtitle-1 pb-2">
                  <v-icon size="small" class="mr-2">mdi-information</v-icon>
                  News Statistics
                </v-card-title>
                <v-card-text>
                  <div class="d-flex justify-space-between mb-2">
                    <span class="text-caption text-grey">Total Articles:</span>
                    <span class="font-weight-bold">{{ filteredNews.length }}</span>
                  </div>
                  <div class="d-flex justify-space-between mb-2">
                    <span class="text-caption text-grey">Positive:</span>
                    <span class="font-weight-bold text-success">{{ newsStats.positive }}</span>
                  </div>
                  <div class="d-flex justify-space-between mb-2">
                    <span class="text-caption text-grey">Negative:</span>
                    <span class="font-weight-bold text-error">{{ newsStats.negative }}</span>
                  </div>
                  <div class="d-flex justify-space-between">
                    <span class="text-caption text-grey">Neutral:</span>
                    <span class="font-weight-bold">{{ newsStats.neutral }}</span>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <v-col cols="12" md="8">
              <!-- Top Gainers/Losers News Highlights -->
              <v-row v-if="actualGainers.length > 0 || actualLosers.length > 0" class="mb-4">
                <v-col v-if="actualGainers.length > 0 && actualGainers[0].RelatedNews?.length > 0" cols="12" md="6">
                  <v-card elevation="2" class="gainer-news-card">
                    <v-card-title class="text-subtitle-2 pb-2 bg-success-lighten-5">
                      <v-icon size="small" class="mr-2" color="success">mdi-trending-up</v-icon>
                      Top Gainer: {{ actualGainers[0].Holding?.Ticker }}
                      <v-chip size="x-small" color="success" class="ml-2">
                        +{{ actualGainers[0].PriceChangePct?.toFixed(2) }}%
                      </v-chip>
                    </v-card-title>
                    <v-card-text class="pa-2">
                      <div 
                        v-for="(news, idx) in actualGainers[0].RelatedNews?.slice(0, 3)"
                        :key="news.id_news || idx"
                        class="news-item-compact mb-2 pa-2"
                      >
                        <div class="d-flex align-center mb-1">
                          <v-chip
                            :color="getSentimentColor(news.sentiment)"
                            size="x-small"
                            variant="flat"
                            class="mr-2"
                          >
                            {{ getSentimentLabel(news.sentiment) }}
                          </v-chip>
                          <span class="text-caption text-grey">
                            {{ formatTimeAgo(news.published_at) }}
                          </span>
                        </div>
                        <div class="text-body-2 font-weight-medium mb-1">
                          <a :href="news.link" target="_blank" class="news-link-compact">
                            {{ news.title }}
                          </a>
                        </div>
                        <p class="text-caption text-grey-darken-1 mb-0">
                          {{ news.summary?.substring(0, 100) }}{{ news.summary?.length > 100 ? '...' : '' }}
                        </p>
                      </div>
                      <div v-if="actualGainers[0].RelatedNews?.length > 3" class="text-caption text-grey text-center mt-2">
                        +{{ actualGainers[0].RelatedNews.length - 3 }} more articles
                      </div>
                    </v-card-text>
                  </v-card>
                </v-col>

                <v-col v-if="actualLosers.length > 0 && actualLosers[0].RelatedNews?.length > 0" cols="12" md="6">
                  <v-card elevation="2" class="loser-news-card">
                    <v-card-title class="text-subtitle-2 pb-2 bg-error-lighten-5">
                      <v-icon size="small" class="mr-2" color="error">mdi-trending-down</v-icon>
                      Top Loser: {{ actualLosers[0].Holding?.Ticker }}
                      <v-chip size="x-small" color="error" class="ml-2">
                        {{ actualLosers[0].PriceChangePct?.toFixed(2) }}%
                      </v-chip>
                    </v-card-title>
                    <v-card-text class="pa-2">
                      <div 
                        v-for="(news, idx) in actualLosers[0].RelatedNews?.slice(0, 3)"
                        :key="news.id_news || idx"
                        class="news-item-compact mb-2 pa-2"
                      >
                        <div class="d-flex align-center mb-1">
                          <v-chip
                            :color="getSentimentColor(news.sentiment)"
                            size="x-small"
                            variant="flat"
                            class="mr-2"
                          >
                            {{ getSentimentLabel(news.sentiment) }}
                          </v-chip>
                          <span class="text-caption text-grey">
                            {{ formatTimeAgo(news.published_at) }}
                          </span>
                        </div>
                        <div class="text-body-2 font-weight-medium mb-1">
                          <a :href="news.link" target="_blank" class="news-link-compact">
                            {{ news.title }}
                          </a>
                        </div>
                        <p class="text-caption text-grey-darken-1 mb-0">
                          {{ news.summary?.substring(0, 100) }}{{ news.summary?.length > 100 ? '...' : '' }}
                        </p>
                      </div>
                      <div v-if="actualLosers[0].RelatedNews?.length > 3" class="text-caption text-grey text-center mt-2">
                        +{{ actualLosers[0].RelatedNews.length - 3 }} more articles
                      </div>
                    </v-card-text>
                  </v-card>
                </v-col>
              </v-row>

              <v-card elevation="2">
                <v-card-title class="d-flex align-center">
                  <v-icon size="small" class="mr-2">mdi-newspaper</v-icon>
                  Portfolio News Feed
                  <v-spacer></v-spacer>
                  <v-btn
                    icon
                    size="small"
                    @click="refreshNews"
                    :loading="newsLoading"
                  >
                    <v-icon>mdi-refresh</v-icon>
                  </v-btn>
                </v-card-title>
                <v-card-text>
                  <NewsFeed
                    :news-items="filteredNews"
                    :loading="newsLoading"
                    :loading-more="newsLoadingMore"
                    :error="newsError"
                    :has-more="newsHasMore"
                    :show-ticker="true"
                    @load-more="loadMoreNews"
                    @retry="fetchPortfolioNews"
                  />
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>
        </v-container>

        <v-container v-else-if="activeView === 'allocation'" fluid>
          <v-card class="pa-4">
            <h3>Allocation View</h3>
            <p>This section is under construction.</p>
          </v-card>
        </v-container>

        <v-container v-else-if="activeView === 'statistics'" fluid>
          <v-row>
            <v-col cols="12">
              <v-card>
                <v-card-title class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px; min-height: 56px;">
                  <span>Portfolio vs Benchmark Comparison</span>
                  <div class="d-flex align-center flex-wrap" style="gap: 8px;">
                    <v-btn
                      :color="showPortfolioCustomizer ? 'primary' : undefined"
                      variant="outlined"
                      density="compact"
                      prepend-icon="mdi-tune"
                      style="height: 40px;"
                      @click="showPortfolioCustomizer = !showPortfolioCustomizer"
                    >
                      Customize
                      <v-badge
                        v-if="customPortfolioTickers && customPortfolioTickers.length > 0"
                        :content="customPortfolioTickers.length"
                        color="primary"
                        inline
                        class="ml-1"
                      />
                    </v-btn>
                    <v-select
                      v-model="backtestBenchmark"
                      :items="benchmarkOptions"
                      label="Benchmark"
                      variant="outlined"
                      density="compact"
                      hide-details
                      style="max-width: 200px; min-width: 140px;"
                      @update:model-value="fetchBacktest"
                    ></v-select>
                    <v-select
                      v-model="backtestPeriod"
                      :items="periodOptions"
                      label="Period"
                      variant="outlined"
                      density="compact"
                      hide-details
                      style="max-width: 150px; min-width: 120px;"
                      @update:model-value="fetchBacktest"
                    ></v-select>
                  </div>
                </v-card-title>

                <!-- Portfolio customizer panel -->
                <v-expand-transition>
                  <div v-if="showPortfolioCustomizer && portfolioHoldings && portfolioHoldings.length > 0">
                    <v-divider />
                    <v-card-text class="py-2">
                      <div class="text-caption text-grey font-weight-medium mb-2">
                        SELECT HOLDINGS FOR BACKTEST
                        <v-btn
                          size="x-small"
                          variant="text"
                          class="ml-2"
                          @click="customPortfolioTickers = null; fetchBacktest()"
                        >Reset to all</v-btn>
                      </div>
                      <div class="d-flex flex-wrap" style="gap: 6px;">
                        <v-tooltip
                          v-for="h in sortedPortfolioHoldings"
                          :key="h.id_holding"
                          location="top"
                          max-width="240"
                        >
                          <template v-slot:activator="{ props: tp }">
                            <div
                              v-bind="tp"
                              class="holding-perf-chip"
                              :class="{ 'holding-perf-chip--inactive': customPortfolioTickers && !customPortfolioTickers.includes(h.ticker) }"
                              :style="holdingChipStyle(h.ticker)"
                              @click="toggleCustomTicker(h.ticker)"
                            >
                              <span class="hpc-ticker">{{ h.ticker }}</span>
                              <span
                                v-if="backtestData && backtestData.holding_returns && backtestData.holding_returns[h.ticker]"
                                class="hpc-delta"
                                :style="{ color: holdingTextColor(backtestData.holding_returns[h.ticker].vs_benchmark) }"
                              >
                                {{ backtestData.holding_returns[h.ticker].vs_benchmark >= 0 ? '+' : '' }}{{ backtestData.holding_returns[h.ticker].vs_benchmark }}%
                              </span>
                            </div>
                          </template>
                          <div v-if="backtestData && backtestData.holding_returns && backtestData.holding_returns[h.ticker]" class="pa-1">
                            <div class="font-weight-bold mb-1" style="font-size:0.8rem;">{{ backtestData.holding_returns[h.ticker].name }}</div>
                            <div class="text-caption text-grey">ISIN: {{ backtestData.holding_returns[h.ticker].isin || 'N/A' }}</div>
                            <div class="text-caption">Price: {{ backtestData.holding_returns[h.ticker].current_price }}</div>
                            <div class="text-caption">Return (period): {{ backtestData.holding_returns[h.ticker].total_return >= 0 ? '+' : '' }}{{ backtestData.holding_returns[h.ticker].total_return }}%</div>
                            <div class="text-caption" :style="{ color: holdingTextColor(backtestData.holding_returns[h.ticker].vs_benchmark) }">
                              vs {{ backtestBenchmark }}: {{ backtestData.holding_returns[h.ticker].vs_benchmark >= 0 ? '+' : '' }}{{ backtestData.holding_returns[h.ticker].vs_benchmark }}%
                            </div>
                            <div class="text-caption text-grey">Weight: {{ backtestData.holding_returns[h.ticker].weight }}%</div>
                          </div>
                          <div v-else>{{ h.name || h.ticker }}</div>
                        </v-tooltip>
                      </div>
                    </v-card-text>
                  </div>
                </v-expand-transition>

                <v-card-text>
                  <v-progress-linear v-if="backtestLoading" indeterminate color="primary"></v-progress-linear>
                  
                  <v-alert v-if="backtestError" type="error" class="mb-4">
                    {{ backtestError }}
                  </v-alert>

                  <div v-if="backtestData && !backtestLoading">
                    <BacktestChart
                      :portfolio-data="backtestData.portfolio_values"
                      :benchmark-data="backtestData.benchmark_values"
                      :timestamps="backtestData.timestamps"
                      :benchmark-name="backtestBenchmark"
                      :height="500"
                    />

                    <v-row class="mt-2">
                      <v-col cols="12" md="6">
                        <v-card variant="outlined">
                          <v-card-title class="text-h6">Portfolio Metrics</v-card-title>
                          <v-card-text>
                            <v-list density="compact">
                              <v-list-item>
                                <v-list-item-title>CAGR</v-list-item-title>
                                <template v-slot:append>
                                  <span :class="backtestData.cagr_portfolio >= 0 ? 'text-success' : 'text-error'" class="font-weight-bold">
                                    {{ backtestData.cagr_portfolio >= 0 ? '+' : '' }}{{ backtestData.cagr_portfolio }}%
                                  </span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Max Drawdown</v-list-item-title>
                                <template v-slot:append>
                                  <span class="text-error font-weight-bold">{{ backtestData.max_drawdown_portfolio }}%</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Sharpe Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.sharpe_ratio_portfolio }}</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Sortino Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.sortino_ratio_portfolio }}</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Annualized Volatility</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.volatility_portfolio }}%</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Calmar Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.calmar_ratio_portfolio }}</span>
                                </template>
                              </v-list-item>
                            </v-list>
                          </v-card-text>
                        </v-card>
                      </v-col>

                      <v-col cols="12" md="6">
                        <v-card variant="outlined">
                          <v-card-title class="text-h6">{{ backtestBenchmark }} Metrics</v-card-title>
                          <v-card-text>
                            <v-list density="compact">
                              <v-list-item>
                                <v-list-item-title>CAGR</v-list-item-title>
                                <template v-slot:append>
                                  <span :class="backtestData.cagr_benchmark >= 0 ? 'text-success' : 'text-error'" class="font-weight-bold">
                                    {{ backtestData.cagr_benchmark >= 0 ? '+' : '' }}{{ backtestData.cagr_benchmark }}%
                                  </span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Max Drawdown</v-list-item-title>
                                <template v-slot:append>
                                  <span class="text-error font-weight-bold">{{ backtestData.max_drawdown_benchmark }}%</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Sharpe Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.sharpe_ratio_benchmark }}</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Sortino Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.sortino_ratio_benchmark }}</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Annualized Volatility</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.volatility_benchmark }}%</span>
                                </template>
                              </v-list-item>
                              <v-list-item>
                                <v-list-item-title>Calmar Ratio</v-list-item-title>
                                <template v-slot:append>
                                  <span class="font-weight-bold">{{ backtestData.calmar_ratio_benchmark }}</span>
                                </template>
                              </v-list-item>
                            </v-list>
                          </v-card-text>
                        </v-card>
                      </v-col>
                    </v-row>

                    <v-row v-if="holdingReturns.length > 0" class="mt-2">
                      <v-col cols="12">
                        <v-card variant="outlined">
                          <div class="attrib-header d-flex align-center justify-space-between px-3 py-2">
                            <span class="text-subtitle-2">Holding Attribution vs {{ backtestBenchmark }}</span>
                            <div class="d-flex align-center" style="gap:6px;">
                              <v-chip v-if="underperformers.length > 0" color="error" size="x-small" variant="tonal">{{ underperformers.length }} weak</v-chip>
                              <v-chip v-if="buyDipSignals.length > 0" color="success" size="x-small" variant="tonal">{{ buyDipSignals.length }} buy dip</v-chip>
                              <v-btn-toggle v-model="attribSort" mandatory density="compact" size="x-small" color="primary" variant="outlined">
                                <v-btn value="weight">Weight</v-btn>
                                <v-btn value="return">Return</v-btn>
                                <v-btn value="reversion">Reversion</v-btn>
                              </v-btn-toggle>
                            </div>
                          </div>
                          <v-divider />
                          <div class="attrib-table-head">
                            <span>Ticker</span>
                            <span class="text-right">Total</span>
                            <span class="text-right">1M</span>
                            <span class="text-right">3M</span>
                            <span class="text-right">vs ref</span>
                            <span class="text-right">DD peak</span>
                            <span class="text-right">Wt</span>
                            <span class="text-right">Signal</span>
                          </div>
                          <v-divider />
                          <div
                            v-for="hr in holdingReturns"
                            :key="hr.ticker"
                            class="attrib-row"
                            :style="{ borderLeft: '3px solid ' + holdingHeatmapBg(hr.vs_benchmark) }"
                          >
                            <div class="attrib-info">
                              <span class="attrib-ticker">{{ hr.ticker }}</span>
                              <span class="attrib-name">{{ hr.name }}</span>
                            </div>
                            <span class="attrib-cell" :style="{ color: hr.total_return >= 0 ? '#26a69a' : '#ef5350' }">{{ hr.total_return >= 0 ? '+' : '' }}{{ hr.total_return }}%</span>
                            <span class="attrib-cell" :style="{ color: hr.return_1m >= 0 ? '#26a69a' : '#ef5350' }">{{ hr.return_1m >= 0 ? '+' : '' }}{{ hr.return_1m }}%</span>
                            <span class="attrib-cell" :style="{ color: hr.return_3m >= 0 ? '#26a69a' : '#ef5350' }">{{ hr.return_3m >= 0 ? '+' : '' }}{{ hr.return_3m }}%</span>
                            <span class="attrib-cell" :style="{ color: holdingTextColor(hr.vs_benchmark) }">{{ hr.vs_benchmark >= 0 ? '+' : '' }}{{ hr.vs_benchmark }}%</span>
                            <span class="attrib-cell" :style="{ color: hr.drawdown_from_peak < -10 ? '#ef5350' : '#aaa' }">{{ hr.drawdown_from_peak.toFixed(1) }}%</span>
                            <span class="attrib-cell text-grey">{{ hr.weight }}%</span>
                            <span class="attrib-cell">
                              <span class="signal-badge" :class="'signal-' + hr.signal">{{ hr.signal === 'BUY_DIP' ? 'dip' : hr.signal === 'STRONG' ? 'str' : hr.signal === 'WEAK' ? 'weak' : '-' }}</span>
                            </span>
                          </div>
                        </v-card>
                      </v-col>
                    </v-row>

                    <v-row v-if="buyDipSignals.length > 0" class="mt-2">
                      <v-col cols="12">
                        <v-card variant="outlined" style="border-color: rgba(38,166,154,0.3);">
                          <div class="attrib-header d-flex align-center px-3 py-2" style="gap:6px;">
                            <v-icon size="small" color="success">mdi-trending-down</v-icon>
                            <span class="text-subtitle-2">Buy-the-Dip Candidates</span>
                            <span class="text-caption text-grey ml-1">long-term positive but recently below own avg</span>
                          </div>
                          <v-divider />
                          <div v-for="hr in buyDipSignals" :key="hr.ticker" class="attrib-row" style="border-left: 3px solid rgba(38,166,154,0.6);">
                            <div class="attrib-info">
                              <span class="attrib-ticker">{{ hr.ticker }}</span>
                              <span class="attrib-name">{{ hr.name }}</span>
                            </div>
                            <div class="attrib-metrics">
                              <span class="text-caption" style="color:#aaa;">period: </span>
                              <span class="attrib-return" style="color:#26a69a;">+{{ hr.total_return }}%</span>
                              <span class="text-caption" style="color:#aaa;">1M: </span>
                              <span class="attrib-return" :style="{ color: hr.return_1m >= 0 ? '#26a69a' : '#ef5350' }">{{ hr.return_1m >= 0 ? '+' : '' }}{{ hr.return_1m }}%</span>
                              <span class="text-caption" style="color:#aaa;">rev score: </span>
                              <span class="attrib-return" style="color:#26a69a;">+{{ hr.mean_reversion_score.toFixed(2) }}</span>
                              <span class="text-caption" style="color:#aaa;">DD: </span>
                              <span class="attrib-return" style="color:#ef5350;">{{ hr.drawdown_from_peak.toFixed(1) }}%</span>
                            </div>
                          </div>
                        </v-card>
                      </v-col>
                    </v-row>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>
        </v-container>
        <v-container v-if="activeView == 'tradingView'" class="pa-0" fluid style="height: calc(100vh - 64px);">
          <TradingViewer />
        </v-container>
        <v-container v-if="activeView === 'runningSummary'" fluid>
          <RunningSummaryView :auth-token="getCookie('auth_token')" @updated="fetchRunningSummary" />
        </v-container>
        <v-container v-if="activeView === 'expenses'" fluid class="pa-0">
          <ExpensesView />
        </v-container>
        <v-container v-if="activeView === 'chat'" fluid class="pa-0" style="height: calc(100vh - 64px);">
          <ChatView />
        </v-container>
        <v-container v-if="activeView === 'situations'" fluid class="pa-0">
          <SituationView />
        </v-container>
      </v-main>
    </template>
  </v-app>
</template>

<script>
import LoginRegister from './components/loginRegister.vue'
import ExpensesView from './components/expensesView.vue'
import { API_BASE_URL } from './config'
import { BaselineSeries } from 'lightweight-charts'
import CandleChart from './components/candleChart.vue'
import BacktestChart from './components/backtestChart.vue'
import HoldingView from './components/holdingView.vue'
import SentimentCard from './components/sentimentCard.vue'
import RunningSummaryCard from './components/RunningSummaryCard.vue'
import RunningSummaryView from './components/RunningSummaryView.vue'
import NewsFeed from './components/newsFeed.vue'
import TradingViewer from './components/tradingViewer.vue'
import ChatView from './components/chatView.vue'
import SituationView from './components/situationView.vue'

function decodeJwtPayload(token) {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) return null
    const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const paddedPayload = payload + '='.repeat((4 - (payload.length % 4)) % 4)
    return JSON.parse(atob(paddedPayload))
  } catch {
    return null
  }
}

export default {
  name: 'App',

  components: {
    LoginRegister,
    ExpensesView,
    CandleChart,
    BacktestChart,
    HoldingView,
    SentimentCard,
    RunningSummaryCard,
    RunningSummaryView,
    NewsFeed,
    TradingViewer,
    ChatView,
    SituationView
  },

  data() {
    return {
      // Authentication
      portfolioHoldings: null,
      isAuthenticated: false,
      userEmail: '',
      runningSummary: null,
      runningSummaryGenerating: false,
      runningSummaryWindowDays: 30,

      // Portfolio chart data
      portfolioData: [],
      portfolioLoading: false,
      portfolioError: '',
      selectedInterval: '1d',
      chartWidth: 800,
      statsData: {},
      homeCurrency: 'EUR',
      conversionCache: {},
      conversionPromises: {},

      // Sidebar state
      drawer: true,
      activeView: 'dashboard',

      // Add Holding state
      showAddHolding: false,
      searchQuery: '',
      searchResults: [],
      searchLoading: false,
      selectedResult: null,
      searchTimeout: null,
      addHoldingLoading: false,
      addHoldingError: '',
      addHoldingSuccess: false,
      portfolioAllocations: {},
      hoveredSlice: null,
      pieColors: ['#2196F3', '#4CAF50', '#FF9800', '#E91E63', '#9C27B0', '#00BCD4', '#795548', '#607D8B', '#FF5722', '#3F51B5'],
      pieColors2: ['#3F51B5', '#8BC34A', '#FFC107', '#F44336', '#673AB7', '#009688', '#FF5722', '#03A9F4', '#CDDC39', '#E91E63'],
      newHolding: {
        name: '',
        ticker: '',
        isin: '',
        exchange: '',
        currency: 'USD',
        quantity: null,
        purchasePrice: null,
        ter: 0,
        policy: 'N/A',
        etf: false
      },

      // Holdings view state
      holdingsSearch: '',
      holdingsSortBy: 'value',
      
      portfolioCostBasisLine: null,

      // News view state
      portfolioNews: [],
      portfolioSentiment: null,
      summaryGenerating: false,
      newsLoading: false,
      newsError: '',
      newsOffset: 0,
      newsLimit: 20,
      newsHasMore: true,
      newsLoadingMore: false,
      newsFilter: 'all',
      newsSearchQuery: '',
      selectedTickerFilter: 'all',

      topGainers: [],
      topLosers: [],
      topGainersLosersLoading: false,

      backtestData: null,
      backtestLoading: false,
      backtestError: '',
      backtestBenchmark: 'SPY',
      backtestPeriod: '1Y',
      showPortfolioCustomizer: false,
      customPortfolioTickers: null,
      attribSort: 'weight',

      sentimentHistory: [],
      sentimentLoading: false,
      sentimentSeries: null,

      benchmarkOptions: [
        { title: 'S&P 500', value: 'SPY' },
        { title: 'NASDAQ 100', value: 'QQQ' },
        { title: 'Dow Jones', value: 'DIA' },
        { title: 'Russell 2000', value: 'IWM' },
        { title: 'MSCI World', value: 'URTH' },
        { title: 'MSCI ACWI', value: 'ACWI' },
        { title: 'STOXX Europe 600', value: 'EXSA.DE' },
        { title: 'DAX', value: '^GDAXI' },
        { title: 'FTSE 100', value: '^FTSE' },
        { title: 'Gold', value: 'GLD' },
        { title: 'Bitcoin', value: 'BTC-USD' }
      ],
      periodOptions: [
        { title: '6 Months', value: '6M' },
        { title: '1 Year', value: '1Y' },
        { title: '3 Years', value: '3Y' },
        { title: '5 Years', value: '5Y' },
        { title: '10 Years', value: '10Y' },
        { title: 'All Time', value: 'ALL' }
      ]
    }
  },

  computed: {
    holdingReturns() {
      if (!this.backtestData || !this.backtestData.holding_returns) return []
      const arr = Object.values(this.backtestData.holding_returns)
      if (this.attribSort === 'weight') return [...arr].sort((a, b) => b.weight - a.weight)
      if (this.attribSort === 'return') return [...arr].sort((a, b) => a.vs_benchmark - b.vs_benchmark)
      if (this.attribSort === 'reversion') return [...arr].sort((a, b) => b.mean_reversion_score - a.mean_reversion_score)
      return arr
    },

    buyDipSignals() {
      if (!this.backtestData || !this.backtestData.holding_returns) return []
      return Object.values(this.backtestData.holding_returns)
        .filter(h => h.signal === 'BUY_DIP')
        .sort((a, b) => b.mean_reversion_score - a.mean_reversion_score)
    },

    underperformers() {
      return this.holdingReturns.filter(h => h.signal === 'WEAK')
    },

    canAddHolding() {
      return this.selectedResult && 
             this.newHolding.quantity > 0 && 
             this.newHolding.purchasePrice > 0 &&
             this.newHolding.ticker
    },

    sortedSectors() {
      if (!this.portfolioAllocations.sectors) return []
      return [...this.portfolioAllocations.sectors].sort((a, b) => b.percentage - a.percentage)
    },

    sortedRegions() {
      if (!this.portfolioAllocations.regions) return []
      return [...this.portfolioAllocations.regions].sort((a, b) => b.percentage - a.percentage)
    },

    sortedCompanies() {
      if (!this.portfolioAllocations.companies) return []
      return [...this.portfolioAllocations.companies].sort((a, b) => b.percentage - a.percentage)
    },

    sectorPieSlices() {
      return this.generatePieSlices(this.sortedSectors, this.pieColors)
    },

    regionPieSlices() {
      return this.generatePieSlices(this.sortedRegions, this.pieColors2)
    },

    companyPieSlices() {
      return this.generatePieSlices(this.sortedCompanies, this.pieColors)
    },

    actualGainers() {
      return this.topGainers.filter(g => g.PriceChangePct > 0)
    },

    actualLosers() {
      return this.topLosers.filter(l => l.PriceChangePct < 0)
    },

    sortedHoldings() {
      let holdings = [...this.filteredHoldings]
      
      if (this.holdingsSortBy === 'ticker') {
        holdings.sort((a, b) => a.ticker.localeCompare(b.ticker))
      } else if (this.holdingsSortBy === 'value') {
        holdings.sort((a, b) => {
          const valueA = a.quantity * a.purchase_price
          const valueB = b.quantity * b.purchase_price
          return valueB - valueA
        })
      } else if (this.holdingsSortBy === 'change') {
        holdings.sort((a, b) => {
          const changeA = a.dayChangePercent || 0
          const changeB = b.dayChangePercent || 0
          return changeB - changeA
        })
      }
      
      return holdings
    },

    sortedPortfolioHoldings() {
      if (!this.portfolioHoldings) return []
      const hr = this.backtestData?.holding_returns || {}
      return [...this.portfolioHoldings].sort((a, b) => {
        const wa = hr[a.ticker]?.weight ?? 0
        const wb = hr[b.ticker]?.weight ?? 0
        return wb - wa
      })
    },

    filteredHoldings() {
      if (!this.portfolioHoldings) return []
      
      let holdings = [...this.portfolioHoldings]
      
      if (this.holdingsSearch) {
        const search = this.holdingsSearch.toLowerCase()
        holdings = holdings.filter(h => 
          h.ticker.toLowerCase().includes(search) ||
          h.name.toLowerCase().includes(search) ||
          (h.isin && h.isin.toLowerCase().includes(search))
        )
      }
      
      return holdings
    },

    filteredNews() {
      let news = [...this.portfolioNews]

      if (this.newsFilter !== 'all') {
        news = news.filter(item => {
          if (this.newsFilter === 'positive') return item.sentiment > 0.3
          if (this.newsFilter === 'negative') return item.sentiment < -0.3
          if (this.newsFilter === 'neutral') return item.sentiment >= -0.3 && item.sentiment <= 0.3
          return true
        })
      }

      if (this.selectedTickerFilter !== 'all') {
        news = news.filter(item => item.ticker === this.selectedTickerFilter)
      }

      if (this.newsSearchQuery) {
        const query = this.newsSearchQuery.toLowerCase()
        news = news.filter(item =>
          item.title.toLowerCase().includes(query) ||
          item.summary.toLowerCase().includes(query)
        )
      }

      return news
    },

    newsStats() {
      const stats = { positive: 0, negative: 0, neutral: 0 }
      this.filteredNews.forEach(item => {
        if (item.sentiment > 0.3) stats.positive++
        else if (item.sentiment < -0.3) stats.negative++
        else stats.neutral++
      })
      return stats
    },

    tickerFilterOptions() {
      const options = [{ title: 'All Holdings', value: 'all' }]
      if (this.portfolioHoldings) {
        const tickers = [...new Set(this.portfolioHoldings.map(h => h.ticker))].sort()
        tickers.forEach(ticker => {
          const holding = this.portfolioHoldings.find(h => h.ticker === ticker)
          options.push({
            title: `${ticker} - ${holding.name}`,
            value: ticker
          })
        })
      }
      return options
    },

  },

  watch: {
    selectedInterval() {
      this.fetchPortfolioHistory()
      this.fetchSentimentHistory()
    },

    isAuthenticated(newVal) {
      if (newVal) {
        setTimeout(() => {
          this.updateChartWidth()
          this.fetchPortfolioHistory()
        }, 100)
      }
    },

    homeCurrency(newVal) {
      localStorage.setItem('homeCurrency', newVal)
      this.conversionCache = {}
    },

    activeView(newVal) {
      if (newVal === 'news' && this.portfolioNews.length === 0) {
        this.fetchPortfolioNews()
        this.fetchPortfolioSentiment()
      } else if ((newVal === 'dashboard' || newVal === 'holdings') && !this.portfolioSentiment) {
        this.fetchPortfolioSentiment()
        this.fetchRunningSummary()
        if (this.topGainers.length === 0 && this.topLosers.length === 0) {
          this.fetchTopGainersLosers()
        }
      } else if (newVal === 'statistics' && !this.backtestData) {
        this.fetchBacktest()
      } else if (newVal === 'runningSummary' && !this.runningSummary) {
        this.fetchRunningSummary()
      }
    }
  },

  mounted() {
    const token = this.getCookie('auth_token')
    const email = this.getCookie('user_email')
    const savedCurrency = localStorage.getItem('homeCurrency')
    
    if (savedCurrency) {
      this.homeCurrency = savedCurrency
    }
    
    if (token && !this.isTokenExpired(token)) {
      this.isAuthenticated = true
      this.userEmail = email || ''
      setTimeout(() => {
        this.updateChartWidth()
        this.fetchPortfolioHistory()
        this.fetchPortfolioHoldings()
        this.getPortfolioAllocation()
        this.fetchPortfolioStatistics()
        this.fetchPortfolioSentiment()
        this.fetchSentimentHistory()
        this.fetchTopGainersLosers()
        this.fetchRunningSummary()
        if (this.activeView === 'news') {
          this.fetchPortfolioNews()
        }
      }, 100)
    } else if (token) {
      this.deleteCookie('auth_token')
      this.deleteCookie('user_email')
      this.isAuthenticated = false
      this.userEmail = ''
    }

    window.addEventListener('resize', this.updateChartWidth)
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.updateChartWidth)
  },

  methods: {
    holdingHeatmapBg(vsBenchmark) {
      const v = Math.max(-30, Math.min(30, vsBenchmark))
      if (v >= 0) {
        const i = Math.min(1, v / 30)
        return `rgba(38,166,154,${(0.15 + i * 0.6).toFixed(2)})`
      }
      const i = Math.min(1, Math.abs(v) / 30)
      return `rgba(239,83,80,${(0.15 + i * 0.6).toFixed(2)})`
    },

    holdingTextColor(vsBenchmark) {
      return vsBenchmark >= 0 ? '#26a69a' : '#ef5350'
    },

    holdingChipStyle(ticker) {
      const inactive = this.customPortfolioTickers && !this.customPortfolioTickers.includes(ticker)
      if (!this.backtestData || !this.backtestData.holding_returns || !this.backtestData.holding_returns[ticker]) {
        return inactive ? { opacity: '0.35' } : {}
      }
      const vs = this.backtestData.holding_returns[ticker].vs_benchmark
      const bg = this.holdingHeatmapBg(vs)
      return inactive ? { background: bg, opacity: '0.35' } : { background: bg }
    },

    isTokenExpired(token) {
      const payload = decodeJwtPayload(token)
      if (!payload || typeof payload.exp !== 'number') {
        return true
      }
      return Date.now() >= payload.exp * 1000
    },

    async getPortfolioAllocation() {
      try {
        const token = this.getCookie('auth_token')
        if (!token) return []

        const response = await fetch(`${API_BASE_URL}/api/portfolio/allocation`, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio allocation')
        }
        this.portfolioAllocations = await response.json()
        console.log('Portfolio Allocations:', this.portfolioAllocations)
      } catch (error) {
        console.error('Error fetching portfolio allocation:', error)
      }
    },

    async convertToHomeCurrency(amount, fromCurrency) {
      if (!amount || amount === 0) return 0
      if (fromCurrency === this.homeCurrency) return amount
      
      const cacheKey = `${fromCurrency}_${this.homeCurrency}`
      
      if (this.conversionCache[cacheKey]) {
        return amount * this.conversionCache[cacheKey]
      }
      
      if (this.conversionPromises[cacheKey]) {
        const rate = await this.conversionPromises[cacheKey]
        return amount * rate
      }
      
      this.conversionPromises[cacheKey] = (async () => {
        try {
          const response = await fetch(
            `${API_BASE_URL}/api/convert_currency?amount=1&from_currency=${fromCurrency}&to_currency=${this.homeCurrency}`,
            { headers: { 'Authorization': `Bearer ${this.getCookie('auth_token')}` } }
          )
          
          if (response.ok) {
            const data = await response.json()
            const rate = data.converted_amount
            this.conversionCache[cacheKey] = rate
            delete this.conversionPromises[cacheKey]
            return rate
          }
        } catch (error) {
          console.error('Currency conversion error:', error)
          delete this.conversionPromises[cacheKey]
        }
        return 1
      })()
      
      const rate = await this.conversionPromises[cacheKey]
      return amount * rate
    },

    // Cookie utilities
    getCookie(name) {
      const nameEQ = name + '='
      const cookies = document.cookie.split(';')
      for (let cookie of cookies) {
        cookie = cookie.trim()
        if (cookie.indexOf(nameEQ) === 0) {
          return cookie.substring(nameEQ.length)
        }
      }
      return null
    },

    deleteCookie(name) {
      document.cookie = `${name}=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/;`
    },

    // Search functionality
    debouncedSearch() {
      if (this.searchTimeout) {
        clearTimeout(this.searchTimeout)
      }
      this.searchTimeout = setTimeout(() => {
        this.searchTicker()
      }, 400)
    },

    async searchTicker() {
      if (!this.searchQuery || this.searchQuery.length < 2) {
        this.searchResults = []
        return
      }

      this.searchLoading = true
      try {
        // Determine search type (ISIN if starts with 2 letters followed by numbers)
        const isIsin = /^[A-Z]{2}[A-Z0-9]{9,10}$/i.test(this.searchQuery)
        const searchType = isIsin ? 'isin' : 'ticker'
        
        const response = await fetch(
          `${API_BASE_URL}/api/search?identifier=${encodeURIComponent(this.searchQuery)}&search_type=${searchType}`,
          { headers: { 'Authorization': `Bearer ${this.getCookie('auth_token')}` } }
        )
        
        if (response.ok) {
          this.searchResults = await response.json()
        } else {
          this.searchResults = []
        }
      } catch (error) {
        console.error('Search error:', error)
        this.searchResults = []
      } finally {
        this.searchLoading = false
      }
    },

    selectSearchResult(result) {
      this.selectedResult = result
      this.searchResults = []
      
      // Auto-fill the form
      this.newHolding.name = result.name || ''
      this.newHolding.ticker = result.ticker || ''
      this.newHolding.isin = result.isin || ''
      this.newHolding.exchange = result.exchange || ''
      this.newHolding.currency = result.currency || 'USD'
      this.newHolding.etf = result.type === 'ETF'
      
      // Parse TER if available
      if (result.ter) {
        const terValue = parseFloat(result.ter.replace('%', ''))
        this.newHolding.ter = isNaN(terValue) ? 0 : terValue
      } else {
        this.newHolding.ter = 0
      }
      
      this.newHolding.policy = result.distribution_policy || 'N/A'
      
      // Set current price as default purchase price
      if (result.price && result.price !== 'N/A') {
        this.newHolding.purchasePrice = parseFloat(result.price)
      }
      
      this.addHoldingError = ''
      this.addHoldingSuccess = false
    },

    updateHoldingDayChange(holdingId, changePercent) {
      const holding = this.portfolioHoldings?.find(h => h.id_holding === holdingId)
      if (holding) {
        holding.dayChangePercent = changePercent
      }
    },

    clearSearch() {
      this.searchQuery = ''
      this.searchResults = []
      this.selectedResult = null
      this.resetNewHolding()
    },

    resetNewHolding() {
      this.newHolding = {
        name: '',
        ticker: '',
        isin: '',
        exchange: '',
        currency: 'USD',
        quantity: null,
        purchasePrice: null,
        ter: 0,
        policy: 'N/A',
        etf: false
      }
      this.addHoldingError = ''
      this.addHoldingSuccess = false
    },

    async addHolding() {
      const token = this.getCookie('auth_token')
      if (!token) {
        this.addHoldingError = 'Not authenticated'
        return
      }

      this.addHoldingLoading = true
      this.addHoldingError = ''
      this.addHoldingSuccess = false

      try {
        const formData = new FormData()
        formData.append('Name', this.newHolding.name)
        formData.append('Ticker', this.newHolding.ticker)
        formData.append('ISIN', this.newHolding.isin)
        formData.append('Exchange', this.newHolding.exchange)
        formData.append('Currency', this.newHolding.currency)
        formData.append('Quantity', this.newHolding.quantity.toString())
        formData.append('PurchasePrice', this.newHolding.purchasePrice.toString())
        formData.append('TER', this.newHolding.ter.toString())
        formData.append('Policy', this.newHolding.policy)
        formData.append('ETF', this.newHolding.etf ? 'true' : 'false')

        const response = await fetch(`${API_BASE_URL}/api/asset/holdings`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`
          },
          body: formData
        })

        if (response.ok) {
          this.addHoldingSuccess = true
          this.clearSearch()
          // Refresh holdings
          this.fetchPortfolioHoldings()
          this.fetchPortfolioHistory()
          
          // Auto-hide success message after 3 seconds
          setTimeout(() => {
            this.addHoldingSuccess = false
          }, 3000)
        } else {
          const errorText = await response.text()
          this.addHoldingError = errorText || 'Failed to add holding'
        }
      } catch (error) {
        console.error('Add holding error:', error)
        this.addHoldingError = error.message || 'Failed to add holding'
      } finally {
        this.addHoldingLoading = false
      }
    },

    onPortfolioChartReady({ chart, candleSeries }) {
      if (this._portfolioChart !== chart) {
        this.sentimentSeries = null
        this.portfolioCostBasisLine = null
      }

      this._portfolioChart = chart
      this._portfolioCandleSeries = candleSeries

      if (this.portfolioCostBasisLine) {
        try { candleSeries.removePriceLine(this.portfolioCostBasisLine) } catch (_) {}
        this.portfolioCostBasisLine = null
      }

      if (this.statsData.total_cost && this.statsData.total_cost > 0) {
        this.portfolioCostBasisLine = candleSeries.createPriceLine({
          price: this.statsData.total_cost,
          color: '#808080',
          lineWidth: 2,
          lineStyle: 2,
          axisLabelVisible: true,
          title: 'Cost Basis'
        })
      }

      if (!this.sentimentSeries) {
        const sentimentPane = chart.addPane()
        sentimentPane.setHeight(90)
        this.sentimentSeries = chart.addSeries(BaselineSeries, {
          baseValue: { type: 'price', price: 0 },
          topLineColor: '#26a79a',
          topFillColor1: 'rgba(38,167,154,0.28)',
          topFillColor2: 'rgba(38,167,154,0.05)',
          bottomLineColor: '#ef5250',
          bottomFillColor1: 'rgba(239,82,80,0.05)',
          bottomFillColor2: 'rgba(239,82,80,0.28)',
          lineWidth: 1.5,
          priceLineVisible: false,
          lastValueVisible: true,
          title: 'Sentiment'
        }, sentimentPane.paneIndex())

        if (this.sentimentHistory && this.sentimentHistory.length > 0) {
          this.sentimentSeries.setData(
            this.sentimentHistory.map(p => ({ time: p.timestamp, value: p.sentiment }))
          )
        }
      }
    },

    updateCostBasisLine() {
      if (!this._portfolioCandleSeries) return
      if (this.portfolioCostBasisLine) {
        this._portfolioCandleSeries.removePriceLine(this.portfolioCostBasisLine)
      }
      if (this.statsData.total_cost && this.statsData.total_cost > 0) {
        this.portfolioCostBasisLine = this._portfolioCandleSeries.createPriceLine({
          price: this.statsData.total_cost,
          color: '#808080',
          lineWidth: 2,
          lineStyle: 2,
          axisLabelVisible: true,
          title: 'Cost Basis'
        })
      }
    },

    // Fetch portfolio value history
    async fetchPortfolioHistory() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.portfolioLoading = true
      this.portfolioError = ''

      try {
        const response = await fetch(
          `${API_BASE_URL}/api/portfolio/history?interval=${this.selectedInterval}`,
          {
            headers: {
              'Authorization': `Bearer ${token}`
            }
          }
        )

        if (!response.ok) {
          throw new Error('Failed to fetch portfolio history')
        }

        const data = await response.json()

        this.portfolioData = data.map(item => ({
          timestamp: item.timestamp,
          open: item.open,
          high: item.high,
          low: item.low,
          close: item.close,
          volume: 0
        }))
        
        this.$nextTick(() => {
          if (this.$refs.portfolioChart) {
            this.updateCostBasisLine()
          }
        })
      } catch (error) {
        console.error('Error fetching portfolio history:', error)
        this.portfolioError = error.message || 'Failed to load portfolio data'
      } finally {
        this.portfolioLoading = false
      }
    },

    async fetchPortfolioStatistics() {
      const token = this.getCookie('auth_token')
      if (!token) return

      try {
        const url = `${API_BASE_URL}/api/portfolio/stats`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio statistics')
        }
        const data = await response.json()
        this.statsData = data
        console.log('Portfolio Statistics:', data)
      } catch (error) {
        console.error('Error fetching portfolio statistics:', error)
      }
    },

    async fetchPortfolioHoldings() {
      const token = this.getCookie('auth_token')
      if (!token) return

      try {
        const url = `${API_BASE_URL}/api/portfolio/holdings`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio holdings')
        }
        const data = await response.json()
        this.portfolioHoldings = data
        console.log('Portfolio Holdings:', data)
      } catch (error) {
        console.error('Error fetching portfolio holdings:', error)
      }
    },

    async fetchPortfolioNews() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.newsLoading = true
      this.newsError = ''
      this.newsOffset = 0
      this.portfolioNews = []

      try {
        const url = `${API_BASE_URL}/api/portfolio/news?limit=${this.newsLimit}&offset=0`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio news')
        }
        const data = await response.json()
        this.portfolioNews = data || []
        this.newsHasMore = data && data.length === this.newsLimit
      } catch (error) {
        console.error('Error fetching portfolio news:', error)
        this.newsError = error.message || 'Failed to load news'
      } finally {
        this.newsLoading = false
      }
    },

    async fetchPortfolioSentiment() {
      const token = this.getCookie('auth_token')
      if (!token) return

      try {
        const today = new Date().toISOString().split('T')[0]
        const url = `${API_BASE_URL}/api/portfolio/daily_sentiment?date=${today}`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to fetch portfolio sentiment')
        }
        const data = await response.json()
        this.portfolioSentiment = data
      } catch (error) {
        console.error('Error fetching portfolio sentiment:', error)
      }
    },

    async generateSummary() {
      const token = this.getCookie('auth_token')
      if (!token) return
      this.summaryGenerating = true
      try {
        const response = await fetch(`${API_BASE_URL}/api/news/generate-summary`, {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        })
        if (response.ok) {
          const data = await response.json()
          this.portfolioSentiment = {
            sentiment: data.sentiment,
            summary: data.summary,
            date: data.date
          }
        } else if (response.status === 429) {
          const err = await response.json()
          alert(`Cooldown: ${err.error}. Try again in about ${err.retry_after_min} minute(s).`)
        } else {
          const err = await response.json()
          alert(err.error || 'Failed to generate summary')
        }
      } catch (error) {
        console.error('Error generating summary:', error)
        alert('Failed to generate summary. Ensure the Python backend is running.')
      } finally {
        this.summaryGenerating = false
      }
    },

    async fetchRunningSummary() {
      const token = this.getCookie('auth_token')
      if (!token) return
      try {
        const response = await fetch(
          `${API_BASE_URL}/api/running-summary?window_days=${this.runningSummaryWindowDays}`,
          { headers: { 'Authorization': `Bearer ${token}` } }
        )
        if (response.ok) {
          this.runningSummary = await response.json()
        } else if (response.status === 404) {
          this.runningSummary = null
        }
      } catch (error) {
        console.error('Error fetching running summary:', error)
      }
    },

    async generateRunningSummary() {
      const token = this.getCookie('auth_token')
      if (!token) return
      this.runningSummaryGenerating = true
      try {
        const response = await fetch(
          `${API_BASE_URL}/api/running-summary/generate`,
          {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${token}`,
              'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: `window_days=${this.runningSummaryWindowDays}`
          }
        )
        if (response.ok) {
          this.runningSummary = await response.json()
        } else if (response.status === 429) {
          const err = await response.json()
          alert(`Cooldown: ${err.error}. Try again in about ${err.retry_after_min} minute(s).`)
        } else {
          const err = await response.json()
          alert(err.error || 'Failed to generate running summary')
        }
      } catch (error) {
        console.error('Error generating running summary:', error)
        alert('Failed to generate running summary. Ensure the Python backend is running.')
      } finally {
        this.runningSummaryGenerating = false
      }
    },

    async fetchBacktest() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.backtestLoading = true
      this.backtestError = ''

      try {
        const now = new Date()
        let startDate

        switch (this.backtestPeriod) {
          case '6M':
            startDate = new Date(now.setMonth(now.getMonth() - 6))
            break
          case '1Y':
            startDate = new Date(now.setFullYear(now.getFullYear() - 1))
            break
          case '3Y':
            startDate = new Date(now.setFullYear(now.getFullYear() - 3))
            break
          case '5Y':
            startDate = new Date(now.setFullYear(now.getFullYear() - 5))
            break
          case '10Y':
            startDate = new Date(now.setFullYear(now.getFullYear() - 10))
            break
          case 'ALL':
            startDate = new Date('2000-01-01')
            break
          default:
            startDate = new Date(now.setFullYear(now.getFullYear() - 1))
        }

        const endDate = new Date()
        const startDateStr = startDate.toISOString().split('T')[0]
        const endDateStr = endDate.toISOString().split('T')[0]

        let url = `${API_BASE_URL}/api/portfolio/backtest?start_date=${startDateStr}&end_date=${endDateStr}&benchmark=${this.backtestBenchmark}`
        if (this.customPortfolioTickers && this.customPortfolioTickers.length > 0) {
          url += `&tickers=${encodeURIComponent(this.customPortfolioTickers.join(','))}`
        }
        
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })

        if (!response.ok) {
          throw new Error('Failed to fetch backtest data')
        }

        this.backtestData = await response.json()
      } catch (error) {
        console.error('Error fetching backtest:', error)
        this.backtestError = error.message || 'Failed to load backtest data'
      } finally {
        this.backtestLoading = false
      }
    },

    async loadMoreNews() {
      const token = this.getCookie('auth_token')
      if (!token || this.newsLoadingMore || !this.newsHasMore) return

      this.newsLoadingMore = true
      const newOffset = this.newsOffset + this.newsLimit

      try {
        const url = `${API_BASE_URL}/api/portfolio/news?limit=${this.newsLimit}&offset=${newOffset}`
        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        })
        if (!response.ok) {
          throw new Error('Failed to load more news')
        }
        const data = await response.json()
        if (data && data.length > 0) {
          this.portfolioNews = [...this.portfolioNews, ...data]
          this.newsOffset = newOffset
          this.newsHasMore = data.length === this.newsLimit
        } else {
          this.newsHasMore = false
        }
      } catch (error) {
        console.error('Error loading more news:', error)
      } finally {
        this.newsLoadingMore = false
      }
    },

    refreshNews() {
      this.fetchPortfolioNews()
      this.fetchPortfolioSentiment()
    },

    async fetchTopGainersLosers() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.topGainersLosersLoading = true

      try {
        const [gainersResponse, losersResponse] = await Promise.all([
          fetch(`${API_BASE_URL}/api/portfolio/top_gainers`, {
            headers: { 'Authorization': `Bearer ${token}` }
          }),
          fetch(`${API_BASE_URL}/api/portfolio/top_losers`, {
            headers: { 'Authorization': `Bearer ${token}` }
          })
        ])

        if (gainersResponse.ok) {
          this.topGainers = await gainersResponse.json() || []
        }
        if (losersResponse.ok) {
          this.topLosers = await losersResponse.json() || []
        }
      } catch (error) {
        console.error('Error fetching top gainers/losers:', error)
      } finally {
        this.topGainersLosersLoading = false
      }
    },

    updateChartWidth() {
      const container = document.querySelector('.chart-wrapper')
      if (container) {
        this.chartWidth = container.clientWidth - 20
      }
    },

    async fetchSentimentHistory() {
      const token = this.getCookie('auth_token')
      if (!token) return

      this.sentimentLoading = true
      try {
        const response = await fetch(
          `${API_BASE_URL}/api/portfolio/sentiment_history?interval=${this.selectedInterval}`,
          { headers: { 'Authorization': `Bearer ${token}` } }
        )
        if (!response.ok) throw new Error('Failed to fetch sentiment history')
        this.sentimentHistory = await response.json() || []
        if (this.sentimentSeries && this.sentimentHistory.length > 0) {
          try {
            this.sentimentSeries.setData(
              this.sentimentHistory.map(p => ({ time: p.timestamp, value: p.sentiment }))
            )
          } catch (_) {
            this.sentimentSeries = null
          }
        }
      } catch (error) {
        console.error('Error fetching sentiment history:', error)
        this.sentimentHistory = []
      } finally {
        this.sentimentLoading = false
      }
    },

    logout() {
      this.deleteCookie('auth_token')
      this.deleteCookie('user_email')
      this.isAuthenticated = false
      this.userEmail = ''
    },

    openHoldingDetail(holding) {
      console.log('Opening holding detail:', holding)
    },

    scrollLegendIntoView(type, index) {
      this.$nextTick(() => {
        const legendItem = document.querySelector(`.legend-item[data-type="${type}"][data-index="${index}"]`)
        if (legendItem) {
          legendItem.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
        }
      })
    },

    generatePieSlices(items, colors) {
      if (!items || items.length === 0) return []
      
      const total = items.reduce((sum, item) => sum + item.percentage, 0)
      if (total === 0) return []
      
      const slices = []
      let currentAngle = -90

      items.forEach((item, index) => {
        const percentage = (item.percentage / total) * 100
        const angle = (percentage / 100) * 360
        const startAngle = currentAngle
        const endAngle = currentAngle + angle

        const startRad = (startAngle * Math.PI) / 180
        const endRad = (endAngle * Math.PI) / 180

        const radius = 80
        const x1 = Math.cos(startRad) * radius
        const y1 = Math.sin(startRad) * radius
        const x2 = Math.cos(endRad) * radius
        const y2 = Math.sin(endRad) * radius

        const largeArc = angle > 180 ? 1 : 0

        const path = `M 0 0 L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${x2} ${y2} Z`

        slices.push({
          path,
          color: colors[index % colors.length],
          percentage: item.percentage,
          name: item.name
        })

        currentAngle = endAngle
      })

      return slices
    },

    toggleCustomTicker(ticker) {
      if (!this.customPortfolioTickers) {
        this.customPortfolioTickers = this.portfolioHoldings
          .map(h => h.ticker)
          .filter(t => t !== ticker)
      } else {
        const idx = this.customPortfolioTickers.indexOf(ticker)
        if (idx > -1) {
          const updated = [...this.customPortfolioTickers]
          updated.splice(idx, 1)
          this.customPortfolioTickers = updated.length > 0 ? updated : null
        } else {
          this.customPortfolioTickers = [...this.customPortfolioTickers, ticker]
          if (this.portfolioHoldings && this.customPortfolioTickers.length === this.portfolioHoldings.length) {
            this.customPortfolioTickers = null
          }
        }
      }
      this.fetchBacktest()
    },

    formatCurrency(value) {
      if (value === null || value === undefined) return '€0.00'
      return new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency: 'EUR',
        minimumFractionDigits: 2,
        maximumFractionDigits: 2
      }).format(value)
    },

    getSentimentColor(sentiment) {
      if (sentiment > 0.3) return '#4CAF50'
      if (sentiment < -0.3) return '#EF5350'
      return '#9E9E9E'
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
.chart-wrapper {
  width: 100%;
}

.add-holding-form {
  background-color: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 8px;
  margin: 0 8px;
}

.search-results {
  max-height: 200px;
  overflow-y: auto;
  background-color: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), 0.12);
  border-radius: 4px;
}

.search-results .v-list-item {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
  cursor: pointer;
}

.search-results .v-list-item:hover {
  background-color: rgba(var(--v-theme-primary), 0.08);
}

.search-results .v-list-item:last-child {
  border-bottom: none;
}

.text-primary {
  color: rgb(var(--v-theme-primary)) !important;
}

.bg-primary-lighten-4 {
  background-color: rgba(var(--v-theme-primary), 0.15) !important;
}

/* Holdings table styling */
.holdings-table {
  width: 100%;
}

.holdings-table :deep(thead th) {
  font-size: 0.75rem !important;
  font-weight: 600 !important;
  text-transform: uppercase !important;
  letter-spacing: 0.5px !important;
  color: rgba(var(--v-theme-on-surface), 0.6) !important;
  background-color: rgba(var(--v-theme-surface-variant), 0.4) !important;
  padding: 12px 8px !important;
  white-space: nowrap;
}

.holdings-table :deep(table) {
  table-layout: fixed;
  width: 100%;
}

.pie-chart-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  height: 380px;
}

.pie-chart {
  width: 160px;
  height: 160px;
  flex-shrink: 0;
}

.pie-slice {
  cursor: pointer;
  transition: opacity 0.2s ease, transform 0.2s ease, filter 0.2s ease;
  transform-origin: center;
  filter: brightness(1);
}

.pie-slice:hover,
.pie-slice-active {
  opacity: 0.9;
  transform: scale(1.05);
  filter: brightness(1.1);
}

.pie-legend {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-height: 180px;
  overflow-y: auto;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  transition: all 0.2s ease;
  cursor: pointer;
  border: 1px solid transparent;
}

.legend-item:hover {
  background-color: rgba(var(--v-theme-primary), 0.08);
  border-color: rgba(var(--v-theme-primary), 0.3);
  transform: translateX(2px);
}

.legend-item-active {
  background-color: rgba(var(--v-theme-primary), 0.15);
  border-color: rgba(var(--v-theme-primary), 0.4);
  transform: translateX(2px);
}

.legend-color {
  width: 12px;
  height: 12px;
  border-radius: 2px;
  flex-shrink: 0;
}

.legend-text {
  flex: 1;
  font-size: 0.75rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.legend-value {
  font-size: 0.8rem;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.stat-item {
  padding: 4px 0;
}

.stat-item-inline {
  display: flex;
  align-items: center;
  padding: 2px 0;
}

.gainer-item,
.loser-item {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
  transition: background-color 0.2s ease;
}

.gainer-item:last-child,
.loser-item:last-child {
  border-bottom: none;
}

.gainer-item:hover {
  background-color: rgba(76, 175, 80, 0.05);
}

.loser-item:hover {
  background-color: rgba(239, 83, 80, 0.05);
}

.news-item-compact {
  border-left: 3px solid transparent;
  background-color: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 4px;
  transition: all 0.2s ease;
}

.news-item-compact:hover {
  border-left-color: rgb(var(--v-theme-primary));
  background-color: rgba(var(--v-theme-surface-variant), 0.5);
}

.news-link-compact {
  color: inherit;
  text-decoration: none;
  transition: color 0.2s;
}

.news-link-compact:hover {
  color: rgb(var(--v-theme-primary));
}

.gainer-news-card,
.loser-news-card {
  border-top: 3px solid;
}

.gainer-news-card {
  border-top-color: #4CAF50;
}

.loser-news-card {
  border-top-color: #EF5350;
}

.bg-success-lighten-4,
.bg-success-lighten-5 {
  background-color: rgba(76, 175, 80, 0.08) !important;
}

.bg-error-lighten-4,
.bg-error-lighten-5 {
  background-color: rgba(239, 83, 80, 0.08) !important;
}

.holding-perf-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 4px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.18);
  cursor: pointer;
  transition: filter 0.15s ease, opacity 0.15s ease;
  user-select: none;
  background: rgba(var(--v-theme-on-surface), 0.06);
}

.holding-perf-chip:hover {
  filter: brightness(1.25);
}

.holding-perf-chip--inactive {
  opacity: 0.35;
}

.hpc-ticker {
  font-size: 0.75rem;
  font-weight: 600;
}

.hpc-delta {
  font-size: 0.68rem;
  font-weight: 500;
}

.attrib-table-head {
  display: grid;
  grid-template-columns: 1fr 60px 60px 60px 70px 68px 46px 48px;
  padding: 3px 12px 3px 13px;
  font-size: 0.68rem;
  color: rgba(var(--v-theme-on-surface), 0.4);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.attrib-row {
  display: grid;
  grid-template-columns: 1fr 60px 60px 60px 70px 68px 46px 48px;
  align-items: center;
  padding: 4px 12px 4px 10px;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.07);
}

.attrib-row:last-of-type {
  border-bottom: none;
}

.attrib-cell {
  font-size: 0.75rem;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.signal-badge {
  font-size: 0.62rem;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 3px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  float: right;
}

.signal-BUY_DIP {
  background: rgba(38,166,154,0.18);
  color: #26a69a;
  border: 1px solid rgba(38,166,154,0.35);
}

.signal-STRONG {
  background: rgba(33,150,243,0.15);
  color: #2196F3;
  border: 1px solid rgba(33,150,243,0.3);
}

.signal-WEAK {
  background: rgba(239,83,80,0.15);
  color: #ef5350;
  border: 1px solid rgba(239,83,80,0.3);
}

.signal-NEUTRAL {
  color: rgba(var(--v-theme-on-surface), 0.3);
}

.attrib-info {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}

.attrib-ticker {
  font-size: 0.78rem;
  font-weight: 700;
  white-space: nowrap;
  flex-shrink: 0;
}

.attrib-name {
  font-size: 0.7rem;
  color: rgba(var(--v-theme-on-surface), 0.45);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.attrib-metrics {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.attrib-return {
  font-size: 0.78rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
</style>