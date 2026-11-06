import { useEffect, useRef, useState } from 'react'
import { ApiError, generateSpaceBrief, type AIBrief } from '../api'
import { errorMessage } from '../useAsync'
import { ErrorState, Loading } from './Layout'

export default function AIUsageGuide({ spaceId }: { spaceId: number }) {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<AIBrief>()
  const [error, setError] = useState<{ message: string; requestId?: string }>()
  const pending = useRef<AbortController | null>(null)

  useEffect(() => {
    setLoading(false)
    setResult(undefined)
    setError(undefined)
    return () => {
      pending.current?.abort()
      pending.current = null
    }
  }, [spaceId])

  async function generate() {
    if (pending.current) return
    const controller = new AbortController()
    pending.current = controller
    setLoading(true)
    setResult(undefined)
    setError(undefined)
    try {
      const next = await generateSpaceBrief(spaceId, controller.signal)
      if (!controller.signal.aborted) setResult(next)
    } catch (e: unknown) {
      if (!controller.signal.aborted) setError({
        message: errorMessage(e),
        requestId: e instanceof ApiError ? e.requestId : undefined,
      })
    } finally {
      if (pending.current === controller) {
        pending.current = null
        setLoading(false)
      }
    }
  }

  return (
    <section className="detail-section" aria-labelledby="ai-guide-title" aria-busy={loading}>
      <h2 id="ai-guide-title">AI 이용 안내</h2>
      <p className="muted">공개 공간 소개·편의시설·이용 규칙을 바탕으로 만든 안내입니다. 예약 전 원문을 확인해 주세요.</p>
      {!error && (
        <button type="button" className="btn btn--ghost" disabled={loading} onClick={generate}>
          {loading ? 'AI 안내 생성 중' : result ? 'AI 안내 다시 만들기' : 'AI 이용 안내 만들기'}
        </button>
      )}
      {loading && <Loading label="AI 이용 안내를 만들고 있습니다" />}
      {error && (
        <>
          <ErrorState message={error.message} onRetry={generate} />
          {error.requestId && <p className="muted">요청 ID: {error.requestId}</p>}
        </>
      )}
      {result && (
        <div role="status" aria-live="polite">
          <p className="prewrap">{result.text}</p>
          <dl className="facts">
            <div><dt>이 요청의 처리 위치</dt><dd>{result.backend === 'gcp' ? 'GCP' : '노트북'}</dd></div>
            <div><dt>요청 ID</dt><dd>{result.requestId}</dd></div>
            <div><dt>서버 처리 시간</dt><dd>{result.durationMs}ms</dd></div>
          </dl>
        </div>
      )}
    </section>
  )
}
