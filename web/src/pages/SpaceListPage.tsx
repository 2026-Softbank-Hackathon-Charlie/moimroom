import { useSearchParams } from 'react-router'
import { listSpaces } from '../api'
import SpaceCard from '../components/SpaceCard'
import { ErrorState, Loading } from '../components/Layout'
import { AREAS, CATEGORY_LABEL, won } from '../format'
import { useAsync } from '../useAsync'

const SORTS = [
  ['recommended', '추천순'],
  ['rating', '평점 높은순'],
  ['price_asc', '낮은 가격순'],
  ['price_desc', '높은 가격순'],
  ['capacity', '넓은 공간순'],
] as const

const PRICE_CAPS = [10000, 20000, 30000, 50000]

export default function SpaceListPage() {
  const [params, setParams] = useSearchParams()
  const area = params.get('area') ?? ''
  const category = params.get('category') ?? ''
  const headcount = Number(params.get('headcount')) || 0
  const maxPrice = Number(params.get('maxPrice')) || 0
  const sort = params.get('sort') ?? 'recommended'

  const spaces = useAsync(
    () => listSpaces({ area, category, minCapacity: headcount, maxPrice, sort, limit: 100 }),
    [area, category, headcount, maxPrice, sort],
  )

  function update(key: string, value: string) {
    const next = new URLSearchParams(params)
    if (value) next.set(key, value)
    else next.delete(key)
    setParams(next, { replace: true })
  }

  // Carry the searched date and headcount into the detail page.
  const carry = new URLSearchParams()
  if (params.get('date')) carry.set('date', params.get('date')!)
  if (headcount) carry.set('headcount', String(headcount))
  const search = carry.size ? `?${carry}` : ''

  const title = [area || '서울 전체', category ? CATEGORY_LABEL[category as keyof typeof CATEGORY_LABEL] : '공간']
    .filter(Boolean)
    .join(' ')

  return (
    <div className="container page">
      <h1 className="page__title">{title}</h1>
      <div className="filters" role="search">
        <label>
          <span>지역</span>
          <select value={area} onChange={(e) => update('area', e.target.value)}>
            <option value="">전체</option>
            {AREAS.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>유형</span>
          <select value={category} onChange={(e) => update('category', e.target.value)}>
            <option value="">전체</option>
            {Object.entries(CATEGORY_LABEL).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>인원</span>
          <input
            type="number"
            min={1}
            max={50}
            placeholder="제한 없음"
            value={headcount || ''}
            onChange={(e) => update('headcount', e.target.value === '' ? '' : String(Math.max(1, Number(e.target.value))))}
          />
        </label>
        <label>
          <span>시간당 요금</span>
          <select value={maxPrice || ''} onChange={(e) => update('maxPrice', e.target.value)}>
            <option value="">제한 없음</option>
            {PRICE_CAPS.map((p) => (
              <option key={p} value={p}>
                {won(p)} 이하
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>정렬</span>
          <select value={sort} onChange={(e) => update('sort', e.target.value === 'recommended' ? '' : e.target.value)}>
            {SORTS.map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </label>
      </div>

      {spaces.loading && !spaces.data && <Loading />}
      {spaces.error && <ErrorState message={spaces.error} onRetry={spaces.reload} />}
      {spaces.data && !spaces.error && (
        <>
          <p className="result-count">공간 {spaces.data.length}곳</p>
          {spaces.data.length === 0 ? (
            <div className="state">
              <p>조건에 맞는 공간이 없습니다.</p>
              <button type="button" className="btn btn--ghost" onClick={() => setParams({}, { replace: true })}>
                필터 초기화
              </button>
            </div>
          ) : (
            <div className="card-grid">
              {spaces.data.map((s) => (
                <SpaceCard key={s.id} space={s} search={search} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
