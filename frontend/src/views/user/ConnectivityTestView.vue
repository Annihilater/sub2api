<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('connectivityTest.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('connectivityTest.description') }}</p>
      </div>

      <!-- Config Card -->
      <div class="card p-6 space-y-5">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('connectivityTest.testConfig') }}</h2>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <!-- API Key selector -->
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('connectivityTest.apiKey') }}
            </label>
            <select
              v-model="selectedApiKey"
              class="input w-full"
              :disabled="isRunning"
            >
              <option value="">{{ t('connectivityTest.selectApiKey') }}</option>
              <option v-for="key in apiKeys" :key="key.id" :value="key.key">
                {{ key.name }} ({{ key.key.slice(0, 12) }}...)
              </option>
            </select>
          </div>

          <!-- Model -->
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('connectivityTest.model') }}
            </label>
            <input
              v-model="model"
              type="text"
              class="input w-full"
              :disabled="isRunning"
              placeholder="claude-sonnet-4-5"
            />
          </div>

          <!-- Endpoint -->
          <div class="sm:col-span-2">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('connectivityTest.endpoint') }}
            </label>
            <input
              v-model="endpoint"
              type="text"
              class="input w-full font-mono text-sm"
              :disabled="isRunning"
            />
          </div>

          <!-- Prompt -->
          <div class="sm:col-span-2">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('connectivityTest.prompt') }}
              <span class="text-xs text-gray-400 ml-1">{{ t('connectivityTest.promptHint') }}</span>
            </label>
            <textarea
              v-model="prompt"
              rows="3"
              class="input w-full text-sm resize-none"
              :disabled="isRunning"
            />
          </div>
        </div>

        <!-- Actions -->
        <div class="flex items-center gap-3">
          <button
            @click="startTest"
            :disabled="isRunning || !selectedApiKey"
            class="btn-primary flex items-center gap-2"
          >
            <span v-if="isRunning" class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
            <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 0 1 0 1.972l-11.54 6.347a1.125 1.125 0 0 1-1.667-.986V5.653Z" />
            </svg>
            {{ isRunning ? t('connectivityTest.testing') : t('connectivityTest.startTest') }}
          </button>
          <button
            v-if="isRunning"
            @click="stopTest"
            class="btn-secondary flex items-center gap-2"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5.25 7.5A2.25 2.25 0 0 1 7.5 5.25h9a2.25 2.25 0 0 1 2.25 2.25v9a2.25 2.25 0 0 1-2.25 2.25h-9a2.25 2.25 0 0 1-2.25-2.25v-9Z" />
            </svg>
            {{ t('connectivityTest.stop') }}
          </button>
          <button
            v-if="testResult"
            @click="resetTest"
            class="btn-secondary"
            :disabled="isRunning"
          >
            {{ t('connectivityTest.reset') }}
          </button>
        </div>
      </div>

      <!-- Live Metrics Panel -->
      <div v-if="isRunning || testResult" class="card p-6 space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('connectivityTest.metrics') }}</h2>
          <!-- Status badge -->
          <span
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
            :class="statusBadgeClass"
          >
            <span v-if="isRunning" class="inline-block h-2 w-2 animate-pulse rounded-full bg-current" />
            <span v-else-if="testResult?.success" class="inline-block h-2 w-2 rounded-full bg-current" />
            <span v-else class="inline-block h-2 w-2 rounded-full bg-current" />
            {{ statusLabel }}
          </span>
        </div>

        <!-- Metric Grid -->
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.elapsed') }}</div>
            <div class="metric-value" :class="elapsedSeconds > 55 ? 'text-yellow-500' : 'text-gray-900 dark:text-white'">
              {{ elapsedSeconds.toFixed(1) }}<span class="text-sm font-normal ml-0.5">s</span>
            </div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.tokensReceived') }}</div>
            <div class="metric-value">{{ tokenCount }}</div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.chunksReceived') }}</div>
            <div class="metric-value">{{ chunkCount }}</div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.throughput') }}</div>
            <div class="metric-value">
              {{ tokensPerSecond.toFixed(1) }}<span class="text-sm font-normal ml-0.5">tok/s</span>
            </div>
          </div>
        </div>

        <!-- Timeline -->
        <div class="space-y-1">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('connectivityTest.timeline') }}</div>
          <div class="relative h-2 rounded-full bg-gray-100 dark:bg-gray-700 overflow-hidden">
            <!-- Progress bar - yellow warning zone at 55s, red at 60s -->
            <div
              class="absolute left-0 top-0 h-full rounded-full transition-all duration-500"
              :class="progressBarClass"
              :style="{ width: progressPct + '%' }"
            />
            <!-- 60s marker -->
            <div class="absolute top-0 h-full w-px bg-red-500 opacity-60" style="left: 100%" />
          </div>
          <div class="flex justify-between text-xs text-gray-400">
            <span>0s</span>
            <span class="text-yellow-500">55s</span>
            <span class="text-red-500">60s</span>
          </div>
        </div>

        <!-- Events Log -->
        <div>
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('connectivityTest.events') }}</div>
          <div
            ref="eventsLogRef"
            class="h-48 overflow-y-auto rounded-lg bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 p-3 font-mono text-xs space-y-0.5"
          >
            <div
              v-for="(event, i) in events"
              :key="i"
              class="flex gap-2"
              :class="event.color"
            >
              <span class="shrink-0 text-gray-400 tabular-nums">{{ event.time }}</span>
              <span>{{ event.msg }}</span>
            </div>
            <div v-if="events.length === 0" class="text-gray-400 italic">{{ t('connectivityTest.noEvents') }}</div>
          </div>
        </div>

        <!-- Streaming preview -->
        <div v-if="streamedText">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('connectivityTest.streamPreview') }}</div>
          <div class="rounded-lg bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-700 dark:text-gray-300 max-h-36 overflow-y-auto whitespace-pre-wrap break-words">
            {{ streamedText }}
          </div>
        </div>
      </div>

      <!-- Final Result Card -->
      <div v-if="testResult && !isRunning">
        <div
          class="card p-5 border-l-4"
          :class="testResult.success ? 'border-l-green-500' : 'border-l-red-500'"
        >
          <div class="flex items-start gap-3">
            <div class="mt-0.5">
              <svg v-if="testResult.success" class="h-5 w-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <svg v-else class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <p class="font-medium" :class="testResult.success ? 'text-green-700 dark:text-green-400' : 'text-red-700 dark:text-red-400'">
                {{ testResult.success ? t('connectivityTest.testPassed') : t('connectivityTest.testFailed') }}
              </p>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">{{ testResult.summary }}</p>
              <div v-if="testResult.diagnosis" class="mt-2 text-sm text-gray-500 dark:text-gray-400 space-y-1">
                <p class="font-medium text-gray-700 dark:text-gray-300">{{ t('connectivityTest.diagnosis') }}:</p>
                <p>{{ testResult.diagnosis }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { keysAPI } from '@/api/keys'
import type { ApiKey } from '@/types'

const { t } = useI18n()

// ── Config ──────────────────────────────────────────────────────────────────

const apiKeys = ref<ApiKey[]>([])
const selectedApiKey = ref('')
const model = ref('claude-sonnet-4-5')
const endpoint = ref(window.location.origin + '/copilot/v1/messages')
const prompt = ref(
  'Please write a very detailed, comprehensive essay about the history of the Internet, including ARPANET, the birth of TCP/IP, the World Wide Web, search engines, social media, mobile internet, cloud computing, and current AI trends. Write at least 2000 words with detailed explanations.'
)

// ── State ────────────────────────────────────────────────────────────────────

const isRunning = ref(false)
const elapsedSeconds = ref(0)
const tokenCount = ref(0)
const chunkCount = ref(0)
const streamedText = ref('')
const events = ref<{ time: string; msg: string; color: string }[]>([])
const eventsLogRef = ref<HTMLElement | null>(null)

interface TestResult {
  success: boolean
  summary: string
  diagnosis?: string
}
const testResult = ref<TestResult | null>(null)

// ── Timers & abort ───────────────────────────────────────────────────────────

let abortController: AbortController | null = null
let timerInterval: ReturnType<typeof setInterval> | null = null
let startTime = 0

// ── Computed ─────────────────────────────────────────────────────────────────

const tokensPerSecond = computed(() => {
  if (elapsedSeconds.value < 0.5) return 0
  return tokenCount.value / elapsedSeconds.value
})

const progressPct = computed(() => Math.min((elapsedSeconds.value / 60) * 100, 100))

const progressBarClass = computed(() => {
  if (elapsedSeconds.value >= 60) return 'bg-red-500'
  if (elapsedSeconds.value >= 55) return 'bg-yellow-500'
  return 'bg-blue-500'
})

const statusBadgeClass = computed(() => {
  if (isRunning.value) return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
  if (testResult.value?.success) return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
  return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
})

const statusLabel = computed(() => {
  if (isRunning.value) return t('connectivityTest.streaming')
  if (testResult.value?.success) return t('connectivityTest.completed')
  return t('connectivityTest.failed')
})

// ── Helpers ───────────────────────────────────────────────────────────────────

function addEvent(msg: string, color = 'text-gray-300') {
  const elapsed = ((Date.now() - startTime) / 1000).toFixed(2)
  events.value.push({ time: `+${elapsed}s`, msg, color })
  nextTick(() => {
    if (eventsLogRef.value) {
      eventsLogRef.value.scrollTop = eventsLogRef.value.scrollHeight
    }
  })
}

// Count tokens roughly (split by spaces/newlines as proxy)
function countTokens(text: string): number {
  return text.split(/\s+/).filter(Boolean).length
}

// ── API key loading ───────────────────────────────────────────────────────────

onMounted(async () => {
  try {
    const res = await keysAPI.list(1, 100)
    apiKeys.value = res.items || []
    if (apiKeys.value.length > 0) {
      selectedApiKey.value = apiKeys.value[0].key
    }
  } catch (e) {
    console.error('Failed to load API keys', e)
  }
})

onUnmounted(() => {
  stopTest()
})

// ── Test Logic ────────────────────────────────────────────────────────────────

function resetTest() {
  testResult.value = null
  elapsedSeconds.value = 0
  tokenCount.value = 0
  chunkCount.value = 0
  streamedText.value = ''
  events.value = []
}

function stopTest() {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
  if (timerInterval) {
    clearInterval(timerInterval)
    timerInterval = null
  }
  isRunning.value = false
}

async function startTest() {
  if (!selectedApiKey.value) return
  resetTest()
  isRunning.value = true
  startTime = Date.now()

  // Start elapsed timer
  timerInterval = setInterval(() => {
    elapsedSeconds.value = (Date.now() - startTime) / 1000
    // Warn at 55s
    if (elapsedSeconds.value >= 55 && elapsedSeconds.value < 56) {
      addEvent(t('connectivityTest.approaching60s'), 'text-yellow-400')
    }
  }, 100)

  abortController = new AbortController()

  addEvent(t('connectivityTest.eventConnecting', { endpoint: endpoint.value }), 'text-blue-400')
  addEvent(t('connectivityTest.eventModel', { model: model.value }), 'text-gray-400')
  addEvent(t('connectivityTest.eventStreamMode'), 'text-gray-400')

  try {
    const response = await fetch(endpoint.value, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'x-api-key': selectedApiKey.value,
        'anthropic-version': '2023-06-01',
      },
      body: JSON.stringify({
        model: model.value,
        max_tokens: 4096,
        stream: true,
        messages: [{ role: 'user', content: prompt.value }]
      }),
      signal: abortController.signal
    })

    if (!response.ok) {
      const body = await response.text()
      addEvent(t('connectivityTest.eventHttpError', { status: response.status, body }), 'text-red-400')
      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultHttpError', { status: response.status }),
        diagnosis: body.slice(0, 300)
      }
      stopTest()
      return
    }

    addEvent(t('connectivityTest.eventConnected', { status: response.status }), 'text-green-400')

    const reader = response.body?.getReader()
    if (!reader) throw new Error('No response body reader')

    const decoder = new TextDecoder()
    let buffer = ''
    let firstChunkAt: number | null = null
    let streamEndedCleanly = false

    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        addEvent(t('connectivityTest.eventStreamEnd'), 'text-green-400')
        streamEndedCleanly = true
        break
      }

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const data = line.slice(6).trim()
        if (!data || data === '[DONE]') {
          if (data === '[DONE]') {
            addEvent('[DONE] received', 'text-green-400')
            streamEndedCleanly = true
          }
          continue
        }

        try {
          const evt = JSON.parse(data)

          // Count chunks
          chunkCount.value++

          if (firstChunkAt === null) {
            firstChunkAt = Date.now() - startTime
            addEvent(t('connectivityTest.eventFirstChunk', { ms: firstChunkAt }), 'text-cyan-400')
          }

          // Extract text from Anthropic SSE events
          if (evt.type === 'content_block_delta' && evt.delta?.type === 'text_delta') {
            const text = evt.delta.text || ''
            streamedText.value += text
            tokenCount.value += countTokens(text)
          } else if (evt.type === 'message_stop') {
            addEvent(t('connectivityTest.eventMessageStop'), 'text-green-400')
            streamEndedCleanly = true
          } else if (evt.type === 'message_start') {
            const model = evt.message?.model || ''
            if (model) addEvent(t('connectivityTest.eventActualModel', { model }), 'text-cyan-400')
          } else if (evt.type === 'error') {
            addEvent(t('connectivityTest.eventError', { msg: JSON.stringify(evt.error) }), 'text-red-400')
          }
        } catch {
          // skip unparseable
        }
      }
    }

    const totalMs = Date.now() - startTime
    const totalSec = (totalMs / 1000).toFixed(2)

    if (streamEndedCleanly) {
      addEvent(t('connectivityTest.eventSuccess', { sec: totalSec, tokens: tokenCount.value }), 'text-green-400')
      testResult.value = {
        success: true,
        summary: t('connectivityTest.resultSuccess', { sec: totalSec, tokens: tokenCount.value, chunks: chunkCount.value }),
        diagnosis: totalMs > 55000
          ? t('connectivityTest.diagnosisSlowButOk', { sec: totalSec })
          : t('connectivityTest.diagnosisGood')
      }
    } else {
      addEvent(t('connectivityTest.eventUncleanEnd', { sec: totalSec }), 'text-yellow-400')
      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultUnclean', { sec: totalSec }),
        diagnosis: t('connectivityTest.diagnosisUnclean')
      }
    }
  } catch (err: any) {
    if (err.name === 'AbortError') {
      addEvent(t('connectivityTest.eventAborted'), 'text-yellow-400')
      const totalSec = ((Date.now() - startTime) / 1000).toFixed(2)
      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultAborted', { sec: totalSec }),
      }
    } else {
      const totalMs = Date.now() - startTime
      const totalSec = (totalMs / 1000).toFixed(2)
      addEvent(t('connectivityTest.eventFetchError', { msg: err.message }), 'text-red-400')

      // Diagnose based on elapsed time
      let diagnosis = ''
      if (totalMs >= 58000 && totalMs <= 62000) {
        diagnosis = t('connectivityTest.diagnosis60s')
      } else if (totalMs < 5000) {
        diagnosis = t('connectivityTest.diagnosisQuickFail')
      } else {
        diagnosis = t('connectivityTest.diagnosisGeneral', { sec: totalSec })
      }

      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultError', { msg: err.message }),
        diagnosis
      }
    }
  } finally {
    stopTest()
  }
}
</script>

<style scoped>
.metric-box {
  @apply rounded-lg bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 p-3;
}
.metric-label {
  @apply text-xs text-gray-500 dark:text-gray-400 mb-1;
}
.metric-value {
  @apply text-xl font-bold text-gray-900 dark:text-white tabular-nums;
}
</style>
