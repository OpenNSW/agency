import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
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

function jsonOk(body: unknown) {
  return {
    ok: true,
    status: 200,
    headers: new Headers({ 'content-type': 'application/json' }),
    json: (): Promise<unknown> => Promise.resolve(body),
  }
}

describe('http client', () => {
  const mockedFetch = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', mockedFetch)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('attaches authorization header when attachToken is true', async () => {
    vi.mocked(userManager).getUser.mockResolvedValue({
      access_token: 'test-bearer-token',
    } as never)
    mockedFetch.mockResolvedValue(jsonOk({ status: 'ok' }))

    const res = await http.request({
      url: 'http://localhost:8080/api/v1/test',
      attachToken: true,
    })

    expect(mockedFetch).toHaveBeenCalledTimes(1)
    expect(mockedFetch.mock.calls[0]?.[0]).toBe('http://localhost:8080/api/v1/test')
    expect(mockedFetch.mock.calls[0]?.[1]).toMatchObject({
      headers: { Authorization: 'Bearer test-bearer-token' },
    })
    expect(res).toEqual({ data: { status: 'ok' } })
  })

  it('omits null and undefined query parameters', async () => {
    mockedFetch.mockResolvedValue(jsonOk({ items: [] }))

    await http.request({
      url: 'http://localhost:8080/api/v1/search',
      params: { q: 'tea', page: 1, empty: undefined, nullVal: null },
    })

    expect(mockedFetch.mock.calls[0]?.[0]).toBe('http://localhost:8080/api/v1/search?q=tea&page=1')
  })

  it('throws when the response is not ok', async () => {
    mockedFetch.mockResolvedValue({ ok: false, status: 404 })

    await expect(
      http.request({
        url: 'http://localhost:8080/api/v1/notfound',
      }),
    ).rejects.toThrow('HTTP error! status: 404')
  })
})
