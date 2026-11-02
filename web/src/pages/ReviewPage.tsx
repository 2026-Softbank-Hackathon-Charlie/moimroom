import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { createReview, type Review } from '../api'
import { StarInput } from '../components/StarRating'
import { errorMessage } from '../useAsync'

export default function ReviewPage() {
  const [params] = useSearchParams()
  const [code, setCode] = useState(params.get('code') ?? '')
  const [rating, setRating] = useState(5)
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [done, setDone] = useState<Review>()

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      setDone(await createReview({ code, rating, body }))
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (done) {
    return (
      <div className="container page narrow center">
        <h1 className="page__title">후기가 등록되었습니다</h1>
        <p className="muted">소중한 후기는 다른 분들이 공간을 고르는 데 큰 도움이 됩니다.</p>
        <Link to={`/spaces/${done.spaceId}`} className="btn btn--primary">
          공간 페이지에서 보기
        </Link>
      </div>
    )
  }

  const length = body.trim().length
  return (
    <div className="container page narrow">
      <h1 className="page__title">이용 후기 작성</h1>
      <form className="form" onSubmit={onSubmit}>
        <label className="field">
          <span>예약 코드</span>
          <input value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} required maxLength={12} />
        </label>
        <div className="field">
          <span>별점</span>
          <StarInput value={rating} onChange={setRating} />
        </div>
        <label className="field">
          <span>후기</span>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={6}
            maxLength={1000}
            placeholder="공간의 분위기, 청결 상태, 편의시설은 어땠나요?"
            required
          />
          <small className="hint">{length} / 1000자 (5자 이상)</small>
        </label>
        {error && (
          <div className="alert" role="alert">
            {error}
          </div>
        )}
        <button type="submit" className="btn btn--primary btn--block" disabled={busy || length < 5}>
          후기 등록
        </button>
      </form>
    </div>
  )
}
