# graylog-cli Search TUI v2 Design

## 요약

`graylog-cli search --tui`를 `graylog_view_tui.jpg`의 구조에 맞춰 새로 설계한다. 기존 TUI 설계 문서는 유지하고, 이 문서는 Header, Body 테이블, 명령 프롬프트, Status/Help footer를 가진 v2 설계로 다룬다.

v2의 목표는 Graylog 검색 결과를 터미널 안에서 테이블 형태로 빠르게 탐색하고, 검색 조건과 표시 조건을 즉시 바꾸며, 선택한 로그는 목록 안에서 펼치거나 팝업으로 자세히 확인할 수 있게 하는 것이다.

## 화면 구조

기본 화면은 네 영역으로 나눈다.

- Header: 현재 검색의 상세 메타 정보를 표시한다.
- Body: 로그 결과를 가변 컬럼 테이블로 표시한다.
- Command prompt: 단축키로 열린 입력 프롬프트를 표시한다.
- Status/Help footer: 상태 정보와 도움말을 토글해 표시한다.

기본 화면은 테이블 중심이다. 로그 목록의 각 행은 기본 1줄로 표시하고, `Space`로 선택 행을 펼치거나 닫는다. `Enter`는 화면 전환이 아니라 현재 목록 위에 상세 팝업을 띄운다.

## 텍스트 와이어프레임

기본 목록 화면은 다음 형태를 목표로 한다.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ graylog-cli search --tui | view: logs                                        │
│ range: last 15m | query: level:3 AND service:api | page: 2/18 | total: 342   │
│ offset: 20 | limit: 20 | sort: timestamp:DESC | app: api | local filter: on  │
├──────┬──────────────────────┬────────┬────────────────────┬────────────────┤
│ #    │ timestamp            │ level  │ source             │ message        │
├──────┼──────────────────────┼────────┼────────────────────┼────────────────┤
│ 021  │ 2026-06-24 14:01:22  │ ERROR  │ api-1              │ failed to call │
│ 022  │ 2026-06-24 14:01:18  │ WARN   │ api-2              │ retrying req   │
│ 023  │ 2026-06-24 14:01:03  │ ERROR  │ api-1              │ timeout while  │
│      │ message: timeout while fetching backend response                    │
│      │ request_id: abc-123 | application: api | part: checkout             │
│ 024  │ 2026-06-24 14:00:58  │ INFO   │ api-4              │ accepted       │
│                                                                              │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│ / query: level:3 AND service:api                                             │
├──────────────────────────────────────────────────────────────────────────────┤
│ STATUS page 2/18 | row 4/20 | cached pages 3 | refreshed 14:01:24 | r retry │
└──────────────────────────────────────────────────────────────────────────────┘
```

상세는 화면 전환이 아니라 목록 위 팝업으로 표시한다.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ... logs view remains behind the popup ...                                  │
│                 ┌──────────────── detail ────────────────┐                  │
│                 │ timestamp: 2026-06-24T14:01:03.120Z    │                  │
│                 │ level: ERROR                           │                  │
│                 │ source: api-1                          │                  │
│                 │ message: timeout while fetching ...     │                  │
│                 │ request_id: abc-123                     │                  │
│                 │ application: api                        │                  │
│                 │ part: checkout                          │                  │
│                 │ duration_ms: 30000                      │                  │
│                 └──────────────── Esc close ──────────────┘                  │
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

Histogram view에서도 Header는 같은 위치에 표시한다. 다만 page와 offset은 logs view의 현재 위치를 의미하며, Histogram view에서는 추가로 선택 bucket과 bucket 수를 표시한다. 상세 팝업이 열려도 Header는 logs view의 상태를 유지한다.

Header는 편집 가능한 영역이 아니다. 검색 조건 변경은 단축키로 여는 프롬프트를 통해 수행한다.

Header는 항목별 고정폭 output box로 렌더링한다. 예를 들어 `view`, `range`, `query`, `page`, `sort`, `filter`, `refresh`는 각각 독립 box를 가지며, box 순서와 폭은 config로 지정할 수 있다. 값이 길어도 box 폭은 변하지 않고 내부 값만 clipping한다. Footer도 같은 방식으로 `status`, `row`, `cached-pages`, `refreshed` 같은 항목을 고정폭 box로 표시한다.

화면은 Header, Body, Footer를 각각 독립 `viewport.Model`로 보유하고 렌더링한다. 세 viewport는 서로 다른 테두리 box로 감싸 화면 영역을 명확히 분리한다. Header/Footer는 스크롤 대상이 아니며, Body만 로그 목록, 그룹, 히스토그램, 상세 팝업 배경의 주 표시 영역으로 사용한다. 프롬프트가 열리면 별도 네 번째 영역을 만들지 않고 Footer viewport의 첫 줄에 포함한다.

## Body Table

Body는 config 기반 컬럼 테이블로 구성한다. 첫 번째 컬럼은 항상 일련번호(`#`)이며 데이터 컬럼 설정과 별도로 표시한다. 일련번호는 기본적으로 서버 offset 기준 전역 번호이고, config의 `row-number-mode`가 `page`이면 현재 페이지 내부 번호로 표시한다.

