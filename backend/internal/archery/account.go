package archery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const passwordChangePath = "/admin/password_change/"

// TwoFactorState 是 Archery 两步验证的两个通道是否已绑定。
// Archery 的 state 接口只判断配置记录是否存在，因此 "enabled" 等价于「已绑定」。
type TwoFactorState struct {
	TOTP bool
	SMS  bool
}

// TwoFactorEnrollment 是待确认的绑定：TOTP 带密钥与二维码，短信只带手机号。
type TwoFactorEnrollment struct {
	AuthType  string
	SecretKey string
	QRCode    string
	Phone     string
}

type twoFAResponse struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

// Django admin 的改密失败页用 errorlist / errornote 呈现校验错误。
var (
	djangoErrorListPattern = regexp.MustCompile(`(?is)<ul[^>]*class\s*=\s*"[^"]*\berrorlist\b[^"]*"[^>]*>(.*?)</ul>`)
	djangoErrorItemPattern = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	djangoErrorNotePattern = regexp.MustCompile(`(?is)<p[^>]*class\s*=\s*"[^"]*\berrornote\b[^"]*"[^>]*>(.*?)</p>`)
)

// TwoFactorState 查询当前账号两步验证的绑定状态。
func (c *Client) TwoFactorState(ctx context.Context) (TwoFactorState, error) {
	if !c.cfg.SessionMode() {
		return TwoFactorState{}, apiError(KindUnsupported, "两步验证仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("engineer", c.cfg.Username)

	var payload struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			TOTP string `json:"totp"`
			SMS  string `json:"sms"`
		} `json:"data"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/user/2fa/state/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return TwoFactorState{}, err
	}
	if payload.Status != 0 {
		return TwoFactorState{}, apiError(KindServer, fallback(payload.Msg, "无法获取两步验证状态"))
	}
	return TwoFactorState{
		TOTP: payload.Data.TOTP == "enabled",
		SMS:  payload.Data.SMS == "enabled",
	}, nil
}

// StartTOTPEnrollment 生成 TOTP 密钥并取回二维码。
// Archery 只在 /api/v1/user/2fa/ 返回密钥，二维码要再单独请求一次，
// 插件侧把它转成 data URL，避免 iframe 里拿不到 sidecar 的会话 Cookie。
func (c *Client) StartTOTPEnrollment(ctx context.Context) (TwoFactorEnrollment, error) {
	form := url.Values{}
	form.Set("engineer", c.cfg.Username)
	form.Set("enable", "true")
	form.Set("auth_type", "totp")

	payload, err := c.postTwoFA(ctx, "/api/v1/user/2fa/", form)
	if err != nil {
		return TwoFactorEnrollment{}, err
	}
	var data struct {
		AuthType string `json:"auth_type"`
		Key      string `json:"key"`
	}
	if len(payload.Data) > 0 {
		if err := decodeJSON(payload.Data, &data); err != nil {
			return TwoFactorEnrollment{}, apiError(KindHTTP, "无法解析 Archery 两步验证密钥")
		}
	}
	key := strings.TrimSpace(data.Key)
	if key == "" {
		return TwoFactorEnrollment{}, apiError(KindServer, "Archery 未返回 TOTP 密钥")
	}
	qrCode, err := c.fetchQRCode(ctx, key)
	if err != nil {
		return TwoFactorEnrollment{}, err
	}
	return TwoFactorEnrollment{AuthType: "totp", SecretKey: key, QRCode: qrCode}, nil
}

// SendSMSCode 让 Archery 给指定手机号发送绑定验证码。
func (c *Client) SendSMSCode(ctx context.Context, phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return apiError(KindInvalidConfig, "短信验证码需要填写手机号")
	}
	form := url.Values{}
	form.Set("engineer", c.cfg.Username)
	form.Set("enable", "true")
	form.Set("auth_type", "sms")
	form.Set("phone", phone)
	_, err := c.postTwoFA(ctx, "/api/v1/user/2fa/", form)
	return err
}

// ConfirmTwoFactor 先校验验证码，再保存绑定，顺序与 Archery 网页一致。
func (c *Client) ConfirmTwoFactor(ctx context.Context, authType, otp, secretKey, phone string) error {
	authType, err := normalizeAuthType(authType)
	if err != nil {
		return err
	}
	otp = strings.TrimSpace(otp)
	if otp == "" {
		return apiError(KindInvalidConfig, "请输入 6 位验证码")
	}
	body := map[string]any{
		"engineer":  c.cfg.Username,
		"auth_type": authType,
		"otp":       otp,
	}
	if authType == "sms" {
		body["phone"] = strings.TrimSpace(phone)
	} else {
		body["key"] = strings.TrimSpace(secretKey)
	}

	var verified struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/user/2fa/verify/",
		JSON:    body,
		Out:     &verified,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return err
	}
	if verified.Status != 0 {
		return apiError(KindServer, fallback(verified.Msg, "验证码不正确"))
	}

	form := url.Values{}
	form.Set("engineer", c.cfg.Username)
	form.Set("auth_type", authType)
	if authType == "sms" {
		form.Set("phone", strings.TrimSpace(phone))
	} else {
		form.Set("key", strings.TrimSpace(secretKey))
	}
	_, err = c.postTwoFA(ctx, "/api/v1/user/2fa/save/", form)
	return err
}

// DisableTwoFactor 关闭指定通道；Archery 会直接删除对应配置记录。
func (c *Client) DisableTwoFactor(ctx context.Context, authType string) error {
	authType, err := normalizeAuthType(authType)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("engineer", c.cfg.Username)
	form.Set("enable", "false")
	form.Set("auth_type", authType)
	_, err = c.postTwoFA(ctx, "/api/v1/user/2fa/", form)
	return err
}

// VerifyPassword 用当前连接密码做一次 Archery 账号校验。
// Archery 网页在开关两步验证前也要求先验证密码。
func (c *Client) VerifyPassword(ctx context.Context, password string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "账号校验仅支持会话登录模式")
	}
	if password == "" {
		return apiError(KindInvalidConfig, "当前连接未保存密码，无法校验账号")
	}
	form := url.Values{}
	form.Set("engineer", c.cfg.Username)
	form.Set("password", password)

	var payload struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/user/auth/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return err
	}
	if payload.Status != 0 {
		return apiError(KindForbidden, fallback(payload.Msg, "当前连接密码校验失败，请更新 DBX 连接中的密码"))
	}
	return nil
}

// ChangePassword 通过 Django admin 的改密页提交新密码。
// 成功是 302 跳转、失败是 200 + errorlist，因此不能走 do 的 JSON 解码路径。
func (c *Client) ChangePassword(ctx context.Context, oldPassword, newPassword string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "修改密码仅支持会话登录模式")
	}
	oldPassword = strings.TrimSpace(oldPassword)
	newPassword = strings.TrimSpace(newPassword)
	if oldPassword == "" {
		return apiError(KindInvalidConfig, "请输入旧密码")
	}
	if newPassword == "" {
		return apiError(KindInvalidConfig, "请输入新密码")
	}
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	_, authHeader := c.snapshot()

	// 先打开改密页：既确认账号有 admin 权限，也顺便拿到表单里的 CSRF token。
	resp, page, err := c.execute(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    passwordChangePath,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}, authHeader)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// 后台改密页对非 staff 账号与会话失效都会 302 到 admin 登录页，再用一个已登录端点区分。
		if c.sessionLost(ctx, authHeader) {
			return apiError(KindAuthRequired, "Archery 会话已失效，请重新打开连接后再试")
		}
		return apiError(KindForbidden, "当前账号没有 Archery 后台改密权限，请先在 Archery 网页修改密码")
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return apiError(KindForbidden, "当前账号无法访问 Archery 后台改密页")
	}
	if resp.StatusCode >= 400 {
		return statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	csrf := c.csrfToken()
	if csrf == "" {
		if match := csrfInputPattern.FindSubmatch(page); len(match) == 2 {
			csrf = string(match[1])
		}
	}

	form := url.Values{}
	form.Set("old_password", oldPassword)
	form.Set("new_password1", newPassword)
	form.Set("new_password2", newPassword)
	if csrf != "" {
		form.Set("csrfmiddlewaretoken", csrf)
	}
	resp, body, err := c.execute(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    passwordChangePath,
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}, authHeader)
	if err != nil {
		return err
	}

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		if location := resp.Header.Get("Location"); strings.Contains(location, "/admin/login") {
			return apiError(KindForbidden, "当前账号没有 Archery 后台改密权限，请先在 Archery 网页修改密码")
		}
		return nil
	case resp.StatusCode == http.StatusForbidden:
		return apiError(KindForbidden, "修改密码请求被拒绝（CSRF 校验失败），请重新打开连接后再试")
	case resp.StatusCode == http.StatusNotFound:
		return apiError(KindNotFound, "Archery 未提供修改密码入口（HTTP 404）")
	case resp.StatusCode >= 400:
		return statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if message := djangoFormErrors(body); message != "" {
		return apiError(KindServer, message)
	}
	return apiError(KindServer, "修改密码失败：Archery 未返回结果")
}

func (c *Client) postTwoFA(ctx context.Context, path string, form url.Values) (*twoFAResponse, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "两步验证仅支持会话登录模式")
	}
	var payload twoFAResponse
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    path,
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return nil, err
	}
	if payload.Status != 0 {
		return nil, apiError(KindServer, fallback(payload.Msg, "两步验证配置失败"))
	}
	return &payload, nil
}

func (c *Client) fetchQRCode(ctx context.Context, key string) (string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return "", err
	}
	epoch, authHeader := c.snapshot()
	call := httpCall{
		Method:  http.MethodGet,
		Path:    "/user/qrcode/" + url.PathEscape(key) + "/",
		Accept:  "image/png,image/*",
		Timeout: c.cfg.Timeout,
	}
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return "", err
	}
	if c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return "", reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return "", err
		}
	}
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return "", apiError(KindForbidden, "无权限获取两步验证二维码")
	case resp.StatusCode == http.StatusNotFound:
		return "", apiError(KindNotFound, "Archery 未提供二维码接口（HTTP 404）")
	case resp.StatusCode >= 400:
		return "", statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	case len(body) == 0:
		return "", apiError(KindServer, "Archery 返回了空二维码")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(body), nil
}

// sessionLost 用实例清单端点探测会话是否仍然有效。
func (c *Client) sessionLost(ctx context.Context, authHeader string) bool {
	resp, body, err := c.execute(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/group/user_all_instances/",
		Timeout: c.cfg.Timeout,
	}, authHeader)
	if err != nil {
		return false
	}
	return c.authLost(resp, body)
}

func normalizeAuthType(authType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "totp":
		return "totp", nil
	case "sms":
		return "sms", nil
	default:
		return "", apiError(KindInvalidConfig, "验证方式只支持 totp 或 sms")
	}
}

// djangoFormErrors 提取 Django admin 表单页里的校验错误文案。
func djangoFormErrors(body []byte) string {
	messages := make([]string, 0, 4)
	for _, list := range djangoErrorListPattern.FindAllSubmatch(body, -1) {
		for _, item := range djangoErrorItemPattern.FindAllSubmatch(list[1], -1) {
			if text := cleanHTMLText(item[1]); text != "" {
				messages = append(messages, text)
			}
		}
	}
	if len(messages) == 0 {
		if match := djangoErrorNotePattern.FindSubmatch(body); len(match) == 2 {
			if text := cleanHTMLText(match[1]); text != "" {
				messages = append(messages, text)
			}
		}
	}
	return strings.Join(messages, "；")
}
