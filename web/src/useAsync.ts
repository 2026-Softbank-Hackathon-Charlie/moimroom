import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from './api'

interface AsyncState<T> {
  data: T | undefined
  error: string | undefined
  loading: boolean
  reload: () => void
}

/** Runs fn whenever deps change; results of superseded runs are dropped. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): AsyncState<T> {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)
  const run = useRef(0)

  useEffect(() => {
    const id = ++run.current
    setLoading(true)
    setError(undefined)
    fn().then(
      (v) => {
        if (id !== run.current) return
        setData(v)
        setLoading(false)
      },
      (e: unknown) => {
        if (id !== run.current) return
        setError(e instanceof ApiError ? e.message : '일시적인 오류가 발생했습니다.')
        setLoading(false)
      },
    )
    // fn is recreated every render; deps decide when to rerun.
  }, [...deps, tick])

  const reload = useCallback(() => setTick((t) => t + 1), [])
  return { data, error, loading, reload }
}

export function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : '일시적인 오류가 발생했습니다.'
}
