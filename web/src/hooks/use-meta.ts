import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export interface Meta {
  version: string
  billing_url?: string
  proxy_url?: string
}

const META_QUERY_KEY = ['meta'] as const

export function useMeta() {
  return useQuery({
    queryKey: META_QUERY_KEY,
    queryFn: () => api.get<Meta>('/meta'),
    staleTime: Infinity,
    retry: false,
  })
}

export function useBillingEnabled(): boolean {
  const { data } = useMeta()
  return Boolean(data?.billing_url)
}

// Public base URL agents must point their `base_url` at, including the `/v1`
// suffix. In cloud the backend exposes proxy_url via /api/meta (e.g.
// "https://api.agentorbit.tech"). In self-host setups where the UI and proxy
// share an origin, fall back to window.location.origin.
export function useProxyBaseUrl(): string {
  const { data } = useMeta()
  const root = data?.proxy_url?.replace(/\/+$/, '') || window.location.origin
  return `${root}/v1`
}
