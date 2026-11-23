<template>
  <div class="chat-root">
    <div class="chat-layout">
      <div
        v-if="isMobile && sidebarOpen"
        class="chat-backdrop"
        @click="sidebarOpen = false"
      ></div>
      <aside class="chat-sidebar" :class="{ 'chat-sidebar-hidden': !sidebarOpen && isMobile }">
        <div class="d-flex align-center py-2 px-2">
          <span class="text-subtitle-2 font-weight-bold">Conversations</span>
          <v-spacer></v-spacer>
          <v-btn icon size="small" color="primary" title="New conversation" @click="newConversation">
            <v-icon size="18">mdi-plus</v-icon>
          </v-btn>
          <v-btn v-if="isMobile" icon size="small" variant="text" @click="sidebarOpen = false">
            <v-icon size="18">mdi-close</v-icon>
          </v-btn>
        </div>
        <v-divider></v-divider>
        <div class="chat-sidebar-list">
          <div
            v-for="conv in conversations"
            :key="conv.id"
            class="conv-row"
            :class="{ 'conv-row-active': conv.id === activeConversationId }"
            @click="selectConversation(conv.id)"
          >
            <span class="text-caption flex-grow-1 text-truncate">{{ conv.title }}</span>
            <v-btn icon size="x-small" variant="text" @click.stop="removeConversation(conv.id)">
              <v-icon size="14">mdi-delete</v-icon>
            </v-btn>
          </div>
          <div v-if="conversations.length === 0" class="text-center text-caption text-grey py-6">
            No conversations yet
          </div>
        </div>
      </aside>

      <section class="chat-main">
        <div class="chat-toolbar">
          <v-btn v-if="isMobile" icon size="small" variant="text" @click="sidebarOpen = true">
            <v-icon size="20">mdi-menu</v-icon>
          </v-btn>
          <span class="text-caption text-medium-emphasis text-truncate">
            {{ activeTitle || 'New conversation' }}
          </span>
          <v-spacer></v-spacer>
          <v-chip v-if="chatModel" size="x-small" variant="tonal">{{ chatModel }}</v-chip>
        </div>

        <div ref="messagesArea" class="messages-area px-2 px-md-4 py-3">
          <div v-if="messages.length === 0 && !streaming" class="chat-empty">
            <v-icon size="42" class="mb-2">mdi-robot-outline</v-icon>
            <p class="text-body-2 mb-1">Ask about your portfolio, holdings, news or summaries.</p>
            <p class="text-caption text-medium-emphasis">Answers show the reasoning and the context that produced them.</p>
          </div>

          <div
            v-for="(msg, idx) in messages"
            :key="msg.id || idx"
            class="message-row"
            :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
          >
            <div class="message-bubble" :class="msg.role === 'user' ? 'message-bubble-user' : 'message-bubble-assistant'">
              <div v-if="msg.role === 'assistant' && msg.content" class="markdown-content" v-html="formatMarkdown(msg.content)"></div>
              <div v-else-if="msg.role === 'assistant'" class="d-flex align-center">
                <template v-if="msg.status === 'streaming'">
                  <v-progress-circular size="14" indeterminate color="primary" class="mr-2"></v-progress-circular>
                  <span class="text-caption text-grey">Generating</span>
                </template>
                <span v-else class="text-caption text-grey">The model returned no answer.</span>
              </div>
              <div v-else class="white-space-pre-wrap">{{ msg.content }}</div>

              <template v-if="msg.role === 'assistant'">
                <div v-if="reasoningOf(msg)" class="trace-block">
                  <div class="trace-toggle" @click="toggle('reason-' + (msg.id || idx))">
                    <v-icon size="14" class="mr-1">mdi-brain</v-icon>
                    <span class="text-caption">Thinking</span>
                    <v-spacer></v-spacer>
                    <v-icon size="14">{{ isOpen('reason-' + (msg.id || idx)) ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
                  </div>
                  <v-expand-transition>
                    <div v-show="isOpen('reason-' + (msg.id || idx))" class="trace-body reasoning-text">
                      {{ reasoningOf(msg) }}
                    </div>
                  </v-expand-transition>
                </div>

                <div v-if="msg.traceParsed" class="trace-block">
                  <div class="trace-toggle" @click="toggle('trace-' + (msg.id || idx))">
                    <v-icon size="14" class="mr-1">mdi-file-tree-outline</v-icon>
                    <span class="text-caption">Prompt &amp; tools</span>
                    <v-spacer></v-spacer>
                    <span class="text-caption text-medium-emphasis mr-2">{{ traceSummary(msg.traceParsed) }}</span>
                    <v-icon size="14">{{ isOpen('trace-' + (msg.id || idx)) ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
                  </div>
                  <v-expand-transition>
                    <div v-show="isOpen('trace-' + (msg.id || idx))" class="trace-body">
                      <div v-for="step in msg.traceParsed.steps || []" :key="step.round" class="trace-step">
                        <div class="text-caption font-weight-medium">
                          Round {{ step.round }} · {{ step.duration_ms }} ms ·
                          {{ step.prompt_tokens || 0 }} in / {{ step.completion_tokens || 0 }} out
                          <span v-if="step.cached_tokens"> · {{ step.cached_tokens }} cached</span>
                        </div>
                        <div v-for="(call, ci) in step.tool_calls || []" :key="ci" class="trace-tool">
                          <v-icon size="12" class="mr-1">mdi-wrench-outline</v-icon>
                          <span class="font-weight-medium">{{ call.name }}</span>
                          <span class="text-medium-emphasis ml-1">{{ call.arguments }}</span>
                          <span class="text-medium-emphasis ml-1">· {{ call.duration_ms }} ms</span>
                          <div class="trace-result">{{ call.result }}</div>
                        </div>
                      </div>
                      <div class="trace-section">
                        <div class="trace-section-title">System prompt</div>
                        <pre class="trace-pre">{{ msg.traceParsed.system }}</pre>
                      </div>
                      <div class="trace-section">
                        <div class="trace-section-title">Portfolio context sent</div>
                        <pre class="trace-pre">{{ msg.traceParsed.context }}</pre>
                      </div>
                      <div class="trace-section">
                        <div class="trace-section-title">Tools offered</div>
                        <div class="d-flex flex-wrap ga-1">
                          <v-chip v-for="tool in msg.traceParsed.tools || []" :key="tool" size="x-small" variant="outlined">
                            {{ tool }}
                          </v-chip>
                        </div>
                      </div>
                    </div>
                  </v-expand-transition>
                </div>

                <div v-if="msg.search_results && msg.search_results.length" class="trace-block">
                  <div class="trace-toggle" @click="toggle('sources-' + (msg.id || idx))">
                    <v-icon size="14" class="mr-1">mdi-web</v-icon>
                    <span class="text-caption">Web sources ({{ msg.search_results.length }})</span>
                    <v-spacer></v-spacer>
                    <v-icon size="14">{{ isOpen('sources-' + (msg.id || idx)) ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
                  </div>
                  <v-expand-transition>
                    <div v-show="isOpen('sources-' + (msg.id || idx))" class="trace-body">
                      <div v-for="(sr, si) in msg.search_results" :key="'m' + si" class="mb-2">
                        <a :href="sr.url" target="_blank" rel="noopener noreferrer" class="text-caption">{{ sr.title || sr.url }}</a>
                        <div v-if="sr.snippet" class="text-caption text-medium-emphasis">{{ sr.snippet }}</div>
                      </div>
                    </div>
                  </v-expand-transition>
                </div>
              </template>
            </div>
          </div>

          <div v-if="streaming" class="message-row justify-start">
            <div class="message-bubble message-bubble-assistant">
              <div v-if="streamContent" class="markdown-content" v-html="formatMarkdown(streamContent)"></div>
              <div v-else class="d-flex align-center mb-1">
                <v-progress-circular size="14" indeterminate color="primary" class="mr-2"></v-progress-circular>
                <span class="text-caption text-grey">{{ streamReasoning ? 'Thinking' : 'Connecting' }}</span>
              </div>

              <div v-if="streamReasoning" class="trace-block">
                <div class="trace-toggle" @click="toggle('live-reason')">
                  <v-icon size="14" class="mr-1">mdi-brain</v-icon>
                  <span class="text-caption">Thinking</span>
                  <v-spacer></v-spacer>
                  <v-icon size="14">{{ isOpen('live-reason', true) ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
                </div>
                <v-expand-transition>
                  <div v-show="isOpen('live-reason', true)" class="trace-body reasoning-text">{{ streamReasoning }}</div>
                </v-expand-transition>
              </div>

              <div v-if="streamTools.length" class="trace-block">
                <div class="trace-body">
                  <div v-for="(tool, ti) in streamTools" :key="ti" class="trace-tool">
                    <v-icon size="12" class="mr-1">{{ tool.done ? 'mdi-check' : 'mdi-progress-clock' }}</v-icon>
                    <span class="font-weight-medium">{{ tool.name }}</span>
                    <span class="text-medium-emphasis ml-1">{{ tool.arguments }}</span>
                    <span v-if="tool.done" class="text-medium-emphasis ml-1">· {{ tool.duration }} ms</span>
                  </div>
                </div>
              </div>

              <div v-if="streamSearchResults.length" class="trace-block">
                <div class="trace-toggle" @click="toggle('live-sources', true)">
                  <v-icon size="14" class="mr-1">mdi-web</v-icon>
                  <span class="text-caption">Web sources ({{ streamSearchResults.length }})</span>
                  <v-spacer></v-spacer>
                  <v-icon size="14">{{ isOpen('live-sources', true) ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
                </div>
                <v-expand-transition>
                  <div v-show="isOpen('live-sources', true)" class="trace-body">
                    <div v-for="(sr, si) in streamSearchResults" :key="'s' + si" class="mb-1">
                      <a :href="sr.url" target="_blank" rel="noopener noreferrer" class="text-caption">{{ sr.title || sr.url }}</a>
                    </div>
                  </div>
                </v-expand-transition>
              </div>
            </div>
          </div>

          <div v-if="error" class="message-row justify-start">
            <div class="message-bubble message-bubble-error">
              <span class="text-caption">{{ error }}</span>
              <v-btn size="x-small" variant="tonal" color="error" class="ml-2" @click="retryLast">Retry</v-btn>
            </div>
          </div>
        </div>

        <div class="chat-input">
          <div class="chat-input-field">
            <v-textarea
              v-model="input"
              :disabled="streaming"
              variant="outlined"
              density="compact"
              hide-details
              rows="1"
              auto-grow
              max-rows="4"
              placeholder="Ask about your portfolio..."
              @keydown.enter.exact.prevent="sendMessage"
            ></v-textarea>
          </div>
          <v-btn color="primary" class="ml-2 chat-send" :loading="streaming" :disabled="!input.trim() || streaming" @click="sendMessage">
            <v-icon>mdi-send</v-icon>
          </v-btn>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { API_BASE_URL } from '../config'
import { formatMarkdown } from '../utils/markdown'

function getCookie(name) {
  const nameEQ = name + '='
  for (const part of document.cookie.split(';')) {
    const cookie = part.trim()
    if (cookie.indexOf(nameEQ) === 0) return cookie.substring(nameEQ.length)
  }
  return null
}

const isMobile = ref(window.innerWidth < 960)
const onResize = () => {
  isMobile.value = window.innerWidth < 960
}
const sidebarOpen = ref(!isMobile.value)
const conversations = ref([])
const activeConversationId = ref(null)
const messages = ref([])
const input = ref('')
const streaming = ref(false)
const streamContent = ref('')
const streamReasoning = ref('')
const streamSearchResults = ref([])
const streamTools = ref([])
const expanded = ref({})
const error = ref('')
const messagesArea = ref(null)
const chatModel = ref('')
let lastSentMessage = ''
let pollTimer = null

const activeTitle = computed(() => conversations.value.find((c) => c.id === activeConversationId.value)?.title || '')

async function apiFetch(path, options = {}) {
  const token = getCookie('auth_token')
  const headers = { ...(options.headers || {}) }
  if (token) headers.Authorization = `Bearer ${token}`
  if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
  return fetch(`${API_BASE_URL}${path}`, { ...options, headers })
}

function isOpen(key, fallback = false) {
  return expanded.value[key] ?? fallback
}

function toggle(key, fallback = false) {
  expanded.value[key] = !isOpen(key, fallback)
}

function reasoningOf(message) {
  const trace = message.traceParsed
  if (!trace || !trace.steps) return ''
  return trace.steps.map((step) => step.reasoning || '').filter(Boolean).join('\n\n')
}

function traceSummary(trace) {
  const tokens = `${trace.prompt_tokens || 0} in / ${trace.completion_tokens || 0} out`
  const cost = trace.cost_usd ? ` · $${trace.cost_usd.toFixed(4)}` : ''
  return `${trace.duration_ms || 0} ms · ${tokens}${cost}`
}

function parseMessage(message) {
  if (message.search_results && typeof message.search_results === 'string') {
    try {
      message.search_results = JSON.parse(message.search_results)
    } catch {
      message.search_results = []
    }
  } else if (!message.search_results) {
    message.search_results = []
  }
  if (message.trace && typeof message.trace === 'string') {
    try {
      message.traceParsed = JSON.parse(message.trace)
    } catch {
      message.traceParsed = null
    }
  }
  return message
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

async function loadUsage() {
  try {
    const res = await apiFetch('/api/usage')
    if (res.ok) {
      const data = await res.json()
      chatModel.value = data.chat_model || ''
    }
  } catch {
    chatModel.value = ''
  }
}

async function selectConversation(id) {
  activeConversationId.value = id
  error.value = ''
  expanded.value = {}
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  if (isMobile.value) sidebarOpen.value = false
  try {
    const res = await apiFetch(`/api/chat/conversations/${id}`)
    if (res.ok) {
      const data = await res.json()
      messages.value = (data.messages || [])
        .map(parseMessage)
        .filter((message) => !(message.role === 'assistant' && !message.content && message.status === 'failed'))
      scrollToBottom()
      if (messages.value.some((m) => m.status === 'streaming')) {
        pollTimer = setTimeout(() => {
          if (activeConversationId.value === id) selectConversation(id)
        }, 3000)
      }
    }
  } catch (e) {
    console.error('Failed to load conversation', e)
  }
}

async function newConversation() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  activeConversationId.value = null
  messages.value = []
  input.value = ''
  error.value = ''
  expanded.value = {}
  if (isMobile.value) sidebarOpen.value = false
}

async function removeConversation(id) {
  const res = await apiFetch(`/api/chat/conversations/${id}`, { method: 'DELETE' })
  if (!res.ok) return
  conversations.value = conversations.value.filter((c) => c.id !== id)
  if (activeConversationId.value === id) {
    activeConversationId.value = null
    messages.value = []
    if (conversations.value.length > 0) await selectConversation(conversations.value[0].id)
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesArea.value) messagesArea.value.scrollTop = messagesArea.value.scrollHeight
  })
}

function handleEvent(parsed) {
  if (parsed.delta) {
    streamContent.value += parsed.delta
  } else if (parsed.reasoning) {
    streamReasoning.value += parsed.reasoning
  } else if (parsed.status) {
    streamTools.value.push({ name: parsed.status, arguments: '', duration: 0, done: true })
  } else if (parsed.tool) {
    streamTools.value.push({ name: parsed.tool.name, arguments: parsed.tool.arguments, duration: 0, done: false })
  } else if (parsed.tool_result) {
    const entry = [...streamTools.value].reverse().find((tool) => tool.name === parsed.tool_result.name && !tool.done)
    if (entry) {
      entry.done = true
      entry.duration = parsed.tool_result.duration_ms
    }
  } else if (parsed.search_results) {
    streamSearchResults.value.push(...parsed.search_results)
  } else if (parsed.trace) {
    streamTrace = parsed.trace
  } else if (parsed.error) {
    error.value = parsed.error
  }
  scrollToBottom()
}

let streamTrace = null

async function sendMessage() {
  const text = input.value.trim()
  if (!text || streaming.value) return
  lastSentMessage = text
  input.value = ''
  error.value = ''
  streaming.value = true
  streamContent.value = ''
  streamReasoning.value = ''
  streamSearchResults.value = []
  streamTools.value = []
  streamTrace = null

  messages.value.push({ id: 'user-' + Date.now(), role: 'user', content: text })
  scrollToBottom()

  try {
    const res = await apiFetch('/api/chat', {
      method: 'POST',
      body: JSON.stringify({ conversation_id: activeConversationId.value, message: text }),
    })
    if (!res.ok) {
      let errMsg = 'Request failed'
      try {
        errMsg = (await res.json()).error || errMsg
      } catch {}
      if (res.status === 429) errMsg = 'Cooldown active, wait a moment before sending again.'
      error.value = errMsg
      messages.value.pop()
      return
    }

    if (!activeConversationId.value) {
      const convRes = await apiFetch('/api/chat/conversations')
      if (convRes.ok) {
        conversations.value = await convRes.json()
        if (conversations.value.length > 0) activeConversationId.value = conversations.value[0].id
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
            handleEvent(JSON.parse(payload))
          } catch {}
        }
      }
    }

    if (streamContent.value.trim()) {
      messages.value.push(parseMessage({
        id: 'assistant-' + Date.now(),
        role: 'assistant',
        content: streamContent.value,
        search_results: streamSearchResults.value,
        trace: streamTrace,
      }))
      scrollToBottom()
    }
    loadConversations()
  } catch (e) {
    console.error('Chat error', e)
    error.value = 'Failed to reach the model. Check that the backend is running.'
    messages.value.pop()
  } finally {
    streaming.value = false
    streamContent.value = ''
    streamReasoning.value = ''
    streamTools.value = []
  }
}

async function retryLast() {
  if (!lastSentMessage) return
  input.value = lastSentMessage
  error.value = ''
  await sendMessage()
}

onMounted(async () => {
  window.addEventListener('resize', onResize)
  await Promise.all([loadConversations(), loadUsage()])
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
})
</script>

<style scoped>
.chat-root {
  height: calc(100vh - 108px);
  /* 100vh includes the phone browser chrome, which pushes the input off screen. */
  height: calc(100dvh - 108px);
  min-height: 420px;
}

.chat-layout {
  display: flex;
  height: 100%;
  gap: 12px;
  position: relative;
}

.chat-sidebar {
  width: 250px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: #1a1a1a;
  overflow: hidden;
}

.chat-sidebar-list {
  flex: 1;
  overflow-y: auto;
}

.conv-row {
  display: flex;
  align-items: center;
  padding: 6px 10px;
  cursor: pointer;
  border-left: 2px solid transparent;
}

.conv-row:hover {
  background: rgba(255, 255, 255, 0.04);
}

.conv-row-active {
  background: rgba(var(--v-theme-primary), 0.12);
  border-left-color: rgb(var(--v-theme-primary));
}

.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: #1a1a1a;
  overflow: hidden;
}

.chat-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.messages-area {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-empty {
  margin: auto;
  text-align: center;
  color: rgba(255, 255, 255, 0.7);
}

.message-row {
  display: flex;
  width: 100%;
}

.message-bubble {
  max-width: 88%;
  padding: 8px 12px;
  border-radius: 12px;
  font-size: 14px;
}

.message-bubble-user {
  background: rgba(var(--v-theme-primary), 0.15);
}

.message-bubble-assistant {
  background: rgba(var(--v-theme-surface-variant), 0.35);
}

.message-bubble-error {
  background: rgba(239, 83, 80, 0.15);
  display: flex;
  align-items: center;
}

.white-space-pre-wrap {
  white-space: pre-wrap;
}

.trace-block {
  margin-top: 6px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  padding-top: 4px;
}

.trace-toggle {
  display: flex;
  align-items: center;
  cursor: pointer;
  user-select: none;
  color: rgba(255, 255, 255, 0.65);
}

.trace-body {
  margin-top: 4px;
}

.reasoning-text {
  white-space: pre-wrap;
  font-size: 12px;
  line-height: 1.45;
  color: rgba(255, 255, 255, 0.6);
  max-height: 240px;
  overflow-y: auto;
  font-style: italic;
}

.trace-step {
  padding: 4px 0;
  border-bottom: 1px dashed rgba(255, 255, 255, 0.06);
}

.trace-tool {
  font-size: 11px;
  padding: 2px 0 2px 8px;
  border-left: 2px solid rgba(var(--v-theme-primary), 0.4);
  margin-top: 4px;
}

.trace-result {
  white-space: pre-wrap;
  color: rgba(255, 255, 255, 0.5);
  max-height: 120px;
  overflow-y: auto;
}

.trace-section {
  margin-top: 8px;
}

.trace-section-title {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: rgba(255, 255, 255, 0.45);
}

.trace-pre {
  white-space: pre-wrap;
  font-size: 11px;
  line-height: 1.4;
  color: rgba(255, 255, 255, 0.65);
  max-height: 180px;
  overflow-y: auto;
  background: rgba(0, 0, 0, 0.25);
  padding: 6px 8px;
  border-radius: 6px;
  margin-top: 2px;
}

.chat-input {
  display: flex;
  align-items: flex-end;
  padding: 8px 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  gap: 8px;
}

.chat-input-field {
  flex: 1;
  min-width: 0;
}

.chat-send {
  flex-shrink: 0;
  min-width: 46px;
}

.chat-backdrop {
  position: fixed;
  inset: 0;
  z-index: 19;
  background: rgba(0, 0, 0, 0.55);
}

@media (max-width: 959px) {
  .chat-root {
    height: calc(100vh - 150px);
    height: calc(100dvh - 150px);
  }

  .chat-sidebar {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 20;
    height: 100%;
    width: 78vw;
    max-width: 300px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  }

  .chat-sidebar-hidden {
    display: none;
  }

  .message-bubble {
    max-width: 94%;
  }
}
</style>
