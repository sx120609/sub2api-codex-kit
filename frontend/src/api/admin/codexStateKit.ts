import { apiClient } from '../client'

export interface CodexStateKitLenCount {
  len: number
  count: number
}

export interface CodexStateKitPoolToken {
  len: number
  ageSecs: number
  isBound: boolean
  isValid: boolean
}

export interface CodexStateKitModelView {
  model: string
  status: string
  ageSecs: number | null
  len: number | null
  capturedAt: string | null
  distribution: CodexStateKitLenCount[]
  poolTokens: CodexStateKitPoolToken[]
  boundOverride: number | null
}

export interface CodexStateKitStatus {
  status: string
  ageSecs: number | null
  len: number | null
  source: string | null
  capturedAt: string | null
  models: CodexStateKitModelView[]
  boundTokenLen: number
  enabled: boolean
  accountId: number
  chatgptAccountId?: string
  globalEnabled: boolean
}

export interface CodexStateKitUpdate {
  enabled?: boolean
  bound_token_len?: number | null
  clear_bound_token_len?: boolean
  models?: string[]
}

export async function getCodexStateKit(accountId: number): Promise<CodexStateKitStatus> {
  const { data } = await apiClient.get<CodexStateKitStatus>(
    `/admin/openai/accounts/${accountId}/codex-state-kit`
  )
  return data
}

export async function refreshCodexStateKit(accountId: number): Promise<CodexStateKitStatus> {
  const { data } = await apiClient.post<CodexStateKitStatus>(
    `/admin/openai/accounts/${accountId}/codex-state-kit/refresh`
  )
  return data
}

export async function updateCodexStateKit(
  accountId: number,
  patch: CodexStateKitUpdate
): Promise<CodexStateKitStatus> {
  const { data } = await apiClient.put<CodexStateKitStatus>(
    `/admin/openai/accounts/${accountId}/codex-state-kit`,
    patch
  )
  return data
}

export interface CodexStateKitIngestResult extends CodexStateKitStatus {
  accepted: number
  skipped: number
  degraded: number
}

export async function ingestCodexStateKit(
  accountId: number,
  payload: {
    model?: string
    tokens: string[]
    chatgpt_account_id?: string
    source?: string
    accept_degraded?: boolean
    batches?: { model: string; tokens: string[] }[]
  }
): Promise<CodexStateKitIngestResult> {
  const { data } = await apiClient.post<CodexStateKitIngestResult>(
    `/admin/openai/accounts/${accountId}/codex-state-kit/tokens`,
    payload
  )
  return data
}
