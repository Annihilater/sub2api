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
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.apiKey') }}
            </label>
            <Select
              :model-value="selectedApiKeyId"
              :options="apiKeyOptions"
              :placeholder="t('connectivityTest.selectApiKey')"
              :disabled="isRunning"
              searchable
              @update:model-value="onApiKeySelectChange"
            >
              <template #selected="{ option }">
                <span v-if="option" class="flex items-center gap-2 min-w-0">
                  <span class="font-medium truncate">{{ option.label }}</span>
                  <span class="shrink-0 text-xs text-gray-400 font-mono">{{ option.keyPreview }}</span>
                </span>
                <span v-else class="text-gray-400">{{ t('connectivityTest.selectApiKey') }}</span>
              </template>
              <template #option="{ option }">
                <span class="flex items-center justify-between gap-3 w-full min-w-0">
                  <span class="font-medium truncate">{{ option.label }}</span>
                  <span class="shrink-0 text-xs text-gray-400 dark:text-gray-500 font-mono">{{ option.keyPreview }}</span>
                </span>
              </template>
            </Select>
          </div>

          <!-- Model selector -->
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.model') }}
              <span v-if="modelsLoading" class="ml-2 inline-block h-3 w-3 animate-spin rounded-full border border-gray-400 border-t-transparent align-middle" />
            </label>
            <Select
              v-if="availableModels.length > 0"
              v-model="model"
              :options="modelOptions"
              :disabled="isRunning"
              searchable
            />
            <input
              v-else
              v-model="model"
              type="text"
              class="input w-full"
              :disabled="isRunning"
              placeholder="claude-sonnet-4-5"
            />
          </div>

          <!-- Endpoint -->
          <div class="sm:col-span-2">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.endpoint') }}
            </label>
            <input
              v-model="endpoint"
              type="text"
              class="w-full rounded-lg border border-gray-200 dark:border-dark-600 bg-white dark:bg-dark-800 px-3 py-2.5 font-mono text-sm text-gray-900 dark:text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              :disabled="isRunning"
            />
          </div>

          <!-- Timeout threshold + Stream duration -->
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.timeoutThreshold') }}
              <span class="text-xs text-gray-400 ml-1">{{ t('connectivityTest.timeoutThresholdHint') }}</span>
            </label>
            <input
              v-model.number="timeoutThreshold"
              type="number"
              min="10"
              max="600"
              class="w-full rounded-lg border border-gray-200 dark:border-dark-600 bg-white dark:bg-dark-800 px-3 py-2.5 text-sm text-gray-900 dark:text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              :disabled="isRunning"
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.streamDuration') }}
              <span class="text-xs text-gray-400 ml-1">{{ t('connectivityTest.streamDurationHint') }}</span>
            </label>
            <input
              v-model.number="streamDuration"
              type="number"
              min="10"
              max="600"
              class="w-full rounded-lg border border-gray-200 dark:border-dark-600 bg-white dark:bg-dark-800 px-3 py-2.5 text-sm text-gray-900 dark:text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              :disabled="isRunning"
            />
          </div>

          <!-- Prompt -->
          <div class="sm:col-span-2">
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              {{ t('connectivityTest.prompt') }}
              <span class="text-xs text-gray-400 ml-1">{{ t('connectivityTest.promptHint') }}</span>
            </label>
            <textarea
              v-model="prompt"
              rows="3"
              class="w-full rounded-lg border border-gray-200 dark:border-dark-600 bg-white dark:bg-dark-800 px-3 py-2.5 text-sm text-gray-900 dark:text-white shadow-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed resize-none transition-colors"
              :disabled="isRunning"
            />
          </div>
        </div>

        <!-- Actions -->
        <div class="flex items-center gap-3 pt-1">
          <!-- Start button -->
          <button
            v-if="!isRunning"
            @click="startTest"
            :disabled="!selectedApiKeyId || !model"
            class="inline-flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-semibold text-white shadow-sm transition-all focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-40 disabled:cursor-not-allowed"
            :class="!selectedApiKeyId || !model
              ? 'bg-gray-300 dark:bg-gray-700 cursor-not-allowed'
              : 'bg-primary-600 hover:bg-primary-700 active:bg-primary-800 focus:ring-primary-500'"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 0 1 0 1.972l-11.54 6.347a1.125 1.125 0 0 1-1.667-.986V5.653Z" />
            </svg>
            {{ t('connectivityTest.startTest') }}
          </button>

          <!-- Stop button (while running) -->
          <button
            v-if="isRunning"
            @click="stopTest"
            class="inline-flex items-center gap-2 rounded-lg bg-red-600 hover:bg-red-700 active:bg-red-800 px-5 py-2.5 text-sm font-semibold text-white shadow-sm transition-all focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
          >
            <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
              <path d="M5.25 7.5A2.25 2.25 0 0 1 7.5 5.25h9a2.25 2.25 0 0 1 2.25 2.25v9a2.25 2.25 0 0 1-2.25 2.25h-9a2.25 2.25 0 0 1-2.25-2.25v-9Z" />
            </svg>
            {{ t('connectivityTest.stop') }}
          </button>

          <!-- Running spinner label -->
          <div v-if="isRunning" class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
            <span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-gray-300 dark:border-gray-600 border-t-primary-500" />
            {{ t('connectivityTest.testing') }}
          </div>

          <!-- Reset button -->
          <button
            v-if="testResult && !isRunning"
            @click="resetTest"
            class="inline-flex items-center gap-2 rounded-lg border border-gray-200 dark:border-dark-600 bg-white dark:bg-dark-800 hover:bg-gray-50 dark:hover:bg-dark-700 px-4 py-2.5 text-sm font-medium text-gray-700 dark:text-gray-300 shadow-sm transition-all focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-2"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99" />
            </svg>
            {{ t('connectivityTest.reset') }}
          </button>
        </div>
      </div>

      <!-- Live Metrics Panel -->
      <div v-if="isRunning || testResult" class="card p-6 space-y-5">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('connectivityTest.metrics') }}</h2>
          <!-- Status badge -->
          <span
            class="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold"
            :class="statusBadgeClass"
          >
            <span
              class="h-2 w-2 rounded-full bg-current"
              :class="isRunning ? 'animate-pulse' : ''"
            />
            {{ statusLabel }}
          </span>
        </div>

        <!-- Metric Grid -->
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.elapsed') }}</div>
            <div class="metric-value" :class="elapsedSeconds > warnThreshold ? 'text-yellow-500 dark:text-yellow-400' : ''">
              {{ elapsedSeconds.toFixed(1) }}<span class="text-sm font-normal ml-0.5 opacity-70">s</span>
            </div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.tokensReceived') }}</div>
            <div class="metric-value">{{ tokenCount.toLocaleString() }}</div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.chunksReceived') }}</div>
            <div class="metric-value">{{ chunkCount.toLocaleString() }}</div>
          </div>
          <div class="metric-box">
            <div class="metric-label">{{ t('connectivityTest.throughput') }}</div>
            <div class="metric-value">
              {{ tokensPerSecond.toFixed(1) }}<span class="text-sm font-normal ml-0.5 opacity-70">tok/s</span>
            </div>
          </div>
        </div>

        <!-- Timeline -->
        <div class="space-y-2">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('connectivityTest.timeline') }}</div>
          <div class="relative h-3 rounded-full bg-gray-100 dark:bg-gray-700/60 overflow-hidden">
            <div
              class="absolute left-0 top-0 h-full rounded-full transition-all duration-300"
              :class="progressBarClass"
              :style="{ width: progressPct + '%' }"
            />
            <!-- warn threshold marker -->
            <div class="absolute top-0 h-full w-0.5 bg-yellow-400/60" :style="{ left: warnPct + '%' }" />
          </div>
          <div class="flex justify-between text-xs">
            <span class="text-gray-400">0s</span>
            <span class="text-yellow-500 font-medium">{{ warnThreshold }}s</span>
            <span class="text-red-500 font-medium">{{ timeoutThreshold }}s</span>
          </div>
        </div>

        <!-- Events Log -->
        <div>
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('connectivityTest.events') }}</div>
          <div
            ref="eventsLogRef"
            class="h-44 overflow-y-auto rounded-xl bg-gray-950/5 dark:bg-black/20 border border-gray-200 dark:border-dark-700 p-3 font-mono text-xs"
          >
            <div
              v-for="(event, i) in events"
              :key="i"
              class="flex gap-2 leading-5"
              :class="event.color"
            >
              <span class="shrink-0 text-gray-400 tabular-nums w-14 text-right">{{ event.time }}</span>
              <span class="break-all">{{ event.msg }}</span>
            </div>
            <div v-if="events.length === 0" class="text-gray-400 italic py-1">{{ t('connectivityTest.noEvents') }}</div>
          </div>
        </div>

        <!-- Streaming preview -->
        <div v-if="streamedText">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('connectivityTest.streamPreview') }}</div>
          <div
            ref="streamPreviewRef"
            class="h-40 overflow-y-auto rounded-xl bg-gray-950/5 dark:bg-black/20 border border-gray-200 dark:border-dark-700 p-3 text-xs text-gray-700 dark:text-gray-300 whitespace-pre-wrap break-words leading-relaxed"
          >{{ streamedText }}</div>
        </div>
      </div>

      <!-- Final Result Card -->
      <div v-if="testResult && !isRunning">
        <div
          class="card p-5 border-l-4"
          :class="testResult.success ? 'border-l-green-500' : 'border-l-red-500'"
        >
          <div class="flex items-start gap-3">
            <div class="mt-0.5 shrink-0">
              <div v-if="testResult.success" class="flex h-8 w-8 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
                <svg class="h-4 w-4 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </div>
              <div v-else class="flex h-8 w-8 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
                <svg class="h-4 w-4 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
                </svg>
              </div>
            </div>
            <div class="flex-1 min-w-0">
              <p class="font-semibold text-base" :class="testResult.success ? 'text-green-700 dark:text-green-400' : 'text-red-700 dark:text-red-400'">
                {{ testResult.success ? t('connectivityTest.testPassed') : t('connectivityTest.testFailed') }}
              </p>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">{{ testResult.summary }}</p>
              <div v-if="testResult.diagnosis" class="mt-3 rounded-lg bg-gray-50 dark:bg-dark-800 border border-gray-200 dark:border-dark-700 p-3">
                <p class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 mb-1">{{ t('connectivityTest.diagnosis') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">{{ testResult.diagnosis }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import { keysAPI } from '@/api/keys'
import type { ApiKey } from '@/types'

const { t, locale } = useI18n()

// ── Config ──────────────────────────────────────────────────────────────────

const apiKeys = ref<ApiKey[]>([])
const selectedApiKeyId = ref<number | ''>('')
const selectedApiKey = ref('')
const availableModels = ref<string[]>([])
const modelsLoading = ref(false)
const model = ref('claude-sonnet-4-5')
const endpoint = ref('/copilot/v1/messages')

/** 超时阈值（秒），超时诊断判断依据 */
const timeoutThreshold = ref(60)
/** 警告阈值：超时阈值的 91.67%（同原来 55/60 比例）*/
const warnThreshold = computed(() => Math.round(timeoutThreshold.value * (55 / 60)))
/** 警告阈值在进度条上的百分比位置 */
const warnPct = computed(() => (warnThreshold.value / timeoutThreshold.value) * 100)

/** 要求模型持续流式输出的时长（秒） */
const streamDuration = ref(70)

// Computed options for Select components — API keys 按名称倒序，模型按名称正序
const apiKeyOptions = computed(() =>
  [...apiKeys.value]
    .sort((a, b) => b.name.localeCompare(a.name))
    .map(k => ({
      value: k.id,
      label: k.name,
      keyPreview: `${k.key.slice(0, 6)}…${k.key.slice(-4)}`
    }))
)

const modelOptions = computed(() =>
  [...availableModels.value]
    .sort((a, b) => a.localeCompare(b))
    .map(m => ({ value: m, label: m }))
)

// Prompt — 两套，根据 locale 自动切换（仅显示用，不含流式持续时长追加）
const promptEn = 'Please write a very detailed, comprehensive essay about the history of the Internet, including ARPANET, the birth of TCP/IP, the World Wide Web, search engines, social media, mobile internet, cloud computing, and current AI trends. Write at least 2000 words with detailed explanations.'
const promptZh = '请写一篇非常详细全面的文章，介绍互联网的发展历史，包括 ARPANET 的诞生、TCP/IP 协议的出现、万维网、搜索引擎、社交媒体、移动互联网、云计算以及当前 AI 趋势。请用中文撰写，字数不少于 2000 字，并配以详细说明。'

const prompt = ref(locale.value === 'zh' ? promptZh : promptEn)

// Auto-switch prompt when locale changes (only if user hasn't modified it)
watch(locale, (lang) => {
  const isDefault = prompt.value === promptEn || prompt.value === promptZh
  if (isDefault) {
    prompt.value = lang === 'zh' ? promptZh : promptEn
  }
})

/** 构建实际发送给模型的提示词（在用户提示词末尾追加流式持续时长要求） */
function buildFinalPrompt(): string {
  const suffix = locale.value === 'zh'
    ? `\n\n请保持流式输出至少 ${streamDuration.value} 秒，生成足够长的详细回答。`
    : `\n\nPlease keep your response streaming for at least ${streamDuration.value} seconds by generating a sufficiently long and detailed answer.`
  return prompt.value + suffix
}

// ── State ────────────────────────────────────────────────────────────────────

const isRunning = ref(false)
const elapsedSeconds = ref(0)
const tokenCount = ref(0)
const chunkCount = ref(0)
const streamedText = ref('')
const events = ref<{ time: string; msg: string; color: string }[]>([])
const eventsLogRef = ref<HTMLElement | null>(null)
const streamPreviewRef = ref<HTMLElement | null>(null)

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
let warnedThreshold = false

// ── Computed ─────────────────────────────────────────────────────────────────

const tokensPerSecond = computed(() => {
  if (elapsedSeconds.value < 0.5) return 0
  return tokenCount.value / elapsedSeconds.value
})

const progressPct = computed(() =>
  Math.min((elapsedSeconds.value / timeoutThreshold.value) * 100, 100)
)

const progressBarClass = computed(() => {
  if (elapsedSeconds.value >= timeoutThreshold.value) return 'bg-red-500'
  if (elapsedSeconds.value >= warnThreshold.value) return 'bg-yellow-500'
  return 'bg-primary-500'
})

const statusBadgeClass = computed(() => {
  if (isRunning.value) return 'bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-400 ring-1 ring-blue-200 dark:ring-blue-800'
  if (testResult.value?.success) return 'bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-400 ring-1 ring-green-200 dark:ring-green-800'
  return 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 ring-1 ring-red-200 dark:ring-red-800'
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

function countTokens(text: string): number {
  return text.split(/\s+/).filter(Boolean).length
}

// ── API key + model loading ───────────────────────────────────────────────────

async function fetchModelsForKey(apiKey: string) {
  if (!apiKey) {
    availableModels.value = []
    return
  }
  modelsLoading.value = true
  try {
    const res = await fetch('/copilot/v1/models', {
      headers: { 'x-api-key': apiKey }
    })
    if (res.ok) {
      const data = await res.json()
      // OpenAI-compatible models list: { data: [ { id: string } ] }
      const models: string[] = (data?.data ?? []).map((m: { id: string }) => m.id).filter(Boolean)
      availableModels.value = models
      // Set default model if current not in list
      if (models.length > 0 && !models.includes(model.value)) {
        // prefer a claude-sonnet variant
        const preferred = models.find(m => m.toLowerCase().includes('sonnet')) ?? models[0]
        model.value = preferred
      }
    } else {
      availableModels.value = []
    }
  } catch {
    availableModels.value = []
  } finally {
    modelsLoading.value = false
  }
}

function onApiKeySelectChange(val: string | number | boolean | null) {
  const id = typeof val === 'number' ? val : Number(val)
  selectedApiKeyId.value = id || ''
  const key = apiKeys.value.find(k => k.id === id)
  selectedApiKey.value = key?.key ?? ''
  fetchModelsForKey(selectedApiKey.value)
}

onMounted(async () => {
  try {
    const res = await keysAPI.list(1, 100)
    apiKeys.value = res.items || []
    if (apiKeys.value.length > 0) {
      // 使用排序后第一个（名称倒序最大的）
      const sorted = [...apiKeys.value].sort((a, b) => b.name.localeCompare(a.name))
      const first = sorted[0]
      selectedApiKeyId.value = first.id
      selectedApiKey.value = first.key
      await fetchModelsForKey(first.key)
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
  warnedThreshold = false
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
  if (!selectedApiKey.value || !model.value) return
  resetTest()
  isRunning.value = true
  startTime = Date.now()

  // 缓存本次测试的阈值（防止测试中途用户修改）
  const threshold = timeoutThreshold.value
  const warn = warnThreshold.value

  // Start elapsed timer
  timerInterval = setInterval(() => {
    elapsedSeconds.value = (Date.now() - startTime) / 1000
    if (elapsedSeconds.value >= warn && !warnedThreshold) {
      warnedThreshold = true
      addEvent(t('connectivityTest.approachingTimeout', { sec: threshold }), 'text-yellow-400')
    }
  }, 100)

  abortController = new AbortController()

  addEvent(t('connectivityTest.eventConnecting', { endpoint: endpoint.value }), 'text-blue-400')
  addEvent(t('connectivityTest.eventModel', { model: model.value }), 'text-gray-400')
  addEvent(t('connectivityTest.eventStreamMode'), 'text-gray-400')

  const finalPrompt = buildFinalPrompt()

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
        messages: [{ role: 'user', content: finalPrompt }]
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
            // Auto-scroll stream preview
            nextTick(() => {
              if (streamPreviewRef.value) {
                streamPreviewRef.value.scrollTop = streamPreviewRef.value.scrollHeight
              }
            })
          } else if (evt.type === 'message_stop') {
            addEvent(t('connectivityTest.eventMessageStop'), 'text-green-400')
            streamEndedCleanly = true
          } else if (evt.type === 'message_start') {
            const actualModel = evt.message?.model || ''
            if (actualModel) addEvent(t('connectivityTest.eventActualModel', { model: actualModel }), 'text-cyan-400')
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
        diagnosis: totalMs > warn * 1000
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
  } catch (err: unknown) {
    const error = err as Error
    if (error.name === 'AbortError') {
      addEvent(t('connectivityTest.eventAborted'), 'text-yellow-400')
      const totalSec = ((Date.now() - startTime) / 1000).toFixed(2)
      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultAborted', { sec: totalSec }),
      }
    } else {
      const totalMs = Date.now() - startTime
      const totalSec = (totalMs / 1000).toFixed(2)
      addEvent(t('connectivityTest.eventFetchError', { msg: error.message }), 'text-red-400')

      // 判断是否为超时阈值附近的断连（±2 秒内）
      const nearTimeout = Math.abs(totalMs - threshold * 1000) <= 2000

      let diagnosis = ''
      if (nearTimeout) {
        diagnosis = t('connectivityTest.diagnosisTimeout', { sec: threshold })
      } else if (totalMs < 5000) {
        diagnosis = t('connectivityTest.diagnosisQuickFail')
      } else {
        diagnosis = t('connectivityTest.diagnosisGeneral', { sec: totalSec })
      }

      testResult.value = {
        success: false,
        summary: t('connectivityTest.resultError', { msg: error.message }),
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
  @apply rounded-xl bg-gray-50 dark:bg-dark-800/60 border border-gray-200 dark:border-dark-700 p-4;
}
.metric-label {
  @apply text-xs font-medium text-gray-500 dark:text-gray-400 mb-1.5;
}
.metric-value {
  @apply text-2xl font-bold text-gray-900 dark:text-white tabular-nums;
}
</style>
