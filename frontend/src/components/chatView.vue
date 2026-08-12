<template>
  <v-container fluid class="chat-container pa-0">
    <v-row no-gutters class="chat-layout">
      <v-col cols="12" md="3" class="chat-sidebar">
        <v-card class="d-flex flex-column chat-sidebar-card" elevation="0" rounded="0">
          <v-card-title class="d-flex align-center py-2">
            <span class="text-subtitle-1 font-weight-bold">Conversations</span>
            <v-spacer></v-spacer>
            <v-btn icon size="small" color="primary" @click="newConversation" title="New conversation">
              <v-icon>mdi-plus</v-icon>
            </v-btn>
          </v-card-title>
          <v-divider></v-divider>
          <v-list dense nav class="flex-grow-1 overflow-y-auto">
            <v-list-item
              v-for="conv in conversations"
              :key="conv.id"
              :active="conv.id === activeConversationId"
              @click="selectConversation(conv.id)"
            >
              <v-list-item-title class="text-caption">{{ conv.title }}</v-list-item-title>
              <template v-slot:append>
                <v-btn icon size="x-small" variant="text" @click.stop="removeConversation(conv.id)">
                  <v-icon size="small">mdi-delete</v-icon>
                </v-btn>
              </template>
            </v-list-item>
            <div v-if="conversations.length === 0" class="text-center text-caption text-grey py-8">
              No conversations yet
            </div>
          </v-list>
        </v-card>
      </v-col>

      <v-col cols="12" md="9" class="chat-main">
        <v-card class="d-flex flex-column chat-card" elevation="0" rounded="0">
          <v-card-text class="messages-area pa-4" ref="messagesArea">
            <div v-if="messages.length === 0 && !streaming" class="text-center text-grey py-10">
              <v-icon size="48" class="mb-2">mdi-robot</v-icon>
              <p class="text-body-2">Ask about your portfolio, holdings, news or AI summaries.</p>
            </div>
            <div
              v-for="(msg, idx) in messages"
              :key="msg.id || idx"
              class="message-row"
              :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
            >
              <div class="message-bubble" :class="msg.role === 'user' ? 'message-bubble-user' : 'message-bubble-assistant'">
                <div v-if="msg.role === 'assistant'" class="markdown-content" v-html="formatMarkdown(msg.content)"></div>
                <div v-else class="white-space-pre-wrap">{{ msg.content }}</div>
                <div v-if="msg.role === 'assistant' && msg.search_results && msg.search_results.length" class="search-results">
                  <div class="text-caption font-weight-bold">Web sources</div>
                  <div v-for="(sr, si) in msg.search_results" :key="'m' + si" class="search-result-item">
                    <a :href="sr.url" target="_blank" rel="noopener">{{ sr.title || sr.url }}</a>
                    <div v-if="sr.snippet" class="text-caption text-grey">{{ sr.snippet }}</div>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="streaming" class="message-row justify-start">
              <div class="message-bubble message-bubble-assistant">
                <div v-if="streamContent" class="markdown-content" v-html="formatMarkdown(streamContent)"></div>
                <div v-else class="d-flex align-center">
                  <v-progress-circular size="16" indeterminate color="primary" class="mr-2"></v-progress-circular>
                  <span class="text-caption text-grey">Thinking</span>
                </div>
                <div v-if="streamSearchResults.length" class="search-results">
                  <div class="text-caption font-weight-bold">Web sources</div>
                  <div v-for="(sr, si) in streamSearchResults" :key="'s' + si" class="search-result-item">
                    <a :href="sr.url" target="_blank" rel="noopener">{{ sr.title || sr.url }}</a>
                    <div v-if="sr.snippet" class="text-caption text-grey">{{ sr.snippet }}</div>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="error" class="message-row justify-start">
              <div class="message-bubble message-bubble-error">
                <span class="text-caption">{{ error }}</span>
                <v-btn size="x-small" variant="tonal" color="error" class="ml-2" @click="retryLast">Retry</v-btn>
              </div>
            </div>
          </v-card-text>
          <v-divider></v-divider>
          <v-card-text class="pa-3">
            <div class="d-flex align-center">
              <v-text-field
                v-model="input"
                :disabled="streaming"
                variant="outlined"
                density="compact"
                hide-details
                placeholder="Ask about your portfolio..."
                @keydown.enter.prevent="sendMessage"
              ></v-text-field>
              <v-btn
                color="primary"
                class="ml-2"
                :loading="streaming"
                :disabled="!input.trim() || streaming"
                @click="sendMessage"
              >
                <v-icon>mdi-send</v-icon>
              </v-btn>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
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

const conversations = ref([])
const activeConversationId = ref(null)
const messages = ref([])
const input = ref('')
const streaming = ref(false)
const streamContent = ref('')
const streamSearchResults = ref([])
const error = ref('')
const messagesArea = ref(null)
let lastSentMessage = ''

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

async function loadConversations() {
  try {
    const res = await apiFetch('/api/chat/conversations')
    if (res.ok) {
      conversations.value = await res.json()
      if (conversations.value.length > 0 && !activeConversationId.value) {
        await selectConversation(conversations.value[0].id)
      }
    }
  } catch (e) {
    console.error('Failed to load conversations', e)
  }
}

