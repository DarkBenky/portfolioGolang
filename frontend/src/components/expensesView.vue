<template>
  <v-container fluid class="pa-2">

    <v-row dense class="mb-1">
      <v-col cols="12" class="py-0">
        <v-tabs v-model="activeTab" color="primary" bg-color="surface" density="compact">
          <v-tab value="all">All Transactions</v-tab>
          <v-tab value="import">Bank Import</v-tab>
          <v-tab value="savings">Savings</v-tab>
          <v-tab value="reports">Reports</v-tab>
        </v-tabs>
      </v-col>
    </v-row>

    <v-tabs-window v-model="activeTab">

      <v-tabs-window-item value="all">

        <v-row dense>
          <v-col cols="3" class="py-0">
            <v-card variant="outlined" class="text-center pa-2">
              <div class="d-flex align-center justify-center mb-1">
                <v-icon size="16" color="error" class="mr-1">mdi-arrow-down-bold</v-icon>
                <span class="text-caption text-grey font-weight-medium">TOTAL OUT</span>
              </div>
              <div class="text-h6 font-weight-bold text-error">-€{{ unifiedTotalOut.toFixed(0) }}</div>
            </v-card>
          </v-col>
          <v-col cols="3" class="py-0">
            <v-card variant="outlined" class="text-center pa-2">
              <div class="d-flex align-center justify-center mb-1">
                <v-icon size="16" color="success" class="mr-1">mdi-arrow-up-bold</v-icon>
                <span class="text-caption text-grey font-weight-medium">TOTAL IN</span>
              </div>
              <div class="text-h6 font-weight-bold text-success">+€{{ unifiedTotalIn.toFixed(0) }}</div>
            </v-card>
          </v-col>
          <v-col cols="3" class="py-0">
            <v-card variant="outlined" class="text-center pa-2">
              <div class="d-flex align-center justify-center mb-1">
                <v-icon size="16" :color="unifiedNet >= 0 ? 'primary' : 'warning'" class="mr-1">mdi-swap-vertical-bold</v-icon>
                <span class="text-caption text-grey font-weight-medium">NET</span>
              </div>
              <div class="text-h6 font-weight-bold" :class="unifiedNet >= 0 ? 'text-primary' : 'text-warning'">
                {{ unifiedNet >= 0 ? '+' : '' }}€{{ unifiedNet.toFixed(0) }}
              </div>
            </v-card>
          </v-col>
          <v-col cols="3" class="py-0">
            <v-card variant="outlined" class="text-center pa-2">
              <div class="d-flex align-center justify-center mb-1">
                <v-icon size="16" color="purple" class="mr-1">mdi-piggy-bank</v-icon>
                <span class="text-caption text-grey font-weight-medium">SAVED</span>
              </div>
              <div class="text-h6 font-weight-bold text-purple">€{{ totalSaved.toFixed(0) }}</div>
            </v-card>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" class="py-0 d-flex align-center">
            <v-icon size="16" color="grey" class="mr-1">mdi-calendar-range</v-icon>
            <span class="text-caption text-grey font-weight-medium mr-2">PERIOD</span>
            <v-btn-toggle v-model="timeGrouping" mandatory density="compact" divided color="primary" variant="outlined" size="x-small">
              <v-btn value="day" size="x-small">Day</v-btn>
              <v-btn value="week" size="x-small">Week</v-btn>
              <v-btn value="month" size="x-small">Month</v-btn>
              <v-btn value="quarter" size="x-small">Quarter</v-btn>
              <v-btn value="year" size="x-small">Year</v-btn>
            </v-btn-toggle>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" md="4" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-pie</v-icon>CATEGORY BREAKDOWN
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 220px;">
                  <Pie v-if="groupedCategoryChartData.labels.length" :data="groupedCategoryChartData" :options="pieOptions" />
                  <div v-else class="d-flex justify-center align-center text-caption text-grey" style="height:100%">No data</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="4" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-bar</v-icon>SPENDING OVER TIME
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 220px;">
                  <Bar v-if="groupedChartData.labels.length" :data="groupedChartData" :options="barOptions" />
                  <div v-else class="d-flex justify-center align-center text-caption text-grey" style="height:100%">No data</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="4" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-line</v-icon>INCOME vs EXPENSE
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 220px;">
                  <Line v-if="incomeExpenseChart.labels.length" :data="incomeExpenseChart" :options="lineOptions" />
                  <div v-else class="d-flex justify-center align-center text-caption text-grey" style="height:100%">No data</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" md="6" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-timeline-variant</v-icon>CATEGORIES BY {{ periodLabel.toUpperCase() }}
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 240px; overflow-y: auto;">
                  <div v-for="(val, cat) in topCategories" :key="cat" class="cat-bar-row">
                    <v-chip :color="getCategoryColor(cat)" size="x-small" variant="flat" class="cat-chip">{{ cat }}</v-chip>
                    <v-progress-linear :model-value="cat === 'Income' ? 0 : (val / maxCategoryVal * 100)" :color="getCategoryColor(cat)" height="8" rounded class="cat-progress" />
                    <span class="cat-amount">€{{ val.toFixed(0) }}</span>
                  </div>
                  <div v-if="!Object.keys(topCategories).length" class="text-center text-caption text-grey py-8">No data</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="6" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>SUMMARY
              </v-card-title>
              <v-card-text class="pa-2">
                <div class="detail-grid">
                  <div class="detail-row-c">
                    <span class="detail-label">Total transactions</span>
                    <span class="detail-value text-caption font-weight-bold">{{ unifiedList.length }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Outgoing count</span>
                    <span class="detail-value text-caption text-error font-weight-bold">{{ unifiedOutCount }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Incoming count</span>
                    <span class="detail-value text-caption text-success font-weight-bold">{{ unifiedInCount }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Avg per expense</span>
                    <span class="detail-value text-caption font-weight-bold">€{{ avgExpense.toFixed(2) }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Avg per income</span>
                    <span class="detail-value text-caption font-weight-bold">€{{ avgIncome.toFixed(2) }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Largest expense</span>
                    <span class="detail-value text-caption text-error font-weight-bold">€{{ largestExpense.toFixed(0) }}</span>
                  </div>
                  <div class="detail-row-c">
                    <span class="detail-label">Top category</span>
                    <span class="detail-value text-caption font-weight-bold">{{ topCategoryName }}</span>
                  </div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-format-list-bulleted</v-icon>TRANSACTIONS
                <v-spacer />
                <v-btn color="primary" size="x-small" prepend-icon="mdi-plus" @click="openAddDialog" variant="tonal">Add</v-btn>
              </v-card-title>
              <v-card-text class="pa-1">
                <v-row dense>
                  <v-col cols="2" class="py-0">
                    <v-text-field v-model="unifiedSearch" label="Search" variant="outlined" density="compact" prepend-inner-icon="mdi-magnify" clearable hide-details />
                  </v-col>
                  <v-col cols="2" class="py-0">
                    <v-select v-model="unifiedCategoryFilter" :items="['All', ...allCategories]" label="Category" variant="outlined" density="compact" hide-details />
                  </v-col>
                  <v-col cols="2" class="py-0">
                    <v-select v-model="unifiedDirectionFilter" :items="['All', 'In', 'Out']" label="Type" variant="outlined" density="compact" hide-details />
                  </v-col>
                  <v-col cols="2" class="py-0">
                    <v-select v-model="unifiedSourceFilter" :items="['All', 'bank', 'manual']" label="Source" variant="outlined" density="compact" hide-details />
                  </v-col>
                  <v-col cols="2" class="py-0">
                    <v-select v-model="selectedPeriodKey" :items="periodKeys" :label="periodLabel" variant="outlined" density="compact" hide-details clearable />
                  </v-col>
                  <v-col cols="2" class="py-0" />
                </v-row>

                <v-data-table :headers="unifiedHeaders" :items="filteredUnified" :loading="loading || bankLoading" :items-per-page="15" :items-per-page-options="pageOptions" density="compact" class="mt-1">
                  <template v-slot:[`item.date`]="{ item }">
                    <span class="text-caption">{{ item.date }}</span>
                  </template>
                  <template v-slot:[`item.amount`]="{ item }">
                    <span :class="item.direction === 'in' ? 'text-success' : 'text-error'" class="text-caption font-weight-bold">
                      {{ item.direction === 'in' ? '+' : '-' }}€{{ item.amount.toFixed(2) }}
                    </span>
                  </template>
                  <template v-slot:[`item.category`]="{ item }">
                    <v-menu offset-y>
                      <template v-slot:activator="{ props }">
                        <v-chip :color="getCategoryColor(item.category)" size="x-small" variant="flat" class="text-caption" v-bind="props" style="cursor:pointer">{{ item.category }}</v-chip>
                      </template>
                      <v-list density="compact">
                        <v-list-item v-for="cat in allCategories" :key="cat" :value="cat" @click="quickRecategorize(item, cat)" :active="item.category === cat">
                          <template v-slot:prepend>
                            <v-icon size="12" :color="getCategoryColor(cat)" class="mr-2">mdi-circle</v-icon>
                          </template>
                          <v-list-item-title class="text-caption">{{ cat }}</v-list-item-title>
                        </v-list-item>
                      </v-list>
                    </v-menu>
                  </template>
                  <template v-slot:[`item.source`]="{ item }">
                    <v-chip :color="item.source === 'bank' ? 'blue' : 'grey'" size="x-small" variant="tonal">{{ item.source }}</v-chip>
                  </template>
                  <template v-slot:[`item.description`]="{ item }">
                    <span class="text-caption">{{ truncate(item.description, 45) }}</span>
                  </template>
                  <template v-slot:[`item.actions`]="{ item }">
                    <v-btn icon="mdi-pencil" size="x-small" variant="text" density="compact" @click="openEditDialog(item)" />
                    <v-btn icon="mdi-delete" size="x-small" variant="text" color="error" density="compact" @click="deleteUnifiedRow(item)" />
                  </template>
                </v-data-table>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </v-tabs-window-item>

      <v-tabs-window-item value="import">
        <v-row justify="center" dense>
          <v-col cols="12" md="6" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-file-xml-box</v-icon>IMPORT CAMT.053 XML
              </v-card-title>
              <v-card-text class="pa-2">
                <p class="text-caption mb-2 text-grey">Upload bank XML. Duplicates auto-skipped. No PII stored.</p>
                <v-file-input v-model="xmlFile" label="Select XML file" accept=".xml" variant="outlined" density="compact" prepend-icon="mdi-file-xml-box" show-size clearable />
                <v-btn color="primary" size="small" :loading="importLoading" :disabled="!xmlFile" @click="uploadXML" class="mt-1" prepend-icon="mdi-upload">Import</v-btn>
                <v-alert v-if="importResult" :type="importResult.imported > 0 ? 'success' : 'info'" class="mt-2" variant="tonal" density="compact">
                  <span class="text-caption">Imported: {{ importResult.imported }} | Skipped: {{ importResult.skipped }} | Total: {{ importResult.total }}</span>
                </v-alert>
                <v-alert v-if="importError" type="error" class="mt-2" variant="tonal" density="compact">{{ importError }}</v-alert>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </v-tabs-window-item>

      <v-tabs-window-item value="savings">
        <v-row dense>
          <v-col cols="4" class="py-0">
            <v-card variant="outlined" class="text-center pa-3">
              <v-icon size="36" color="purple">mdi-piggy-bank</v-icon>
              <div class="text-caption text-grey font-weight-medium mt-1">TOTAL SAVED</div>
              <div class="text-h5 font-weight-bold text-purple">€{{ totalSaved.toFixed(0) }}</div>
            </v-card>
          </v-col>
          <v-col cols="4" class="py-0">
            <v-card variant="outlined" class="text-center pa-3">
              <v-icon size="36" color="indigo">mdi-counter</v-icon>
              <div class="text-caption text-grey font-weight-medium mt-1">ROUNDUPS</div>
              <div class="text-h5 font-weight-bold text-indigo">{{ savingsTransactions.length }}</div>
            </v-card>
          </v-col>
          <v-col cols="4" class="py-0">
            <v-card variant="outlined" class="text-center pa-3">
              <v-icon size="36" color="teal">mdi-calculator</v-icon>
              <div class="text-caption text-grey font-weight-medium mt-1">AVERAGE</div>
              <div class="text-h5 font-weight-bold text-teal">€{{ savingsTransactions.length ? (totalSaved / savingsTransactions.length).toFixed(1) : '0' }}</div>
            </v-card>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" md="8" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-line</v-icon>CUMULATIVE SAVINGS
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 260px;">
                  <Line v-if="savingsCumulativeChartData.labels.length" :data="savingsCumulativeChartData" :options="savingsLineOptions" />
                  <div v-else class="d-flex justify-center align-center text-caption text-grey" style="height:100%">No data yet</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="4" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-chart-bar</v-icon>MONTHLY
              </v-card-title>
              <v-card-text class="pa-1">
                <div style="height: 260px;">
                  <Bar v-if="savingsMonthlyChartData.labels.length" :data="savingsMonthlyChartData" :options="savingsBarOptions" />
                  <div v-else class="d-flex justify-center align-center text-caption text-grey" style="height:100%">No data</div>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <v-row dense class="mt-2">
          <v-col cols="12" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-format-list-bulleted</v-icon>SAVINGS ROUNDUPS
              </v-card-title>
              <v-card-text class="pa-1">
                <v-data-table :headers="savingsHeaders" :items="savingsTransactions" :loading="bankLoading" :items-per-page="15" :items-per-page-options="pageOptions" density="compact">
                  <template v-slot:[`item.amount`]="{ item }">
                    <span class="text-purple text-caption font-weight-bold">€{{ item.amount.toFixed(2) }}</span>
                  </template>
                  <template v-slot:[`item.description`]="{ item }">
                    <span class="text-caption">{{ item.description }}</span>
                  </template>
                </v-data-table>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </v-tabs-window-item>

      <v-tabs-window-item value="reports">
        <v-row dense>
          <v-col cols="12" md="8" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-robot</v-icon>AI EXPENSE REPORT
              </v-card-title>
              <v-card-text class="pa-2">
                <p class="text-caption text-grey mb-2">Generate an AI-powered analysis of your spending patterns using DeepSeek.</p>
                <div class="d-flex ga-2 mb-3">
                  <v-btn color="primary" size="small" variant="tonal" :loading="reportLoading === 'week'" @click="generateReport('week')" prepend-icon="mdi-calendar-week">Weekly</v-btn>
                  <v-btn color="primary" size="small" variant="tonal" :loading="reportLoading === 'month'" @click="generateReport('month')" prepend-icon="mdi-calendar-month">Monthly</v-btn>
                  <v-btn color="primary" size="small" variant="tonal" :loading="reportLoading === 'quarter'" @click="generateReport('quarter')" prepend-icon="mdi-calendar-range">Quarterly</v-btn>
                </div>

                <v-alert v-if="reportError" type="error" variant="tonal" density="compact" class="mb-2 text-caption">{{ reportError }}</v-alert>

                <v-card v-if="latestReport" variant="outlined" class="mb-3 pa-3" @click="openReportDialog(latestReport)" style="cursor:pointer;">
                  <div class="d-flex align-center mb-1">
                    <v-chip :color="latestReport.period === 'week' ? 'blue' : latestReport.period === 'month' ? 'green' : 'purple'" size="x-small" class="mr-2">{{ latestReport.period }}</v-chip>
                    <span class="text-caption font-weight-bold">{{ latestReport.period_start }} to {{ latestReport.period_end }}</span>
                    <v-spacer />
                    <span class="text-caption text-grey">{{ latestReport.created_at }}</span>
                  </div>
                  <div class="text-caption" style="white-space: pre-wrap; max-height: 120px; overflow: hidden;">{{ latestReport.summary?.slice(0, 400) }}{{ (latestReport.summary?.length || 0) > 400 ? '...' : '' }}</div>
                  <div class="text-caption text-primary mt-1">Click to read full report</div>
                </v-card>
                <div v-else-if="!reportLoading" class="text-center text-caption text-grey py-4">No reports yet. Generate one above.</div>
              </v-card-text>
            </v-card>
          </v-col>

          <v-col cols="12" md="4" class="py-0">
            <v-card variant="outlined">
              <v-card-title class="text-caption text-grey font-weight-medium pa-2 d-flex align-center">
                <v-icon size="14" class="mr-1">mdi-history</v-icon>PREVIOUS REPORTS
              </v-card-title>
              <v-card-text class="pa-0">
                <v-list density="compact" class="py-0" v-if="reports.length > 0">
                  <v-list-item v-for="r in reports" :key="r.id" density="compact" @click="openReportDialog(r)" class="px-2">
                    <template v-slot:prepend>
                      <v-chip :color="r.period === 'week' ? 'blue' : r.period === 'month' ? 'green' : 'purple'" size="x-small" class="mr-1">{{ r.period[0].toUpperCase() }}</v-chip>
                    </template>
                    <v-list-item-title class="text-caption">{{ r.period_start }} - {{ r.period_end }}</v-list-item-title>
                    <v-list-item-subtitle class="text-caption text-grey">{{ r.created_at }}</v-list-item-subtitle>
                  </v-list-item>
                </v-list>
                <div v-else class="text-center text-caption text-grey py-6">No past reports</div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </v-tabs-window-item>

    </v-tabs-window>

    <v-dialog v-model="reportDialog" max-width="700px">
      <v-card v-if="viewingReport">
        <v-card-title class="text-subtitle-2 pa-3 d-flex align-center">
          <v-chip :color="viewingReport.period === 'week' ? 'blue' : viewingReport.period === 'month' ? 'green' : 'purple'" size="x-small" class="mr-2">{{ viewingReport.period }}</v-chip>
          {{ viewingReport.period_start }} to {{ viewingReport.period_end }}
          <v-spacer />
          <v-btn icon="mdi-close" size="x-small" variant="text" @click="reportDialog = false" />
        </v-card-title>
        <v-card-text class="pa-3 pt-0">
          <div class="text-caption text-grey mb-2">Generated {{ viewingReport.created_at }}</div>
          <div style="white-space: pre-wrap; font-size: 13px; line-height: 1.6;">{{ viewingReport.summary }}</div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="editDialog" max-width="400px">
      <v-card>
        <v-card-title class="text-subtitle-2 pa-3">{{ editForm.source === 'bank' ? 'Edit Bank Transaction' : (editForm.id ? 'Edit Expense' : 'Add Expense') }}</v-card-title>
        <v-card-text class="pa-3 pt-0">
          <v-text-field v-model="editForm.description" label="Description" variant="outlined" density="compact" class="mb-2" hide-details />
          <template v-if="editForm.source !== 'bank'">
            <v-text-field v-model.number="editForm.amount" label="Amount" type="number" variant="outlined" density="compact" prefix="€" class="mt-2" hide-details />
            <v-text-field v-model="editForm.date" label="Date" type="date" variant="outlined" density="compact" class="mt-2" hide-details />
          </template>
          <v-select v-model="editForm.category" :items="allCategories" label="Category" variant="outlined" density="compact" class="mt-2" hide-details />
          <v-switch v-if="editForm.source === 'bank'" v-model="editForm.is_savings_roundup" label="Savings roundup" color="purple" density="compact" hide-details class="mt-2" />
        </v-card-text>
        <v-card-actions class="pa-3 pt-0">
          <v-spacer />
          <v-btn variant="text" size="small" @click="editDialog = false">Cancel</v-btn>
          <v-btn color="primary" size="small" :loading="editSaving" @click="saveEdit">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar v-model="snackbar" :color="snackbarColor" :timeout="2500">{{ snackbarText }}</v-snackbar>
  </v-container>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import {
  Chart as ChartJS,
  ArcElement, Tooltip, Legend,
  CategoryScale, LinearScale,
  PointElement, LineElement,
  BarElement, Title, Filler
} from 'chart.js'
import { Pie, Line, Bar } from 'vue-chartjs'
import { API_BASE_URL } from '@/config.js'

ChartJS.register(
  ArcElement, Tooltip, Legend,
  CategoryScale, LinearScale,
  PointElement, LineElement,
  BarElement, Title, Filler
)

function getCookie(name) {
  const value = `; ${document.cookie}`
  const parts = value.split(`; ${name}=`)
  if (parts.length === 2) return parts.pop().split(';').shift()
  return null
}

function truncate(str, len) {
  if (!str) return ''
  return str.length > len ? str.slice(0, len) + '...' : str
}

function getWeekKey(dateStr) {
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  const jan1 = new Date(d.getFullYear(), 0, 1)
  const dayOfYear = Math.floor((d - jan1) / 86400000)
  const weekNum = Math.ceil((dayOfYear + jan1.getDay() + 1) / 7)
  return `${d.getFullYear()}-W${String(weekNum).padStart(2, '0')}`
}

function getQuarterKey(dateStr) {
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  const q = Math.floor(d.getMonth() / 3) + 1
  return `${d.getFullYear()}-Q${q}`
}

function getGroupKey(dateStr, grouping) {
  if (!dateStr) return 'Unknown'
  switch (grouping) {
    case 'day': return dateStr.slice(0, 10)
    case 'week': return getWeekKey(dateStr)
    case 'month': return dateStr.slice(0, 7)
    case 'quarter': return getQuarterKey(dateStr)
    case 'year': return dateStr.slice(0, 4)
    default: return dateStr.slice(0, 7)
  }
}

const activeTab = ref('all')
const timeGrouping = ref('month')

const expenses = ref([])
const loading = ref(false)
const snackbar = ref(false)
const snackbarText = ref('')
const snackbarColor = ref('success')

const bankTransactions = ref([])
const savingsTransactions = ref([])
const bankLoading = ref(false)

const editDialog = ref(false)
const editSaving = ref(false)
const editForm = ref({ id: 0, description: '', amount: 0, category: 'Other', date: '', is_savings_roundup: false, source: 'manual', rawId: '' })

const unifiedSearch = ref('')
const unifiedCategoryFilter = ref('All')
const unifiedDirectionFilter = ref('All')
const unifiedSourceFilter = ref('All')
const selectedPeriodKey = ref(null)

const xmlFile = ref(null)
const importLoading = ref(false)
const importResult = ref(null)
const importError = ref(null)

const reportLoading = ref(null)
const reportError = ref(null)
const reports = ref([])
const latestReport = ref(null)
const reportDialog = ref(false)
const viewingReport = ref(null)

const allCategories = ['Income', 'Groceries', 'Dining', 'Transport', 'Entertainment', 'Shopping', 'Utilities', 'Healthcare', 'Savings', 'Investments', 'Subscriptions', 'Insurance', 'Housing', 'Snacks', 'Services', 'Transfer', 'Food', 'Transportation', 'Education', 'Other']

const categoryColors = {
  Food: '#FF6384', Transportation: '#36A2EB', Entertainment: '#FFCE56',
  Utilities: '#4BC0C0', Healthcare: '#9966FF', Shopping: '#FF9F40',
  Education: '#C9CBCF', Other: '#B0BEC5', Income: '#4CAF50',
  Groceries: '#8BC34A', Dining: '#FF7043', Transport: '#29B6F6', Savings: '#AB47BC',
  Investments: '#00BCD4', Subscriptions: '#E040FB', Insurance: '#607D8B',
  Housing: '#795548', Snacks: '#FF9800', Services: '#9E9E9E', Transfer: '#78909C'
}

const unifiedHeaders = [
  { title: 'Date', key: 'date', align: 'start', width: '90px' },
  { title: 'Description', key: 'description', align: 'start' },
  { title: 'Category', key: 'category', align: 'center', width: '100px' },
  { title: 'Amount', key: 'amount', align: 'end', width: '100px' },
  { title: 'Src', key: 'source', align: 'center', width: '60px' },
  { title: '', key: 'actions', align: 'center', width: '60px', sortable: false }
]

const savingsHeaders = [
  { title: 'Date', key: 'booking_date', align: 'start', width: '100px' },
  { title: 'Description', key: 'description', align: 'start' },
  { title: 'Amount', key: 'amount', align: 'end', width: '90px' }
]

const pageOptions = [10, 15, 25, 50, { value: -1, title: 'All' }]

const pieOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom', labels: { boxWidth: 10, font: { size: 9 } } } } }
const barOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { ticks: { font: { size: 9 }, maxRotation: 45 } }, y: { beginAtZero: true, ticks: { callback: v => '€' + v, font: { size: 9 } } } } }
const lineOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom', labels: { boxWidth: 10, font: { size: 9 } } } }, scales: { x: { ticks: { font: { size: 9 }, maxRotation: 45 } }, y: { beginAtZero: true, ticks: { callback: v => '€' + v, font: { size: 9 } } } } }
const savingsLineOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { ticks: { font: { size: 9 } } }, y: { beginAtZero: true, ticks: { callback: v => '€' + v, font: { size: 9 } } } } }
const savingsBarOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { ticks: { font: { size: 9 } } }, y: { beginAtZero: true, ticks: { callback: v => '€' + v, font: { size: 9 } } } } }