config가 없을 때 기본 데이터 컬럼은 다음 순서를 따른다.

1. `timestamp`
2. `level`
3. `source`
4. `message`
5. `--fields`로 지정한 필드 또는 표시 대상으로 선택된 추가 필드

컬럼 목록과 폭은 config의 `SearchTUI.columns`로 지정할 수 있다. `--fields`로 지정한 필드는 config 컬럼 뒤에 중복 없이 추가한다. config에 폭이 없는 추가 필드는 남은 폭을 나눠 갖고, 터미널 폭이 부족하면 오른쪽 컬럼부터 줄이며 셀 내용은 clipping한다.

목록 화면의 모든 로그 행은 기본 1줄로 표시한다. 긴 `message`와 JSON 문자열은 원문 앞부분을 `message` 컬럼 폭에 맞게 clipping한다. 각 row는 Header/Body/Footer 테두리 안쪽 viewport 폭을 넘지 않도록 최종 라인 전체를 한 번 더 clipping해 터미널 자동 줄바꿈이 발생하지 않게 한다. 선택 행에서 `Space`를 누르면 해당 로그를 펼치고, 다시 누르면 닫는다. 펼침 영역은 데이터에 맞춰 필요한 줄 수를 사용한다. `expanded-max-lines`가 양수이면 해당 줄 수로 제한하고, 기본값 `0`은 제한 없이 모든 펼침 라인을 표시한다.

펼침 상태는 선택 로그의 stable key로 보존한다. 키 우선순위는 `gl2_message_id`, `_id`, 없으면 `offset:index`이다. `persist-expanded-rows`가 true이면 스크롤, 페이지 이동, 캐시된 페이지 복귀 사이에서도 펼침 상태를 유지한다. 캐시에서 제거된 페이지의 펼침 상태는 함께 제거할 수 있다.

Body 스크롤은 로그 행 index가 아니라 실제 렌더링 라인 index를 기준으로 계산한다. 일반 로그 행은 1줄이고, 펼쳐진 로그는 `expanded-max-lines`까지 추가 라인을 차지한다. 선택 행이 Body 마지막 표시 줄을 넘어가면 `page-scroll-step` 설정값만큼 화면을 이동하되, 선택 행이 화면 밖으로 사라지지 않도록 보정한다.

선택된 로그는 특정 셀만이 아니라 행 전체를 선택 상태로 표시한다. 하이라이트는 `timestamp`, `level`, `source`, `message`, 추가 필드를 포함한 전체 라인 폭에 적용한다. 선택 행 앞에 별도 커서 문자(`▶` 등)는 표시하지 않는다.

