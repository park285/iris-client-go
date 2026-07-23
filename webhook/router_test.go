package webhook

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouterUsesFirstMatchingRoute(t *testing.T) {
	calls := []string{}
	router, err := NewRouter([]MessageRoute{
		Route(func(context.Context, MessageContext) { calls = append(calls, "feed") }, MatchEventType(EventTypeKakaoFeed), MatchEventKind(KakaoFeedKindUserJoined)),
		Route(func(context.Context, MessageContext) { calls = append(calls, "generic") }, MatchEventType(EventTypeKakaoFeed)),
	}, func(context.Context, MessageContext) { calls = append(calls, "fallback") })
	if err != nil {
		t.Fatal(err)
	}
	router.HandleMessage(context.Background(), &Message{JSON: &MessageJSON{EventPayload: []byte(`{"type":"kakao_feed","kind":"user_joined"}`)}})
	if len(calls) != 1 || calls[0] != "feed" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestRouterFallbackAndMessageHandlerFunc(t *testing.T) {
	called := false
	router, err := NewRouter(nil, func(_ context.Context, ctx MessageContext) { called = ctx.RoomID() == "7" })
	if err != nil {
		t.Fatal(err)
	}
	var handler MessageHandlerFunc = router.HandleMessage
	handler.HandleMessage(context.Background(), &Message{Room: "7"})
	if !called {
		t.Fatal("fallback was not called")
	}
}

func TestRouterCopiesRouteConfiguration(t *testing.T) {
	calls := 0
	predicates := []MessagePredicate{MatchText}
	routes := []MessageRoute{Route(func(context.Context, MessageContext) { calls++ }, predicates...)}
	router, err := NewRouter(routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	predicates[0] = func(MessageContext) bool { return false }
	routes[0] = MessageRoute{}
	router.HandleMessage(context.Background(), &Message{})
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRouterRejectsInvalidOrUnboundedRoutes(t *testing.T) {
	_, err := NewRouter([]MessageRoute{Route(nil, MatchText)}, nil)
	if !errors.Is(err, ErrMessageRouteHandlerRequired) {
		t.Fatalf("err=%v", err)
	}
	_, err = NewRouter([]MessageRoute{Route(func(context.Context, MessageContext) {})}, nil)
	if !errors.Is(err, ErrMessageRoutePredicateRequired) {
		t.Fatalf("err=%v", err)
	}
	predicates := make([]MessagePredicate, MaxMessageRoutePredicates+1)
	for i := range predicates {
		predicates[i] = MatchText
	}
	_, err = NewRouter([]MessageRoute{Route(func(context.Context, MessageContext) {}, predicates...)}, nil)
	if !errors.Is(err, ErrTooManyMessageRoutePredicates) {
		t.Fatalf("err=%v", err)
	}

	routes := make([]MessageRoute, MaxMessageRoutes+1)
	for i := range routes {
		routes[i] = Route(func(context.Context, MessageContext) {}, MatchText)
	}
	_, err = NewRouter(routes, nil)
	if !errors.Is(err, ErrTooManyMessageRoutes) {
		t.Fatalf("err=%v", err)
	}
}

func TestMatchPredicatesNormalizeConfiguration(t *testing.T) {
	ctx := NewMessageContext(&Message{JSON: &MessageJSON{Route: " r ", Type: " 1 ", EventPayload: []byte(`{"type":"kakao_feed","status":" recognized "}`)}})
	for name, predicate := range map[string]MessagePredicate{
		"route":  MatchRoute(" r "),
		"type":   MatchMessageType(" 1 "),
		"text":   MatchText,
		"status": MatchEventStatus(" recognized "),
	} {
		if !predicate(ctx) {
			t.Fatalf("%s did not match", name)
		}
	}
	if MatchRoute(" ")(ctx) {
		t.Fatal("blank match configuration must fail closed")
	}
}

func TestRouterPreservesContextAndSupportsConcurrentDispatch(t *testing.T) {
	var calls atomic.Int64
	router, err := NewRouter(
		[]MessageRoute{Route(func(ctx context.Context, _ MessageContext) {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Errorf("context error=%v", ctx.Err())
			}
			calls.Add(1)
		}, MatchText)},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			router.HandleMessage(ctx, &Message{})
		}()
	}
	wait.Wait()
	if got := calls.Load(); got != 64 {
		t.Fatalf("calls=%d", got)
	}
}
