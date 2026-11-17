import { describe, it, expect, vi, beforeEach } from 'vitest'
import { http } from '@/http'
import { fetchConsignments } from './service'

vi.mock('@/http', () => ({
  API_BASE_URL: 'http://localhost:8080',
  http: {
    request: vi.fn(),
  },
}))

describe('consignment service', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetchConsignments sends search and pagination params', async () => {
    const mockResponse = { data: { items: [], total: 0, page: 1, pageSize: 20 } }
    vi.mocked(http.request).mockResolvedValue(mockResponse)

    const result = await fetchConsignments({ q: 'tea', page: 1, pageSize: 20 })

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/consignments',
        method: 'GET',
        params: { q: 'tea', page: 1, pageSize: 20 },
        attachToken: true,
      }),
    )
    expect(result).toEqual(mockResponse.data)
  })
})
