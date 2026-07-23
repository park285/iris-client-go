package webhook

import (
	"context"
	"errors"
	"strings"
)

const (
	MaxMessageRoutes          = 128
	MaxMessageRoutePredicates = 16
)

var (
	ErrMessageRouteHandlerRequired   = errors.New("webhook: message route handler is required")
	ErrMessageRoutePredicateRequired = errors.New("webhook: message route predicate is required")
	ErrTooManyMessageRoutes          = errors.New("webhook: too many message routes")
	ErrTooManyMessageRoutePredicates = errors.New("webhook: too many message route predicates")
)

var (
	_ MessageHandler = MessageHandlerFunc(nil)
	_ MessageHandler = (*Router)(nil)
)

type MessageHandlerFunc func(context.Context, *Message)

func (f MessageHandlerFunc) HandleMessage(ctx context.Context, message *Message) {
	if f != nil {
		f(ctx, message)
	}
}

type ContextHandlerFunc func(context.Context, MessageContext)
type MessagePredicate func(MessageContext) bool

type MessageRoute struct {
	handler    ContextHandlerFunc
	predicates []MessagePredicate
}

func Route(handler ContextHandlerFunc, predicates ...MessagePredicate) MessageRoute {
	return MessageRoute{handler: handler, predicates: append([]MessagePredicate(nil), predicates...)}
}

type Router struct {
	routes   []MessageRoute
	fallback ContextHandlerFunc
}

func NewRouter(routes []MessageRoute, fallback ContextHandlerFunc) (*Router, error) {
	if len(routes) > MaxMessageRoutes {
		return nil, ErrTooManyMessageRoutes
	}
	copied := make([]MessageRoute, len(routes))
	for i, route := range routes {
		if route.handler == nil {
			return nil, ErrMessageRouteHandlerRequired
		}
		if len(route.predicates) > MaxMessageRoutePredicates {
			return nil, ErrTooManyMessageRoutePredicates
		}
		if len(route.predicates) == 0 {
			return nil, ErrMessageRoutePredicateRequired
		}
		predicates := append([]MessagePredicate(nil), route.predicates...)
		for _, predicate := range predicates {
			if predicate == nil {
				return nil, ErrMessageRoutePredicateRequired
			}
		}
		copied[i] = MessageRoute{handler: route.handler, predicates: predicates}
	}
	return &Router{routes: copied, fallback: fallback}, nil
}

func (r *Router) HandleMessage(ctx context.Context, message *Message) {
	if r == nil {
		return
	}
	messageContext := NewMessageContext(message)
	for _, route := range r.routes {
		if route.matches(messageContext) {
			route.handler(ctx, messageContext)
			return
		}
	}
	if r.fallback != nil {
		r.fallback(ctx, messageContext)
	}
}

func (r MessageRoute) matches(ctx MessageContext) bool {
	for _, predicate := range r.predicates {
		if !predicate(ctx) {
			return false
		}
	}
	return true
}

func MatchText(ctx MessageContext) bool { return ctx.IsText() }

func MatchRoute(routes ...string) MessagePredicate {
	return matchNormalized(routes, MessageContext.Route)
}

func MatchMessageType(messageTypes ...string) MessagePredicate {
	return matchNormalized(messageTypes, MessageContext.MessageType)
}

func MatchEventType(eventTypes ...string) MessagePredicate {
	return matchNormalized(eventTypes, MessageContext.EventType)
}

func MatchEventKind(eventKinds ...string) MessagePredicate {
	return matchNormalized(eventKinds, MessageContext.EventKind)
}

func MatchEventStatus(eventStatuses ...string) MessagePredicate {
	return matchNormalized(eventStatuses, MessageContext.EventStatus)
}

func matchNormalized(values []string, field func(MessageContext) string) MessagePredicate {
	accepted := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := strings.TrimSpace(value); normalized != "" {
			accepted[normalized] = struct{}{}
		}
	}
	return func(ctx MessageContext) bool {
		_, ok := accepted[field(ctx)]
		return ok
	}
}
