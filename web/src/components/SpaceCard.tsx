import { Link } from 'react-router'
import { photoUrl, type Space } from '../api'
import { CATEGORY_LABEL, won } from '../format'
import PhotoPlaceholder from './PhotoPlaceholder'

interface Props {
  space: Space
  /** Extra query string carried to the detail page, e.g. the searched date. */
  search?: string
}

export default function SpaceCard({ space, search = '' }: Props) {
  return (
    <Link to={`/spaces/${space.id}${search}`} className="space-card">
      <div className="space-card__photo">
        {space.coverPhotoId ? (
          <img src={photoUrl(space.id, space.coverPhotoId)} alt={space.name} loading="lazy" />
        ) : (
          <PhotoPlaceholder category={space.category} label={space.name} />
        )}
        <span className="badge badge--overlay">{CATEGORY_LABEL[space.category]}</span>
      </div>
      <div className="space-card__body">
        <p className="space-card__meta">
          {space.area} <span aria-hidden="true">|</span> 최대 {space.capacity}인
        </p>
        <h3 className="space-card__name">{space.name}</h3>
        <p className="space-card__summary">{space.summary}</p>
        <div className="space-card__footer">
          <strong>
            {won(space.pricePerHour)}
            <small> / 시간</small>
          </strong>
          {space.reviewCount > 0 ? (
            <span className="rating" aria-label={`평점 ${space.rating.toFixed(1)}점, 후기 ${space.reviewCount}개`}>
              ★ {space.rating.toFixed(1)} <small>({space.reviewCount})</small>
            </span>
          ) : (
            <span className="tag-new">새로 등록</span>
          )}
        </div>
      </div>
    </Link>
  )
}
