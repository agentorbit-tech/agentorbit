import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export interface Meta {
  version: string
  billing_url?: string
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
