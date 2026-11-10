package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nixing178/dbx-plugin-archery/internal/archery"
)

// Expire 模拟 Archery 侧会话超时。
func (f *rpcFake) Expire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.valid = false
}

func TestSessionKeepAliveReturnsSessionState(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/sessionKeepAlive", json.RawMessage(`{"connectionId":"conn-1"}`))
	if rpcErr != nil {
		t.Fatalf("sessionKeepAlive: %+v", rpcErr)
	}
	session, ok := result.(map[string]any)["session"].(*archery.KeepAliveResult)
	if !ok {
		t.Fatalf("session payload = %#v", result)
	}
	if !session.Enabled || !session.Alive || session.Skipped {
		t.Fatalf("session = %+v, want enabled+alive+fresh", session)
	}
	if session.IntervalSec != 120 {
		t.Fatalf("intervalSec = %d, want default 120", session.IntervalSec)
	}
}

func TestSessionKeepAliveRelogsAfterExpiry(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}
	fake.Expire()

	result, rpcErr := service.Handle(ctx, "archery/sessionKeepAlive", json.RawMessage(`{"connectionId":"conn-1"}`))
	if rpcErr != nil {
		t.Fatalf("sessionKeepAlive after expiry: %+v", rpcErr)
	}
	session := result.(map[string]any)["session"].(*archery.KeepAliveResult)
	if !session.Alive || !session.Relogged {
		t.Fatalf("session = %+v, want alive+relogged", session)
	}
	if fake.LoginCount() != 2 {
		t.Fatalf("logins = %d, want 2", fake.LoginCount())
	}
}

func TestSessionKeepAliveRespectsDisabledConfig(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "demo-mysql", "db_type": "mysql"}},
	})
	service := NewService()
	ctx := context.Background()
	params := connectParams(t, fake, func(payload map[string]any) {
		config := payload["connection"].(map[string]any)["external_config"].(map[string]any)
		config["keep_alive_secs"] = 0
	})
	if _, err := service.Handle(ctx, "connection/connect", params); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/sessionKeepAlive", json.RawMessage(`{"connectionId":"conn-1"}`))
	if rpcErr != nil {
		t.Fatalf("sessionKeepAlive: %+v", rpcErr)
	}
	session := result.(map[string]any)["session"].(*archery.KeepAliveResult)
	if session.Enabled {
		t.Fatalf("session = %+v, want disabled", session)
	}
}
