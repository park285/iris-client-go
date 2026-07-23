# Webhook routing context

`webhook.Router` is a bounded, first-match dispatcher for already authenticated Iris webhook
messages. It normalizes the wire envelope once into an immutable `webhook.MessageContext` and
passes the caller's `context.Context` as the first handler argument.

```go
router, err := webhook.NewRouter(
    []webhook.MessageRoute{
        webhook.Route(
            func(ctx context.Context, message webhook.MessageContext) {
                handleJoinedMembers(ctx, message.RoomID(), message.EventPayload())
            },
            webhook.MatchEventType(webhook.EventTypeKakaoFeed),
            webhook.MatchEventStatus(webhook.KakaoFeedStatusRecognized),
            webhook.MatchEventKind(webhook.KakaoFeedKindUserJoined),
        ),
        webhook.Route(handleText, webhook.MatchText),
    },
    handleUnmatched,
)
if err != nil {
    return err
}
```

## Wiring the router

In the default in-memory mode, pass the router as the `webhook.NewHandler` handler argument.
The handler authenticates each delivery (including HMAC nonce replay protection) and dispatches
the message to the router on its worker pool. Cross-delivery message deduplication is a no-op
by default; configure a backend through `webhook.WithDeduplicator` to enable it:

```go
handler := webhook.NewHandler(context.Background(), token, router, logger)
```

In durable admission mode, HTTP 200 means the message was durably committed by the
`webhook.MessageAdmitter`; processing is owned by the consumer's inbox loop, not by the HTTP
handler. Use `webhook.NewDurableHandler` for admission and dispatch each stored message through
the router after it is claimed:

```go
handler, err := webhook.NewDurableHandler(ctx, token, inbox, logger)
if err != nil {
    return err
}

// Consumer-owned inbox loop: claim a committed message, restore it as *webhook.Message,
// then dispatch through the router.
for {
    msg, ack, err := inbox.ClaimNext(ctx)
    if err != nil {
        return err
    }
    router.HandleMessage(ctx, msg)
    ack()
}
```

> **Warning:** do not combine a non-nil `webhook.NewHandler` handler argument with
> `webhook.WithDurableAdmission`. In durable admission mode the `MessageHandler` is never
> invoked — `NewHandler` returns before creating the dispatch scheduler, so the handler argument
> becomes dead code. `NewHandler` logs a construction-time warning for this combination; use
> `webhook.NewDurableHandler` when durable admission is intended.

The router intentionally implements `webhook.MessageHandler` only. It does not authenticate,
deduplicate, durably commit, retry, or reorder webhook deliveries.

## Bounds and ordering

- At most `webhook.MaxMessageRoutes` routes may be configured.
- Each route may contain at most `webhook.MaxMessageRoutePredicates` predicates.
- Predicates within a route use AND semantics.
- Routes are evaluated in declaration order and only the first match runs.
- The fallback runs only when no route matches.
- Route and predicate slices are copied during construction, so later caller mutation cannot
  change dispatch behavior.

## Message context

`webhook.MessageContext` snapshots normalized route, room, sender, user, message type, thread,
`StableMessageIdentity`, source-generation, mention and semantic event header fields, including
`schemaVersion`. `EventPayload` returns a copy. `EventType` prefers `eventPayload.type` and falls
back to the raw webhook message type, which permits the same router to handle existing events and
additive semantic events such as `kakao_feed`.

This layer does not change SSE cursor behavior. `EventStreamReconnect` retains its existing
in-memory cursor, bounded channel and reconnect backoff contracts; durable cursor storage remains a
consumer concern and should only be added with a demonstrated SSE consumer requirement.
