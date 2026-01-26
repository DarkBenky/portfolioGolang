<template>
  <v-container fluid class="pa-4">
    <v-row>
      <v-col cols="12">
        <v-card class="elevation-2">
          <v-card-title class="text-h5 font-weight-bold">
            Expense Tracker
          </v-card-title>
          <v-card-text>
            <v-row>
              <v-col cols="12" md="6">
                <v-text-field
                  v-model="newExpense.description"
                  label="Description"
                  variant="outlined"
                  density="comfortable"
                ></v-text-field>
              </v-col>
              <v-col cols="12" md="3">
                <v-text-field
                  v-model.number="newExpense.amount"
                  label="Amount"
                  type="number"
                  variant="outlined"
                  density="comfortable"
                  prefix="€"
                ></v-text-field>
              </v-col>
              <v-col cols="12" md="3">
                <v-select
                  v-model="newExpense.category"
                  :items="categories"
                  label="Category"
                  variant="outlined"
                  density="comfortable"
                ></v-select>
              </v-col>
            </v-row>
            <v-row>
              <v-col cols="12" md="3">
                <v-text-field
                  v-model="newExpense.date"
                  label="Date"
                  type="date"
                  variant="outlined"
                  density="comfortable"
                ></v-text-field>
              </v-col>
              <v-col cols="12" md="9" class="d-flex align-center">
                <v-btn
                  v-if="!editMode"
                  color="primary"
                  @click="addExpense"
                  :loading="loading"
                  class="mr-2"
                >
                  Add Expense
                </v-btn>
                <v-btn
                  v-else
                  color="success"
                  @click="updateExpense"
                  :loading="loading"
                  class="mr-2"
                >
                  Update Expense
                </v-btn>
                <v-btn
                  v-if="editMode"
                  color="grey"
                  @click="cancelEdit"
                  variant="outlined"
                >
                  Cancel
                </v-btn>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" md="6">
        <v-card class="elevation-2">
          <v-card-title class="text-h6 font-weight-bold">
            Expenses by Category
          </v-card-title>
          <v-card-text>
            <div style="height: 300px;">
              <Pie v-if="categoryChartData.labels.length" :data="categoryChartData" :options="chartOptions" />
              <div v-else class="text-center text-grey">No data available</div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card class="elevation-2">
          <v-card-title class="text-h6 font-weight-bold">
            Total by Category
          </v-card-title>
          <v-card-text>
            <v-list density="compact">
              <v-list-item
                v-for="(value, category) in categoryStats"
                :key="category"
                class="px-0"
              >
                <template v-slot:prepend>
                  <v-chip :color="getCategoryColor(category)" size="small" class="mr-2">
                    {{ category }}
                  </v-chip>
                </template>
                <v-list-item-title class="font-weight-bold">
                  €{{ value.toFixed(2) }}
                </v-list-item-title>
              </v-list-item>
            </v-list>
            <v-divider class="my-2"></v-divider>
            <v-list-item class="px-0">
              <v-list-item-title class="text-h6 font-weight-bold">
                Total: €{{ totalExpenses.toFixed(2) }}
              </v-list-item-title>
            </v-list-item>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12">
        <v-card class="elevation-2">
          <v-card-title class="text-h6 font-weight-bold">
            Monthly Expenses
          </v-card-title>
          <v-card-text>
            <div style="height: 300px;">
              <Line v-if="monthlyChartData.labels.length" :data="monthlyChartData" :options="lineChartOptions" />
              <div v-else class="text-center text-grey">No data available</div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12">
        <v-card class="elevation-2">
          <v-card-title class="text-h6 font-weight-bold">
            Recent Expenses
          </v-card-title>
          <v-card-text>
            <v-data-table
              :headers="headers"
              :items="expenses"
              :loading="loading"
              class="elevation-1"
              :items-per-page="10"
            >
              <template v-slot:[`item.amount`]="{ item }">
                <span class="font-weight-bold">€{{ item.amount.toFixed(2) }}</span>
              </template>
              <template v-slot:[`item.category`]="{ item }">
                <v-chip :color="getCategoryColor(item.category)" size="small">
                  {{ item.category }}
                </v-chip>
              </template>
              <template v-slot:[`item.actions`]="{ item }">
                <v-btn
                  icon="mdi-pencil"
                  size="small"
                  variant="text"
                  @click="editExpense(item)"
                ></v-btn>
                <v-btn
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  @click="deleteExpense(item.id)"
                ></v-btn>
              </template>
            </v-data-table>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-snackbar v-model="snackbar" :color="snackbarColor" :timeout="3000">
      {{ snackbarText }}
    </v-snackbar>
  </v-container>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { Chart as ChartJS, ArcElement, Tooltip, Legend, CategoryScale, LinearScale, PointElement, LineElement, Title } from 'chart.js'
import { Pie, Line } from 'vue-chartjs'
import { API_BASE_URL } from '@/config.js'

ChartJS.register(ArcElement, Tooltip, Legend, CategoryScale, LinearScale, PointElement, LineElement, Title)

function getCookie(name) {
  const value = `; ${document.cookie}`
  const parts = value.split(`; ${name}=`)
  if (parts.length === 2) return parts.pop().split(';').shift()
  return null
}

const expenses = ref([])
const categoryStats = ref({})
const monthlyStats = ref({})
const loading = ref(false)
const editMode = ref(false)
const snackbar = ref(false)
const snackbarText = ref('')
const snackbarColor = ref('success')

