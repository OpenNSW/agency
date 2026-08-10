/* eslint-disable @typescript-eslint/no-unsafe-assignment */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { http } from './http'
import { userManager } from '@/features/user/oidcUserManager'

vi.mock('./runtimeConfig', () => ({
  getRequiredEnv: () => 'http://localhost:8080',
}))

vi.mock('@/features/user/oidcUserManager', () => ({
  userManager: {
    getUser: vi.fn(),
  },
}))

describe('http client', () => {
  const mockedFetch = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', mockedFetch)
  })

  it('attaches authorization header when attachToken is true', async () => {
    vi.spyOn(userManager, 'getUser').mockResolvedValue({
      access_token: 'test-bearer-token',
    } as never)

    const mockResponse = {
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: (): Promise<unknown> => Promise.resolve({ status: 'ok' }),
    }
    mockedFetch.mockResolvedValue(mockResponse)

    const res = await http.request({
      url: 'http://localhost:8080/api/v1/test',
      attachToken: true,
    })

    expect(mockedFetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/test',
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer test-bearer-token',
        }),
      }),
    )
    expect(res).toEqual({ data: { status: 'ok' } })
  })

  it('formats query string parameters correctly', async () => {
    const mockResponse = {
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: (): Promise<unknown> => Promise.resolve({ items: [] }),
    }
    mockedFetch.mockResolvedValue(mockResponse)

    await http.request({
      url: 'http://localhost:8080/api/v1/search',
      params: { q: 'tea', page: 1, empty: undefined, nullVal: null },
    })

    expect(mockedFetch).toHaveBeenCalledWith('http://localhost:8080/api/v1/search?q=tea&page=1', expect.anything())
  })

  it('throws error when response is not ok', async () => {
    const mockResponse = {
      ok: false,
      status: 404,
    }
    mockedFetch.mockResolvedValue(mockResponse)

    await expect(
      http.request({
        url: 'http://localhost:8080/api/v1/notfound',
      }),
    ).rejects.toThrow('HTTP error! status: 404')
  })
})
