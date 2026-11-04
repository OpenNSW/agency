import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useDebounce } from './useDebounce'

describe('useDebounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps the previous value until the delay elapses', () => {
    const { result, rerender } = renderHook(({ val }) => useDebounce(val, 400), {
      initialProps: { val: 'hello' },
    })

    expect(result.current).toBe('hello')

    rerender({ val: 'world' })
    expect(result.current).toBe('hello')

    act(() => {
      vi.advanceTimersByTime(400)
    })

    expect(result.current).toBe('world')
  })
})