const categories = [
  'Food',
  'Transportation',
  'Entertainment',
  'Utilities',
  'Healthcare',
  'Shopping',
  'Education',
  'Other'
]

const categoryColors = {
  'Food': '#FF6384',
  'Transportation': '#36A2EB',
  'Entertainment': '#FFCE56',
  'Utilities': '#4BC0C0',
  'Healthcare': '#9966FF',
  'Shopping': '#FF9F40',
  'Education': '#FF6384',
  'Other': '#C9CBCF'
}

const newExpense = ref({
  id: 0,
  description: '',
  amount: 0,
  category: '',
  date: new Date().toISOString().split('T')[0]
})

const headers = [
  { title: 'Date', key: 'date', align: 'start' },
  { title: 'Description', key: 'description', align: 'start' },
  { title: 'Category', key: 'category', align: 'center' },
  { title: 'Amount', key: 'amount', align: 'end' },
  { title: 'Actions', key: 'actions', align: 'center', sortable: false }
]

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      position: 'bottom'
    }
  }
}

const lineChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      display: false
    }
  },
  scales: {
    y: {
      beginAtZero: true,
      ticks: {
        callback: function(value) {
          return '€' + value
        }
      }
    }
  }
}

const categoryChartData = computed(() => {
  const labels = Object.keys(categoryStats.value)
  const data = Object.values(categoryStats.value)
  const backgroundColor = labels.map(label => categoryColors[label] || '#C9CBCF')

  return {
    labels,
    datasets: [{
      data,
      backgroundColor,
      borderWidth: 1
    }]
  }
})

const monthlyChartData = computed(() => {
  const labels = Object.keys(monthlyStats.value).sort()
  const data = labels.map(label => monthlyStats.value[label])

  return {
    labels,
    datasets: [{
      label: 'Monthly Expenses',
      data,
      borderColor: '#36A2EB',
      backgroundColor: 'rgba(54, 162, 235, 0.2)',
      tension: 0.4,
      fill: true
    }]
  }
})

const totalExpenses = computed(() => {
  return Object.values(categoryStats.value).reduce((sum, val) => sum + val, 0)
})

function getCategoryColor(category) {
  return categoryColors[category] || '#C9CBCF'
}

async function fetchExpenses() {
  loading.value = true
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (response.ok) {
      expenses.value = await response.json()
    }
  } catch (error) {
    showSnackbar('Failed to load expenses', 'error')
  } finally {
    loading.value = false
  }
}

async function fetchCategoryStats() {
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses/stats/category`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (response.ok) {
      categoryStats.value = await response.json()
    }
  } catch (error) {
    console.error('Failed to load category stats', error)
  }
}

async function fetchMonthlyStats() {
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses/stats/monthly`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (response.ok) {
      monthlyStats.value = await response.json()
    }
  } catch (error) {
    console.error('Failed to load monthly stats', error)
  }
}

async function addExpense() {
  if (!newExpense.value.description || !newExpense.value.amount || !newExpense.value.category) {
    showSnackbar('Please fill in all fields', 'error')
    return
  }

  loading.value = true
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify(newExpense.value)
    })

    if (response.ok) {
      showSnackbar('Expense added successfully', 'success')
      resetForm()
      await Promise.all([fetchExpenses(), fetchCategoryStats(), fetchMonthlyStats()])
    } else {
      showSnackbar('Failed to add expense', 'error')
    }
  } catch (error) {
    showSnackbar('Failed to add expense', 'error')
  } finally {
    loading.value = false
  }
}

async function updateExpense() {
  loading.value = true
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify(newExpense.value)
    })

    if (response.ok) {
      showSnackbar('Expense updated successfully', 'success')
      resetForm()
      await Promise.all([fetchExpenses(), fetchCategoryStats(), fetchMonthlyStats()])
    } else {
      showSnackbar('Failed to update expense', 'error')
    }
  } catch (error) {
    showSnackbar('Failed to update expense', 'error')
  } finally {
    loading.value = false
  }
}

async function deleteExpense(id) {
  if (!confirm('Are you sure you want to delete this expense?')) {
    return
  }

  loading.value = true
  try {
    const token = getCookie('auth_token')
    const response = await fetch(`${API_BASE_URL}/api/expenses?id=${id}`, {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (response.ok) {
      showSnackbar('Expense deleted successfully', 'success')
      await Promise.all([fetchExpenses(), fetchCategoryStats(), fetchMonthlyStats()])
    } else {
      showSnackbar('Failed to delete expense', 'error')
    }
  } catch (error) {
    showSnackbar('Failed to delete expense', 'error')
  } finally {
    loading.value = false
  }
}

function editExpense(expense) {
  newExpense.value = { ...expense }
  editMode.value = true
}

function cancelEdit() {
  resetForm()
}

function resetForm() {
  newExpense.value = {
    id: 0,
    description: '',
    amount: 0,
    category: '',
    date: new Date().toISOString().split('T')[0]
  }
  editMode.value = false
}

function showSnackbar(text, color) {
  snackbarText.value = text
  snackbarColor.value = color
  snackbar.value = true
}

onMounted(async () => {
  await Promise.all([fetchExpenses(), fetchCategoryStats(), fetchMonthlyStats()])
})
</script>

<style scoped>
.v-card {
  margin-bottom: 16px;
}
</style>