const unifiedList = computed(() => {
  const list = []
  for (const e of (expenses.value || [])) {
    list.push({ id: `m-${e.id}`, rawId: e.id, date: e.date, description: e.description, amount: e.amount, category: e.category, direction: 'out', source: 'manual', is_savings_roundup: false })
  }
  for (const t of (bankTransactions.value || [])) {
    if (t.is_savings_roundup) continue
    const dir = t.direction === 'CRDT' ? 'in' : 'out'
    list.push({ id: `b-${t.id}`, rawId: t.id, date: t.booking_date, description: t.description, amount: t.amount, category: t.category, direction: dir, source: 'bank', is_savings_roundup: t.is_savings_roundup, ntry_ref: t.ntry_ref })
  }
  list.sort((a, b) => (b.date || '').localeCompare(a.date || ''))
  return list
})

const filteredUnified = computed(() => {
  let result = unifiedList.value || []
  const search = unifiedSearch.value?.toLowerCase() || ''
  if (search) {
    result = result.filter(t => (t.description || '').toLowerCase().includes(search))
  }
  if (unifiedCategoryFilter.value !== 'All') {
    result = result.filter(t => t.category === unifiedCategoryFilter.value)
  }
  if (unifiedDirectionFilter.value === 'In') {
    result = result.filter(t => t.direction === 'in')
  } else if (unifiedDirectionFilter.value === 'Out') {
    result = result.filter(t => t.direction === 'out')
  }
  if (unifiedSourceFilter.value !== 'All') {
    result = result.filter(t => t.source === unifiedSourceFilter.value)
  }
  if (selectedPeriodKey.value) {
    result = result.filter(t => getGroupKey(t.date, timeGrouping.value) === selectedPeriodKey.value)
  }
  return result
})

