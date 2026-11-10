package archery

import (
	"context"
	"testing"
	"time"
)

// ExpireSession 模拟服务端会话超时：此后所有登录态请求都会被重定向到登录页。
func (fs *fakeServer) ExpireSession() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.validSession = ""
}

func (fs *fakeServer) InstanceCalls() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.instanceCalls
}

// 会话有效时心跳只探测一次，不该多做一次登录。
func TestKeepAliveProbesWithoutRelogin(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	client := newTestClient(t, fs)
	ctx := context.Background()
	if err := client.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	callsBefore := fs.InstanceCalls()

	result, err := client.KeepAlive(ctx)
	if err != nil {
		t.Fatalf("KeepAlive: %v", err)
	}
	if !result.Enabled || !result.Alive {
		t.Fatalf("result = %+v, want enabled+alive", result)
	}
	if result.Relogged || result.Restored || result.Skipped {
		t.Fatalf("result = %+v, want a fresh plain probe", result)
	}
	if calls := fs.InstanceCalls() - callsBefore; calls != 1 {
		t.Fatalf("instance calls = %d, want 1", calls)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("login count = %d, want 1 (no relogin)", fs.LoginCount())
	}
	if result.IntervalSec != 120 {
		t.Fatalf("intervalSec = %d, want 120", result.IntervalSec)
	}
}

// 服务端会话超时后，心跳必须用保存的密码自动重登，让会话重新有效。
func TestKeepAliveRelogsAfterSessionExpiry(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	client := newTestClient(t, fs)
	ctx := context.Background()
	if err := client.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	fs.ExpireSession()

	result, err := client.KeepAlive(ctx)
	if err != nil {
		t.Fatalf("KeepAlive after expiry: %v", err)
	}
	if !result.Alive || !result.Relogged {
		t.Fatalf("result = %+v, want alive+relogged", result)
	}
	if fs.LoginCount() != 2 {
		t.Fatalf("login count = %d, want 2 (one relogin)", fs.LoginCount())
	}
	if !client.authenticated() {
		t.Fatal("client should be authenticated after keepalive relogin")
	}
}

// 间隔内的重复调用直接复用上次结果，避免 UI 高频触发打爆服务端。
func TestKeepAliveThrottlesRepeatedCalls(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	client := newTestClient(t, fs)
	ctx := context.Background()
	if err := client.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	if _, err := client.KeepAlive(ctx); err != nil {
		t.Fatalf("first KeepAlive: %v", err)
	}
	callsBefore := fs.InstanceCalls()
	second, err := client.KeepAlive(ctx)
	if err != nil {
		t.Fatalf("second KeepAlive: %v", err)
	}
	if !second.Skipped || !second.Alive {
		t.Fatalf("second result = %+v, want skipped+alive", second)
	}
	if calls := fs.InstanceCalls() - callsBefore; calls != 0 {
		t.Fatalf("instance calls during throttle = %d, want 0", calls)
	}
}

// 配置里显式关闭自动续期时，心跳不发起任何网络请求。
func TestKeepAliveDisabled(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	client := newTestClient(t, fs, func(cfg *Config) { cfg.KeepAlive = -time.Second })
	ctx := context.Background()
	if err := client.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	callsBefore := fs.InstanceCalls()

	result, err := client.KeepAlive(ctx)
	if err != nil {
		t.Fatalf("KeepAlive: %v", err)
	}
	if result.Enabled {
		t.Fatalf("result = %+v, want disabled", result)
	}
	if calls := fs.InstanceCalls() - callsBefore; calls != 0 {
		t.Fatalf("instance calls = %d, want 0 when disabled", calls)
	}
}

// sidecar 重启后首次调用就是心跳：应先用落盘会话恢复，而不是直接密码登录。
func TestKeepAliveRestoresPersistedSession(t *testing.T) {
	fs := newFake(t, fakeOptions{
		instances: []any{map[string]any{"id": 1, "instance_name": "prod", "db_type": "mysql"}},
	})
	store := NewFileSessionStore(t.TempDir() + "/session.json")

	first := newTestClient(t, fs, func(cfg *Config) { cfg.SessionStore = store })
	if err := first.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	second := newTestClient(t, fs, func(cfg *Config) { cfg.SessionStore = store })
	result, err := second.KeepAlive(context.Background())
	if err != nil {
		t.Fatalf("KeepAlive after restart: %v", err)
	}
	if !result.Alive || !result.Restored || result.Relogged {
		t.Fatalf("result = %+v, want alive+restored", result)
	}
	if fs.LoginCount() != 1 {
		t.Fatalf("login count = %d, want 1 (restored, not relogin)", fs.LoginCount())
	}
}