테이블 정렬은 TUI 안에서 컬럼과 방향을 선택해 변경한다. 정렬 변경은 서버 재조회가 필요한 검색 옵션 변경으로 취급한다. v2의 sort는 단일 필드만 지원한다. 새 sort를 적용하면 기존 sort는 교체된다.

컬럼 폭은 TUI 안에서 조정할 수 있다. `c`로 컬럼 폭 조정 모드에 들어가고, `1`~`9`로 화면상 데이터 컬럼을 선택한 뒤 `+`/`-`로 폭을 조정한다. 일련번호 컬럼은 조정 대상에서 제외한다. `Esc`로 조정 모드를 종료하면 변경된 폭을 현재 `--config` 파일에 저장할지 묻는다. 저장하면 `SearchTUI.columns`만 갱신하고 기존 `GraylogEndpoint` 설정은 보존한다.

## Keymap Reference

Command prompt 영역은 항상 입력 가능한 명령줄이 아니다. 사용자가 단축키를 누르면 해당 목적의 전용 모드나 프롬프트가 열린다. 키 처리 우선순위는 `save prompt > local filter prompt > generic prompt > detail popup > column resize mode > current view` 순서다. `Esc`는 항상 가장 안쪽 상태만 닫는다.

Global keys:

- `q`, `ctrl+c`: TUI 종료
- `Tab`: footer status/help 토글. 상세 팝업은 Pretty 단일뷰이므로 탭 전환에 `Tab`을 사용하지 않는다.

Logs view:

- `up/down`: 선택 로그 행 이동
- `PgUp/PgDn`: `page-scroll-step` 설정값만큼 선택 로그 행 이동
- `left/right`, `p/n`: 이전/다음 페이지
- `g`: 페이지 이동 프롬프트
- `r`: 현재 조건으로 재조회
- `Space`: 선택 로그 펼침/닫힘 토글
- `Enter`: 선택 로그 상세 팝업 열기
- `/`: query 변경 프롬프트
- `f`: 로컬 필터 프롬프트
- `s`: sort 변경 프롬프트
- `l`: limit 변경 프롬프트
- `a`: application 변경 프롬프트
- `P`: part 변경 프롬프트
- `h`: Histogram view 열기
- `G`: 원본 로그 보기와 유사 로그 그룹 보기 전환
- `c`: 컬럼 폭 조정 모드 진입

Expanded row:

- 별도 포커스 모드는 만들지 않는다.
- 펼친 행도 logs view 선택, 페이지 이동, 새로고침 키를 그대로 사용한다.
- 같은 행에서 `Space`를 다시 누르면 닫는다.

Detail popup:

- `Enter`: logs view에서 팝업 열기
- `Esc`: 팝업 닫기
- `up/down`: 팝업 내부 한 줄 스크롤
- `PgUp/PgDn`: 팝업 내부 페이지 단위 스크롤
- `Home/End`: 팝업 처음/끝 이동
- 팝업이 열려 있을 때 logs view 이동키는 팝업에 소비된다.

Column resize mode:

- `c`: logs view에서 진입
- `1`~`9`: 화면상 데이터 컬럼 선택
- `+` 또는 `=`: 선택 컬럼 폭 증가
- `-`: 선택 컬럼 폭 감소
- `left/right`: 이전/다음 데이터 컬럼 선택
- `Esc`: 조정 모드 종료 후 저장 여부 프롬프트 표시
- 일련번호 컬럼은 조정 대상에서 제외한다.

Column save prompt:

- `y`: 현재 `--config` 파일에 `SearchTUI.columns` 저장
- `n` 또는 `Esc`: 저장하지 않고 세션 변경만 유지
- 저장 실패 시 Status에 오류를 표시하고 TUI는 유지한다.

Local filter prompt:

- 필드 선택 단계 `up/down`: candidate 필드명 선택 이동
- 필드 선택 단계 `Enter`: 선택한 필드의 값 선택 단계로 진입
- 필드 선택 단계 `Ctrl+U` 또는 빈 입력 `Enter`: 현재 로컬 필터 삭제
- 필드 선택 단계 `Esc`: 프롬프트 닫기
- 값 선택 단계 `up/down`: candidate 값 선택 이동
- 값 선택 단계 텍스트 입력: 직접 입력값 갱신
- 값 선택 단계 `Enter`: 선택 후보 또는 직접 입력값으로 로컬 필터 적용
- 값 선택 단계 `Ctrl+U` 또는 빈 입력 `Enter`: 현재 로컬 필터 삭제
- 값 선택 단계 `Esc`: 필드 선택 단계로 복귀

Histogram view:

- `up/down`: histogram bucket 선택 이동
- `Enter`: 선택 bucket의 시간 범위로 검색 조건을 좁히고 logs view로 복귀
- `Esc`: 검색 조건 변경 없이 logs view로 복귀
- `r`: histogram query 재조회
- `q`: 종료

Generic prompts:

- `Enter`: 입력 적용
- `Esc`: 입력 취소
- 잘못된 입력은 화면을 종료하지 않고 Status 영역에 오류를 표시한다.

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

캐시 page 수는 `SearchTUI.max-cached-pages`로 제어한다. 기본값은 8이다.

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

그룹 보기에서도 선택한 그룹에 `Enter`를 누르면 대표 로그의 상세 팝업을 연다. 그룹 내부의 개별 로그 탐색은 v2 범위에서는 별도 drill-down 화면으로 만들지 않고, 로컬 필터나 서버 query를 좁혀 원본 테이블에서 확인한다.

로컬 필터 프롬프트는 현재 캐시된 원본 결과에서 후보를 만들고, `필드 선택 -> 값 선택/입력 -> 적용` 흐름으로 동작한다. 로컬 필터가 이미 적용된 상태에서도 candidate 필드와 값은 필터 적용 전의 캐시 원본 데이터를 기준으로 보여준다. 사용자가 `f`를 누르면 필드 후보 목록을 보여준다. 필드 후보는 `level`, `source`, `application`, `part`, `message`, 그리고 표시 중인 field 중 `timestamp`를 제외한 값을 사용한다.

필드 선택 단계의 키맵은 다음과 같다.

- `up/down`: candidate 필드명 선택 이동
- `Enter`: 선택한 필드의 값 선택 단계로 진입
- `Ctrl+U` 또는 빈 입력 `Enter`: 현재 로컬 필터 삭제
- `Esc`: 프롬프트 닫기

값 선택 단계는 선택한 필드의 후보 값과 count를 보여준다. 일반 필드는 distinct value와 count를 후보로 보여준다. `message` 필드는 전체 원문을 후보로 나열하지 않고, 숫자 토큰을 `<num>`으로 정규화한 짧은 message pattern 후보와 count를 보여준다. 이 단계에서도 사용자는 후보 선택 대신 직접 텍스트를 입력할 수 있다.

값 선택 단계의 키맵은 다음과 같다.

- `up/down`: candidate 값 선택 이동
- 텍스트 입력: 직접 입력값 갱신
- `Enter`: 선택 후보 또는 직접 입력값으로 로컬 필터 적용
- `Ctrl+U` 또는 빈 입력 `Enter`: 현재 로컬 필터 삭제
- `Esc`: 필드 선택 단계로 복귀

후보 선택으로 적용된 로컬 필터는 `field contains value` 방식으로 평가한다. 예를 들어 `source` 필드에서 `api-1` 값을 선택하면 `source contains "api-1"`로 캐시된 메시지를 거르고, `message` 필드에서 pattern 또는 직접 입력값을 적용하면 `message contains "..."`로 거른다. 로컬 필터는 한 번에 하나만 적용한다.

