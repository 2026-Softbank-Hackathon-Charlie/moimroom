# platform #91 로컬 검증

PostgreSQL 17 격리 컨테이너에서 실제 앱 API 전체 race 테스트 통과. 앱 전용 LOGIN principal로 읽기/쓰기 성공과 pg_authid 접근 거부 확인. commit 후 실제 TCP 연결 응답 유실, 동일 키 동시 요청 8개, 다른 본문 409, 실패 트랜잭션의 영수증 rollback, 개인정보 비노출 검사 통과.

실제 pg_dump -Fc / pg_restore 뒤 앱으로 같은 키를 재시도해 독립 oracle의 원래 예약 코드와 일치하고 예약·영수증 각각 1행 유지. 복원+앱 확인은 3초. 백업 후 추가 fixture 쓰기는 0건이며 일반 운영 RPO/RTO 수치가 아니다. 두 DB와 백업은 같은 disposable 로컬 인스턴스 안에 유지했으며 외부로 반출하지 않았다. cross-environment restore residency 정책 검증은 아니다.

runner의 /work는 moimroom, /evidence는 증거 디렉터리, /tools/docker와 Docker socket은 로컬 테스트용이다. sandbox는 hop-issue91-postgres / hop-issue91 네트워크를 사용했고 완료 후 제거했다. 운영 변경·알림 발송은 없다. gateway hold/실제 이전·역방향 롤백을 통한 재시도와 외부 부작용은 이 테스트의 보장 범위가 아니다. workspace migration-engine/docs/test-records/2026-11-06-platform-91에도 결과를 보존했다.