const unifiedTotalOut = computed(() => unifiedList.value.filter(t => t.direction === 'out').reduce((s, t) => s + t.amount, 0))
const unifiedTotalIn = computed(() => unifiedList.value.filter(t => t.direction === 'in').reduce((s, t) => s + t.amount, 0))
const unifiedNet = computed(() => unifiedTotalIn.value - unifiedTotalOut.value)
const unifiedOutCount = computed(() => unifiedList.value.filter(t => t.direction === 'out').length)
const unifiedInCount = computed(() => unifiedList.value.filter(t => t.direction === 'in').length)

const avgExpense = computed(() => {
  const outs = unifiedList.value.filter(t => t.direction === 'out')
  return outs.length ? outs.reduce((s, t) => s + t.amount, 0) / outs.length : 0
})

const avgIncome = computed(() => {
  const ins = unifiedList.value.filter(t => t.direction === 'in')
  return ins.length ? ins.reduce((s, t) => s + t.amount, 0) / ins.length : 0
})

const largestExpense = computed(() => {
  const outs = unifiedList.value.filter(t => t.direction === 'out')
  return outs.length ? Math.max(...outs.map(t => t.amount)) : 0
})

const topCategoryName = computed(() => {
  const map = {}
  for (const t of unifiedList.value) {
    if (t.direction === 'out') map[t.category] = (map[t.category] || 0) + t.amount
  }
  let top = ''
  let max = 0
  for (const [k, v] of Object.entries(map)) {
    if (v > max) { max = v; top = k }
  }
  return top || '-'
})

