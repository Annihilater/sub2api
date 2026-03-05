<template>
  <div class="min-w-[140px]">
    <!-- Loading -->
    <div v-if="loading && !quotaInfo" class="space-y-1">
      <div v-for="n in 3" :key="n" class="h-2.5 animate-pulse rounded bg-gray-200 dark:bg-gray-700" :style="{ width: `${50 + n * 15}%` }" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-xs text-red-500 dark:text-red-400" :title="error">
      {{ t('admin.copilotUsage.fetchFailed') }}
    </div>

    <!-- Not a copilot account -->
    <span v-else-if="!isCopilot" class="text-xs text-gray-300 dark:text-dark-600">—</span>

    <!-- Quota data -->
    <div v-else-if="quotaInfo" class="space-y-1 text-xs">
      <!-- Plan -->
      <div class="flex items-center gap-1">
        <span class="text-gray-400 dark:text-gray-500">{{ t('admin.accounts.copilot.quota.plan') }}:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">{{ quotaInfo.plan_type || quotaInfo.plan || '—' }}</span>
      </div>

      <!-- Premium Interactions with mini progress bar -->
      <div v-if="quotaInfo.premium_interactions">
        <div class="flex items-center justify-between gap-1">
          <span class="text-gray-400 dark:text-gray-500 truncate">Premium:</span>
          <span
            class="font-medium tabular-nums"
            :class="piClass"
          >
            <template v-if="quotaInfo.premium_interactions.unlimited">∞</template>
            <template v-else>{{ quotaInfo.premium_interactions.remaining }}/{{ quotaInfo.premium_interactions.entitlement }}</template>
          </span>
        </div>
        <!-- Mini progress bar -->
        <div v-if="!quotaInfo.premium_interactions.unlimited" class="mt-0.5 h-1 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
          <div
            class="h-full rounded-full transition-all duration-300"
            :class="piBarClass"
            :style="{ width: piBarWidth }"
          />
        </div>
      </div>

      <!-- Chat -->
      <div v-if="quotaInfo.chat" class="flex items-center gap-1">
        <span class="text-gray-400 dark:text-gray-500">Chat:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">
          <template v-if="quotaInfo.chat.unlimited">∞</template>
          <template v-else>{{ quotaInfo.chat.remaining }}/{{ quotaInfo.chat.entitlement }}</template>
        </span>
      </div>

      <!-- Completions -->
      <div v-if="quotaInfo.completions" class="flex items-center gap-1">
        <span class="text-gray-400 dark:text-gray-500">Compl:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">
          <template v-if="quotaInfo.completions.unlimited">∞</template>
          <template v-else>{{ quotaInfo.completions.remaining }}/{{ quotaInfo.completions.entitlement }}</template>
        </span>
      </div>

      <!-- Reset date -->
      <div v-if="quotaInfo.quota_reset_date" class="flex items-center gap-1">
        <span class="text-gray-400 dark:text-gray-500">{{ t('admin.accounts.copilot.quota.resetDate') }}:</span>
        <span class="text-gray-600 dark:text-gray-400">{{ quotaInfo.quota_reset_date }}</span>
      </div>
    </div>

    <!-- No data yet (copilot account, not loaded) -->
    <span v-else class="text-xs text-gray-300 dark:text-dark-600">—</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CopilotQuotaInfo } from '@/api/admin/accounts'

const props = withDefaults(
  defineProps<{
    isCopilot?: boolean
    quotaInfo?: CopilotQuotaInfo | null
    loading?: boolean
    error?: string | null
  }>(),
  {
    isCopilot: false,
    quotaInfo: null,
    loading: false,
    error: null
  }
)

const { t } = useI18n()

const pi = computed(() => props.quotaInfo?.premium_interactions)

const piClass = computed(() => {
  if (!pi.value || pi.value.unlimited) return 'text-green-600 dark:text-green-400'
  const rem = pi.value.remaining ?? 0
  if (rem < 0) return 'text-red-500 dark:text-red-400'
  const pct = pi.value.percent_remaining ?? 100
  if (pct <= 20) return 'text-orange-500 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
})

const piBarClass = computed(() => {
  if (!pi.value) return 'bg-gray-300'
  const pct = pi.value.percent_remaining ?? 100
  if (pct <= 0) return 'bg-red-500'
  if (pct <= 20) return 'bg-orange-500'
  if (pct <= 50) return 'bg-yellow-400'
  return 'bg-green-500'
})

const piBarWidth = computed(() => {
  if (!pi.value || pi.value.unlimited) return '100%'
  const pct = pi.value.percent_remaining ?? 0
  return `${Math.max(0, Math.min(100, pct))}%`
})
</script>