대량 검색에서는 현재 페이지를 먼저 렌더링한다. Histogram view 조회, 유사 로그 그룹, 필드 값 후보 계산은 백그라운드에서 점진적으로 수행한다. 계산 중에는 Status 영역에 진행 상태를 표시한다. `Esc`는 열린 프롬프트가 없을 때 진행 중인 백그라운드 계산 취소로 동작할 수 있다. 계산이 취소되어도 현재 로그 목록과 선택 상태는 유지한다.

`--follow --tui`에서도 Histogram view는 자동으로 갱신하지 않는다. Histogram view에서 최신 분포가 필요하면 사용자가 `r`을 눌러 명시적으로 재조회한다. logs view로 돌아가면 기존 follow refresh 정책을 따른다.

## 긴 메시지, 펼침, 상세 팝업

목록 행은 기본적으로 항상 1줄이다. 긴 `message`, 멀티라인 문자열, JSON 문자열은 컬럼 폭에 맞게 clipping한다. 사용자가 `Space`를 누르면 선택 로그 아래에 펼침 영역을 추가해 주요 필드와 message 내용을 여러 줄로 보여준다.

펼침 영역은 데이터 줄 수에 맞춰 높이를 정한다. `SearchTUI.expanded-max-lines`가 양수이면 최대 높이를 해당 줄 수로 제한하고, 기본값 `0`이면 제한 없이 모든 펼침 라인을 표시한다. `SearchTUI.message-wrap`이 true이면 message를 펼침 영역 폭에 맞춰 wrap한다. false이면 원문 줄 단위로만 나누고 긴 줄은 clipping한다.

`Enter`를 누르면 선택 로그의 상세 팝업을 연다. 상세 팝업은 화면 전환이 아니라 현재 logs view 위에 겹쳐 표시한다. 팝업은 Pretty 단일뷰이며, `SearchTUI.detail-fields`에 지정된 필드를 우선 표시하고 나머지 필드는 정렬된 key/value 목록으로 이어서 표시한다. 빈 값은 기본적으로 숨기고, `detail-show-empty-fields`가 true이면 빈 필드도 표시한다.

상세 팝업 높이는 데이터 줄 수에 맞추되 `SearchTUI.detail-popup-max-lines`를 넘지 않는다. 기본값은 20줄이다. 내용이 더 많으면 `up/down`, `PgUp/PgDn`, `Home/End`로 팝업 내부를 스크롤한다. `Esc`를 누르면 팝업만 닫고 logs view 선택 행과 스크롤 위치는 유지한다.

상세 팝업 테두리는 Header/Body/Footer의 일반 테두리와 구분되도록 굵은 border를 사용한다.

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

## SearchTUI Config Reference

TUI 설정은 기존 config 파일의 `[SearchTUI]` 섹션에 둔다. 항목명은 TOML에서 kebab-case를 사용한다. config가 없거나 값이 잘못되면 기본값으로 fallback한다.

예시:

```toml
[SearchTUI]
expanded-max-lines = 0
detail-popup-max-lines = 20
row-number-width = 4
row-number-mode = "absolute"
max-cached-pages = 8
persist-expanded-rows = true
follow-refresh-pauses-on-nonfirst-page = true

message-wrap = true
cell-overflow = "clip"
min-column-width = 4
column-resize-step = 2
page-scroll-step = 10
detail-popup-width-ratio = 0.85
detail-popup-position = "center"
detail-show-empty-fields = false
detail-fields = ["timestamp", "level", "source", "message", "application", "part"]

filter-candidate-fields = ["level", "source", "application", "part", "message"]
filter-candidate-limit = 20
message-pattern-candidate-limit = 20
filter-match-mode = "contains"
filter-case-sensitive = false

header-visible = true
footer-visible = true
header-box-gap = 1
footer-box-gap = 1
box-overflow = "clip"

[[SearchTUI.columns]]
field = "timestamp"
width = 20

[[SearchTUI.columns]]
field = "level"
width = 8

[[SearchTUI.columns]]
field = "source"
width = 18

[[SearchTUI.columns]]
field = "message"
width = 60

[[SearchTUI.header-boxes]]
name = "view"
width = 16

[[SearchTUI.header-boxes]]
name = "range"
width = 28

[[SearchTUI.header-boxes]]
name = "query"
width = 40

[[SearchTUI.header-boxes]]
name = "page"
width = 14

[[SearchTUI.header-boxes]]
name = "sort"
width = 24

[[SearchTUI.footer-boxes]]
name = "status"
width = 18

[[SearchTUI.footer-boxes]]
name = "row"
width = 14

[[SearchTUI.footer-boxes]]
name = "cached-pages"
width = 16

[[SearchTUI.footer-boxes]]
name = "refreshed"
width = 18
```