const groupedCategoryChartData = computed(() => {
  const map = {}
  const period = selectedPeriodKey.value
  for (const t of unifiedList.value) {
    if (t.direction !== 'out') continue
    if (period && getGroupKey(t.date, timeGrouping.value) !== period) continue
    map[t.category] = (map[t.category] || 0) + t.amount
  }
  const labels = Object.keys(map)
  return { labels, datasets: [{ data: Object.values(map), backgroundColor: labels.map(l => categoryColors[l] || '#C9CBCF'), borderWidth: 1 }] }
})

const topCategories = computed(() => {
  const map = {}
  const period = selectedPeriodKey.value
  for (const t of unifiedList.value) {
    if (period && getGroupKey(t.date, timeGrouping.value) !== period) continue
    if (t.direction === 'out') map[t.category] = (map[t.category] || 0) + t.amount
  }
  const sorted = Object.entries(map).sort((a, b) => b[1] - a[1])
  return Object.fromEntries(sorted)
})

const maxCategoryVal = computed(() => {
  const vals = Object.values(topCategories.value)
  return vals.length ? Math.max(...vals) : 1
})

const groupedChartData = computed(() => {
  const map = {}
  for (const t of unifiedList.value) {
    if (t.direction !== 'out') continue
    const key = getGroupKey(t.date, timeGrouping.value)
    map[key] = (map[key] || 0) + t.amount
  }
  const sortedKeys = Object.keys(map).sort()
  return {
    labels: sortedKeys,
    datasets: [{
      label: 'Spending',
      data: sortedKeys.map(k => parseFloat(map[k].toFixed(2))),
      backgroundColor: 'rgba(239, 83, 80, 0.7)',
      borderColor: '#EF5350', borderWidth: 1
    }]
  }
})

