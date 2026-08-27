<template>
  <v-container fluid class="pa-4">
    <v-row>
      <v-col cols="12" md="4">
        <v-card class="d-flex flex-column" elevation="0" outlined>
          <v-card-title class="d-flex align-center">
            <span class="text-h6">Situation Tasks</span>
            <v-spacer></v-spacer>
            <v-btn color="primary" size="small" prepend-icon="mdi-plus" @click="openCreate">New</v-btn>
          </v-card-title>
          <v-divider></v-divider>
          <v-card-text class="overflow-y-auto" style="max-height: 60vh;">
            <v-alert v-if="tasks.length === 0" type="info" variant="tonal" class="mb-2">
              No situation tasks yet. Create one to track a subject with daily reports.
            </v-alert>
            <v-card
              v-for="task in tasks"
              :key="task.id"
              class="mb-2"
              outlined
              :color="task.enabled ? undefined : 'grey'"
            >
              <v-card-text>
                <div class="d-flex align-center">
                  <div class="text-subtitle-1 font-weight-bold">{{ task.subject }}</div>
                  <v-spacer></v-spacer>
                  <v-switch
                    :model-value="task.enabled"
                    density="compact"
                    hide-details
                    color="primary"
                    @update:model-value="toggleTask(task, $event)"
                  ></v-switch>
                </div>
                <div class="d-flex flex-wrap mt-2">
                  <v-chip
                    v-for="st in task.sub_topics"
                    :key="st"
                    size="x-small"
                    variant="tonal"
                    class="mr-1 mb-1"
                  >
                    {{ st }}
                  </v-chip>
                </div>
                <div class="text-caption text-grey mt-2">
                  Daily at {{ String(task.daily_hour).padStart(2, '0') }}:00
                  <span v-if="task.last_report_date"> | Last report: {{ task.last_report_date }}</span>
                </div>
                <div v-if="task.last_report_summary" class="text-caption mt-1">
                  {{ truncate(task.last_report_summary, 120) }}
                </div>
                <div class="d-flex flex-wrap align-center mt-2">
                  <v-btn
                    size="x-small"
                    variant="tonal"
                    color="primary"
                    :loading="generatingTaskId === task.id"
                    @click="generateTask(task)"
                  >
                    Generate now
                  </v-btn>
                  <v-btn
                    size="x-small"
                    variant="tonal"
                    icon="mdi-pencil"
                    title="Edit"
                    class="ml-1"
                    @click="openEdit(task)"
                  ></v-btn>
                  <v-btn
                    size="x-small"
                    variant="tonal"
                    color="error"
                    icon="mdi-delete"
                    title="Delete"
                    class="ml-1"
                    @click="deleteTask(task)"
                  ></v-btn>
                </div>
              </v-card-text>
            </v-card>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="8">
        <v-card elevation="0" outlined>
          <v-card-title class="d-flex align-center flex-wrap">
            <span class="text-h6">Reports</span>
            <v-spacer></v-spacer>
            <v-menu :close-on-content-click="false" location="bottom end">
              <template v-slot:activator="{ props }">
                <v-btn v-bind="props" size="small" variant="tonal" class="mr-2" prepend-icon="mdi-calendar">
                  {{ selectedDate }}
                </v-btn>
              </template>
              <v-date-picker
                v-model="selectedDate"
                :events="reportDates"
                event-color="primary"
                show-adjacent-months
              ></v-date-picker>
            </v-menu>
            <v-btn size="small" variant="tonal" prepend-icon="mdi-refresh" @click="loadReports">Refresh</v-btn>
          </v-card-title>
          <v-divider></v-divider>
          <v-card-text>
            <div class="d-flex align-center mb-3">
              <v-chip v-if="reports.length" size="x-small" variant="tonal">
                {{ reports.length }} report(s) for {{ selectedDate }}
              </v-chip>
              <span v-else class="text-caption text-grey">No reports for {{ selectedDate }}</span>
            </div>
            <v-alert v-if="reports.length === 0" type="info" variant="tonal" class="mb-3">
              No reports for this day. Pick a marked date in the calendar or generate a report.
            </v-alert>
            <div v-for="report in reports" :key="report.id" class="mb-3">
              <v-card outlined>
                <v-card-title class="d-flex align-center py-2">
                  <span class="text-subtitle-1">{{ report.subject }}</span>
                  <v-spacer></v-spacer>
                  <v-chip size="x-small" variant="tonal" :color="reportColor(report.status)">
                    {{ report.status }}
                  </v-chip>
                </v-card-title>
                <v-card-text>
                  <div v-if="report.status === 'running'" class="d-flex align-center mb-2">
                    <v-progress-circular size="16" indeterminate color="primary" class="mr-2"></v-progress-circular>
                    <span class="text-caption text-grey">Generating report...</span>
                  </div>
                  <template v-else>
                    <div v-if="report.summary" class="text-body-2 mb-2">
                      <strong>Summary:</strong> {{ report.summary }}
                    </div>
                    <div
                      class="markdown-content"
                      v-html="formatMarkdown(truncate(report.content, expandedReports[report.id] ? undefined : 1200))"
                    ></div>
                    <v-btn
                      v-if="report.content && report.content.length > 1200"
                      size="x-small"
                      variant="text"
                      color="primary"
                      @click="toggleExpand(report.id)"
                    >
                      {{ expandedReports[report.id] ? 'Show less' : 'Show more' }}
                    </v-btn>
                    <div v-if="report.search_results && report.search_results.length" class="mt-3">
                      <div class="d-flex align-center sources-header" @click="toggleSources(report.id)">
                        <span class="text-caption font-weight-bold">Sources used ({{ report.search_results.length }})</span>
                        <v-icon size="small" class="ml-1">
                          {{ sourcesOpen[report.id] ? 'mdi-chevron-up' : 'mdi-chevron-down' }}
                        </v-icon>
                      </div>
                      <div v-if="sourcesOpen[report.id]" class="mt-1">
                        <div v-for="(src, si) in report.search_results" :key="si" class="mb-1">
                          <a :href="src.url" target="_blank" rel="noopener">{{ src.title || src.url }}</a>
                          <div v-if="src.snippet" class="text-caption text-grey">{{ src.snippet }}</div>
                        </div>
                      </div>
                    </div>
                  </template>
                </v-card-text>
              </v-card>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-dialog v-model="dialog" max-width="560">
      <v-card>
        <v-card-title>{{ editingId ? 'Edit task' : 'New task' }}</v-card-title>
        <v-card-text>
          <v-text-field
            v-model="form.subject"
            label="Subject"
            placeholder="e.g. Russian invasion of Ukraine"
            outlined
          ></v-text-field>
          <div class="text-subtitle-2 mt-2 mb-1">Sub-topics</div>
          <div class="d-flex flex-wrap mb-2">
            <v-chip
              v-for="(st, i) in form.sub_topics"
              :key="i"
              size="small"
              variant="tonal"
              closable
              class="mr-1 mb-1"
              @click:close="removeSubTopic(i)"
            >
              {{ st }}
            </v-chip>
          </div>
          <div class="d-flex">
            <v-text-field
              v-model="newSubTopic"
              label="Add sub-topic"
              outlined
              hide-details
              @keydown.enter.prevent="addSubTopic"
            ></v-text-field>
            <v-btn class="ml-2" @click="addSubTopic">Add</v-btn>
          </div>
          <v-select
            v-model="form.daily_hour"
            :items="hourOptions"
            label="Generate time (hour)"
            outlined
            class="mt-2"
          ></v-select>
        </v-card-text>
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn variant="text" @click="dialog = false">Cancel</v-btn>
          <v-btn color="primary" :loading="saving" @click="saveTask">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { API_BASE_URL } from '../config'
