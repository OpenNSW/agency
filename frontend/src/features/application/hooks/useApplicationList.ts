import { useCallback, useEffect, useState } from 'react'
import { type AgencyApplication } from '../types'
import { fetchApplications } from '../service'

const PAGE_SIZE = 20

export function useApplicationList(consignmentId: string | undefined) {
  const [applications, setApplications] = useState<AgencyApplication[]>([])
  const [loading, setLoading] = useState(!!consignmentId)
  const [error, setError] = useState<Error | null>(null)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [refetchIndex, setRefetchIndex] = useState(0)

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  useEffect(() => {
    if (!consignmentId) return

    const id = consignmentId
    const controller = new AbortController()
    async function fetchData() {
      try {
        setLoading(true)
        setError(null)
        const result = await fetchApplications({ consignmentId: id, page, pageSize: PAGE_SIZE }, controller.signal)
        setApplications(result.items)
        setTotal(result.total)
      } catch (err) {
        if (err instanceof Error && err.name === 'AbortError') return
        setError(err instanceof Error ? err : new Error('Failed to fetch tasks'))
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }

    void fetchData()
    return () => controller.abort()
  }, [consignmentId, page, refetchIndex])

  const refetch = useCallback(() => setRefetchIndex((i) => i + 1), [])

  return {
    data: applications,
    status: { loading, error },
    pagination: { page, setPage, total, totalPages },
    refetch,
  }
}