const incomeExpenseChart = computed(() => {
  const outMap = {}
  const inMap = {}
  for (const t of unifiedList.value) {
    const key = getGroupKey(t.date, timeGrouping.value)
    if (t.direction === 'out') outMap[key] = (outMap[key] || 0) + t.amount
    else inMap[key] = (inMap[key] || 0) + t.amount
  }
  const allKeys = [...new Set([...Object.keys(outMap), ...Object.keys(inMap)])].sort()
  return {
    labels: allKeys,
    datasets: [
      { label: 'Expense', data: allKeys.map(k => parseFloat((outMap[k] || 0).toFixed(2))), borderColor: '#EF5350', backgroundColor: 'rgba(239, 83, 80, 0.1)', tension: 0.3, fill: true, pointRadius: 2 },
      { label: 'Income', data: allKeys.map(k => parseFloat((inMap[k] || 0).toFixed(2))), borderColor: '#4CAF50', backgroundColor: 'rgba(76, 175, 80, 0.1)', tension: 0.3, fill: true, pointRadius: 2 }
    ]
  }
})

const periodKeys = computed(() => {
  const set = new Set()
  for (const t of unifiedList.value) {
    if (t.direction === 'out') set.add(getGroupKey(t.date, timeGrouping.value))
  }
  return [...set].sort()
})