import { formatMarkdown } from '../utils/markdown'

function getCookie(name) {
  const nameEQ = name + '='
  const cookies = document.cookie.split(';')
  for (let cookie of cookies) {
    cookie = cookie.trim()
    if (cookie.indexOf(nameEQ) === 0) {
      return cookie.substring(nameEQ.length)
    }
  }
  return null
}

function truncate(text, max) {
  if (!text) return ''
  if (!max) return text
  return text.length > max ? text.slice(0, max) + '...' : text
}

function todayStr() {
  return new Date().toISOString().split('T')[0]
}

const tasks = ref([])
const reports = ref([])
const reportDates = ref([])
const selectedDate = ref(todayStr())
const loading = ref(false)
const generatingTaskId = ref(null)
const dialog = ref(false)
const saving = ref(false)
const editingId = ref(null)
const form = ref({ subject: '', sub_topics: [], daily_hour: 9 })
const newSubTopic = ref('')
const expandedReports = ref({})
const sourcesOpen = ref({})
let reportsPollTimer = null
const hourOptions = Array.from({ length: 24 }, (_, i) => ({ title: String(i).padStart(2, '0') + ':00', value: i }))

async function apiFetch(path, options = {}) {
  const token = getCookie('auth_token')
  const headers = { ...(options.headers || {}) }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  if (options.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json'
  }
  return fetch(`${API_BASE_URL}${path}`, { ...options, headers })
}

async function loadTasks() {
  try {
    const res = await apiFetch('/api/situation/tasks')
    if (res.ok) {
      tasks.value = await res.json()
    }
  } catch (e) {
    console.error('Failed to load tasks', e)
  }
}

