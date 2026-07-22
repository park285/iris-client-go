package rebind

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

// RebindingClientConfig는 RebindingClient 구성을 담는다.
// ResolveBaseURL은 호출 중 mutex를 점유하지 않는다. 한 RebindingClient 안에서는 refresh가
// single-flight로 합쳐지지만, 서로 다른 client 인스턴스에서는 동시에 호출될 수 있다.
type RebindingClientConfig struct {
	ResolveBaseURL func() (string, error)
	BotToken       string
	// ResolveInterval 동안 성공 URL 또는 resolver 오류 snapshot을 재사용한다.
	// 0 이하이면 비동시 호출마다 즉시 resolve하는 기존 의미론을 유지한다.
	ResolveInterval time.Duration
	// StaleCloseGrace만큼 기다린 뒤 교체된 이전 클라이언트를 닫는다. 0이면 즉시 닫는다.
	// 진행 중 요청(특히 h3 active conn)이 있는 환경에서는 per-attempt timeout × retry 이상을 권장.
	StaleCloseGrace time.Duration
	Logger          *slog.Logger
	ClientOptions   []ClientOption
}

type rebindRefresh struct {
	done   chan struct{}
	client *H2CClient
	err    error
}

type RebindingClient struct {
	cfg RebindingClientConfig

	mu                sync.Mutex
	cachedURL         string
	cached            *H2CClient
	resolveValidUntil time.Time
	resolveErr        error
	refresh           *rebindRefresh
	closed            bool
	closeSignal       chan struct{}
	staleClosers      sync.WaitGroup
	now               func() time.Time
}

// 생성자는 실패하지 않고 per-call 검증 의미론을 current()에 보존한다.
func NewRebindingClient(cfg RebindingClientConfig) *RebindingClient {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ResolveInterval < 0 {
		cfg.ResolveInterval = 0
	}
	return &RebindingClient{
		cfg:         cfg,
		closeSignal: make(chan struct{}),
		now:         time.Now,
	}
}

func (c *RebindingClient) current(ctx context.Context) (*H2CClient, error) {
	if c.cfg.ResolveBaseURL == nil {
		return nil, fmt.Errorf("iris: rebinding client: resolve base URL func is nil")
	}
	if c.cfg.BotToken == "" {
		return nil, fmt.Errorf("iris: rebinding client: bot token is empty")
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, rebindingClientClosedError()
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if c.resolveSnapshotFreshLocked() {
		if c.resolveErr != nil {
			err := c.resolveErr
			c.mu.Unlock()
			return nil, err
		}
		cached := c.cached
		c.mu.Unlock()
		return cached, nil
	}
	if refresh := c.refresh; refresh != nil {
		c.mu.Unlock()
		return c.waitForRefresh(ctx, refresh)
	}
	refresh := &rebindRefresh{done: make(chan struct{})}
	c.refresh = refresh
	c.mu.Unlock()

	go c.runRefresh(refresh)
	return c.waitForRefresh(ctx, refresh)
}

func (c *RebindingClient) resolveSnapshotFreshLocked() bool {
	return c.cfg.ResolveInterval > 0 &&
		!c.resolveValidUntil.IsZero() &&
		c.now().Before(c.resolveValidUntil)
}

func (c *RebindingClient) waitForRefresh(ctx context.Context, refresh *rebindRefresh) (*H2CClient, error) {
	select {
	case <-refresh.done:
		return refresh.client, refresh.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeSignal:
		return nil, rebindingClientClosedError()
	}
}

func (c *RebindingClient) runRefresh(refresh *rebindRefresh) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.completeRefreshPanic(refresh)
			c.logRecoveredPanic("rebinding_client_refresh_panic_recovered", recovered)
		}
	}()

	c.refreshCurrent(refresh)
}

