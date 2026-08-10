import { renderHook } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { useSignOutHandler } from './useSignOutHandler'
import { useAuth } from 'react-oidc-context'

vi.mock('react-oidc-context', () => ({
  useAuth: vi.fn(),
}))

describe('useSignOutHandler', () => {
  it('calls signoutRedirect when returned handler is executed', () => {
    const signoutRedirectMock = vi.fn().mockResolvedValue(undefined)
    vi.mocked(useAuth).mockReturnValue({
      signoutRedirect: signoutRedirectMock,
    } as never)

    const { result } = renderHook(() => useSignOutHandler())
    result.current()

    expect(signoutRedirectMock).toHaveBeenCalledTimes(1)
  })
})
