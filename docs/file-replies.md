# 파일 reply 전송

Iris의 `type=file` reply는 JSON 요청이 아니라 `multipart/form-data` 요청입니다. SDK는
metadata manifest, 단일 `file` part, 요청 본문 SHA-256과 재시도용 `GetBody`를 같은
파일 소스에서 결정론적으로 구성합니다.

## 메모리 데이터 전송

```go
payload := []byte("report body")
file := iris.NewReplyFileBytes("report.txt", "text/plain", payload)

accepted, err := client.SendFile(
    ctx,
    roomID,
    file,
    iris.WithClientRequestID("report:2026-07-21:room-123"),
    iris.WithThreadID(threadID),
    iris.WithThreadScope(2),
)
```

`NewReplyFileBytes`는 입력 slice를 복제하지 않습니다. `SendFile`이 반환되기 전까지
slice를 변경하거나 재사용하면 안 됩니다.

## 파일 경로 전송

```go
accepted, err := client.SendFilePath(
    ctx,
    roomID,
    "/var/lib/chatbot/report.pdf",
    "application/pdf",
    iris.WithClientRequestID("report:2026-07-21:room-123"),
)
```

`SendFilePath`는 regular file 하나를 열어 요청이 끝날 때까지 유지하고 모든 반환
경로에서 닫습니다. content type을 빈 문자열로 전달하면 확장자로 추론하며, 알 수 없는
확장자는 `application/octet-stream`을 사용합니다.

## 스트리밍 소스 전송

파일이 이미 열려 있거나 별도 저장소가 `io.ReaderAt`을 제공하는 경우 전체 내용을
메모리에 올리지 않고 전송할 수 있습니다.

```go
info, err := fileHandle.Stat()
if err != nil {
    return err
}

file := iris.NewReplyFile(
    "report.pdf",
    "application/pdf",
    info.Size(),
    fileHandle,
)
accepted, err := client.SendFile(ctx, roomID, file)
```

`ReaderAt`과 그 데이터의 수명은 호출자가 소유합니다. `SendFile`이 반환될 때까지 읽을
수 있어야 하고 내용과 길이가 바뀌지 않아야 합니다. SDK는 source를 닫지 않습니다.

## 계약과 자원 상한

- reply 한 건에는 파일이 정확히 하나만 포함됩니다.
- 파일 크기는 1 byte 이상 30 MiB 이하입니다.
- multipart body는 31 MiB, metadata는 64 KiB 이하입니다.
- 파일명은 UTF-8 기준 최대 255 bytes이며 path separator, control character,
  double quote, semicolon을 허용하지 않습니다.
- MIME type은 parameter 없는 `type/subtype` 형식이며 최대 127 bytes입니다.
- SDK는 파일 전체를 복제하거나 multipart body를 메모리에 버퍼링하지 않습니다.
  manifest SHA-256과 요청 HMAC body SHA-256을 요청 전에 계산한 뒤 실제 요청을
  스트리밍합니다. 따라서 기본 경로는 같은 source를 순차적으로 세 번 읽습니다.
  파일 경로의 후속 읽기는 일반적으로 OS page cache를 사용하며, 최대 파일 크기로 I/O가
  상한 처리됩니다.
- context 취소는 digest 계산 중에도 확인합니다. 짧은 source는 네트워크 요청 전에
  오류로 처리합니다.
- HTTP 429 또는 `clientRequestId`가 있는 요청의 transport 오류를 재시도할 때 같은
  boundary와 metadata를 유지하고 `ReaderAt` 기반 새 body를 생성하므로 요청 서명과
  content length가 결정론적으로 유지됩니다.

기존 `iris.Sender`에는 메서드를 추가하지 않았습니다. 파일 전송은 별도
`iris.FileSender` capability이므로 기존 mock과 사용자 구현의 source compatibility를
보존합니다. `iris.H2CClient`와 `iris.RebindingClient`가 이 capability를 구현합니다.
