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

handler := webhook.NewHandler(
    context.Background(),
    token,
    router,
    logger,
    webhook.WithDurableAdmission(inbox),
)
```

The router intentionally implements `webhook.MessageHandler` only. It does not authenticate,
deduplicate, durably commit, retry, or reorder webhook deliveries. Services that require durable
admission must keep their existing `webhook.MessageAdmitter` and pass it through
`webhook.WithDurableAdmission` before using the router for post-admission dispatch.

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
`StableMessageIdentity`, source-generation, mention and semantic event header fields.
`EventPayload` returns a copy. `EventType` prefers `eventPayload.type` and falls back to the raw
webhook message type, which permits the same router to handle existing events and additive
semantic events such as `kakao_feed`.

This layer does not change SSE cursor behavior. `EventStreamReconnect` retains its existing
in-memory cursor, bounded channel and reconnect backoff contracts; durable cursor storage remains a
consumer concern and should only be added with a demonstrated SSE consumer requirement.
