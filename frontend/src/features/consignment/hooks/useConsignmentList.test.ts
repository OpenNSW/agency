import { renderHook, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useConsignmentList } from './useConsignmentList'
import * as consignmentService from '../service'

vi.mock('../service', () => ({
  fetchConsignments: vi.fn(),
}))

describe('useConsignmentList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads consignments on mount', async () => {
    const mockItems = [{ id: 'C1', consignmentNumber: 'CN-100', status: 'SUBMITTED' }]
    vi.mocked(consignmentService.fetchConsignments).mockResolvedValue({
      items: mockItems as never,
      total: 1,
      page: 1,
      pageSize: 20,
    })

    const { result } = renderHook(() => useConsignmentList(''))

    expect(result.current.status.loading).toBe(true)

    await waitFor(() => {
      expect(result.current.status.loading).toBe(false)
    })

    expect(result.current.data).toEqual(mockItems)
    expect(result.current.pagination.total).toBe(1)
    expect(result.current.pagination.totalPages).toBe(1)
  })
})
