<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button class="btn btn-secondary whitespace-nowrap" @click="loadFingerprint">
            <Icon name="sparkles" size="sm" class="text-violet-500" />
            {{ t('admin.accounts.modeltraceHistory.bank') }}: {{ fingerprintCommit ? fingerprintCommit.slice(0, 10) : '…' }}
          </button>
          <button class="btn btn-primary shrink-0 whitespace-nowrap" :disabled="refreshingBank" @click="refreshBank">
            {{ refreshingBank ? t('admin.accounts.modeltraceHistory.refreshing') : t('admin.accounts.modeltraceHistory.refreshBank') }}
          </button>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-64">
            <select v-model="accountId" class="input" @change="reload">
              <option :value="0" disabled>{{ t('admin.accounts.modeltraceHistory.chooseAccount') }}</option>
              <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }} (#{{ a.id }})</option>
            </select>
          </div>
          <div class="relative w-full sm:w-56">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
            <input v-model="modelFilter" type="text" :placeholder="t('admin.accounts.modeltraceHistory.modelFilter')" class="input pl-10" @keyup.enter="reload" />
          </div>
          <div class="w-full sm:w-36">
            <select v-model="outcomeFilter" class="input" @change="reload">
              <option value="all">{{ t('admin.accounts.modeltraceHistory.filterAll') }}</option>
              <option value="success">{{ t('admin.accounts.modeltraceHistory.filterSuccess') }}</option>
              <option value="failed">{{ t('admin.accounts.modeltraceHistory.filterFailed') }}</option>
            </select>
          </div>
          <button class="btn btn-primary shrink-0 whitespace-nowrap" :disabled="loading" @click="reload">{{ t('admin.accounts.modeltraceHistory.query') }}</button>
          <button v-if="accountId" class="btn btn-danger ml-auto shrink-0 whitespace-nowrap" :disabled="loading" @click="showDeleteDialog = true">
            {{ t('admin.accounts.modeltraceHistory.deleteRecords') }}
          </button>
        </div>
        <div v-if="notice" class="mt-2 text-sm text-green-600 dark:text-green-400">{{ notice }}</div>
      </template>

      <template #table>
        <div v-if="!accountId" class="flex min-h-40 items-center justify-center p-10 text-sm text-gray-400">
          {{ t('admin.accounts.modeltraceHistory.selectFirst') }}
        </div>
        <div v-else-if="loading" class="flex min-h-40 items-center justify-center p-10 text-sm text-gray-400">
          {{ t('admin.accounts.modeltraceHistory.loading') }}
        </div>
        <div v-else-if="!items.length" class="flex min-h-40 items-center justify-center p-10 text-sm text-gray-400">
          {{ t('admin.accounts.modeltraceHistory.empty') }}
        </div>
        <div v-else class="table-wrapper">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead>
              <tr>
                <th>{{ t('admin.accounts.modeltraceHistory.colTime') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colModel') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colVerdict') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colPredicted') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colProbability') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colDuration') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colProxy') }}</th>
                <th>{{ t('admin.accounts.modeltraceHistory.colBank') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in items" :key="row.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/50" @click="selected = row">
                <td class="whitespace-nowrap text-gray-500 dark:text-gray-400">{{ formatTime(row.occurred_at) }}</td>
                <td class="font-medium text-gray-900 dark:text-gray-100">{{ row.model }}</td>
                <td><span :class="verdictClass(row)">{{ verdictLabel(row) }}</span></td>
                <td>{{ row.fingerprint_predicted_model || '—' }}</td>
                <td>{{ row.fingerprint_probability != null ? (row.fingerprint_probability * 100).toFixed(1) + '%' : '—' }}</td>
                <td class="whitespace-nowrap">{{ row.duration_ms ? (row.duration_ms / 1000).toFixed(1) + 's' : '—' }}</td>
                <td>{{ row.proxy_name || '—' }}</td>
                <td class="font-mono text-xs text-gray-400">{{ row.fingerprint_commit ? row.fingerprint_commit.slice(0, 8) : '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template #pagination>
        <div v-if="accountId && total > 0" class="flex items-center justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.modeltraceHistory.total', { total }) }}</span>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="page <= 1" @click="page--; load()">{{ t('admin.accounts.modeltraceHistory.prev') }}</button>
            <span class="text-gray-500 dark:text-gray-400">{{ page }} / {{ totalPages }}</span>
            <button class="btn btn-secondary" :disabled="page >= totalPages" @click="page++; load()">{{ t('admin.accounts.modeltraceHistory.next') }}</button>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <!-- 详情抽屉 -->
    <div v-if="selected" class="fixed inset-0 z-50 flex justify-end bg-black/40" @click.self="selected = null">
      <div class="h-full w-full max-w-lg overflow-y-auto bg-white p-6 shadow-xl dark:bg-dark-900">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="text-lg font-bold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.modeltraceHistory.detailTitle') }}</h2>
          <button class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="selected = null">✕</button>
        </div>
        <dl class="space-y-3 text-sm">
          <template v-for="(value, key) in detailRows" :key="key">
            <div class="flex justify-between gap-4 border-b border-gray-100 pb-2 dark:border-dark-800">
              <dt class="text-gray-500 dark:text-gray-400">{{ key }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-gray-200">{{ value }}</dd>
            </div>
          </template>
        </dl>
      </div>
    </div>
    <!-- 删除确认 -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.accounts.modeltraceHistory.deleteRecords')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiClient } from '@/api/client'
import * as tickets from '@/api/admin/codexTickets'
import type { TicketAttempt } from '@/api/admin/codexTickets'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t, te, locale } = useI18n()

interface AccountOption { id: number; name: string }

const accounts = ref<AccountOption[]>([])
const accountId = ref(0)
const modelFilter = ref('')
const outcomeFilter = ref('all')
const items = ref<TicketAttempt[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const selected = ref<TicketAttempt | null>(null)
const fingerprintCommit = ref('')
const refreshingBank = ref(false)
const showDeleteDialog = ref(false)
const deleting = ref(false)
const notice = ref('')

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function loadAccounts() {
  const { data } = await apiClient.get<{ items: AccountOption[] }>('/admin/accounts', { params: { page: 1, page_size: 200, platform: 'openai' } })
  accounts.value = data.items || []
  // 默认选中第一个账号并自动查询，避免打开页面时误以为无数据
  if (!accountId.value && accounts.value.length) {
    accountId.value = accounts.value[0].id
    load()
  }
}

async function loadFingerprint() {
  try {
    const fp = await tickets.fingerprint()
    fingerprintCommit.value = fp.commit
  } catch { /* 忽略，保留旧值 */ }
}

async function refreshBank() {
  refreshingBank.value = true
  try {
    const fp = await tickets.refreshFingerprint()
    fingerprintCommit.value = fp.commit
  } finally {
    refreshingBank.value = false
  }
}

const currentAccountName = computed(() => accounts.value.find(a => a.id === accountId.value)?.name || `#${accountId.value}`)

const deleteScope = computed(() => {
  const filterKey = outcomeFilter.value === 'success' ? 'filterSuccess' : outcomeFilter.value === 'failed' ? 'filterFailed' : 'filterAll'
  let scope = t(`admin.accounts.modeltraceHistory.${filterKey}`)
  const model = modelFilter.value.trim()
  if (model) scope += ` / ${model}`
  return scope
})

const deleteConfirmMessage = computed(() => t('admin.accounts.modeltraceHistory.deleteConfirm', { name: currentAccountName.value, scope: deleteScope.value }))

async function confirmDelete() {
  if (!accountId.value || deleting.value) return
  deleting.value = true
  try {
    const { deleted } = await tickets.deleteHistory(accountId.value, {
      model: modelFilter.value.trim() || undefined,
      filter: outcomeFilter.value
    })
    showDeleteDialog.value = false
    notice.value = t('admin.accounts.modeltraceHistory.deleted', { count: deleted })
    reload()
  } finally {
    deleting.value = false
  }
}

function triggerLabel(row: TicketAttempt) {
  const v = row.trigger
  if (!v) return '—'
  const key = `admin.accounts.modeltraceHistory.trigger_${v}`
  return te(key) ? t(key) : v
}

async function load() {
  if (!accountId.value) return
  loading.value = true
  try {
    const data = await tickets.history(accountId.value, {
      model: modelFilter.value.trim() || undefined,
      filter: outcomeFilter.value,
      page: page.value,
      page_size: pageSize
    })
    items.value = data.items || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function reload() { notice.value = ''; page.value = 1; load() }

function formatTime(v: string) {
  const d = new Date(v)
  return d.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
}

function verdictLabel(row: TicketAttempt) {
  if (row.outcome !== 'success') return t('admin.accounts.modeltraceHistory.vFailed')
  if (row.fingerprint_matched === true) return t('admin.accounts.modeltraceHistory.vNormal')
  if (row.fingerprint_matched === false) return t('admin.accounts.modeltraceHistory.vDegraded')
  return t('admin.accounts.modeltraceHistory.vUncertain')
}

function verdictClass(row: TicketAttempt) {
  if (row.outcome !== 'success') return 'rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (row.fingerprint_matched === true) return 'rounded-full bg-green-50 px-2 py-0.5 text-xs text-green-700 dark:bg-green-900/30 dark:text-green-300'
  if (row.fingerprint_matched === false) return 'rounded-full bg-red-50 px-2 py-0.5 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
}

const detailRows = computed<Record<string, string>>(() => {
  const row = selected.value
  if (!row) return {}
  return {
    [t('admin.accounts.modeltraceHistory.colTime')]: formatTime(row.occurred_at),
    [t('admin.accounts.modeltraceHistory.colModel')]: row.model,
    [t('admin.accounts.modeltraceHistory.detailOutcome')]: row.outcome,
    [t('admin.accounts.modeltraceHistory.detailTrigger')]: triggerLabel(row),
    [t('admin.accounts.modeltraceHistory.colVerdict')]: verdictLabel(row),
    [t('admin.accounts.modeltraceHistory.colPredicted')]: row.fingerprint_predicted_model || '—',
    [t('admin.accounts.modeltraceHistory.colProbability')]: row.fingerprint_probability != null ? (row.fingerprint_probability * 100).toFixed(2) + '%' : '—',
    [t('admin.accounts.modeltraceHistory.detailMatched')]: row.fingerprint_matched == null ? '—' : String(row.fingerprint_matched),
    [t('admin.accounts.modeltraceHistory.detailExpectedCount')]: row.challenge_expected_count != null ? String(row.challenge_expected_count) : '—',
    [t('admin.accounts.modeltraceHistory.detailParsedCount')]: row.parsed_number_count != null ? String(row.parsed_number_count) : '—',
    [t('admin.accounts.modeltraceHistory.detailHttpStatus')]: row.http_status != null ? String(row.http_status) : '—',
    [t('admin.accounts.modeltraceHistory.detailDuration')]: row.duration_ms ? row.duration_ms + ' ms' : '—',
    [t('admin.accounts.modeltraceHistory.colProxy')]: row.proxy_name || '—',
    [t('admin.accounts.modeltraceHistory.detailReason')]: row.reason_code || '—',
    [t('admin.accounts.modeltraceHistory.detailVerification')]: row.verification_method || '—',
    [t('admin.accounts.modeltraceHistory.colBank')]: row.fingerprint_commit || '—'
  }
})

onMounted(() => {
  loadAccounts()
  loadFingerprint()
})
</script>