func (c *RebindingClient) refreshCurrent(refresh *rebindRefresh) {
	baseURL, err := c.cfg.ResolveBaseURL()
	if err != nil {
		c.completeRefreshError(refresh, fmt.Errorf("iris: rebinding client: resolve base URL: %w", err))
		return
	}

	c.mu.Lock()
	if c.closed {
		err := rebindingClientClosedError()
		c.completeRefreshLocked(refresh, nil, err)
		c.mu.Unlock()
		return
	}
	if c.cached != nil && c.cachedURL == baseURL {
		cached := c.cached
		c.completeRefreshLocked(refresh, cached, nil)
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	next := NewH2CClient(baseURL, c.cfg.BotToken, c.cfg.ClientOptions...)
	if err := next.InitError(); err != nil {
		c.completeRefreshError(refresh, fmt.Errorf("iris: rebinding client: initialize %s: %w", baseURL, err))
		return
	}

	c.mu.Lock()
	if c.closed {
		err := rebindingClientClosedError()
		c.completeRefreshLocked(refresh, nil, err)
		c.mu.Unlock()
		_ = next.Close()
		return
	}
	if c.cached != nil && c.cachedURL == baseURL {
		cached := c.cached
		c.completeRefreshLocked(refresh, cached, nil)
		c.mu.Unlock()
		_ = next.Close()
		return
	}
	previous := c.cached
	c.cachedURL = baseURL
	c.cached = next
	c.scheduleStaleCloseLocked(previous)
	c.completeRefreshLocked(refresh, next, nil)
	c.mu.Unlock()
}

func (c *RebindingClient) completeRefreshError(refresh *rebindRefresh, err error) {
	c.mu.Lock()
	if c.closed {
		err = rebindingClientClosedError()
	}
	c.completeRefreshLocked(refresh, nil, err)
	c.mu.Unlock()
}

func (c *RebindingClient) completeRefreshPanic(refresh *rebindRefresh) {
	c.mu.Lock()
	if c.refresh != refresh {
		c.mu.Unlock()
		return
	}
	err := fmt.Errorf("iris: rebinding client: refresh panicked")
	if c.closed {
		err = rebindingClientClosedError()
	}
	c.completeRefreshLocked(refresh, nil, err)
	c.mu.Unlock()
}

func (c *RebindingClient) completeRefreshLocked(refresh *rebindRefresh, cl *H2CClient, err error) {
	if !c.closed && c.cfg.ResolveInterval > 0 {
		c.resolveValidUntil = c.now().Add(c.cfg.ResolveInterval)
		c.resolveErr = err
	} else {
		c.resolveValidUntil = time.Time{}
		c.resolveErr = nil
	}
	refresh.client = cl
	refresh.err = err
	c.refresh = nil
	close(refresh.done)
}

func rebindingClientClosedError() error {
	return fmt.Errorf("iris: rebinding client: client is closed")
}

func (c *RebindingClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	cached := c.cached
	c.cached = nil
	c.cachedURL = ""
	c.resolveValidUntil = time.Time{}
	c.resolveErr = nil
	c.closed = true
	close(c.closeSignal)
	c.mu.Unlock()

	c.staleClosers.Wait()

	if cached == nil {
		return nil
	}

	return cached.Close()
}

// scheduleStaleCloseLocked는 base URL 회전으로 교체된 이전 client를 grace 기간 뒤에
// 닫아, 회전 순간 해당 client로 진행 중이던 요청(특히 active conn을 끊는 h3)이 끝날 시간을
// 준다. RebindingClient.Close()는 closeSignal로 대기 중인 stale close를 즉시 깨운다.
// mu를 잡은 상태에서 호출해야 하며(WaitGroup Add가 Close의 Wait보다 happens-before),
// 실제 teardown은 goroutine에서 lock 밖으로 수행한다.
func (c *RebindingClient) scheduleStaleCloseLocked(cl *H2CClient) {
	if cl == nil {
		return
	}

	c.staleClosers.Add(1)
	go c.runStaleClose(cl, c.cfg.StaleCloseGrace)
}

func (c *RebindingClient) runStaleClose(cl interface{ Close() error }, grace time.Duration) {
	defer c.staleClosers.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			c.logRecoveredPanic("rebinding_client_close_stale_panic_recovered", recovered)
		}
	}()

	if grace > 0 {
		c.awaitStaleCloseGrace(grace)
	}
	c.closeStaleClient(cl)
}

