# v6.0.0 전환

v6.0.0은 루트 모듈 경로를 `github.com/park285/iris-client-go/v6`으로 옮긴 major 버전입니다. v5.2.1 이후 루트 모듈에서
`/v5/valkeydedup` 패키지를 제거했기 때문에 같은 major로 게시할 수 없습니다.

## 소비자가 바꿀 것

- import와 `go.mod` require를 `/v5`에서 `/v6`으로 바꿉니다. 그 밖의 공개 Go API와 Iris 인증·webhook 계약은 v5.2.1과 같습니다.
- Valkey nonce 저장소는 독립 모듈 `github.com/park285/iris-client-go/adapters/valkeydedup`을 씁니다.
- `ReplyStatusSnapshot.Reason`이 추가되었습니다. `iris.ReplyStatusRoomNotJoined(snapshot)`이 참이면 활성 KakaoTalk 계정이 대상 방에 없으므로 같은 방으로 재발급하지 않습니다.

v5와 v6은 다른 모듈 경로라 한 저장소의 서비스가 서로 다른 시점에 전환해도 됩니다.