const periodLabel = computed(() => {
  const m = { day: 'Day', week: 'Week', month: 'Month', quarter: 'Qtr', year: 'Year' }
  return m[timeGrouping.value] || 'Period'
})

const totalSaved = computed(() => (savingsTransactions.value || []).reduce((s, t) => s + t.amount, 0))

const savingsByMonth = computed(() => {
  const map = {}
  for (const t of (savingsTransactions.value || [])) {
    const month = t.booking_date?.slice(0, 7) || 'Unknown'
    map[month] = (map[month] || 0) + t.amount
  }
  return map
})

const savingsCountByMonth = computed(() => {
  const map = {}
  for (const t of (savingsTransactions.value || [])) {
    const month = t.booking_date?.slice(0, 7) || 'Unknown'
    map[month] = (map[month] || 0) + 1
  }
  return map
})

const savingsCumulativeChartData = computed(() => {
  const sorted = [...(savingsTransactions.value || [])].sort((a, b) => (a.booking_date || '').localeCompare(b.booking_date || ''))
  const labels = []
  const data = []
  let cumulative = 0
  for (const t of sorted) {
    cumulative += t.amount
    labels.push(t.booking_date)
    data.push(parseFloat(cumulative.toFixed(2)))
  }
  return { labels, datasets: [{ label: 'Cumulative Savings', data, borderColor: '#AB47BC', backgroundColor: 'rgba(171,71,188,0.1)', tension: 0.3, fill: true, pointRadius: 2 }] }
})

