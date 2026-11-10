//go:build live

package archery

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveArchery 只在显式提供环境变量时运行：
//
//	$env:ARCHERY_URL="https://archery.example.com"; $env:ARCHERY_USER="demo_user"; go test -tags live ./internal/archery -run TestLive -v
//
// 不提供 ARCHERY_PASSWORD 时只验证 CSRF/登录握手（预期得到 auth_required），不会真正登录。
func TestLiveArchery(t *testing.T) {
	baseURL := os.Getenv("ARCHERY_URL")
	username := os.Getenv("ARCHERY_USER")
	password := os.Getenv("ARCHERY_PASSWORD")
	if baseURL == "" || username == "" {
		t.Skip("ARCHERY_URL / ARCHERY_USER 未设置")
	}

	client, err := NewClient(Config{
		BaseURL:      baseURL,
		Username:     username,
		Password:     password,
		Timeout:      15 * time.Second,
		QueryTimeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.CloseIdleConnections()

	ctx := context.Background()
	version, err := client.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	t.Logf("Archery version = %s", version)

	if password == "" {
		err := client.EnsureSession(ctx)
		if !IsKind(err, KindAuthRequired) {
			t.Fatalf("expected auth_required from CSRF handshake, got %v", err)
		}
		t.Logf("CSRF/login handshake reached Archery: %v", err)
		return
	}

	if err := client.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	instances, err := client.Instances(ctx)
	if err != nil {
		t.Fatalf("Instances: %v", err)
	}
	if len(instances) == 0 {
		t.Fatalf("no accessible instance for %s", username)
	}
	t.Logf("accessible instances = %d (first: %s)", len(instances), instances[0].Name)

	databases, err := client.Resources(ctx, InstanceRef{ID: instances[0].ID, Name: instances[0].Name}, "database", "", "", "")
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	if len(databases) == 0 {
		t.Fatalf("no database on instance %s", instances[0].Name)
	}
	t.Logf("databases on %s = %d (first: %s)", instances[0].Name, len(databases), databases[0].Name)

	result, err := client.Query(ctx, QueryRequest{
		InstanceName: instances[0].Name,
		DBName:       databases[0].Name,
		SQL:          "select 1 as ok",
		LimitNum:     10,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	t.Logf("query ok: columns=%v rows=%v elapsed=%dms", result.Columns, result.Rows, result.ElapsedMS)
}
