import { describe, it, expect, vi, beforeEach } from 'vitest'
import { http } from '@/http'
import { fetchApplications, fetchApplicationDetail, submitReview, submitFeedback, getDownloadUrl } from './service'

vi.mock('@/http', () => ({
  API_BASE_URL: 'http://localhost:8080',
  http: {
    request: vi.fn(),
  },
}))

describe('application service', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetchApplications queries API with formatted parameters', async () => {
    const mockResponse = { data: { items: [], total: 0, page: 1, pageSize: 20 } }
    vi.mocked(http.request).mockResolvedValue(mockResponse)

    const result = await fetchApplications({ consignmentId: 'C123', page: 1, pageSize: 20 })

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/applications',
        method: 'GET',
        params: { consignmentId: 'C123', page: 1, pageSize: 20 },
        attachToken: true,
      }),
    )
    expect(result).toEqual(mockResponse.data)
  })

  it('fetchApplicationDetail queries specific task id endpoint', async () => {
    const mockApp = { taskId: 'T-100', title: 'Inspection Application' }
    vi.mocked(http.request).mockResolvedValue({ data: mockApp })

    const result = await fetchApplicationDetail('T-100')

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/applications/T-100',
        method: 'GET',
        attachToken: true,
      }),
    )
    expect(result).toEqual(mockApp)
  })

  it('submitReview sends POST request to review endpoint', async () => {
    const mockResult = { status: 'APPROVED' }
    vi.mocked(http.request).mockResolvedValue({ data: mockResult })

    const formValues = { decision: 'APPROVED', remarks: 'Looks good' }
    const result = await submitReview('T-100', formValues)

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/applications/T-100/review',
        method: 'POST',
        data: formValues,
        attachToken: true,
      }),
    )
    expect(result).toEqual(mockResult)
  })

  it('submitFeedback sends POST request to feedback endpoint', async () => {
    const mockResult = { status: 'FEEDBACK_REQUESTED' }
    vi.mocked(http.request).mockResolvedValue({ data: mockResult })

    const content = { comment: 'Please clarify declaration' }
    const result = await submitFeedback('T-100', content)

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/applications/T-100/feedback',
        method: 'POST',
        data: content,
        attachToken: true,
      }),
    )
    expect(result).toEqual(mockResult)
  })

  it('getDownloadUrl fetches download URL metadata', async () => {
    const mockMetadata = { download_url: 'http://localhost:8080/downloads/file.pdf', expires_at: 1700000000 }
    vi.mocked(http.request).mockResolvedValue({ data: mockMetadata })

    const result = await getDownloadUrl('file-key-123')

    expect(http.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'http://localhost:8080/api/v1/storage/file-key-123',
        method: 'GET',
        attachToken: true,
      }),
    )
    expect(result).toEqual({ url: 'http://localhost:8080/downloads/file.pdf', expiresAt: 1700000000 })
  })
})
