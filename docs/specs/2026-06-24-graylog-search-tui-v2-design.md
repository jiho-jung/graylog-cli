# graylog-cli Search TUI v2 Design

## 요약

`graylog-cli search --tui`를 `graylog_view_tui.jpg`의 구조에 맞춰 새로 설계한다. 기존 TUI 설계 문서는 유지하고, 이 문서는 Header, Body 테이블, 명령 프롬프트, Status/Help footer를 가진 v2 설계로 다룬다.

v2의 목표는 Graylog 검색 결과를 터미널 안에서 테이블 형태로 빠르게 탐색하고, 검색 조건과 표시 조건을 즉시 바꾸며, 선택한 로그는 별도 상세 화면에서 JSON tree, pretty, raw 형태로 확인할 수 있게 하는 것이다.

## 화면 구조

기본 화면은 네 영역으로 나눈다.

- Header: 현재 검색의 상세 메타 정보를 표시한다.
- Body: 로그 결과를 가변 컬럼 테이블로 표시한다.
- Command prompt: 단축키로 열린 입력 프롬프트를 표시한다.
- Status/Help footer: 상태 정보와 도움말을 토글해 표시한다.

기본 화면은 테이블 중심이다. 로그 상세는 같은 화면의 하단 패널이 아니라 `Enter`로 진입하는 별도 상세 화면에서 보여준다.

## 텍스트 와이어프레임

