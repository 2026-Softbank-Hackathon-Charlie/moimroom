# 0002. 실제 DB·버킷 자격증명 사용을 읽기 전용 프로브로 제공한다

- 상태: 채택
- 날짜: 2026-11-07

## 상황

환경 동등성 검증에서 Secret 존재와 실제 사용을 구분해야 한다. 준비 상태나 오래된 DB
풀 연결만으로 자격증명을 현재 사용할 수 있는지 확인할 수 없다.

## 결정

GET /internal/secret-usage는 64자리 hex challenge를 받고 새 DB 연결에서 SELECT로
동일 nonce를 읽는다. 동일한 runtime DB 설정을 쓰며 연결을 재사용하지 않는다.
앱의 S3 client로 서명된 BucketExists를 호출한다. 명시적 키가 없으면 unknown이다.
응답은 version, challenge, 작업별 pass/fail/unknown만 포함하며 원본 오류를 노출하지 않는다.
no-store, 3초 상한, 동시 실행 1개로 부하를 제한한다. 일반 GET readiness처럼 공개될 수 있는
읽기 전용 경로이며 서비스 proxy의 접근 제어를 앱 인증으로 오인하지 않는다.

## 검토한 대안

- readyz만 사용: DB 연결 재사용과 구분되지 않고 dependency별 결과가 없다.
- 운영 데이터를 쓰는 시험: 이번 검사 범위를 넘어 별도의 승인·정리 절차가 필요하다.

## 결과

신규 DB 인증/질의와 버킷 접근만 검사한다. Secret 모든 키의 사용, 비밀번호 강제 인증 설정,
S3 객체 읽기·쓰기 권한, IAM 최소 권한을 증명하지 않는다. raw provider 오류나 Secret 값은
응답에 없다. HyperDX는 기존 request 로그로 이 경로의 호출을 볼 수 있지만 작업별 결과의
저장 위치는 플랫폼 parity 감사 기록이다. 임시 PostgreSQL·MinIO에서 정상 자격증명 통과와
잘못된 DB 비밀번호·S3 키 실패를 확인했고 운영에는 적용하지 않았다.