async function loadCalendarDates() {
  try {
    const res = await apiFetch('/api/situation/reports/history')
    if (res.ok) {
      reportDates.value = await res.json()
    }
  } catch (e) {
    console.error('Failed to load report dates', e)
  }
}

async function loadReports() {
  if (reportsPollTimer) {
    clearTimeout(reportsPollTimer)
    reportsPollTimer = null
  }
  try {
    const res = await apiFetch(`/api/situation/reports?date=${selectedDate.value}`)
    if (res.ok) {
      reports.value = (await res.json()).map(r => {
        if (typeof r.search_results === 'string') {
          try {
            r.search_results = JSON.parse(r.search_results)
          } catch (e) {
            r.search_results = []
          }
        } else if (!Array.isArray(r.search_results)) {
          r.search_results = []
        }
        return r
      })
      if (reports.value.some(r => r.status === 'running')) {
        reportsPollTimer = setTimeout(() => {
          loadReports()
          loadTasks()
        }, 4000)
      }
    }
  } catch (e) {
    console.error('Failed to load reports', e)
  }
}

function toggleExpand(reportId) {
  expandedReports.value[reportId] = !expandedReports.value[reportId]
}

function toggleSources(reportId) {
  sourcesOpen.value[reportId] = !sourcesOpen.value[reportId]
}

function openCreate() {
  editingId.value = null
  form.value = { subject: '', sub_topics: [], daily_hour: 9 }
  newSubTopic.value = ''
  dialog.value = true
}

function openEdit(task) {
  editingId.value = task.id
  form.value = {
    subject: task.subject,
    sub_topics: [...(task.sub_topics || [])],
    daily_hour: task.daily_hour
  }
  newSubTopic.value = ''
  dialog.value = true
}

function addSubTopic() {
  const v = newSubTopic.value.trim()
  if (v && !form.value.sub_topics.includes(v)) {
    form.value.sub_topics.push(v)
  }
  newSubTopic.value = ''
}

function removeSubTopic(i) {
  form.value.sub_topics.splice(i, 1)
}

async function saveTask() {
  const subject = form.value.subject.trim()
  if (!subject) return
  saving.value = true
  try {
    if (editingId.value) {
      const res = await apiFetch(`/api/situation/tasks/${editingId.value}`, {
        method: 'PUT',
        body: JSON.stringify(form.value)
      })
      if (res.ok) {
        const updated = await res.json()
        const idx = tasks.value.findIndex(t => t.id === updated.id)
        if (idx >= 0) tasks.value[idx] = updated
      }
    } else {
      const res = await apiFetch('/api/situation/tasks', {
        method: 'POST',
        body: JSON.stringify(form.value)
      })
      if (res.ok) {
        tasks.value.unshift(await res.json())
      }
    }
    dialog.value = false
  } catch (e) {
    console.error('Failed to save task', e)
  } finally {
    saving.value = false
  }
}

async function deleteTask(task) {
  if (!confirm('Delete ' + task.subject + '? Reports are kept for reference.')) return
  try {
    const res = await apiFetch(`/api/situation/tasks/${task.id}`, { method: 'DELETE' })
    if (res.ok) {
      tasks.value = tasks.value.filter(t => t.id !== task.id)
      await loadCalendarDates()
    }
  } catch (e) {
    console.error('Failed to delete task', e)
  }
}

async function toggleTask(task, value) {
  try {
    const path = value ? `/api/situation/tasks/${task.id}/resume` : `/api/situation/tasks/${task.id}/pause`
    const res = await apiFetch(path, { method: 'POST' })
    if (res.ok) {
      task.enabled = value
    }
  } catch (e) {
    console.error('Failed to toggle task', e)
  }
}

async function generateTask(task) {
  generatingTaskId.value = task.id
  try {
    const res = await apiFetch(`/api/situation/tasks/${task.id}/generate`, { method: 'POST' })
    if (res.ok) {
      await loadTasks()
    }
  } catch (e) {
    console.error('Failed to generate task', e)
  } finally {
    generatingTaskId.value = null
  }
}

function reportColor(status) {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'error'
  if (status === 'running') return 'warning'
  return 'grey'
}

onMounted(async () => {
  await Promise.all([loadTasks(), loadCalendarDates()])
  await loadReports()
})
</script>

<style scoped>

.sources-header {
  cursor: pointer;
  user-select: none;
}

.sources-header a {
  color: var(--v-theme-primary);
  text-decoration: none;
  word-break: break-all;
}
.markdown-content {
  overflow-wrap: break-word;
  word-break: break-word;
}
</style>
