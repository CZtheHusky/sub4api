import { apiClient } from '../client'

export interface TicketAttempt {
  id: number
  account_id: number
  model: string
  occurred_at: string
  outcome: string
  trigger?: string
  http_status?: number | null
  ticket_length?: number | null
  duration_ms?: number
  reason_code?: string
  proxy_id?: number
  proxy_name?: string
  expires_at?: string | null
  verification_method?: string
  fingerprint_commit?: string
  fingerprint_predicted_model?: string
  fingerprint_probability?: number | null
  fingerprint_matched?: boolean | null
  challenge_expected_count?: number | null
  parsed_number_count?: number | null
}

export interface TicketDiagnostic {
  model: string
  status: 'normal' | 'degraded' | 'uncertain' | 'failed'
  reason?: string
  predicted_model?: string
  probability?: number
  parsed_number_count?: number
  http_status?: number
  supported_efforts?: string[]
  duration_ms?: number
}

const base = (id: number) => `/admin/accounts/${id}`

export async function fingerprint(): Promise<{ commit: string; models: string[] }> {
  const { data } = await apiClient.get('/admin/accounts/codex-ticket-fingerprint')
  return data
}

export async function refreshFingerprint(): Promise<{ commit: string; models: string[] }> {
  const { data } = await apiClient.post('/admin/accounts/codex-ticket-fingerprint/refresh')
  return data
}

export async function diagnose(id: number, models: string[], signal?: AbortSignal): Promise<{ items: TicketDiagnostic[]; canceled: boolean }> {
  const { data } = await apiClient.post(`${base(id)}/codex-ticket-diagnostic`, { models }, { timeout: Math.max(120000, models.length * 180000), signal })
  return data
}

// 400 effort 快速探针：minimal 档校验层 400，报错档位列表含 none 即降智，零 token 消耗
export async function effortProbe(id: number, models: string[], signal?: AbortSignal): Promise<{ items: TicketDiagnostic[]; canceled: boolean }> {
  const { data } = await apiClient.post(`${base(id)}/codex-effort-probe`, { models }, { timeout: Math.max(60000, models.length * 30000), signal })
  return data
}

export async function history(id: number, params: { model?: string; filter?: string; page: number; page_size: number }): Promise<{ items: TicketAttempt[]; total: number; page: number; page_size: number }> {
  const { data } = await apiClient.get(`${base(id)}/codex-ticket-history`, { params })
  return data
}

export async function deleteHistory(id: number, params: { model?: string; filter?: string }): Promise<{ deleted: number }> {
  const { data } = await apiClient.delete(`${base(id)}/codex-ticket-history`, { params })
  return data
}
