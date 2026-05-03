import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { StatsResult, DailyStatsRow, AgentStatsRow, FinishReasonCount } from '@/types/api'
import type { DateRange } from '@/lib/date'

function toDateKey(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function rangeToDays(range: DateRange): number {
  switch (range) {
    case '24h': return 1
    case '7d': return 7
    case '30d': return 30
    default: return 30
  }
}

export function useStats(orgID: string, from: Date, to: Date, opts?: { pollWhileEmpty?: boolean }) {
  return useQuery({
    queryKey: ['stats', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<StatsResult>(
        `/api/orgs/${orgID}/stats?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
    // While the dashboard shows the empty state, the user may have just
    // started sending traffic. WebSocket invalidation handles the happy path,
    // but if the socket disconnects (closed laptop, flaky proxy, etc.) the
    // empty state would stick. A slow poll fills that gap.
    refetchInterval: (query) => {
      if (!opts?.pollWhileEmpty) return false
      const data = query.state.data as StatsResult | undefined
      const hasActivity = !!data && (data.total_sessions > 0 || data.total_spans > 0)
      return hasActivity ? false : 15000
    },
  })
}

export function useDailyStats(orgID: string, range: DateRange) {
  const days = rangeToDays(range)
  return useQuery({
    queryKey: ['dailyStats', orgID, range],
    queryFn: () =>
      api.get<DailyStatsRow[]>(
        `/api/orgs/${orgID}/stats/daily?days=${days}`
      ),
    enabled: !!orgID,
  })
}

export function useAgentStats(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['agentStats', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<AgentStatsRow[]>(
        `/api/orgs/${orgID}/stats/agents?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}

export function useFinishReasons(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['finishReasons', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<FinishReasonCount[]>(
        `/api/orgs/${orgID}/stats/finish-reasons?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}
