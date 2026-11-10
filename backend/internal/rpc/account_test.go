package rpc

import (
	"context"
	"strings"
	"testing"
)

func TestTwoFactorStateRPC(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{
		twoFAState: map[string]any{"totp": "enabled", "sms": "disabled"},
	})
	service := connectFake(t, fake, nil)

	result, err := service.Handle(context.Background(), "archery/user/twoFactorState", simpleParams(t, nil))
	if err != nil {
		t.Fatalf("twoFactorState: %+v", err)
	}
	state, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", result)
	}
	if state["totp"] != true || state["sms"] != false {
		t.Fatalf("state = %#v", state)
	}
	if got := fake.TwoFAForm().Get("engineer"); got != "tester" {
		t.Fatalf("engineer = %q", got)
	}
}

func TestTwoFactorDisableRPC(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	if _, err := service.Handle(context.Background(), "archery/user/twoFactorDisable", simpleParams(t, map[string]any{
		"authType": "totp",
	})); err != nil {
		t.Fatalf("twoFactorDisable: %+v", err)
	}
	form := fake.TwoFAForm()
	if form.Get("enable") != "false" || form.Get("auth_type") != "totp" {
		t.Fatalf("form = %v", form)
	}
}

func TestTwoFactorDisableRequiresAuthType(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	_, err := service.Handle(context.Background(), "archery/user/twoFactorDisable", simpleParams(t, nil))
	if err == nil || err.Code != -32602 {
		t.Fatalf("expected invalid params, got %+v", err)
	}
}

func TestChangePasswordRPC(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	if _, err := service.Handle(context.Background(), "archery/user/changePassword", simpleParams(t, map[string]any{
		"oldPassword": "secret",
		"newPassword": "newpassword123",
	})); err != nil {
		t.Fatalf("changePassword: %+v", err)
	}
	form := fake.PasswordForm()
	if form.Get("old_password") != "secret" || form.Get("new_password1") != "newpassword123" {
		t.Fatalf("form = %v", form)
	}
}

func TestChangePasswordRequiresBothPasswords(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, nil)

	for _, params := range []map[string]any{
		{"newPassword": "newpassword123"},
		{"oldPassword": "secret"},
		{"oldPassword": "  ", "newPassword": "newpassword123"},
	} {
		_, err := service.Handle(context.Background(), "archery/user/changePassword", simpleParams(t, params))
		if err == nil || err.Code != -32602 {
			t.Fatalf("params %v: expected invalid params, got %+v", params, err)
		}
	}
}

func TestChangePasswordNonStaffAccount(t *testing.T) {
	fake := newRPCFake(t, rpcFakeOptions{passwordRequiresStaff: true})
	service := connectFake(t, fake, nil)

	_, err := service.Handle(context.Background(), "archery/user/changePassword", simpleParams(t, map[string]any{
		"oldPassword": "secret",
		"newPassword": "newpassword123",
	}))
	if err == nil || err.Kind != "forbidden" {
		t.Fatalf("expected forbidden, got %+v", err)
	}
	if !strings.Contains(err.Message, "后台") {
		t.Fatalf("message = %q", err.Message)
	}
}

func TestAccountWritesBlockedInReadOnly(t *testing.T) {
	reads := []string{"archery/user/twoFactorState"}
	writes := []struct {
		method string
		params map[string]any
	}{
		{"archery/user/twoFactorStart", map[string]any{"authType": "totp"}},
		{"archery/user/twoFactorConfirm", map[string]any{"authType": "totp", "otp": "123456", "secretKey": "SECRETKEY"}},
		{"archery/user/twoFactorDisable", map[string]any{"authType": "totp"}},
		{"archery/user/changePassword", map[string]any{"oldPassword": "secret", "newPassword": "newpassword123"}},
	}

	fake := newRPCFake(t, rpcFakeOptions{})
	service := connectFake(t, fake, func(payload map[string]any) {
		config := payload["connection"].(map[string]any)["external_config"].(map[string]any)
		config["read_only"] = true
	})

	for _, method := range reads {
		if _, err := service.Handle(context.Background(), method, simpleParams(t, nil)); err != nil && err.Kind == "read_only" {
			t.Fatalf("%s must stay available in read-only mode: %+v", method, err)
		}
	}
	for _, item := range writes {
		_, err := service.Handle(context.Background(), item.method, simpleParams(t, item.params))
		if err == nil || err.Kind != "read_only" {
			t.Fatalf("%s: expected read_only, got %+v", item.method, err)
		}
	}
}
