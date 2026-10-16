# cockpit

compose 로 뜨는 서비스의 커밋 하나를 봉인된 환경에서 E2E 로 돌리고, 판정·관측·리포트 번들을 같은 모양으로 내는 런 허브다. 허브 바이너리 하나(`cockpit`)가 봉인 compose 환경과 런 상태를 맡고, 그 안에 내장된 러너(`cockpit-runner`)가 봉인 망 안에서 TypeScript 플로우를 돌린다. 소비 저장소는 플로우·수집 스크립트·compose 오버레이·프로젝트 선언만 들고 온다.

## 상태

러너 호스트를 만드는 중이다. 허브 명령은 `sdk write <디렉토리>`(타입 선언과 tsconfig 베이스를 소비 저장소에 쓴다)·`runner-image`(내장 러너로 `cockpit-runner:<해시>` 이미지를 보장하고 태그를 낸다)·`--version` 이다. 러너 명령은 `run <스위트> [--flow <id>]...`·`probe <카나리 포트>`·`edge <포트>=<호스트>:<포트>...` 다.

## 개발

```sh
mise install
make verify
```

`make verify` 가 vet · `go test -race` · 생성물 diff · 라이선스 게이트 · 소비자 이름 0건 검사 · 빌드 · 바이너리 크기 측정을 한 번에 돈다. 소비자 이름 검사는 환경 변수 `COCKPIT_CONSUMER_NAMES` 에 정규식이 있을 때만 돈다. 규칙은 `AGENTS.md`, 런 규칙의 본문은 `docs/` 에 있다.