기본 목록 화면은 다음 형태를 목표로 한다.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ graylog-cli search --tui | view: logs                                        │
│ range: last 15m | query: level:3 AND service:api | page: 2/18 | total: 342   │
│ offset: 20 | limit: 20 | sort: timestamp:DESC | app: api | local filter: on  │
├──────────────────────┬────────┬────────────────────┬────────────────────────┤
│ timestamp            │ level  │ source             │ message                │
├──────────────────────┼────────┼────────────────────┼────────────────────────┤
│ 2026-06-24 14:01:22  │ ERROR  │ api-1              │ failed to call backend  │
│ 2026-06-24 14:01:18  │ WARN   │ api-2              │ retrying request        │
│ 2026-06-24 14:01:12  │ INFO   │ worker-3           │ job completed           │
│▶2026-06-24 14:01:03  │ ERROR  │ api-1              │ timeout while fetching  │
│ 2026-06-24 14:00:58  │ INFO   │ api-4              │ request accepted        │
│ 2026-06-24 14:00:44  │ DEBUG  │ worker-1           │ cache hit               │
│                                                                              │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│ / query: level:3 AND service:api                                             │
├──────────────────────────────────────────────────────────────────────────────┤
│ STATUS page 2/18 | row 4/20 | cached pages 3 | refreshed 14:01:24 | r retry │
└──────────────────────────────────────────────────────────────────────────────┘
```

상세 화면은 별도 전환 화면으로 표시한다.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ graylog-cli search --tui | view: detail | tab: JSON Tree                    │
├──────────────────────────────────────────────────────────────────────────────┤
│ timestamp: 2026-06-24T14:01:03.120Z                                         │
│ level: ERROR                                                                 │
│ source: api-1                                                                │
│ message: timeout while fetching backend response                             │
│ request_id: abc-123                                                          │
│ user_id: 42                                                                  │
│ application: api                                                             │
│ part: checkout                                                               │
│ duration_ms: 30000                                                           │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│ Enter/Esc back | Tab next | / search | n/N match | g/G top/bottom           │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Header

Header에는 현재 TUI 상태를 이해하는 데 필요한 상세 메타 정보를 표시한다.

- View: 현재 화면. `logs`, `histogram`, `detail` 중 하나
- Date range: `--since` 또는 `--from/--to` 기준의 검색 범위
- Query: 현재 Graylog query
- Page: logs view의 현재 페이지와 전체 페이지
- Offset/limit: logs view의 현재 offset과 page limit
- Sort: 현재 서버 정렬 기준
- Filters: `application`, `part`, 로컬 필터 적용 여부
- Refresh: 마지막 조회 시각 또는 follow refresh 상태

Histogram view와 detail view에서도 Header는 같은 위치에 표시한다. 다만 page와 offset은 logs view의 현재 위치를 의미하며, Histogram view에서는 추가로 선택 bucket과 bucket 수를 표시한다.

Header는 편집 가능한 영역이 아니다. 검색 조건 변경은 단축키로 여는 프롬프트를 통해 수행한다.

## Body Table

Body는 가변 컬럼 테이블로 구성한다. 기본 컬럼은 다음 순서를 따른다.

1. `timestamp`
2. `level`
3. `source`
4. `message`
5. `--fields`로 지정한 필드 또는 표시 대상으로 선택된 추가 필드

컬럼 폭은 `timestamp`, `level`, `source`처럼 길이가 예측 가능한 컬럼은 고정 폭을 사용하고, `message`와 추가 필드는 남은 폭을 사용한다. 터미널 폭이 부족할 때는 오른쪽 컬럼부터 줄이고, 셀 내용은 clipping한다.

목록 화면의 모든 행은 항상 1줄로 표시한다. 선택된 행도 목록 안에서는 멀티라인으로 확장하지 않는다. 긴 `message`와 JSON 문자열은 원문 앞부분을 `message` 컬럼 폭에 맞게 clipping해서 보여주며, 전체 내용은 상세 화면에서 확인한다.

선택된 로그는 특정 셀만이 아니라 행 전체를 선택 상태로 표시한다. 하이라이트는 `timestamp`, `level`, `source`, `message`, 추가 필드를 포함한 전체 라인 폭에 적용한다. 선택 커서(`▶`)는 행의 시작에 표시하고, 사용자가 현재 선택한 로그를 테이블 전체 너비에서 바로 인지할 수 있게 한다.

테이블 정렬은 TUI 안에서 컬럼과 방향을 선택해 변경한다. 정렬 변경은 서버 재조회가 필요한 검색 옵션 변경으로 취급한다. v2의 sort는 단일 필드만 지원한다. 새 sort를 적용하면 기존 sort는 교체된다.

## Keymap

Command prompt 영역은 항상 입력 가능한 명령줄이 아니다. 사용자가 단축키를 누르면 해당 목적의 전용 프롬프트가 열린다.

Logs view의 기본 키맵은 다음과 같다.

- `/`: query 변경 프롬프트
- `f`: 로컬 필터 프롬프트
- `s`: 정렬 프롬프트
- `g`: 페이지 이동 프롬프트
- `c`: 컬럼 설정 프롬프트
- `Enter`: 선택 로그 상세 화면 열기
- `Tab`: 상세 화면 탭 전환 또는 footer 모드 전환
- `r`: 현재 조건으로 새로고침
- `G`: 원본 로그 보기와 유사 로그 그룹 보기 전환
- `h`: Histogram view 열기
- `q`: 종료

Histogram view의 기본 키맵은 다음과 같다.

- `up/down`: histogram bucket 선택 이동
- `Enter`: 선택 bucket의 시간 범위로 검색 조건을 좁히고 logs view로 복귀
- `Esc`: 검색 조건 변경 없이 logs view로 복귀
- `r`: histogram query 재조회
- `q`: 종료

Detail view의 기본 키맵은 다음과 같다.

- `Tab`: `JSON Tree`, `Pretty`, `Raw` 탭 전환
- `up/down`: 상세 내용 한 줄 스크롤
- `PgUp/PgDn`: 상세 내용 페이지 단위 스크롤
- `/`: 상세 화면 내부 검색 프롬프트
- `n`: 다음 검색 매치로 이동
- `N`: 이전 검색 매치로 이동
- `g`: 상세 내용 처음으로 이동
- `G`: 상세 내용 끝으로 이동
- `Esc` 또는 `Enter`: logs view로 복귀
- `q`: 종료

프롬프트 입력 중 `Enter`는 적용, `Esc`는 취소로 동작한다. 잘못된 입력은 화면을 종료하지 않고 Status 영역에 오류를 표시한다.

## 검색 옵션과 로컬 필터

TUI 안에서 변경 가능한 서버 재조회 옵션은 다음으로 제한한다.

- Query
- `since` 또는 `from/to`
- `application`
- `part`
- Sort
- Limit

이 옵션을 변경하면 현재 페이지 캐시는 무효화하고 offset을 첫 페이지로 되돌린 뒤 재조회한다.

로컬 필터는 서버 재조회 없이 세션 중 가져온 페이지 캐시에 적용한다. 캐시는 세션 LRU 방식으로 최근 페이지를 유지한다. 로컬 필터는 현재 화면의 행만이 아니라 캐시에 남아 있는 메시지 전체를 대상으로 한다.

로컬 필터가 적용된 상태에서도 Header에는 서버 검색 조건과 로컬 필터 상태를 구분해 표시한다.

## 페이지와 캐시

페이지 이동은 명시적 페이지 이동 방식을 사용한다. 사용자는 `g` 프롬프트에서 페이지 번호를 입력하거나, 페이지 이동 단축키로 다음/이전 페이지를 요청한다.

페이지를 조회하면 결과는 세션 LRU 캐시에 저장한다. 같은 검색 조건에서 이미 조회한 페이지는 캐시를 우선 사용할 수 있다. 서버 검색 조건이 바뀌면 캐시는 비운다.

전체 페이지 수는 Graylog 응답의 total과 limit을 기준으로 계산한다. total을 알 수 없는 오류 상태에서는 현재 페이지와 offset만 표시한다.

## 대량 로그 보기 개선

로그량이 많은 검색에서도 기본 탐색 방식은 명시적 페이지 모델을 유지한다. 무한 스크롤은 도입하지 않는다. 대신 Histogram view, 유사 로그 그룹, 필드 값 후보, 점진 계산으로 읽기 부담을 줄인다.

`h` 키를 누르면 로그 목록에서 Histogram view로 전환한다. Histogram view는 현재 검색 조건의 시간 구간별 로그량을 전용 페이지로 보여주며, 사용자가 폭증 구간을 빠르게 파악하고 특정 시간 bucket으로 검색 범위를 좁히는 데 사용한다.

Histogram view는 Header와 Status/Help footer는 유지하고 Body 전체를 히스토그램 차트에 사용한다. Header 첫 줄은 `view: histogram`을 표시한다. 히스토그램 데이터는 캐시된 로그가 아니라 Graylog histogram query 결과를 사용한다. 기본 interval은 Graylog `auto`를 사용한다.

Histogram view의 각 bucket row는 시간 label, count, bar를 표시한다. 사용자는 `up/down`으로 bucket을 선택한다. 선택한 bucket에서 `Enter`를 누르면 해당 bucket의 시간 구간으로 검색 범위를 좁히고 logs view로 돌아간다. 이때 검색 범위는 선택 bucket의 시작 시각 이상, 다음 bucket의 시작 시각 미만으로 설정한다. 마지막 bucket은 시작 시각 이상, effective end 이하로 설정한다. `Esc`는 검색 조건을 바꾸지 않고 logs view로 돌아간다. `r`은 histogram query를 다시 실행한다.

Histogram view 조회 실패는 TUI를 종료하지 않는다. Status 영역에 오류를 표시하고, 사용자는 `r`로 재시도하거나 `Esc`로 logs view에 돌아갈 수 있다.

Histogram query 결과가 비어 있거나 모든 bucket count가 0이면 Body에는 빈 차트 대신 `no histogram data` 상태를 표시한다. 이 상태에서도 `r`, `Esc`, `q`는 정상 동작한다.

Body는 원본 로그 테이블과 유사 로그 그룹 보기 모드를 가진다. `G` 키로 모드를 전환한다. 유사 로그 그룹은 같은 메시지 패턴의 로그를 한 줄로 묶고 다음 정보를 표시한다.

- Count: 그룹에 포함된 로그 수
- Latest timestamp: 그룹 내 최신 로그 시간
- Level: 대표 또는 최상위 심각도
- Source: 대표 source
- Message pattern: 대표 메시지 패턴

그룹 보기에서도 선택한 그룹에 `Enter`를 누르면 대표 로그의 상세 화면으로 들어간다. 그룹 내부의 개별 로그 탐색은 v2 범위에서는 별도 drill-down 화면으로 만들지 않고, 로컬 필터나 서버 query를 좁혀 원본 테이블에서 확인한다.

로컬 필터 프롬프트는 현재 캐시된 결과에서 필드 값 후보와 count를 보여준다. 후보 대상은 `level`, `source`, `application`, `part`, 그리고 표시 중인 주요 field 값이다. 사용자는 후보를 선택해 로컬 필터를 적용할 수 있고, 직접 입력도 가능하다.

대량 검색에서는 현재 페이지를 먼저 렌더링한다. Histogram view 조회, 유사 로그 그룹, 필드 값 후보 계산은 백그라운드에서 점진적으로 수행한다. 계산 중에는 Status 영역에 진행 상태를 표시한다. `Esc`는 열린 프롬프트가 없을 때 진행 중인 백그라운드 계산 취소로 동작할 수 있다. 계산이 취소되어도 현재 로그 목록과 선택 상태는 유지한다.

`--follow --tui`에서도 Histogram view는 자동으로 갱신하지 않는다. Histogram view에서 최신 분포가 필요하면 사용자가 `r`을 눌러 명시적으로 재조회한다. logs view로 돌아가면 기존 follow refresh 정책을 따른다.

## 긴 메시지와 상세 화면

`Enter`를 누르면 선택된 로그의 상세 화면으로 전환한다. 목록 화면에서는 긴 메시지, 멀티라인 메시지, JSON 메시지를 펼치지 않는다. 상세 화면은 긴 내용을 전체 분석하는 공간으로 사용한다.

상세 화면은 세 탭을 제공한다.

- `JSON Tree`: 기본 탭. JSON 객체와 배열을 tree 형태로 렌더링한다.
- `Pretty`: 기존 pretty renderer와 같은 형태의 사람이 읽기 좋은 출력
- `Raw`: 원본 문자열을 가능한 그대로 보여주는 출력

메시지가 JSON 형태이면 상세 화면의 기본 탭은 `JSON Tree`다. JSON Tree는 기본적으로 전체 펼침 상태로 표시한다. 큰 배열도 생략하지 않고 전체 표시한다. `stacktrace`처럼 긴 멀티라인 문자열은 접지 않고 항상 펼쳐서 표시한다.

Raw 탭은 원문 확인을 위한 탭이다. JSON Tree나 Pretty 렌더링을 위해 값이 재정렬되거나 들여쓰기되더라도, Raw 탭에서는 원본 문자열 형태를 확인할 수 있어야 한다.

상세 화면에서 `Tab`은 `JSON Tree`, `Pretty`, `Raw` 탭을 순서대로 전환한다. `Esc` 또는 `Enter`는 기본 테이블 화면으로 돌아간다. 상세 내용이 화면보다 길면 위/아래 키로 한 줄 단위 스크롤하고, `PgUp`/`PgDn`으로 페이지 단위 스크롤한다.

상세 화면은 긴 JSON과 멀티라인 메시지를 찾기 쉽게 다음 탐색 키를 지원한다.

- `/`: 상세 화면 내부 검색 프롬프트
- `n`: 다음 검색 매치로 이동
- `N`: 이전 검색 매치로 이동
- `g`: 상세 내용 처음으로 이동
- `G`: 상세 내용 끝으로 이동

## Status/Help Footer

Footer는 Status 모드와 Help 모드를 토글할 수 있다.

Status 모드는 다음 정보를 우선 표시한다.

- 현재 페이지와 전체 페이지
- 현재 선택 행
- 현재 결과 수와 total
- 로딩 상태
- 마지막 오류 또는 마지막 refresh 시각
- Histogram view 조회, 그룹, 필드 후보 계산 진행 상태

Help 모드는 현재 화면에서 사용할 수 있는 핵심 키를 표시한다. 오류가 발생하면 Status 영역에 오류 메시지와 복구 방법을 표시한다. 예를 들어 재조회 실패 시 `r`로 retry, `q`로 quit할 수 있음을 보여준다.

## 오류 처리

검색 실패, 파싱 실패, 잘못된 프롬프트 입력은 TUI를 종료하지 않는다. 오류는 Status 영역에 표시하고, 기존 성공 결과가 있으면 Body는 유지한다.

서버 재조회 중에는 로딩 상태를 표시한다. 재조회가 실패하면 기존 캐시와 현재 화면은 가능한 한 유지하고, 사용자가 조건을 수정하거나 refresh할 수 있게 한다.

## 구현 방향

초기 구현은 기존처럼 Bubble Tea 기반으로 진행한다. 기존 non-TUI CLI 출력 동작은 변경하지 않는다.

TUI 모델은 다음 상태를 명시적으로 가진다.

- 서버 검색 조건: query, time range, application, part, sort, limit, offset
- 화면 상태: `logs`, `histogram`, `detail` view
- 테이블 상태: 컬럼 목록, 선택 행, 스크롤 위치
- 대량 보기 상태: 원본/그룹 보기 모드, Histogram view 결과, 선택 bucket, bucket 시간 범위, 그룹 계산 결과, 필드 값 후보
- 프롬프트 상태: 닫힘, query, local filter, sort, page, column 설정
- 상세 상태: 닫힘 또는 열림, 현재 탭, 상세 스크롤 위치, 상세 검색어, 검색 매치 위치
- 캐시 상태: 검색 조건별 최근 페이지 LRU
- Footer 상태: status 또는 help
- 로딩, 오류, 마지막 refresh 시각

서버 재조회 옵션 변경과 로컬 필터 변경은 별도 경로로 처리한다. 서버 재조회 옵션 변경은 캐시를 비우고 Graylog를 다시 호출한다. 로컬 필터 변경은 캐시된 메시지에서만 결과를 다시 계산한다.

Histogram view 결과는 Graylog histogram query를 입력으로 계산한다. 유사 로그 그룹과 필드 값 후보는 캐시된 메시지를 입력으로 계산한다. 이 계산은 UI 렌더링을 막지 않아야 하며, 새 서버 검색 조건이 적용되면 이전 계산 결과를 폐기한다.

## 테스트 계획

단위 테스트는 네트워크 호출 없이 fake fetcher를 사용한다.

필수 상태 전이 테스트:

- 테이블 이동과 선택 행 변경
- 명시적 페이지 이동
- 프롬프트 열기, 입력 적용, 취소
- query, sort, limit 변경 시 캐시 무효화와 첫 페이지 재조회
- 로컬 필터 적용 시 캐시 기반 결과 계산
- 상세 화면 열기/닫기
- `JSON Tree`/`Pretty`/`Raw` 탭 전환
- 상세 화면 내부 검색, 다음/이전 매치 이동, 처음/끝 이동
- Footer status/help 토글
- 오류 발생 시 Status 표시와 기존 결과 유지
- 유사 로그 그룹 보기 토글
- `h` 입력 시 logs view에서 Histogram view로 전환
- Histogram view에서 `Esc` 입력 시 logs view로 복귀
- Histogram view에서 bucket 선택 이동
- Histogram view에서 선택 bucket `Enter` 입력 시 time range 변경 후 logs view로 복귀
- Histogram view에서 마지막 bucket 선택 시 effective end를 종료 시각으로 사용
- 빈 histogram 또는 0-count histogram 상태 표시
- `--follow --tui`의 Histogram view에서 자동 갱신하지 않고 `r`로만 재조회
- 백그라운드 계산 취소 시 기존 목록과 선택 상태 유지

필수 렌더링 테스트:

- Header에 검색 메타 정보 표시
- Header에 view와 Histogram bucket 상태 표시
- 기본 컬럼 순서
- 고정 컬럼과 message 가변 폭 clipping
- 선택 행도 목록에서 1줄로 유지
- 선택 행 전체 라인 하이라이트
- JSON Tree 상세 렌더링
- JSON 배열 전체 표시
- stacktrace와 긴 멀티라인 문자열 전체 표시
- Raw 탭의 원문 문자열 보존
- Header의 현재 view 표시
- Histogram view의 시간 bucket, count, bar 렌더링
- Histogram view의 `no histogram data` 상태 표시
- 유사 로그 그룹의 count, 대표 시간, level, source, message pattern 표시
- 필드 값 후보와 count 표시

수동 smoke test:

```sh
./graylog-cli search '*' --since 15m --limit 20 --tui
```

확인 항목:

- Header가 date range, query, page, offset/limit, sort, filter를 표시한다.
- Body가 가변 컬럼 테이블로 표시된다.
- 선택 행을 포함해 목록의 모든 행이 1줄로 유지된다.
- 선택한 로그는 특정 셀이 아니라 전체 라인이 하이라이트된다.
- `/`, `f`, `s`, `g`, `c` 프롬프트가 열리고 적용/취소된다.
- 로컬 필터가 캐시된 페이지 결과에 적용된다.
- `h`로 Histogram view를 열 수 있다.
- Histogram view에서 bucket을 선택하고 `Enter`로 해당 시간 구간 로그 목록으로 돌아갈 수 있다.
- `G`로 원본 로그와 유사 로그 그룹 보기를 전환할 수 있다.
- `f` 프롬프트에서 필드 값 후보와 count를 보고 필터를 적용할 수 있다.
- `Enter` 상세 화면에서 JSON Tree, Pretty, Raw 탭을 전환할 수 있다.
- 상세 화면에서 `/`, `n`, `N`, `g`, `G`, `PgUp`, `PgDn`으로 긴 JSON과 멀티라인 메시지를 탐색할 수 있다.
- 오류가 Status 영역에 표시되고 `r`로 재시도할 수 있다.

## 가정

- v2 문서는 기존 검색 TUI 문서를 대체하지 않고 별도 설계로 유지한다.
- 기존 CLI compact, pretty, json, ndjson 출력은 변경하지 않는다.
- `--follow --tui`는 별도 요구가 생기기 전까지 기존 refresh 모델과 호환되게 유지한다.
- logs view 안의 inline/mini histogram, 목록 화면의 멀티라인 행 확장, 무한 스크롤, 그룹 내부 drill-down 화면, JSON tree 접기/펼치기 조작, 마우스 조작, 컬럼 폭 수동 조절은 v2 범위에 포함하지 않는다.