func (c *RebindingClient) logRecoveredPanic(message string, recovered any) {
	if c.cfg.Logger != nil {
		c.cfg.Logger.Error(
			message,
			slog.String("panic_type", fmt.Sprintf("%T", recovered)),
			slog.String("stack", string(debug.Stack())),
		)
	}
}

func (c *RebindingClient) awaitStaleCloseGrace(grace time.Duration) {
	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-c.closeSignal:
	}
}

func (c *RebindingClient) closeStaleClient(cl interface{ Close() error }) {
	if err := cl.Close(); err != nil && c.cfg.Logger != nil {
		c.cfg.Logger.Warn("rebinding_client_close_stale_failed", slog.Any("error", err))
	}
}

// Sender/control/KaringClient 포워딩은 인터페이스 메서드별로 시그니처가 달라 공통 헬퍼로
// 추출할 수 없다(가변 반환 타입·SendOption variadic). 각 메서드는 current(ctx)로 활성 H2CClient를
// 얻어 위임하는 얇은 shim이며, 동일 형태가 의도적이다.
//
//nolint:dupl // 위 사유: 포워딩 shim 군집은 의도적으로 동일 형태다.
func (c *RebindingClient) SendMessage(ctx context.Context, room, message string, opts ...SendOption) error {
	cl, err := c.current(ctx)
	if err != nil {
		return err
	}
	return cl.SendMessage(ctx, room, message, opts...)
}

func (c *RebindingClient) SendMessageAccepted(ctx context.Context, room, message string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendMessageAccepted(ctx, room, message, opts...)
}

func (c *RebindingClient) SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendImage(ctx, room, imageData, opts...)
}

func (c *RebindingClient) SendMultipleImages(ctx context.Context, room string, images [][]byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendMultipleImages(ctx, room, images, opts...)
}

func (c *RebindingClient) SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendMarkdown(ctx, room, markdown, opts...)
}

func (c *RebindingClient) GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetReplyStatus(ctx, requestID)
}

func (c *RebindingClient) Ping(ctx context.Context) bool {
	cl, err := c.current(ctx)
	if err != nil {
		return false
	}
	return cl.Ping(ctx)
}

func (c *RebindingClient) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetConfig(ctx)
}

func (c *RebindingClient) GetRooms(ctx context.Context) (*RoomListResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetRooms(ctx)
}

func (c *RebindingClient) UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.UpdateConfig(ctx, name, req)
}

func (c *RebindingClient) GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetBridgeHealth(ctx)
}

func (c *RebindingClient) GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetNativeCoreDiagnostics(ctx)
}

func (c *RebindingClient) GetRuntimeDiagnostics(ctx context.Context) (jsonx.RawMessage, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetRuntimeDiagnostics(ctx)
}

func (c *RebindingClient) GetChatroomFields(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetChatroomFields(ctx, chatID)
}

func (c *RebindingClient) OpenChatroom(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.OpenChatroom(ctx, chatID)
}

func (c *RebindingClient) GetTextPingDiagnostics(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.GetTextPingDiagnostics(ctx, chatID)
}

func (c *RebindingClient) WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.WarmTextPing(ctx, chatID)
}

func (c *RebindingClient) SendKaring(ctx context.Context, req KaringSendRequest) (*KaringDryRunResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendKaring(ctx, req)
}

func (c *RebindingClient) SendKaringContentList(ctx context.Context, req KaringContentListRequest) (*KaringDryRunResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendKaringContentList(ctx, req)
}

func (c *RebindingClient) SendKaringHololive(ctx context.Context, req KaringHololiveRequest) (*KaringDryRunResponse, error) {
	cl, err := c.current(ctx)
	if err != nil {
		return nil, err
	}
	return cl.SendKaringHololive(ctx, req)
}
