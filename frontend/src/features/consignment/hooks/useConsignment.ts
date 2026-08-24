import { useEffect, useState } from 'react'
import { type ConsignmentSummary } from '../types'
import { fetchConsignment } from '../service'

export function useConsignment(consignmentId: string | undefined) {
  const [consignment, setConsignment] = useState<ConsignmentSummary | null>(null)
  const [fetchedId, setFetchedId] = useState<string | undefined>()

  useEffect(() => {
    if (!consignmentId) return

    const id = consignmentId
    const controller = new AbortController()

    void fetchConsignment(id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setConsignment(result)
          setFetchedId(id)
        }
      })
      .catch((error: unknown) => {
        if (error instanceof Error && error.name === 'AbortError') return
        console.error('Failed to fetch consignment:', error)
      })

    return () => controller.abort()
  }, [consignmentId])

  if (!consignmentId || fetchedId !== consignmentId) return null
  return consignment
}
