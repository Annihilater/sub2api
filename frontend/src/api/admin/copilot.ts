/**
 * Admin Copilot API endpoints
 * Handles GitHub Copilot Device OAuth flows for administrators
 */

import { apiClient } from '../client'

export interface CopilotDeviceCodeResponse {
  session_id: string
  user_code: string
  verification_uri: string
  expires_in: number
  interval: number
}

export interface CopilotPollResponse {
  status: 'pending' | 'slow_down' | 'complete'
  message?: string
  github_token?: string
  github_login?: string
  github_name?: string
  github_id?: number
}

export async function startDeviceFlow(): Promise<CopilotDeviceCodeResponse> {
  const { data } = await apiClient.post<CopilotDeviceCodeResponse>(
    '/admin/copilot/oauth/device-code'
  )
  return data
}

export async function pollDeviceFlow(sessionId: string): Promise<CopilotPollResponse> {
  const { data } = await apiClient.post<CopilotPollResponse>(
    '/admin/copilot/oauth/poll',
    { session_id: sessionId }
  )
  return data
}

export default { startDeviceFlow, pollDeviceFlow }