항목 설명:

- `expanded-max-lines`: integer, default `0`. `Space` 펼침 영역의 최대 줄 수다. `0`이면 제한 없이 모든 펼침 라인을 표시하고, 양수이면 해당 줄 수로 제한한다.
- `detail-popup-max-lines`: integer, default `20`. `Enter` 상세 팝업의 최대 줄 수다. 초과 내용은 팝업 내부 스크롤로 본다.
- `row-number-width`: integer, default `4`. 첫 번째 일련번호 컬럼 폭이다.
- `row-number-mode`: string, default `absolute`, allowed `absolute`, `page`. `absolute`는 `offset + row index + 1`, `page`는 페이지 내부 번호를 표시한다.
- `max-cached-pages`: integer, default `8`. 세션 LRU 페이지 캐시 개수다.
- `persist-expanded-rows`: boolean, default `true`. 캐시된 페이지 이동 사이에서 펼침 상태를 유지할지 정한다.
- `follow-refresh-pauses-on-nonfirst-page`: boolean, default `true`. follow 모드에서 첫 페이지가 아닐 때 자동 refresh를 멈출지 정한다.
- `message-wrap`: boolean, default `true`. 펼침 영역에서 message를 화면 폭에 맞춰 wrap할지 정한다.
- `cell-overflow`: string, default `clip`, v2 allowed `clip`. 1줄 셀 내용이 컬럼 폭보다 길 때 처리 방식이다.
- `min-column-width`: integer, default `4`. 컬럼 폭 조정 시 허용하는 최소 폭이다.
- `column-resize-step`: integer, default `2`. `+` 또는 `-` 한 번에 조정할 컬럼 폭이다.
- `page-scroll-step`: integer, default `10`. `PgUp` 또는 `PgDn` 한 번에 이동할 로그 행 수다. `up/down`은 항상 한 줄씩 이동한다.
- `detail-popup-width-ratio`: float, default `0.85`. 상세 팝업 폭을 터미널 폭 대비 비율로 정한다.
- `detail-popup-position`: string, default `center`, allowed `center`, `right`, `bottom`. 상세 팝업 위치다.
- `detail-show-empty-fields`: boolean, default `false`. 상세 팝업에서 빈 필드를 표시할지 정한다.
- `detail-fields`: string array. 상세 팝업에서 우선 표시할 필드 순서다.
- `filter-candidate-fields`: string array. 로컬 필터 필드 후보 목록이다.
- `filter-candidate-limit`: integer, default `20`. 일반 필드별 후보 값 최대 개수다.
- `message-pattern-candidate-limit`: integer, default `20`. message pattern 후보 최대 개수다.
- `filter-match-mode`: string, default `contains`, v2 allowed `contains`. 후보 선택 또는 직접 입력 필터의 매칭 방식이다.
- `filter-case-sensitive`: boolean, default `false`. 로컬 필터 매칭에서 대소문자를 구분할지 정한다.
- `header-visible`: boolean, default `true`. Header 표시 여부다.
- `footer-visible`: boolean, default `true`. Footer 표시 여부다.
- `header-box-gap`: integer, default `1`. Header box 사이 간격이다.
- `footer-box-gap`: integer, default `1`. Footer box 사이 간격이다.
- `box-overflow`: string, default `clip`, v2 allowed `clip`. Header/Footer box 값이 폭보다 길 때 처리 방식이다.
- `SearchTUI.columns`: array of table, fields `field`, `width`. 로그 목록 데이터 컬럼 순서와 폭이다. 일련번호 컬럼은 여기에 넣지 않는다.
- `SearchTUI.header-boxes`: array of table, fields `name`, `width`. Header 항목 순서와 고정 폭이다.
- `SearchTUI.footer-boxes`: array of table, fields `name`, `width`. Footer 항목 순서와 고정 폭이다.