const savingsMonthlyChartData = computed(() => {
  const byMonth = savingsByMonth.value
  const labels = Object.keys(byMonth).sort()
  return { labels, datasets: [{ label: 'Monthly Savings', data: labels.map(l => parseFloat(byMonth[l].toFixed(2))), backgroundColor: 'rgba(171,71,188,0.7)', borderColor: '#AB47BC', borderWidth: 1 }] }
})

function getCategoryColor(cat) { return categoryColors[cat] || '#B0BEC5' }

function showSnackbar(text, color) {
  snackbarText.value = text
  snackbarColor.value = color
  snackbar.value = true
}

async function authFetch(path, options = {}) {
  const token = getCookie('auth_token')
  return fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers: { ...(options.headers || {}), Authorization: `Bearer ${token}` }
  })
}

async function fetchExpenses() {
  loading.value = true
  try {
    const res = await authFetch('/api/expenses')
    if (res.ok) expenses.value = (await res.json()) || []
  } catch (e) {
    expenses.value = []
    showSnackbar('Failed to load expenses', 'error')
  } finally {
    loading.value = false
  }
}

async function fetchBankTransactions() {
  bankLoading.value = true
  try {
    const res = await authFetch('/api/bank/transactions')
    if (res.ok) bankTransactions.value = (await res.json()) || []
  } catch (e) {
    bankTransactions.value = []
    showSnackbar('Failed to load bank transactions', 'error')
  } finally {
    bankLoading.value = false
  }
}

async function fetchSavingsTransactions() {
  try {
    const res = await authFetch('/api/bank/savings')
    if (res.ok) savingsTransactions.value = (await res.json()) || []
  } catch (e) { savingsTransactions.value = [] }
}

async function fetchReports() {
  try {
    const res = await authFetch('/api/expenses/reports')
    if (res.ok) {
      reports.value = (await res.json()) || []
      if (reports.value.length > 0) latestReport.value = reports.value[0]
    }
  } catch (e) { console.error('Failed to load reports', e) }
}

async function generateReport(period) {
  reportLoading.value = period
  reportError.value = null
  try {
    const res = await authFetch(`/api/expenses/report/generate?period=${period}`, { method: 'POST' })
    if (res.ok) {
      const data = await res.json()
      latestReport.value = data
      showSnackbar(`${period} report generated`, 'success')
      await fetchReports()
    } else {
      const err = await res.json()
      reportError.value = err.error || 'Failed to generate report'
    }
  } catch (e) {
    reportError.value = 'Network error generating report'
  } finally {
    reportLoading.value = null
  }
}

function openReportDialog(report) {
  viewingReport.value = report
  reportDialog.value = true
}

