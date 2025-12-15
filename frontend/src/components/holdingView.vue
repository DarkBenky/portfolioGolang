<template>
  <tbody class="holding-tbody">
    <tr class="holding-row" @click="opened = !opened">
    <!-- Mini Chart -->
    <td class="chart-cell">
      <div class="mini-chart-container">
        <svg :width="chartWidth" :height="chartHeight" class="mini-chart">
          <polyline
            :points="chartPoints"
            fill="none"
            :stroke="isPositiveDay ? '#26a79a' : '#ef5250'"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </div>
    </td>

    <!-- Ticker -->
    <td>
      <div class="d-flex align-center">
        <div>
          <div class="font-weight-bold">{{ holding.ticker }}</div>
          <div class="text-caption text-grey text-truncate" style="max-width: 140px;">
            {{ holding.name }}
          </div>
        </div>
        <v-chip v-if="holding.etf" size="x-small" color="blue" class="ml-2">ETF</v-chip>
      </div>
    </td>

    <!-- Current Price -->
    <td class="text-right">
      <span class="font-weight-medium">{{ formatCurrency(currentPrice, holding.currency) }}</span>
    </td>

    <!-- Day Change % -->
    <td class="text-right">
      <v-chip
        :color="isPositiveDay ? 'success' : 'error'"
        size="small"
        variant="tonal"
        label
      >
        {{ isPositiveDay ? '+' : '' }}{{ formatPercent(dayChangePercent) }}
      </v-chip>
    </td>

    <!-- Day Change $ -->
    <td class="text-right">
      <span :class="isPositiveDay ? 'text-success' : 'text-error'">
        {{ isPositiveDay ? '+' : '' }}{{ formatCurrency(dayChange, holding.currency) }}
      </span>
    </td>

    <!-- Total Change % -->
    <td class="text-right">
      <v-chip
        :color="isPositiveTotal ? 'success' : 'error'"
        size="small"
        variant="tonal"
        label
      >
        {{ isPositiveTotal ? '+' : '' }}{{ formatPercent(totalChangePercent) }}
      </v-chip>
    </td>

    <!-- Total Change $ -->
    <td class="text-right">
      <span :class="isPositiveTotal ? 'text-success' : 'text-error'" class="font-weight-medium">
        {{ isPositiveTotal ? '+' : '' }}{{ formatCurrency(totalChange, holding.currency) }}
      </span>
    </td>

    <!-- Current Value -->
    <td class="text-right">
      <div class="font-weight-bold">{{ formatCurrency(currentValue, holding.currency) }}</div>
      <div class="text-caption text-grey">{{ formatQuantity(holding.quantity) }} {{ holding.etf ? 'units' : 'shares' }}</div>
    </td>

    <!-- Expand Icon -->
    <td class="text-center expand-cell">
      <v-icon size="small" :class="{ 'rotate-180': opened }">
        mdi-chevron-down
      </v-icon>
    </td>
  </tr>

  <!-- Expandable Detail Row -->
  <tr v-if="opened" class="detail-row">
    <td colspan="9" class="pa-0">
      <div class="detail-container">
        <!-- Full Price Chart at Top -->
        <v-row class="mb-4">
          <v-col cols="12">
            <v-card variant="outlined">
              <v-card-title class="d-flex justify-space-between align-center pb-2">
                <div class="d-flex align-center">
                  <v-icon size="small" class="mr-2">mdi-chart-line</v-icon>
                  Price Chart - {{ holding.ticker }}
                </div>
                <div class="d-flex ga-2">
                  <v-btn-toggle v-model="chartPeriod" mandatory density="compact" color="primary">
                    <v-btn value="1d" size="x-small">1D</v-btn>
                    <v-btn value="1w" size="x-small">1W</v-btn>
                    <v-btn value="1m" size="x-small">1M</v-btn>
                    <v-btn value="3m" size="x-small">3M</v-btn>
                    <v-btn value="1y" size="x-small">1Y</v-btn>
                  </v-btn-toggle>
                </div>
              </v-card-title>
              <v-card-text class="pa-0">
                <div v-if="chartLoading" class="d-flex justify-center align-center" style="height: 300px;">
                  <v-progress-circular indeterminate color="primary" size="40"></v-progress-circular>
                </div>
                <div v-else-if="fullPriceHistory.length > 0" style="width: 100%;">
                  <CandleChart
                  :data="fullPriceHistory"
                  :height="500"
                  :show-volume="false"
                  theme="dark"/>
                </div>
                <div v-else class="d-flex justify-center align-center text-grey" style="height: 300px;">
                  No chart data available
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- Info Cards Row -->
        <v-row>
          <!-- Left Column: Asset Info -->
          <v-col cols="12" md="4">
            <v-card variant="outlined" class="h-100">
              <v-card-title class="text-subtitle-1 pb-2">
                <v-icon size="small" class="mr-2">mdi-information-outline</v-icon>
                Asset Details
              </v-card-title>
              <v-card-text>
                <div class="detail-grid">
                  <div class="detail-item">
                    <span class="detail-label">Name</span>
                    <span class="detail-value">{{ holding.name }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Ticker</span>
                    <span class="detail-value font-weight-bold">{{ holding.ticker }}</span>
                  </div>
                  <div v-if="holding.isin && holding.isin !== 'N/A'" class="detail-item">
                    <span class="detail-label">ISIN</span>
                    <span class="detail-value text-mono">{{ holding.isin }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Exchange</span>
                    <span class="detail-value">{{ holding.exchange || 'N/A' }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Currency</span>
                    <span class="detail-value">{{ holding.currency }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Type</span>
                    <v-chip :color="holding.etf ? 'blue' : 'grey'" size="x-small">
                      {{ holding.etf ? 'ETF' : 'Stock/Crypto' }}
                    </v-chip>
                  </div>
                  <div v-if="holding.etf && holding.ter > 0" class="detail-item">
                    <span class="detail-label">TER</span>
                    <span class="detail-value">{{ holding.ter.toFixed(2) }}%</span>
                  </div>
                  <div v-if="holding.policy && holding.policy !== 'N/A'" class="detail-item">
                    <span class="detail-label">Distribution</span>
                    <v-chip :color="holding.policy === 'Accumulating' ? 'purple' : 'orange'" size="x-small">
                      {{ holding.policy }}
                    </v-chip>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>

          <!-- Middle Column: Position Info -->
          <v-col cols="12" md="4">
            <v-card variant="outlined" class="h-100">
              <v-card-title class="text-subtitle-1 pb-2">
                <v-icon size="small" class="mr-2">mdi-wallet-outline</v-icon>
                Position
              </v-card-title>
              <v-card-text>
                <div class="detail-grid">
                  <div class="detail-item">
                    <span class="detail-label">Quantity</span>
                    <span class="detail-value font-weight-bold">{{ formatQuantity(holding.quantity) }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Avg. Purchase Price</span>
                    <span class="detail-value">{{ formatCurrency(holding.purchase_price, holding.currency) }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Cost Basis</span>
                    <span class="detail-value">{{ formatCurrency(costBasis, holding.currency) }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Current Price</span>
                    <span class="detail-value font-weight-bold">{{ formatCurrency(currentPrice, holding.currency) }}</span>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Current Value</span>
                    <span class="detail-value font-weight-bold text-primary">{{ formatCurrency(currentValue, holding.currency) }}</span>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>

          <!-- Right Column: Performance -->
          <v-col cols="12" md="4">
            <v-card variant="outlined" class="h-100">
              <v-card-title class="text-subtitle-1 pb-2">
                <v-icon size="small" class="mr-2">mdi-trending-up</v-icon>
                Performance
              </v-card-title>
              <v-card-text>
                <div class="detail-grid">
                  <div class="detail-item">
                    <span class="detail-label">Today's Change</span>
                    <div>
                      <span :class="isPositiveDay ? 'text-success' : 'text-error'" class="font-weight-bold">
                        {{ isPositiveDay ? '+' : '' }}{{ formatCurrency(dayChange, holding.currency) }}
                      </span>
                      <v-chip :color="isPositiveDay ? 'success' : 'error'" size="x-small" class="ml-2">
                        {{ isPositiveDay ? '+' : '' }}{{ dayChangePercent.toFixed(2) }}%
                      </v-chip>
                    </div>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Total Gain/Loss</span>
                    <div>
                      <span :class="isPositiveTotal ? 'text-success' : 'text-error'" class="font-weight-bold">
                        {{ isPositiveTotal ? '+' : '' }}{{ formatCurrency(totalChange, holding.currency) }}
                      </span>
                      <v-chip :color="isPositiveTotal ? 'success' : 'error'" size="x-small" class="ml-2">
                        {{ isPositiveTotal ? '+' : '' }}{{ totalChangePercent.toFixed(2) }}%
                      </v-chip>
                    </div>
                  </div>
                  <div class="detail-item">
                    <span class="detail-label">Return on Investment</span>
                    <span :class="isPositiveTotal ? 'text-success' : 'text-error'" class="detail-value font-weight-bold text-h6">
                      {{ isPositiveTotal ? '+' : '' }}{{ totalChangePercent.toFixed(2) }}%
                    </span>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- Daily Summary Section -->
        <v-row class="mt-4">
          <v-col cols="12">
            <v-card variant="outlined">
              <v-card-title class="text-subtitle-1 pb-2 d-flex justify-space-between align-center">
                <div class="d-flex align-center">
                  <v-icon size="small" class="mr-2">mdi-text-box-outline</v-icon>
                  Today's Summary - {{ holding.ticker }}
                </div>
                <v-chip 
                  v-if="dailySummary?.sentiment !== undefined"
                  :color="getSentimentColor(dailySummary.sentiment)" 
                  size="small"
                  variant="tonal"
                >
                  Sentiment: {{ getSentimentLabel(dailySummary.sentiment) }}
                </v-chip>
              </v-card-title>
              <v-card-text>
                <div v-if="dailySummaryLoading" class="d-flex justify-center py-4">
                  <v-progress-circular indeterminate color="primary" size="30"></v-progress-circular>
                </div>
                <div v-else-if="dailySummary?.summary" class="summary-content">
                  <div class="text-body-2 markdown-content" v-html="formatMarkdown(dailySummary.summary)"></div>
                </div>
                <div v-else class="text-center text-grey py-4">
                  <v-icon size="36" class="mb-2">mdi-text-box-remove-outline</v-icon>
                  <div>No summary available for today</div>
                  <div class="text-caption mt-1">Summaries are generated periodically from recent news</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- Pie Charts for ETFs - Sectors, Regions & Top 10 Holdings -->
        <v-row v-if="holding.etf && (holding.sectors?.length > 0 || holding.regions?.length > 0 || holding.assets?.length > 0)" class="mt-4">
          <v-col v-if="holding.sectors?.length > 0" cols="12" md="4">
            <v-card variant="outlined">
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

          <v-col v-if="holding.regions?.length > 0" cols="12" md="4">
            <v-card variant="outlined">
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

          <v-col v-if="holding.assets?.length > 0" cols="12" md="4">
            <v-card variant="outlined">
              <v-card-title class="text-subtitle-1 pb-2">
                <v-icon size="small" class="mr-2">mdi-chart-pie</v-icon>
                Top 10 Holdings
              </v-card-title>
              <v-card-text>
                <div class="pie-chart-container" style="overflow-y: scroll">
                  <svg viewBox="0 0 200 200" class="pie-chart">
                    <g transform="translate(100, 100)">
                      <path
                        v-for="(slice, index) in topHoldingsPieSlices"
                        :key="'holding-' + index"
                        :d="slice.path"
                        :fill="slice.color"
                        class="pie-slice"
                        :class="{ 'pie-slice-active': hoveredSlice?.type === 'holding' && hoveredSlice?.index === index }"
                        @mouseenter="hoveredSlice = { type: 'holding', index }; scrollLegendIntoView('holding', index)"
                        @mouseleave="hoveredSlice = null"
                      >
                        <title>{{ slice.name }}: {{ slice.percentage.toFixed(2) }}%</title>
                      </path>
                    </g>
                  </svg>
                  <div class="pie-legend">
                    <div 
                      v-for="(asset, index) in topHoldingsForPie" 
                      :key="asset.id_asset" 
                      class="legend-item"
                      :class="{ 'legend-item-active': hoveredSlice?.type === 'holding' && hoveredSlice?.index === index }"
                      :data-type="'holding'"
                      :data-index="index"
                      @mouseenter="hoveredSlice = { type: 'holding', index }; scrollLegendIntoView('holding', index)"
                      @mouseleave="hoveredSlice = null"
                    >
                      <span class="legend-color" :style="{ backgroundColor: pieColors[index % pieColors.length] }"></span>
                      <span class="legend-text">{{ asset.name }}</span>
                      <span class="legend-value">{{ calculateAssetPercentage(index) }}%</span>
                    </div>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- Top 10 Holdings for ETFs -->
        <v-row v-if="holding.etf && holding.assets?.length > 0" class="mt-4">
          <v-col cols="12">
            <v-card variant="outlined">
              <v-card-title class="text-subtitle-1 pb-2 d-flex justify-space-between align-center">
                <div class="d-flex align-center">
                  <v-icon size="small" class="mr-2">mdi-view-list</v-icon>
                  Top Holdings
                </div>
                <v-chip size="small" color="blue" variant="tonal">
                  {{ holding.assets.length }} Assets
                </v-chip>
              </v-card-title>
              <v-card-text class="pa-0">
                <v-table density="compact" class="holdings-sub-table">
                  <thead>
                    <tr>
                      <th class="text-left">Chart</th>
                      <th class="text-left">Weight</th>
                      <th class="text-left">Name</th>
                      <th class="text-left">ISIN</th>
                      <th class="text-left">Country</th>
                      <th class="text-left">Sector</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr 
                      v-for="(asset, index) in holding.assets" 
                      :key="asset.id_asset" 
                      class="asset-row"
                      @click="toggleAssetDetails(asset)"
                    >
                      <td class="chart-cell">
                        <div class="mini-chart-container">
                          <svg :width="60" :height="24" class="mini-chart">
                            <polyline
                              v-if="assetCharts[asset.name]?.length > 0"
                              :points="getAssetChartPoints(asset.name, 60, 24)"
                              fill="none"
                              :stroke="getAssetChartColor(asset.name)"
                              stroke-width="1.5"
                              stroke-linecap="round"
                              stroke-linejoin="round"
                            />
                            <line v-else x1="0" y1="12" x2="60" y2="12" stroke="#666" stroke-width="1" />
                          </svg>
                        </div>
                      </td>
                      <td class="font-weight-bold">{{ calculateAssetPercentage(index) }}%</td>
                      <td>
                        <div class="text-truncate" style="max-width: 200px;">
                          <div class="font-weight-medium">{{ asset.name }}</div>
                        </div>
                      </td>
                      <td class="text-mono text-caption">{{ asset.isin || 'N/A' }}</td>
                      <td>
                        <v-chip 
                          v-if="assetDetails[asset.name]?.country" 
                          size="x-small" 
                          color="secondary" 
                          variant="tonal"
                        >
                          {{ assetDetails[asset.name].country }}
                        </v-chip>
                        <v-chip v-else-if="asset.region" size="x-small" color="secondary" variant="tonal">
                          {{ asset.region }}
                        </v-chip>
                        <span v-else class="text-grey">-</span>
                      </td>
                      <td>
                        <v-chip 
                          v-if="assetDetails[asset.name]?.sector" 
                          size="x-small" 
                          color="primary" 
                          variant="tonal"
                        >
                          {{ assetDetails[asset.name].sector }}
                        </v-chip>
                        <v-chip v-else-if="asset.sector" size="x-small" color="primary" variant="tonal">
                          {{ asset.sector }}
                        </v-chip>
                        <span v-else class="text-grey">-</span>
                      </td>
                    </tr>
                    <tr v-if="expandedAsset && expandedAsset.name" class="asset-detail-row">
                      <td colspan="6" class="pa-0">
                        <div class="asset-detail-container">
                          <v-row dense>
                            <v-col cols="12" md="6">
                              <div class="detail-section">
                                <h4 class="text-subtitle-2 mb-2">Financial Metrics</h4>
                                <div class="metric-grid">
                                  <div class="metric-item">
                                    <span class="metric-label">Market Cap</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.market_cap || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">P/E Ratio</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.pe_ratio || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">P/B Ratio</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.pb_ratio || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">EPS</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.eps || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">Dividend Yield</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.dividend_yield ? assetDetails[expandedAsset.name].dividend_yield + '%' : 'N/A' }}</span>
                                  </div>
                                </div>
                              </div>
                            </v-col>
                            <v-col cols="12" md="6">
                              <div class="detail-section">
                                <h4 class="text-subtitle-2 mb-2">Revenue & Profitability</h4>
                                <div class="metric-grid">
                                  <div class="metric-item">
                                    <span class="metric-label">Revenue</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.revenue || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">Net Income</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.net_income || 'N/A' }}</span>
                                  </div>
                                  <div class="metric-item">
                                    <span class="metric-label">Profit Margin</span>
                                    <span class="metric-value">{{ assetDetails[expandedAsset.name]?.profit_margin ? assetDetails[expandedAsset.name].profit_margin + '%' : 'N/A' }}</span>
                                  </div>
                                </div>
                              </div>
                            </v-col>
                          </v-row>
                          <div v-if="assetDetailsLoading[expandedAsset.name]" class="text-center py-2">
                            <v-progress-circular indeterminate size="20" color="primary"></v-progress-circular>
                          </div>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </v-table>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- News Section -->
        <v-row class="mt-4">
          <v-col cols="12">
            <v-card variant="outlined">
              <v-card-title class="text-subtitle-1 pb-2 d-flex justify-space-between align-center">
                <div class="d-flex align-center">
                  <v-icon size="small" class="mr-2">mdi-newspaper</v-icon>
                  Latest News - {{ holding.ticker }}
                </div>
                <v-btn 
                  v-if="newsList.length > 0" 
                  variant="text" 
                  size="small" 
                  color="primary"
                  @click="loadMoreNews"
                  :loading="newsLoading"
                >
                  Load More
                </v-btn>
              </v-card-title>
              <v-card-text>
                <div v-if="newsLoading && newsList.length === 0" class="d-flex justify-center py-4">
                  <v-progress-circular indeterminate color="primary" size="30"></v-progress-circular>
                </div>
                <div v-else-if="newsList.length === 0" class="text-center text-grey py-4">
                  <v-icon size="48" class="mb-2">mdi-newspaper-remove</v-icon>
                  <div>No news available for {{ holding.ticker }}</div>
                </div>
                <div v-else class="news-list">
                  <div 
                    v-for="news in newsList" 
                    :key="news.id_news" 
                    class="news-item"
                  >
                    <div class="news-header" @click="toggleNewsExpand(news.id_news)">
                      <div class="d-flex align-center flex-grow-1">
                        <v-icon 
                          size="small" 
                          class="mr-2 news-expand-icon"
                          :class="{ 'rotate-180': expandedNews[news.id_news] }"
                        >
                          mdi-chevron-down
                        </v-icon>
                        <span class="news-title">{{ news.title }}</span>
                      </div>
                      <v-chip 
                        :color="getSentimentColor(news.sentiment)" 
                        size="x-small" 
                        variant="tonal"
                        class="ml-2 flex-shrink-0"
                      >
                        {{ news.sentiment?.toFixed(2) || '0.00' }}
                      </v-chip>
                    </div>
                    
                    <!-- Summary Preview (always visible) -->
                    <div class="news-preview text-caption text-grey mt-2" v-if="!expandedNews[news.id_news]">
                      {{ truncateText(news.summary, 150) }}
                    </div>
                    
                    <!-- Expanded Content -->
                    <div v-if="expandedNews[news.id_news]" class="news-expanded mt-3">
                      <!-- Author & Date -->
                      <div class="news-info d-flex flex-wrap ga-3 mb-3">
                        <div v-if="news.author" class="d-flex align-center">
                          <v-icon size="x-small" class="mr-1">mdi-account</v-icon>
                          <span class="text-caption">{{ news.author }}</span>
                        </div>
                        <div class="d-flex align-center">
                          <v-icon size="x-small" class="mr-1">mdi-clock-outline</v-icon>
                          <span class="text-caption">{{ formatNewsDate(news.published_at) }}</span>
                        </div>
                        <div class="d-flex align-center">
                          <v-icon size="x-small" class="mr-1">mdi-emoticon-outline</v-icon>
                          <span class="text-caption">Sentiment: {{ news.sentiment?.toFixed(3) || 'N/A' }}</span>
                          <v-chip 
                            :color="getSentimentColor(news.sentiment)" 
                            size="x-small" 
                            variant="tonal"
                            class="ml-1"
                          >
                            {{ getSentimentLabel(news.sentiment) }}
                          </v-chip>
                        </div>
                      </div>
                      
                      <!-- Summary -->
                      <div class="news-full-summary mb-3">
                        <div class="text-caption text-uppercase text-grey mb-1">Summary</div>
                        <div class="text-body-2 markdown-content" v-html="formatMarkdown(news.summary || 'No summary available')"></div>
                      </div>
                      
                      <!-- Full Text -->
                      <div v-if="news.text" class="news-full-text mb-3">
                        <div class="text-caption text-uppercase text-grey mb-1">Full Article</div>
                        <div class="text-body-2 markdown-content news-text-content" v-html="formatMarkdown(news.text)"></div>
                      </div>
                      
                      <!-- Source Link -->
                      <div class="news-source mt-3 pt-3">
                        <v-btn 
                          v-if="news.link"
                          variant="outlined" 
                          size="small" 
                          color="primary"
                          :href="news.link"
                          target="_blank"
                          prepend-icon="mdi-open-in-new"
                        >
                          View Original Source
                        </v-btn>
                      </div>
                    </div>
                    
                    <!-- Collapsed Meta -->
                    <div v-if="!expandedNews[news.id_news]" class="news-meta text-caption mt-2">
                      <v-icon size="x-small" class="mr-1">mdi-clock-outline</v-icon>
                      {{ formatNewsDate(news.published_at) }}
                      <span v-if="news.author" class="ml-3">
                        <v-icon size="x-small" class="mr-1">mdi-account</v-icon>
                        {{ news.author }}
                      </span>
                    </div>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </div>
    </td>
  </tr>
  </tbody>
</template>

<script>
import CandleChart from './candleChart.vue'

export default {
  name: 'HoldingView',

  components: {
    CandleChart
  },

  props: {
    holding: {
      type: Object,
      required: true
    },
    authToken: {
      type: String,
      required: true
    }
  },

  emits: ['click'],

  data() {
    return {
      opened: false,
      priceHistory: [],
      fullPriceHistory: [],
      currentPrice: 0,
      dayChange: 0,
      dayChangePercent: 0,
      loading: false,
      chartLoading: false,
      chartWidth: 80,
      chartHeight: 30,
      chartPeriod: '1w',
      newsList: [],
      newsLoading: false,
      newsOffset: 0,
      expandedNews: {},
      dailySummary: null,
      dailySummaryLoading: false,
      hoveredSlice: null,
      pieColors: ['#2196F3', '#4CAF50', '#FF9800', '#E91E63', '#9C27B0', '#00BCD4', '#795548', '#607D8B'],
      pieColors2: ['#3F51B5', '#8BC34A', '#FFC107', '#F44336', '#673AB7', '#009688', '#FF5722', '#03A9F4'],
      assetDetails: {},
      assetDetailsLoading: {},
      expandedAsset: null,
      assetCharts: {}
    }
  },

  computed: {
    identifier() {
      return this.holding.isin && this.holding.isin !== 'N/A' ? this.holding.isin : this.holding.ticker
    },

    // Calculate total change since purchase
    currentValue() {
      return this.currentPrice * this.holding.quantity
    },

    costBasis() {
      return this.holding.purchase_price * this.holding.quantity
    },

    totalChange() {
      return this.currentValue - this.costBasis
    },

    totalChangePercent() {
      if (this.costBasis === 0) return 0
      return (this.totalChange / this.costBasis) * 100
    },

    isPositiveDay() {
      return this.dayChangePercent >= 0
    },

    isPositiveTotal() {
      return this.totalChangePercent >= 0
    },

    // Generate SVG points for mini chart
    chartPoints() {
      if (this.priceHistory.length < 2) {
        return `0,${this.chartHeight / 2} ${this.chartWidth},${this.chartHeight / 2}`
      }

      const prices = this.priceHistory.map(p => p.close)
      const minPrice = Math.min(...prices)
      const maxPrice = Math.max(...prices)
      const priceRange = maxPrice - minPrice || 1

      const padding = 2
      const effectiveHeight = this.chartHeight - padding * 2
      const effectiveWidth = this.chartWidth - padding * 2

      const points = prices.map((price, index) => {
        const x = padding + (index / (prices.length - 1)) * effectiveWidth
        const y = padding + effectiveHeight - ((price - minPrice) / priceRange) * effectiveHeight
        return `${x},${y}`
      })

      return points.join(' ')
    },

    sortedSectors() {
      if (!this.holding.sectors) return []
      return [...this.holding.sectors].sort((a, b) => b.percentage - a.percentage)
    },

    sortedRegions() {
      if (!this.holding.regions) return []
      return [...this.holding.regions].sort((a, b) => b.percentage - a.percentage)
    },

    sortedAssets() {
      if (!this.holding.assets) return []
      return this.holding.assets.slice(0, 10)
    },

    // Pie chart slices for sectors
    sectorPieSlices() {
      return this.generatePieSlices(this.sortedSectors, this.pieColors)
    },

    // Pie chart slices for regions
    regionPieSlices() {
      return this.generatePieSlices(this.sortedRegions, this.pieColors2)
    },

    topHoldingsForPie() {
      return this.sortedAssets
    },

    topHoldingsPieSlices() {
      const holdings = this.topHoldingsForPie
      if (holdings.length === 0) return []
      
      const percentages = holdings.map((_, index) => ({
        name: holdings[index].name,
        percentage: this.calculateAssetPercentage(index, true)
      }))
      
      return this.generatePieSlices(percentages, this.pieColors)
    }
  },

  watch: {
    opened(newVal) {
      if (newVal) {
        this.fetchFullPriceHistory()
        this.fetchNews()
        this.fetchDailySummary()
        this.fetchAssetDetailsForHoldings()
        this.fetchAssetCharts()
      }
    },
    chartPeriod() {
      if (this.opened) {
        this.fetchFullPriceHistory()
      }
    }
  },

  mounted() {
    this.fetchData()
  },

  methods: {
    async fetchData() {
      this.loading = true
      try {
        await Promise.all([
          this.fetchPriceHistory(),
          this.fetchAssetChange()
        ])
      } catch (error) {
        console.error('Error fetching holding data:', error)
      } finally {
        this.loading = false
      }
    },

    async fetchPriceHistory() {
      try {
        const response = await fetch(
          `http://localhost:8085/api/asset/history?ticker=${encodeURIComponent(this.identifier)}&period=1d&interval=15m`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.priceHistory = data || []
          
          // Get current price from latest data point
          if (this.priceHistory.length > 0) {
            this.currentPrice = this.priceHistory[this.priceHistory.length - 1].close
          }
        }
      } catch (error) {
        console.error('Error fetching price history:', error)
      }
    },

    async fetchFullPriceHistory() {
      this.chartLoading = true
      try {
        const intervalMap = {
          '1d': '5m',
          '1w': '15m',
          '1m': '1h',
          '3m': '1h',
          '1y': '1d'
        }
        const interval = intervalMap[this.chartPeriod] || '1h'
        
        const response = await fetch(
          `http://localhost:8085/api/asset/history?ticker=${encodeURIComponent(this.identifier)}&period=${this.chartPeriod}&interval=${interval}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.fullPriceHistory = data || []
        }
      } catch (error) {
        console.error('Error fetching full price history:', error)
      } finally {
        this.chartLoading = false
      }
    },

    async fetchAssetChange() {
      try {
        const response = await fetch(
          `http://localhost:8085/api/asset/change?ticker=${encodeURIComponent(this.identifier)}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.dayChange = data.day_change || 0
          this.dayChangePercent = data.day_change_percent || 0
          
          // Update current price from value if we have quantity
          if (this.holding.quantity > 0 && data.current_value) {
            this.currentPrice = data.current_value / this.holding.quantity
          }
        }
      } catch (error) {
        console.error('Error fetching asset change:', error)
      }
    },

    async fetchNews() {
      this.newsLoading = true
      try {
        const response = await fetch(
          `http://localhost:8085/api/asset/news?ticker=${encodeURIComponent(this.identifier)}&limit=5&offset=${this.newsOffset}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          console.log('News data received:', data)
          if (this.newsOffset === 0) {
            this.newsList = data || []
          } else {
            this.newsList = [...this.newsList, ...(data || [])]
          }
        } else {
          console.error('News fetch failed with status:', response.status, await response.text())
        }
      } catch (error) {
        console.error('Error fetching news:', error)
      } finally {
        this.newsLoading = false
      }
    },

    loadMoreNews() {
      this.newsOffset += 5
      this.fetchNews()
    },

    async fetchDailySummary() {
      this.dailySummaryLoading = true
      try {
        const today = new Date().toISOString().split('T')[0] // YYYY-MM-DD format
        const response = await fetch(
          `http://localhost:8085/api/asset/daily_sentiment?ticker=${encodeURIComponent(this.identifier)}&date=${today}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          this.dailySummary = data
        }
      } catch (error) {
        console.error('Error fetching daily summary:', error)
      } finally {
        this.dailySummaryLoading = false
      }
    },

    // Generate SVG pie chart slices
    generatePieSlices(items, colors) {
      if (!items || items.length === 0) return []
      
      const total = items.reduce((sum, item) => sum + item.percentage, 0)
      const slices = []
      let currentAngle = -90 // Start from top

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

    formatCurrency(value, currency = 'USD') {
      if (value === null || value === undefined || isNaN(value)) {
        return '-'
      }
      
      const currencySymbols = {
        'USD': '$',
        'EUR': '€',
        'GBP': '£',
        'CHF': 'CHF ',
        'JPY': '¥'
      }
      
      const symbol = currencySymbols[currency] || currency + ' '
      const absValue = Math.abs(value)
      
      if (absValue >= 1000000) {
        return symbol + (value / 1000000).toFixed(2) + 'M'
      } else if (absValue >= 1000) {
        return symbol + (value / 1000).toFixed(2) + 'K'
      } else {
        return symbol + value.toFixed(2)
      }
    },

    formatPercent(value) {
      if (value === null || value === undefined || isNaN(value)) {
        return '-'
      }
      return Math.abs(value).toFixed(2) + '%'
    },

    formatQuantity(value) {
      if (value === null || value === undefined || isNaN(value)) {
        return '-'
      }
      // For small quantities (like crypto), show more decimals
      if (value < 1) {
        return value.toFixed(6)
      } else if (value < 100) {
        return value.toFixed(4)
      } else {
        return value.toFixed(2)
      }
    },

    formatNewsDate(timestamp) {
      if (!timestamp) return 'Unknown'
      const date = new Date(parseInt(timestamp) * 1000)
      const now = new Date()
      const diff = now - date
      
      if (diff < 3600000) {
        return Math.floor(diff / 60000) + ' min ago'
      } else if (diff < 86400000) {
        return Math.floor(diff / 3600000) + ' hours ago'
      } else if (diff < 604800000) {
        return Math.floor(diff / 86400000) + ' days ago'
      } else {
        return date.toLocaleDateString()
      }
    },

    getSentimentColor(sentiment) {
      if (sentiment > 0.3) return 'success'
      if (sentiment < -0.3) return 'error'
      return 'grey'
    },

    getSentimentLabel(sentiment) {
      if (sentiment > 0.3) return 'Positive'
      if (sentiment < -0.3) return 'Negative'
      return 'Neutral'
    },

    toggleNewsExpand(newsId) {
      this.expandedNews = {
        ...this.expandedNews,
        [newsId]: !this.expandedNews[newsId]
      }
    },

    truncateText(text, maxLength) {
      if (!text) return ''
      if (text.length <= maxLength) return text
      return text.substring(0, maxLength) + '...'
    },

    openNewsLink(link) {
      if (link) {
        window.open(link, '_blank')
      }
    },

    formatMarkdown(text) {
      if (!text) return ''
      
      let html = text
      
      // Escape HTML to prevent XSS
      html = html.replace(/&/g, '&amp;')
                 .replace(/</g, '&lt;')
                 .replace(/>/g, '&gt;')
      
      // Headers (h1-h3)
      html = html.replace(/^### (.+)$/gm, '<h4 class="text-subtitle-2 font-weight-bold mt-3 mb-1">$1</h4>')
      html = html.replace(/^## (.+)$/gm, '<h3 class="text-subtitle-1 font-weight-bold mt-3 mb-1">$1</h3>')
      html = html.replace(/^# (.+)$/gm, '<h2 class="text-h6 font-weight-bold mt-3 mb-2">$1</h2>')
      
      // Bold and italic
      html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
      html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
      html = html.replace(/_(.+?)_/g, '<em>$1</em>')
      
      // Bullet lists
      html = html.replace(/^[\-\*] (.+)$/gm, '<li>$1</li>')
      html = html.replace(/(<li>.*<\/li>\n?)+/g, '<ul class="ml-4 my-2">$&</ul>')
      
      // Numbered lists
      html = html.replace(/^\d+\. (.+)$/gm, '<li>$1</li>')
      
      // Inline code
      html = html.replace(/`([^`]+)`/g, '<code class="px-1 rounded" style="background: rgba(var(--v-theme-surface-variant), 0.5);">$1</code>')
      
      // Links
      html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" class="text-primary">$1</a>')
      
      // Line breaks - convert double newlines to paragraphs
      html = html.replace(/\n\n/g, '</p><p class="mb-2">')
      html = '<p class="mb-2">' + html + '</p>'
      
      // Single line breaks
      html = html.replace(/\n/g, '<br>')
      
      // Clean up empty paragraphs
      html = html.replace(/<p class="mb-2"><\/p>/g, '')
      html = html.replace(/<p class="mb-2">(<h[2-4])/g, '$1')
      html = html.replace(/(<\/h[2-4]>)<\/p>/g, '$1')
      
      return html
    },

    async fetchAssetDetailsForHoldings() {
      if (!this.holding.assets || this.holding.assets.length === 0) return
      
      for (const asset of this.holding.assets.slice(0, 10)) {
        if (!asset.name || !asset.isin) continue
        
        this.assetDetailsLoading[asset.name] = true
        
        try {
          const response = await fetch(
            `http://localhost:8085/api/asset/details?ticker=${encodeURIComponent(asset.isin)}`,
            {
              headers: {
                'Authorization': `Bearer ${this.authToken}`
              }
            }
          )

          if (response.ok) {
            const data = await response.json()
            if (data && data.length > 0) {
              this.assetDetails[asset.name] = data[0]
            }
          }
        } catch (error) {
          console.error(`Error fetching details for ${asset.name}:`, error)
        } finally {
          this.assetDetailsLoading[asset.name] = false
        }
      }
    },

    async fetchAssetCharts() {
      if (!this.holding.assets || this.holding.assets.length === 0) return
      
      for (const asset of this.holding.assets.slice(0, 10)) {
        if (!asset.name || !asset.isin) continue
        
        try {
          const response = await fetch(
            `http://localhost:8085/api/asset/history?ticker=${encodeURIComponent(asset.isin)}&period=1m&interval=1d`,
            {
              headers: {
                'Authorization': `Bearer ${this.authToken}`
              }
            }
          )

          if (response.ok) {
            const data = await response.json()
            this.assetCharts[asset.name] = data || []
          }
        } catch (error) {
          console.error(`Error fetching chart for ${asset.name}:`, error)
        }
      }
    },
    TODO
    async fetchAssetStats(ticker, isin) {
      try {
        console.log(`Fetching stats for ${ticker} / ${isin}`)
        const response = await fetch(
          `http://localhost:8085/api/asset/stats?ticker=${encodeURIComponent(ticker)}&isin=${encodeURIComponent(isin)}`,
          {
            headers: {
              'Authorization': `Bearer ${this.authToken}`
            }
          }
        )

        if (response.ok) {
          const data = await response.json()
          return data || {}
        }
      } catch (error) {
        console.error(`Error fetching stats for ${ticker} / ${isin}:`, error)
      }
      return {}
    },
    toggleAssetDetails(asset) {
      if (this.expandedAsset?.name === asset.name) {
        this.expandedAsset = null
      } else {
        this.expandedAsset = asset
        if (!this.assetDetails[asset.name]) {
          this.fetchAssetDetailsForHoldings()
          this.fetchAssetStats('N/A', asset.isin)
        }
      }
    },

    calculateAssetPercentage(index, returnNumber = false) {
      if (!this.holding.assets || this.holding.assets.length === 0) {
        return returnNumber ? 0 : '0.0'
      }
      
      const totalAssets = this.holding.assets.length
      const top10Count = Math.min(10, totalAssets)
      
      if (index >= top10Count) {
        return returnNumber ? 0 : '0.0'
      }
      
      const basePercentage = 100 / top10Count
      const decay = 0.9
      const weight = Math.pow(decay, index)
      
      const totalWeight = Array.from({ length: top10Count }, (_, i) => Math.pow(decay, i))
        .reduce((sum, w) => sum + w, 0)
      
      const percentage = (weight / totalWeight) * 100
      
      return returnNumber ? percentage : percentage.toFixed(1)
    },

    scrollLegendIntoView(type, index) {
      this.$nextTick(() => {
        const legendItem = this.$el.querySelector(`.legend-item[data-type="${type}"][data-index="${index}"]`)
        if (legendItem) {
          legendItem.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
        }
      })
    },

    getAssetChartPoints(ticker, width, height) {
      const prices = this.assetCharts[ticker] || []
      if (prices.length < 2) {
        return `0,${height / 2} ${width},${height / 2}`
      }

      const closePrices = prices.map(p => p.close)
      const minPrice = Math.min(...closePrices)
      const maxPrice = Math.max(...closePrices)
      const priceRange = maxPrice - minPrice || 1

      const padding = 2
      const effectiveHeight = height - padding * 2
      const effectiveWidth = width - padding * 2

      const points = closePrices.map((price, index) => {
        const x = padding + (index / (closePrices.length - 1)) * effectiveWidth
        const y = padding + effectiveHeight - ((price - minPrice) / priceRange) * effectiveHeight
        return `${x},${y}`
      })

      return points.join(' ')
    },

    getAssetChartColor(ticker) {
      const prices = this.assetCharts[ticker] || []
      if (prices.length < 2) return '#666'
      
      const firstPrice = prices[0].close
      const lastPrice = prices[prices.length - 1].close
      
      return lastPrice >= firstPrice ? '#26a79a' : '#ef5250'
    }
  }
}
</script>

<style scoped>
.holding-tbody {
  display: table-row-group;
}

.holding-row {
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.holding-row:hover {
  background-color: rgba(var(--v-theme-primary), 0.04);
}

.holding-row td {
  padding: 12px 8px !important;
  vertical-align: middle !important;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
}

.chart-cell {
  width: 90px;
  padding: 8px !important;
}

.mini-chart-container {
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 4px;
  padding: 4px;
}

.mini-chart {
  display: block;
}

.expand-cell {
  width: 50px;
  opacity: 0.5;
}

.holding-row:hover .expand-cell {
  opacity: 1;
}

.text-truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Expand icon rotation */
.rotate-180 {
  transform: rotate(180deg);
}

.v-icon {
  transition: transform 0.2s ease-in-out;
}

/* Detail row styles */
.detail-row td {
  padding: 0 !important;
  background-color: rgba(var(--v-theme-surface-variant), 0.15);
}

.detail-row:hover td {
  background-color: rgba(var(--v-theme-surface-variant), 0.15);
}

.detail-container {
  padding: 20px 24px;
  border-bottom: 2px solid rgba(var(--v-theme-primary), 0.2);
}

.detail-grid {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail-label {
  font-size: 0.7rem;
  color: rgba(var(--v-theme-on-surface), 0.5);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 500;
}

.detail-value {
  font-size: 0.9rem;
  color: rgba(var(--v-theme-on-surface), 0.87);
}

.text-mono {
  font-family: 'Roboto Mono', monospace;
  font-size: 0.85rem;
}

.h-100 {
  height: 100%;
}

/* Pie Chart Styles */
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

/* Holdings Sub-Table Styles */
.holdings-sub-table {
  background: transparent !important;
}

.holdings-sub-table th {
  font-size: 0.75rem !important;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: rgba(var(--v-theme-on-surface), 0.5) !important;
  padding: 8px 12px !important;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.1) !important;
}

.holdings-sub-table td {
  padding: 10px 12px !important;
  font-size: 0.85rem;
}

.asset-row {
  transition: background-color 0.15s ease;
  cursor: pointer;
}

.asset-row:hover {
  background-color: rgba(var(--v-theme-primary), 0.04);
}

.asset-detail-row td {
  padding: 0 !important;
  background-color: rgba(var(--v-theme-surface-variant), 0.2);
}

.asset-detail-container {
  padding: 16px 20px;
  border-top: 1px solid rgba(var(--v-border-color), 0.1);
}

.detail-section {
  background: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 8px;
  padding: 12px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.metric-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.metric-label {
  font-size: 0.7rem;
  color: rgba(var(--v-theme-on-surface), 0.5);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 500;
}

.metric-value {
  font-size: 0.9rem;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.87);
}

/* News Styles */
.news-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.news-item {
  padding: 12px 16px;
  border-radius: 8px;
  background: rgba(var(--v-theme-surface-variant), 0.3);
  transition: all 0.2s ease;
  border: 1px solid transparent;
}

.news-item:hover {
  background: rgba(var(--v-theme-surface-variant), 0.5);
  border-color: rgba(var(--v-theme-primary), 0.2);
}

.news-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  cursor: pointer;
}

.news-expand-icon {
  transition: transform 0.2s ease;
}

.news-expand-icon.rotate-180 {
  transform: rotate(180deg);
}

.news-title {
  font-weight: 500;
  font-size: 0.9rem;
  line-height: 1.4;
  color: rgba(var(--v-theme-on-surface), 0.9);
}

.news-preview {
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.news-expanded {
  border-top: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  padding-top: 12px;
}

.news-info {
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.news-full-summary,
.news-full-text {
  background: rgba(var(--v-theme-surface-variant), 0.2);
  border-radius: 6px;
  padding: 12px;
}

.news-text-content {
  max-height: 300px;
  overflow-y: auto;
}

.news-source {
  border-top: 1px solid rgba(var(--v-theme-on-surface), 0.1);
}

.news-meta {
  display: flex;
  align-items: center;
  color: rgba(var(--v-theme-on-surface), 0.5);
}

/* Summary Section */
.summary-content {
  background: rgba(var(--v-theme-surface-variant), 0.2);
  border-radius: 8px;
  padding: 16px;
  border-left: 3px solid rgba(var(--v-theme-primary), 0.5);
}

.summary-content p {
  margin: 0;
  color: rgba(var(--v-theme-on-surface), 0.85);
}

/* Markdown Content Styles */
.markdown-content {
  line-height: 1.7;
}

.markdown-content p {
  margin-bottom: 8px;
}

.markdown-content p:last-child {
  margin-bottom: 0;
}

.markdown-content ul {
  list-style-type: disc;
  padding-left: 20px;
  margin: 8px 0;
}

.markdown-content li {
  margin-bottom: 4px;
}

.markdown-content strong {
  font-weight: 600;
}

.markdown-content em {
  font-style: italic;
}

.markdown-content h2, .markdown-content h3, .markdown-content h4 {
  color: rgba(var(--v-theme-on-surface), 0.95);
}

.markdown-content a {
  text-decoration: underline;
}

.markdown-content a:hover {
  opacity: 0.8;
}
</style>
