package valkeydedup_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/park285/iris-client-go/adapters/valkeydedup"
)

const pipeTimeout = 5 * time.Second

func TestNonceStoreIsDuplicateFirstSeen(t *testing.T) {
	t.Parallel()

	client := newSetReplyClient(t, "300000", "+OK")
	deduplicator := valkeydedup.NewNonceStore(client)

	ctx, cancel := context.WithTimeout(t.Context(), pipeTimeout)
	defer cancel()

	duplicate, err := deduplicator.IsDuplicate(ctx, "message:1", 5*time.Minute)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v, want nil", err)
	}

	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}
}

func TestNonceStoreIsDuplicateFloorsSubMillisecondTTL(t *testing.T) {
	t.Parallel()

	for ttl, wantMillis := range map[time.Duration]string{
		500 * time.Microsecond:  "1",
		0:                       "1",
		-time.Second:            "1",
		1500 * time.Millisecond: "1500",
	} {
		client := newMockValkeyClient(t)
		deduplicator := valkeydedup.NewNonceStore(client)

		if _, err := deduplicator.IsDuplicate(t.Context(), "message:1", ttl); err != nil {
			t.Fatalf("IsDuplicate(ttl=%v) error = %v", ttl, err)
		}

		wantCommands := []string{"SET", "message:1", "1", "NX", "PX", wantMillis}
		if !slices.Equal(client.commands, wantCommands) {
			t.Fatalf("commands for ttl=%v = %v, want %v", ttl, client.commands, wantCommands)
		}
	}
}

func TestNonceStoreIsDuplicateDuplicateKey(t *testing.T) {
	t.Parallel()

	client := newMockValkeyClient(t)

	client.result = valkey.NewResult(valkey.ValkeyMessage(*valkey.Nil), nil)

	deduplicator := valkeydedup.NewNonceStore(client)

	duplicate, err := deduplicator.IsDuplicate(t.Context(), "message:1", time.Minute)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v, want nil", err)
	}

	if !duplicate {
		t.Fatal("IsDuplicate() duplicate = false, want true")
	}
}

func TestNonceStoreIsDuplicateClientError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	client := newMockValkeyClient(t)

	client.result = valkey.NewErrorResult(boom)

	deduplicator := valkeydedup.NewNonceStore(client)

	duplicate, err := deduplicator.IsDuplicate(t.Context(), "message:1", time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("IsDuplicate() error = %v, want wrapping %v", err, boom)
	}

	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}
}

func TestNonceStoreIsDuplicateServerError(t *testing.T) {
	t.Parallel()

	const denied = "NOPERM SET denied"

	client := newSetReplyClient(t, "60000", "-"+denied)

	ctx, cancel := context.WithTimeout(t.Context(), pipeTimeout)
	defer cancel()

	duplicate, err := valkeydedup.NewNonceStore(client).IsDuplicate(ctx, "message:1", time.Minute)
	if duplicate {
		t.Error("IsDuplicate() duplicate = true, want false")
	}

	if cause, ok := errors.AsType[*valkey.ValkeyError](errors.Unwrap(err)); !ok || cause.Error() != denied {
		t.Fatalf("IsDuplicate() error = %v, want wrapping *valkey.ValkeyError %q", err, denied)
	}
}

func newSetReplyClient(t *testing.T, wantMillis, setReply string) valkey.Client {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	closeConn := func(conn net.Conn) {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}

	var server sync.WaitGroup

	server.Go(func() {
		defer closeConn(serverConn)

		if err := serveSetReply(serverConn, wantMillis, setReply); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() {
		closeConn(clientConn)
		server.Wait()
	})

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:       []string{"valkey.test:6379"},
		ForceSingleClient: true,
		PipelineMultiplex: -1,
		ConnWriteTimeout:  pipeTimeout,
		Dialer:            net.Dialer{KeepAlive: -1},
		DisableRetry:      true,
		DisableCache:      true,
		ClientSetInfo:     valkey.DisableClientSetInfo,
		DialCtxFn: func(context.Context, string, *net.Dialer, *tls.Config) (net.Conn, error) {
			return clientConn, nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	t.Cleanup(client.Close)

	return client
}

func serveSetReply(conn net.Conn, wantMillis, setReply string) error {
	reader := bufio.NewReader(conn)

	for _, step := range []struct {
		want  []string
		reply string
	}{
		{[]string{"HELLO", "3"}, "%1\r\n+proto\r\n:3"},
		{[]string{"SET", "message:1", "1", "NX", "PX", wantMillis}, setReply},
	} {
		if err := conn.SetReadDeadline(time.Now().Add(pipeTimeout)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}

		got, err := readRESPCommand(reader)
		if err != nil {
			return fmt.Errorf("read %s: %w", step.want[0], err)
		}

		if !slices.Equal(got, step.want) {
			return fmt.Errorf("received %q, want %q", got, step.want)
		}

		if _, err := io.WriteString(conn, step.reply+"\r\n"); err != nil {
			return fmt.Errorf("reply to %s: %w", got[0], err)
		}
	}

	return nil
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	count, err := readRESPLength(reader, '*')
	if err != nil {
		return nil, err
	}

	var args []string

	for range count {
		size, err := readRESPLength(reader, '$')
		if err != nil {
			return nil, err
		}

		var arg strings.Builder

		if _, err = io.CopyN(&arg, reader, int64(size)); err != nil {
			return nil, fmt.Errorf("read bulk string: %w", err)
		}

		end, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read bulk string terminator: %w", err)
		}

		if end != "\r\n" {
			return nil, fmt.Errorf("invalid bulk string terminator %q", end)
		}

		args = append(args, arg.String())
	}

	return args, nil
}

func readRESPLength(reader *bufio.Reader, prefix byte) (int, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("read RESP length: %w", err)
	}

	header, ok := strings.CutSuffix(line, "\r\n")
	if !ok || len(header) < 2 || header[0] != prefix {
		return 0, fmt.Errorf("invalid RESP header %q, want %c length", line, prefix)
	}

	size, err := strconv.Atoi(header[1:])
	if err != nil || size < 0 {
		return 0, fmt.Errorf("invalid RESP length %q", header[1:])
	}

	return size, nil
}

func TestNonceStoreIsDuplicateContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := newMockValkeyClient(t)
	deduplicator := valkeydedup.NewNonceStore(client)

	duplicate, err := deduplicator.IsDuplicate(ctx, "message:1", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IsDuplicate() error = %v, want wrapping %v", err, context.Canceled)
	}

	if duplicate {
		t.Fatal("IsDuplicate() duplicate = true, want false")
	}
}

type mockValkeyClient struct {
	valkey.Client

	result   valkey.ValkeyResult
	commands []string
}

func newMockValkeyClient(t *testing.T) *mockValkeyClient {
	t.Helper()

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:       []string{"valkey.test:6379"},
		ForceSingleClient: true,
		DialCtxFn: func(context.Context, string, *net.Dialer, *tls.Config) (net.Conn, error) {
			return nil, net.ErrClosed
		},
	})
	if !errors.Is(err, net.ErrClosed) || client == nil {
		t.Fatalf("NewClient() with closed dialer: client = %T, err = %v", client, err)
	}

	t.Cleanup(client.Close)

	return &mockValkeyClient{Client: client}
}

func (c *mockValkeyClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	c.commands = slices.Clone(cmd.Commands())

	if err := ctx.Err(); err != nil {
		return valkey.NewErrorResult(err)
	}

	return c.result
}
