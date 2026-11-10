package rpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nixing178/dbx-plugin-archery/internal/archery"
)

// 公告接口只抓 Archery 首页并解析 alert 块，作为所有页面共用的单一数据源。
func TestAnnouncementParsesDashboardAlert(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		dashboardHTML: `<html><body>
<div class="alert alert-warning alert-dismissible">
  <button type="button" class="close" data-dismiss="alert">×</button>
  <h4><i class="icon fa fa-warning"></i> 注意!</h4>
  <p>核心表保持精简，尽量不加字段。</p>
</div>
</body></html>`,
	})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/announcement", simpleParams(t, nil))
	if rpcErr != nil {
		t.Fatalf("announcement: %+v", rpcErr)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", result)
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	var decoded struct {
		Announcement *archery.Announcement `json:"announcement"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	if decoded.Announcement == nil {
		t.Fatalf("announcement = nil, payload = %s", encoded)
	}
	if decoded.Announcement.Level != "warning" {
		t.Errorf("Level = %q, want warning", decoded.Announcement.Level)
	}
	if decoded.Announcement.Title != "注意!" {
		t.Errorf("Title = %q, want 注意!", decoded.Announcement.Title)
	}
	if !strings.Contains(decoded.Announcement.Content, "核心表保持精简") {
		t.Errorf("Content = %q", decoded.Announcement.Content)
	}
}

// 首页取不到时公告接口静默降级：所有页面继续渲染，只是不显示公告条。
func TestAnnouncementDegradesWhenDashboardUnavailable(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := NewService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, "connection/connect", connectParams(t, fake, nil)); err != nil {
		t.Fatalf("connect: %+v", err)
	}

	result, rpcErr := service.Handle(ctx, "archery/announcement", simpleParams(t, nil))
	if rpcErr != nil {
		t.Fatalf("announcement: %+v", rpcErr)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", result)
	}
	if payload["degraded"] != true {
		t.Errorf("degraded = %#v, want true", payload["degraded"])
	}
	if reason, _ := payload["reason"].(string); reason == "" {
		t.Error("reason should be set when the dashboard page is unavailable")
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	var decoded struct {
		Announcement *archery.Announcement `json:"announcement"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
		t.Fatalf("unmarshal: %v", unmarshalErr)
	}
	if decoded.Announcement != nil {
		t.Errorf("announcement = %+v, want nil", decoded.Announcement)
	}
}