async function selectConversation(id) {
  activeConversationId.value = id
  error.value = ''
  try {
    const res = await apiFetch(`/api/chat/conversations/${id}`)
    if (res.ok) {
      const data = await res.json()
      messages.value = (data.messages || []).map(m => {
        if (m.search_results && typeof m.search_results === 'string') {
          try {
            m.search_results = JSON.parse(m.search_results)
          } catch (e) {
            m.search_results = []
          }
        } else if (!m.search_results) {
          m.search_results = []
        }
        return m
      })
      scrollToBottom()
    }
  } catch (e) {
    console.error('Failed to load conversation', e)
  }
}

async function newConversation() {
  activeConversationId.value = null
  messages.value = []
  input.value = ''
  error.value = ''
}

async function removeConversation(id) {
  try {
    const res = await apiFetch(`/api/chat/conversations/${id}`, { method: 'DELETE' })
    if (res.ok) {
      conversations.value = conversations.value.filter(c => c.id !== id)
      if (activeConversationId.value === id) {
        activeConversationId.value = null
        messages.value = []
        if (conversations.value.length > 0) {
          await selectConversation(conversations.value[0].id)
        }
      }
    }
  } catch (e) {
    console.error('Failed to delete conversation', e)
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesArea.value) {
      messagesArea.value.scrollTop = messagesArea.value.scrollHeight
    }
  })
}

async function sendMessage() {
  const text = input.value.trim()
  if (!text || streaming.value) return
  lastSentMessage = text
  input.value = ''
  error.value = ''
  streaming.value = true
  streamContent.value = ''
  streamSearchResults.value = []

  messages.value.push({ id: 'user-' + Date.now(), role: 'user', content: text })
  scrollToBottom()

  try {
    const res = await apiFetch('/api/chat', {
      method: 'POST',
      body: JSON.stringify({
        conversation_id: activeConversationId.value,
        message: text
      })
    })

    if (!res.ok) {
      let errMsg = 'Request failed'
      try {
        const data = await res.json()
        errMsg = data.error || errMsg
      } catch (e) {}
      if (res.status === 429) {
        errMsg = 'Cooldown active, wait a moment before sending again.'
      }
      error.value = errMsg
      messages.value.pop()
      return
    }

    if (!activeConversationId.value) {
      const convRes = await apiFetch('/api/chat/conversations')
      if (convRes.ok) {
        const convs = await convRes.json()
        conversations.value = convs
        if (convs.length > 0) {
          activeConversationId.value = convs[0].id
        }
      }
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let idx
      while ((idx = buffer.indexOf('\n\n')) >= 0) {
        const rawEvent = buffer.slice(0, idx)
        buffer = buffer.slice(idx + 2)
        for (const line of rawEvent.split('\n')) {
          if (!line.startsWith('data:')) continue
          const payload = line.slice(5).trim()
          if (payload === '[DONE]') continue
          try {
            const parsed = JSON.parse(payload)
            if (parsed.delta) {
              streamContent.value += parsed.delta
              scrollToBottom()
            } else if (parsed.status) {
              streamContent.value += `\n\n_${parsed.status}_\n\n`
              scrollToBottom()
            } else if (parsed.search_results) {
              streamSearchResults.value.push(...parsed.search_results)
              scrollToBottom()
            } else if (parsed.error) {
              error.value = parsed.error
            }
          } catch (e) {}
        }
      }
    }

    if (streamContent.value.trim()) {
      messages.value.push({ id: 'assistant-' + Date.now(), role: 'assistant', content: streamContent.value, search_results: streamSearchResults.value })
      scrollToBottom()
    }
  } catch (e) {
    console.error('Chat error', e)
    error.value = 'Failed to reach the model. Check that the backend is running.'
    messages.value.pop()
  } finally {
    streaming.value = false
    streamContent.value = ''
  }
}

async function retryLast() {
  if (!lastSentMessage) return
  input.value = lastSentMessage
  error.value = ''
  await sendMessage()
}

onMounted(async () => {
  await loadConversations()
})
</script>

<style scoped>
.chat-container {
  height: 100%;
}

.chat-layout {
  height: 100%;
}

.chat-sidebar {
  height: 100%;
}

.chat-sidebar-card {
  height: 100%;
  border-right: 1px solid rgba(var(--v-border-color), 0.12);
}

.search-results {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(var(--v-border-color), 0.12);
}

.search-result-item {
  margin-top: 6px;
}

.search-result-item a {
  color: var(--v-theme-primary);
  text-decoration: none;
  word-break: break-all;
}

.search-result-item a:hover {
  text-decoration: underline;
}

.chat-main {
  height: 100%;
}

.chat-card {
  height: 100%;
}

.messages-area {
  flex-grow: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.message-row {
  display: flex;
  width: 100%;
}

.message-bubble {
  max-width: 80%;
  padding: 10px 14px;
  border-radius: 12px;
}

.message-bubble-user {
  background: rgba(var(--v-theme-primary), 0.15);
}

.message-bubble-assistant {
  background: rgba(var(--v-theme-surface-variant), 0.4);
}

.message-bubble-error {
  background: rgba(239, 83, 80, 0.15);
  display: flex;
  align-items: center;
}

.white-space-pre-wrap {
  white-space: pre-wrap;
}
</style>
