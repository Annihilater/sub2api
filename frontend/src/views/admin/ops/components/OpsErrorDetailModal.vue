<template>
  <BaseDialog :show="show" :title="title" width="full" content-class="!h-[88vh]" body-class="!p-0 !overflow-hidden flex flex-col" :close-on-click-outside="true" @close="close">
    <OpsErrorDetailPanel class="min-h-0 flex-1 overflow-auto" :show="show" :error-id="errorId" :error-type="errorType" />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import OpsErrorDetailPanel from './OpsErrorDetailPanel.vue'

interface Props {
  show: boolean
  errorId: number | null
  errorType?: 'request' | 'upstream'
}

interface Emits {
  (e: 'update:show', value: boolean): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()

const title = computed(() => {
  if (!props.errorId) return t('admin.ops.errorDetail.title')
  return t('admin.ops.errorDetail.titleWithId', { id: String(props.errorId) })
})

function close() {
  emit('update:show', false)
}

</script>