async function uploadXML() {
  if (!xmlFile.value) return
  importLoading.value = true
  importResult.value = null
  importError.value = null
  const formData = new FormData()
  const fileObj = Array.isArray(xmlFile.value) ? xmlFile.value[0] : xmlFile.value
  formData.append('file', fileObj)
  try {
    const res = await authFetch('/api/bank/import', { method: 'POST', body: formData })
    if (res.ok) {
      importResult.value = await res.json()
      await Promise.all([fetchBankTransactions(), fetchSavingsTransactions()])
      showSnackbar(`Imported ${importResult.value.imported} new transactions`, 'success')
    } else {
      const err = await res.json()
      importError.value = err.error || 'Import failed'
    }
  } catch (e) {
    importError.value = 'Network error during import'
  } finally {
    importLoading.value = false
  }
}

function openAddDialog() {
  editForm.value = { id: 0, description: '', amount: 0, category: 'Other', date: new Date().toISOString().split('T')[0], is_savings_roundup: false, source: 'manual', rawId: '' }
  editDialog.value = true
}

function openEditDialog(item) {
  editForm.value = {
    id: item.source === 'bank' ? item.rawId : item.rawId,
    description: item.description || '',
    amount: item.amount || 0,
    category: item.category || 'Other',
    date: item.date || new Date().toISOString().split('T')[0],
    is_savings_roundup: item.is_savings_roundup || false,
    source: item.source,
    rawId: item.rawId
  }
  editDialog.value = true
}

async function saveEdit() {
  editSaving.value = true
  try {
    let res
    if (editForm.value.source === 'bank') {
      res = await authFetch('/api/bank/transactions', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: editForm.value.id, description: editForm.value.description, category: editForm.value.category, is_savings_roundup: editForm.value.is_savings_roundup })
      })
    } else if (editForm.value.id && editForm.value.source === 'manual') {
      res = await authFetch('/api/expenses', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: editForm.value.id, description: editForm.value.description, amount: editForm.value.amount, category: editForm.value.category, date: editForm.value.date })
      })
    } else {
      res = await authFetch('/api/expenses', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ description: editForm.value.description, amount: editForm.value.amount, category: editForm.value.category, date: editForm.value.date })
      })
    }
    if (res.ok) {
      const data = await res.json().catch(() => ({}))
      const base = editForm.value.source === 'bank' ? 'Transaction updated' : (editForm.value.id ? 'Expense updated' : 'Expense added')
      const extra = data.updated > 0 ? ` (also updated ${data.updated} past transaction${data.updated === 1 ? '' : 's'})` : ''
      showSnackbar(base + extra, 'success')
      editDialog.value = false
      await Promise.all([fetchExpenses(), fetchBankTransactions(), fetchSavingsTransactions()])
    } else {
      showSnackbar('Failed to save', 'error')
    }
  } catch (e) {
    showSnackbar('Failed to save', 'error')
  } finally {
    editSaving.value = false
  }
}

async function quickRecategorize(item, newCategory) {
  if (item.category === newCategory) return
  try {
    let res
    if (item.source === 'bank') {
      res = await authFetch('/api/bank/transactions', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: item.rawId, description: item.description, category: newCategory, is_savings_roundup: item.is_savings_roundup || false })
      })
    } else {
      res = await authFetch('/api/expenses', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: item.rawId, description: item.description, amount: item.amount, category: newCategory, date: item.date })
      })
    }
    if (res.ok) {
      item.category = newCategory
      const data = await res.json().catch(() => ({}))
      const extra = data.updated > 0 ? ` (also updated ${data.updated} past transaction${data.updated === 1 ? '' : 's'})` : ''
      showSnackbar('Category updated' + extra, 'success')
    } else {
      showSnackbar('Failed to update category', 'error')
    }
  } catch (e) {
    showSnackbar('Failed to update category', 'error')
  }
}

async function deleteUnifiedRow(item) {
  if (!confirm('Delete this transaction?')) return
  try {
    let res
    if (item.source === 'bank') {
      res = await authFetch(`/api/bank/transactions?id=${item.rawId}`, { method: 'DELETE' })
    } else {
      res = await authFetch(`/api/expenses?id=${item.rawId}`, { method: 'DELETE' })
    }
    if (res.ok) {
      showSnackbar('Transaction deleted', 'success')
      await Promise.all([fetchExpenses(), fetchBankTransactions(), fetchSavingsTransactions()])
    } else {
      showSnackbar('Failed to delete', 'error')
    }
  } catch (e) {
    showSnackbar('Failed to delete', 'error')
  }
}

onMounted(async () => {
  await Promise.all([
    fetchExpenses(),
    fetchBankTransactions(),
    fetchSavingsTransactions(),
    fetchReports()
  ])
})
</script>

<style scoped>
.v-card {
  margin-bottom: 4px;
}

.detail-grid {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail-row-c {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 3px 0;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.15);
}

.detail-row-c:last-child {
  border-bottom: none;
}

.detail-label {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.6);
}

.detail-value {
  font-size: 12px;
  text-align: right;
}

.cat-bar-row {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
  padding: 0 6px;
  gap: 6px;
}

.cat-chip {
  min-width: 72px;
  justify-content: center;
  flex-shrink: 0;
}

.cat-progress {
  flex: 1;
  min-width: 40px;
}

.cat-amount {
  font-size: 11px;
  min-width: 50px;
  text-align: right;
  flex-shrink: 0;
  font-weight: 500;
}
</style>
