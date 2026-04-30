import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, BillingNotConfigured, billingApi } from '@/lib/api'
import { useBillingEnabled } from '@/hooks/use-meta'

export interface ProRequestStatus {
  requested: boolean
  requested_at?: string
}

export interface CreateProRequestBody {
  email: string
  company: string
  message: string
  source: string
}

export interface CreateProRequestResponse {
  submitted: boolean
  requested_at: string
}

const STATUS_KEY = ['pro-request', 'me'] as const

export function useProRequestStatus() {
  const enabled = useBillingEnabled()
  return useQuery<ProRequestStatus, Error>({
    queryKey: STATUS_KEY,
    queryFn: () => billingApi.get<ProRequestStatus>('/pro-request/me'),
    enabled,
    // 401 from billing means JWT cookie not present yet — surface as not-requested
    // and don't retry. Same for BillingNotConfigured (shouldn't happen because
    // `enabled` gates this, but defensive).
    retry: (count, err) => {
      if (err instanceof BillingNotConfigured) return false
      if (err instanceof ApiError && err.status === 401) return false
      return count < 1
    },
    staleTime: 60_000,
  })
}

export function useCreateProRequest() {
  const qc = useQueryClient()
  return useMutation<CreateProRequestResponse, ApiError, CreateProRequestBody>({
    mutationFn: (body) => billingApi.post<CreateProRequestResponse>('/pro-request', body),
    onSuccess: (data) => {
      qc.setQueryData<ProRequestStatus>(STATUS_KEY, {
        requested: true,
        requested_at: data.requested_at,
      })
    },
    onError: (err) => {
      // 409 (already_requested) is a success-state for the UI: flip the cache
      // so the dialog renders the "thanks, we'll be in touch" branch.
      if (err.status === 409) {
        qc.setQueryData<ProRequestStatus>(STATUS_KEY, (prev) => prev ?? { requested: true })
      }
    },
  })
}