컬럼 폭 조정 저장은 현재 실행에 사용된 `--config` 파일을 대상으로 한다. 저장 시 `SearchTUI.columns`만 갱신하고 기존 `GraylogEndpoint` 설정은 보존한다. config 파일이 없으면 새로 만든다.

## 오류 처리

검색 실패, 파싱 실패, 잘못된 프롬프트 입력은 TUI를 종료하지 않는다. 오류는 Status 영역에 표시하고, 기존 성공 결과가 있으면 Body는 유지한다.

서버 재조회 중에는 로딩 상태를 표시한다. 재조회가 실패하면 기존 캐시와 현재 화면은 가능한 한 유지하고, 사용자가 조건을 수정하거나 refresh할 수 있게 한다.

## 구현 방향

초기 구현은 기존처럼 Bubble Tea 기반으로 진행한다. 기존 non-TUI CLI 출력 동작은 변경하지 않는다.

TUI 모델은 다음 상태를 명시적으로 가진다.

- 서버 검색 조건: query, time range, application, part, sort, limit, offset
- 화면 상태: `logs`, `histogram` view
- 테이블 상태: 컬럼 목록과 폭, 선택 행, 스크롤 위치, 펼침 row key set
- 상세 팝업 상태: 닫힘 또는 열림, 팝업 스크롤 위치
- 컬럼 조정 상태: 닫힘 또는 열림, 선택 컬럼, 세션 폭 변경, 저장 확인 프롬프트
- 대량 보기 상태: 원본/그룹 보기 모드, Histogram view 결과, 선택 bucket, bucket 시간 범위, 그룹 계산 결과, 필드 값 후보
- 프롬프트 상태: 닫힘, query, local filter, sort, page, limit, application, part
- 캐시 상태: 검색 조건별 최근 페이지 LRU
- TUI config 상태: 컬럼, Header/Footer box, 펼침/팝업 최대 줄 수, 캐시/필터 동작
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
- `Space` 입력 시 선택 로그 펼침/닫힘 토글
- 펼침 상태가 스크롤과 캐시된 페이지 이동 후에도 유지됨
- 펼침 라인 수가 config 최대값을 넘지 않음
- 상세 팝업 열기/닫기
- 상세 팝업 내부 스크롤, 페이지 단위 스크롤, 처음/끝 이동
- 상세 팝업이 열린 동안 `up/down`이 로그 선택이 아니라 팝업 스크롤에 적용됨
- 컬럼 폭 조정 모드 진입, 숫자키 컬럼 선택, `+/-` 폭 변경, `Esc` 종료
- 컬럼 저장 프롬프트에서 `y/n/Esc` 상태 전이
- 컬럼 저장 선택 시 현재 `--config` 파일의 `SearchTUI.columns`만 갱신
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
- 키 처리 우선순위가 `save prompt > local filter prompt > generic prompt > detail popup > column resize mode > current view`를 따름
- `max-cached-pages`, `persist-expanded-rows`, `follow-refresh-pauses-on-nonfirst-page` 설정 반영

필수 렌더링 테스트:

- Header에 검색 메타 정보 표시
- Header에 view와 Histogram bucket 상태 표시
- Header/Footer 고정폭 box 위치와 clipping
- config 컬럼 순서와 폭
- 첫 번째 일련번호 컬럼 표시
- message 컬럼 clipping
- 접힌 행은 목록에서 1줄로 유지
- 펼친 행은 데이터 줄 수와 config 최대 줄 수에 맞게 표시
- 선택 행 전체 라인 하이라이트
- 상세 팝업 Pretty 렌더링
- 상세 팝업 위치, 폭, 최대 줄 수 반영
- Header의 현재 view 표시
- Histogram view의 시간 bucket, count, bar 렌더링
- Histogram view의 `no histogram data` 상태 표시
- 유사 로그 그룹의 count, 대표 시간, level, source, message pattern 표시
- 로컬 필터의 필드 후보, 값 후보, message pattern 후보, count 표시
- 로컬 필터 후보 선택의 `field contains value` 적용

수동 smoke test:

```sh
./graylog-cli search '*' --since 15m --limit 20 --tui
```

확인 항목:

- Header가 date range, query, page, offset/limit, sort, filter를 표시한다.
- Body가 가변 컬럼 테이블로 표시된다.
- 접힌 행은 1줄로 유지되고, `Space`로 선택 행을 펼치고 닫을 수 있다.
- 펼침 상태가 스크롤과 캐시된 페이지 이동 사이에서 유지된다.
- 선택한 로그는 특정 셀이 아니라 전체 라인이 하이라이트된다.
- `/`, `f`, `s`, `g`, `l`, `a`, `P` 프롬프트가 열리고 적용/취소된다.
- `c` 컬럼 폭 조정 모드에서 숫자키, `+/-`, 저장 확인 프롬프트가 동작한다.
- 로컬 필터가 캐시된 페이지 결과에 적용된다.
- `h`로 Histogram view를 열 수 있다.
- Histogram view에서 bucket을 선택하고 `Enter`로 해당 시간 구간 로그 목록으로 돌아갈 수 있다.
- `G`로 원본 로그와 유사 로그 그룹 보기를 전환할 수 있다.
- `f` 프롬프트에서 필드 후보를 `up/down`으로 선택하고 `Enter`로 값 선택 단계에 진입할 수 있다.
- 값 선택 단계에서 후보 값을 `up/down`으로 선택하고 `Enter`로 `field contains value` 로컬 필터를 적용할 수 있다.
- `message` 필드에서는 message pattern 후보 선택과 직접 입력 필터를 모두 적용할 수 있다.
- `Enter`로 상세 팝업을 열고 `Esc`로 닫을 수 있다.
- 상세 팝업에서 `up/down`, `PgUp/PgDn`, `Home/End`로 긴 내용을 탐색할 수 있다.
- 오류가 Status 영역에 표시되고 `r`로 재시도할 수 있다.

## 가정

- v2 문서는 기존 검색 TUI 문서를 대체하지 않고 별도 설계로 유지한다.
- 기존 CLI compact, pretty, json, ndjson 출력은 변경하지 않는다.
- `--follow --tui`는 별도 요구가 생기기 전까지 기존 refresh 모델과 호환되게 유지한다.
- 상세 팝업은 Pretty 단일뷰로 설계하고 기존 `JSON Tree`/`Pretty`/`Raw` 탭 상세 화면은 v2 기본 동작에서 제거한다.
- 숫자키 컬럼 선택은 데이터 컬럼 1~9에만 매핑하고 일련번호 컬럼은 포함하지 않는다.
- 컬럼 폭 조정 저장은 `SearchTUI.columns`만 갱신한다.
- config 저장 질문의 기본 선택은 저장하지 않음이다.
- logs view 안의 inline/mini histogram, 무한 스크롤, 그룹 내부 drill-down 화면, JSON tree 접기/펼치기 조작, 마우스 조작은 v2 범위에 포함하지 않는다.
