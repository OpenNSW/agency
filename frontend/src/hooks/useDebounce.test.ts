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

  it('returns initial value immediately', () => {
    const { result } = renderHook(() => useDebounce('hello', 400))
    expect(result.current).toBe('hello')
  })

  it('updates debounced value after specified delay', () => {
    const { result, rerender } = renderHook(({ val }) => useDebounce(val, 400), {
      initialProps: { val: 'hello' },
    })

    expect(result.current).toBe('hello')

    rerender({ val: 'world' })
    // Before delay, still initial value
    expect(result.current).toBe('hello')

    act(() => {
      vi.advanceTimersByTime(400)
    })

    expect(result.current).toBe('world')
  })
})
