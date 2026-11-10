package archery

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestTwoFactorStateParsesChannels(t *testing.T) {
	fs := newFake(t, fakeOptions{
		twoFAState: map[string]any{"totp": "enabled", "sms": "disabled"},
	})
	client := newTestClient(t, fs)
	ctx := context.Background()

	state, err := client.TwoFactorState(ctx)
	if err != nil {
		t.Fatalf("TwoFactorState: %v", err)
	}
	if !state.TOTP || state.SMS {
		t.Fatalf("state = %+v, want totp enabled / sms disabled", state)
	}
	if got := fs.TwoFAForm().Get("engineer"); got != "tester" {
		t.Fatalf("engineer = %q, want tester", got)
	}
}

func TestStartTOTPEnrollmentReturnsQRCode(t *testing.T) {
	fs := newFake(t, fakeOptions{
		twoFAStartData: map[string]any{"auth_type": "totp", "key": "SECRETKEY"},
		qrPNG:          []byte("\x89PNG\r\n\x1a\nimage"),
	})
	client := newTestClient(t, fs)

	enrollment, err := client.StartTOTPEnrollment(context.Background())
	if err != nil {
		t.Fatalf("StartTOTPEnrollment: %v", err)
	}
	if enrollment.SecretKey != "SECRETKEY" {
		t.Fatalf("secret = %q", enrollment.SecretKey)
	}
	if !strings.HasPrefix(enrollment.QRCode, "data:image/png;base64,") {
		t.Fatalf("qr = %q", enrollment.QRCode)
	}
	form := fs.TwoFAForm()
	if form.Get("enable") != "true" || form.Get("auth_type") != "totp" {
		t.Fatalf("form = %v", form)
	}
}

func TestSendSMSCodeRequiresPhone(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	err := client.SendSMSCode(context.Background(), "   ")
	if !IsKind(err, KindInvalidConfig) {
		t.Fatalf("expected invalid_config, got %v", err)
	}
}

func TestSendSMSCodeSurfacesServerMessage(t *testing.T) {
	fs := newFake(t, fakeOptions{
		twoFAStartStatus: 1,
		twoFAStartMsg:    "系统未配置短信服务商！",
	})
	client := newTestClient(t, fs)

	err := client.SendSMSCode(context.Background(), "13800000000")
	if err == nil || !strings.Contains(err.Error(), "短信服务商") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fs.TwoFAForm().Get("phone"); got != "13800000000" {
		t.Fatalf("phone = %q", got)
	}
}

func TestConfirmTwoFactorVerifiesThenSaves(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	if err := client.ConfirmTwoFactor(context.Background(), "totp", fakeOTP, "SECRETKEY", ""); err != nil {
		t.Fatalf("ConfirmTwoFactor: %v", err)
	}
	form := fs.TwoFASaveForm()
	if form.Get("auth_type") != "totp" || form.Get("key") != "SECRETKEY" {
		t.Fatalf("save form = %v", form)
	}
}

func TestConfirmTwoFactorStopsOnInvalidCode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	err := client.ConfirmTwoFactor(context.Background(), "totp", "000000", "SECRETKEY", "")
	if err == nil || !strings.Contains(err.Error(), "验证码") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fs.TwoFASaveForm(); len(got) != 0 {
		t.Fatalf("save must not run after failed verification: %v", got)
	}
}

func TestDisableTwoFactorSendsDisableFlag(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	if err := client.DisableTwoFactor(context.Background(), "sms"); err != nil {
		t.Fatalf("DisableTwoFactor: %v", err)
	}
	form := fs.TwoFAForm()
	if form.Get("enable") != "false" || form.Get("auth_type") != "sms" {
		t.Fatalf("form = %v", form)
	}
}

func TestChangePasswordPostsDjangoAdminForm(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs)

	if err := client.ChangePassword(context.Background(), "secret", "newpassword123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	form := fs.PasswordForm()
	if form.Get("old_password") != "secret" || form.Get("new_password1") != "newpassword123" ||
		form.Get("new_password2") != "newpassword123" {
		t.Fatalf("form = %v", form)
	}
	if form.Get("csrfmiddlewaretoken") == "" {
		t.Fatal("csrfmiddlewaretoken missing")
	}
}

func TestChangePasswordReturnsDjangoValidationErrors(t *testing.T) {
	fs := newFake(t, fakeOptions{
		passwordBody: `<html><body>
<div class="errornote">请更正下列错误。</div>
<ul class="errorlist"><li>你的密码必须包含至少 9 个字符。</li></ul>
</body></html>`,
	})
	client := newTestClient(t, fs)

	err := client.ChangePassword(context.Background(), "secret", "short")
	if err == nil || !strings.Contains(err.Error(), "9 个字符") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChangePasswordRejectsNonStaffAccount(t *testing.T) {
	fs := newFake(t, fakeOptions{passwordRequires: true})
	client := newTestClient(t, fs)

	err := client.ChangePassword(context.Background(), "secret", "newpassword123")
	if !IsKind(err, KindForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestChangePasswordReportsExpiredSession(t *testing.T) {
	fs := newFake(t, fakeOptions{passwordSessionLost: true})
	client := newTestClient(t, fs)

	err := client.ChangePassword(context.Background(), "secret", "newpassword123")
	if !IsKind(err, KindAuthRequired) {
		t.Fatalf("expected auth_required, got %v", err)
	}
}

func TestChangePasswordRequiresSessionMode(t *testing.T) {
	fs := newFake(t, fakeOptions{})
	client := newTestClient(t, fs, func(cfg *Config) {
		cfg.AuthMode = "jwt"
	})

	err := client.ChangePassword(context.Background(), "secret", "newpassword123")
	if !IsKind(err, KindUnsupported) {
		t.Fatalf("expected unsupported, got %v", err)
	}
}

func TestChangePasswordHandlesCSRFFailure(t *testing.T) {
	fs := newFake(t, fakeOptions{passwordStatus: http.StatusForbidden})
	client := newTestClient(t, fs)

	err := client.ChangePassword(context.Background(), "secret", "newpassword123")
	if !IsKind(err, KindForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
