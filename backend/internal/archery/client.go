package archery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	userAgent = "dbx-archery-plugin/0.1.0"
	maxBody   = 16 << 20
)

var csrfInputPattern = regexp.MustCompile(`name="csrfmiddlewaretoken"\s+value="([^"]+)"`)

// OTP 是 2FA 验证码。Type 取 totp/sms，短信模式需要 Phone。
type OTP struct {
	Code  string
	Type  string
	Phone string
}

// InstanceRef 同时携带实例 ID 和名称，会话接口用名称、JWT 接口用 ID。
type InstanceRef struct {
	ID   int
	Name string
}

// Client 是 Archery HTTP 客户端。按连接复用，内部持有 CookieJar 与会话状态。
type Client struct {
	baseURL string
	cfg     Config
	http    *http.Client

	loginMu sync.Mutex
	stateMu sync.Mutex
	csrf    string
	session string
	bearer  string
	pending bool
	// restore 表示 Jar 里装的是上次落盘的会话，尚未向服务端验证。
	restore bool
	epoch   int
	// logins 记录完成的密码/JWT 登录次数，restores 记录落盘会话恢复次数：
	// 心跳据此区分「只是续期」「自动重登」「冷启动恢复」。
	logins   int
	restores int

	// keepAliveMu 串行化心跳，keepAliveAt/keepAliveErr 用于间隔内复用结果。
	keepAliveMu     sync.Mutex
	keepAliveAt     time.Time
	keepAliveErr    error
	keepAliveResult KeepAliveResult
}

// KeepAliveResult 描述一次会话续期探测的结果。
type KeepAliveResult struct {
	// Enabled 为 false 表示连接配置关闭了自动续期，本次没有发起请求。
	Enabled bool `json:"enabled"`
	// Alive 表示探测后本地持有可用会话（含自动重登成功的情况）。
	Alive bool `json:"alive"`
	// Skipped 表示距上次心跳不足间隔，直接复用了上次结果。
	Skipped bool `json:"skipped"`
	// Relogged 表示探测期间会话失效，已用连接里保存的密码重新登录。
	Relogged bool `json:"relogged"`
	// Restored 表示本次会话来自落盘快照恢复（sidecar 重启后的首次请求）。
	Restored    bool      `json:"restored"`
	Username    string    `json:"username"`
	IntervalSec int       `json:"intervalSec"`
	CheckedAt   time.Time `json:"checkedAt"`
}

// NewClient 校验配置并构造客户端。
func NewClient(cfg Config) (*Client, error) {
	cfg = cfg.Normalize()
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, apiError(KindInvalidConfig, "invalid Archery base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, apiError(KindInvalidConfig, "unsupported scheme "+parsed.Scheme)
	}
	if cfg.Username == "" {
		return nil, apiError(KindInvalidConfig, "Archery username is required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, apiError(KindInvalidConfig, "cannot create cookie jar")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}
	client := &Client{
		baseURL: cfg.BaseURL,
		cfg:     cfg,
		http: &http.Client{
			Transport: transport,
			Jar:       jar,
			// 超时由每个请求的 context 控制，查询可比连接超时更长。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	client.loadPersisted()
	return client, nil
}

// BaseURL 返回规范化的平台地址。
func (c *Client) BaseURL() string { return c.baseURL }

// ReadOnly 返回连接是否禁止写操作。
func (c *Client) ReadOnly() bool { return c.cfg.ReadOnly }

// Username 返回连接使用的账号。
func (c *Client) Username() string { return c.cfg.Username }

// Config 返回生效配置的副本。
func (c *Client) Config() Config { return c.cfg }

// CloseIdleConnections 释放连接池。
func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

// EnsureSession 在未登录时完成登录。
// 必须串行化：并发请求若在别人重登的瞬间看到空会话，会各自再登录一次。
func (c *Client) EnsureSession(ctx context.Context) error {
	if c.authenticated() {
		return nil
	}
	if c.pendingTwoFA() {
		return apiError(KindTwoFARequired, "该账号启用了 2FA，请输入验证码")
	}
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.authenticated() {
		return nil
	}
	if c.pendingTwoFA() {
		return apiError(KindTwoFARequired, "该账号启用了 2FA，请输入验证码")
	}
	// 上次落盘的会话仍然有效时直接复用，等价于网页版没超时就不用再登录。
	restored, restoreErr := c.restoreSession(ctx)
	if restoreErr != nil {
		return restoreErr
	}
	if restored {
		return nil
	}
	return c.loginLocked(ctx, OTP{})
}

// restoreSession 用落盘 Cookie 探测一次会话；失败则丢弃快照回退到密码登录。
func (c *Client) restoreSession(ctx context.Context) (bool, error) {
	c.stateMu.Lock()
	pending := c.restore
	c.restore = false
	c.stateMu.Unlock()
	if !pending {
		return false, nil
	}
	// 探测同时充当一次“活动”，让服务端按活动时间续期，和网页行为一致。
	resp, body, err := c.execute(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/group/user_all_instances/",
		Timeout: c.cfg.Timeout,
	}, "")
	if err != nil {
		// 网络/超时不能证明会话失效，保留快照，下次打开再试。
		return false, err
	}
	if resp.StatusCode == http.StatusOK && !isHTML(resp) && !looksLikeLoginPage(body) {
		c.finishLogin()
		c.stateMu.Lock()
		c.restores++
		c.stateMu.Unlock()
		return true, nil
	}
	c.dropPersisted()
	c.clearCookie("sessionid")
	return false, nil
}

// Login 使用连接配置登录，otp 为空时只完成密码阶段。
func (c *Client) Login(ctx context.Context, otp OTP) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	// 已处于 2FA 待验证状态时只需校验验证码：重复跑密码阶段会再发一次短信。
	if c.pendingTwoFA() && !c.authenticated() && c.cfg.SessionMode() && strings.TrimSpace(otp.Code) != "" {
		return c.verify2FA(ctx, otp)
	}
	return c.loginLocked(ctx, otp)
}

func (c *Client) loginLocked(ctx context.Context, otp OTP) error {
	var err error
	if !c.cfg.SessionMode() {
		err = c.loginJWT(ctx)
	} else {
		err = c.loginSession(ctx, otp)
	}
	if err == nil {
		c.stateMu.Lock()
		c.logins++
		c.stateMu.Unlock()
	}
	return err
}

// KeepAlive 用一个无副作用的登录态 GET 充当「活动」，让 Archery 按最后活动时间续期会话。
//
// 服务端判定会话失效时，do/envelope 的既有链路会丢弃落盘快照并用连接里保存的密码重登：
// 无 2FA 的账号这一步完全静默，2FA 账号返回 twofa_required，由 UI 弹出验证码框。
// 同一间隔内的重复调用复用上次结果，避免 UI 高频检查放大成请求风暴。
func (c *Client) KeepAlive(ctx context.Context) (*KeepAliveResult, error) {
	interval := c.cfg.KeepAlive
	if interval <= 0 {
		return &KeepAliveResult{
			Enabled:   false,
			Alive:     c.authenticated(),
			Username:  c.cfg.Username,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()
	// 只缓存成功结果：失败必须原样重试，否则会把「会话不可用」当成续期成功。
	if c.keepAliveErr == nil && !c.keepAliveAt.IsZero() && time.Since(c.keepAliveAt) < interval {
		cached := c.keepAliveResult
		cached.Skipped = true
		return &cached, nil
	}

	c.stateMu.Lock()
	loginsBefore := c.logins
	restoresBefore := c.restores
	c.stateMu.Unlock()

	// /group/user_all_instances/ 是登录态、只读、低权限门槛的接口，与恢复探测同一个路径。
	_, err := c.envelope(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/group/user_all_instances/",
		Timeout: c.cfg.Timeout,
	})

	c.stateMu.Lock()
	loginsAfter := c.logins
	restoresAfter := c.restores
	c.stateMu.Unlock()

	result := KeepAliveResult{
		Enabled:     true,
		Alive:       err == nil,
		Relogged:    loginsAfter > loginsBefore,
		Restored:    restoresAfter > restoresBefore,
		Username:    c.cfg.Username,
		IntervalSec: int(interval / time.Second),
		CheckedAt:   time.Now().UTC(),
	}
	c.keepAliveAt = time.Now()
	c.keepAliveErr = err
	c.keepAliveResult = result
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) loginSession(ctx context.Context, otp OTP) error {
	c.resetAuth()
	if err := c.fetchCSRF(ctx); err != nil {
		return err
	}

	form := url.Values{}
	form.Set("username", c.cfg.Username)
	form.Set("password", c.cfg.Password)
	var payload struct {
		Status int             `json:"status"`
		Msg    string          `json:"msg"`
		Data   json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/authenticate/",
		Form:    form,
		Out:     &payload,
		NoAuth:  true,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return err
	}
	if payload.Status != 0 {
		return apiError(KindAuthRequired, fallback(payload.Msg, "Archery 登录失败"))
	}

	sessionKey := strings.TrimSpace(rawString(payload.Data))
	if sessionKey == "" {
		c.finishLogin()
		return nil
	}

	// 2FA：Archery 返回临时 session key，需要客户端写 sessionid 后再校验。
	c.setCookie("sessionid", sessionKey)
	c.stateMu.Lock()
	c.pending = true
	c.stateMu.Unlock()
	if strings.TrimSpace(otp.Code) == "" {
		return apiError(KindTwoFARequired, "该账号启用了 2FA，请输入验证码")
	}
	return c.verify2FA(ctx, otp)
}

func (c *Client) verify2FA(ctx context.Context, otp OTP) error {
	otpType := strings.ToLower(strings.TrimSpace(otp.Type))
	if otpType == "" {
		// UI 只负责收验证码，验证方式由连接配置决定。
		otpType = c.cfg.TwoFAType
	}
	if otpType == "" {
		otpType = "totp"
	}
	phone := strings.TrimSpace(otp.Phone)
	if phone == "" {
		phone = c.cfg.TwoFAPhone
	}
	if otpType == "sms" && phone == "" {
		return apiError(KindInvalidConfig, "短信验证码需要在连接配置中填写 2FA 手机号")
	}
	body := map[string]any{
		"engineer":  c.cfg.Username,
		"auth_type": otpType,
		"otp":       strings.TrimSpace(otp.Code),
	}
	if phone != "" {
		body["phone"] = phone
	}
	var payload struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/user/2fa/verify/",
		JSON:    body,
		Out:     &payload,
		NoAuth:  true,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return err
	}
	if payload.Status != 0 {
		return apiError(KindTwoFARequired, fallback(payload.Msg, "2FA 验证码校验失败"))
	}
	c.finishLogin()
	return nil
}

func (c *Client) loginJWT(ctx context.Context) error {
	c.resetAuth()
	var payload struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
		Detail  string `json:"detail"`
	}
	if err := c.do(ctx, httpCall{
		Method: http.MethodPost,
		Path:   "/api/auth/token/",
		JSON: map[string]string{
			"username": c.cfg.Username,
			"password": c.cfg.Password,
		},
		Out:     &payload,
		NoAuth:  true,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return err
	}
	if payload.Access == "" {
		return apiError(KindAuthRequired, fallback(payload.Detail, "Archery JWT 登录失败"))
	}
	c.stateMu.Lock()
	c.bearer = payload.Access
	c.pending = false
	c.epoch++
	c.stateMu.Unlock()
	return nil
}

func (c *Client) finishLogin() {
	session := c.cookieValue("sessionid")
	c.stateMu.Lock()
	c.session = session
	c.bearer = ""
	c.pending = false
	c.restore = false
	c.epoch++
	c.stateMu.Unlock()
	c.savePersisted()
}

func (c *Client) resetAuth() {
	c.stateMu.Lock()
	c.session = ""
	c.bearer = ""
	c.pending = false
	c.restore = false
	c.stateMu.Unlock()
}

func (c *Client) authenticated() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.session != "" || c.bearer != ""
}

func (c *Client) pendingTwoFA() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.pending
}

func (c *Client) snapshot() (int, string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	header := ""
	if c.bearer != "" {
		header = "Bearer " + c.bearer
	}
	return c.epoch, header
}

// fetchCSRF 打开登录页获取 Django CSRF token（cookie 与隐藏域双保险）。
func (c *Client) fetchCSRF(ctx context.Context) error {
	resp, body, err := c.execute(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/login/",
		NoAuth:  true,
		Timeout: c.cfg.Timeout,
	}, "")
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return statusError(resp.StatusCode, "无法打开 Archery 登录页")
	}
	token := c.cookieValue("csrftoken")
	if token == "" {
		if match := csrfInputPattern.FindSubmatch(body); len(match) == 2 {
			token = string(match[1])
		}
	}
	if token == "" {
		return apiError(KindAuthRequired, "Archery 登录页未返回 CSRF token")
	}
	c.stateMu.Lock()
	c.csrf = token
	c.stateMu.Unlock()
	return nil
}

type httpCall struct {
	Method  string
	Path    string
	Query   url.Values
	Form    url.Values
	JSON    any
	Out     any
	NoAuth  bool
	Accept  string
	Timeout time.Duration
}

func (c *Client) do(ctx context.Context, call httpCall) error {
	if !call.NoAuth {
		if err := c.EnsureSession(ctx); err != nil {
			return err
		}
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return err
	}
	if !call.NoAuth && c.authLost(resp, body) {
		// 服务端已判会话失效，落盘快照一并作废，避免下次启动白探测。
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return err
		}
	}
	return c.decodeResponse(resp, body, call.Out)
}

func (c *Client) relogin(ctx context.Context, epoch int) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	c.stateMu.Lock()
	current := c.epoch
	c.stateMu.Unlock()
	if current != epoch {
		// 其他并发请求已经完成重登。
		return nil
	}
	return c.loginLocked(ctx, OTP{})
}

func (c *Client) execute(ctx context.Context, call httpCall, authHeader string) (*http.Response, []byte, error) {
	timeout := call.Timeout
	if timeout <= 0 {
		timeout = c.cfg.Timeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := c.baseURL + call.Path
	if len(call.Query) > 0 {
		endpoint += "?" + call.Query.Encode()
	}
	var bodyReader io.Reader
	if call.Form != nil {
		bodyReader = strings.NewReader(call.Form.Encode())
	} else if call.JSON != nil {
		encoded, err := json.Marshal(call.JSON)
		if err != nil {
			return nil, nil, apiError(KindInvalidConfig, "cannot encode request body")
		}
		bodyReader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(requestCtx, call.Method, endpoint, bodyReader)
	if err != nil {
		return nil, nil, apiError(KindInvalidConfig, "invalid Archery request")
	}
	req.Header.Set("User-Agent", userAgent)
	accept := call.Accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	if call.Form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if call.JSON != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if c.cfg.SessionMode() {
		req.Header.Set("Origin", c.baseURL)
		req.Header.Set("Referer", c.baseURL+"/")
		// 登录页与 2FA 校验同样受 Django CSRF 保护，不能只在已登录请求里带 token。
		if call.Method != http.MethodGet && call.Method != http.MethodHead {
			if token := c.csrfToken(); token != "" {
				req.Header.Set("X-CSRFToken", token)
			}
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if isTimeout(err) {
			return nil, nil, apiError(KindTimeout, "Archery 请求超时")
		}
		return nil, nil, apiError(KindNetwork, "无法连接 Archery："+err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, nil, apiError(KindNetwork, "读取 Archery 响应失败")
	}
	return resp, body, nil
}

func (c *Client) authLost(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if strings.Contains(location, "/login") {
			return true
		}
	}
	if resp.StatusCode == http.StatusOK && isHTML(resp) && looksLikeLoginPage(body) {
		return true
	}
	return false
}

func (c *Client) decodeResponse(resp *http.Response, body []byte, out any) error {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return apiError(KindAuthRequired, extractMessage(body, "登录状态已失效，请重新登录"))
	}
	if resp.StatusCode == http.StatusForbidden {
		return apiError(KindForbidden, extractMessage(body, "无权限执行该操作（HTTP 403）"))
	}
	if resp.StatusCode == http.StatusNotFound {
		return apiError(KindNotFound, "Archery 接口不存在（HTTP 404）")
	}
	if resp.StatusCode >= 400 {
		return statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if isHTML(resp) {
		if looksLikeLoginPage(body) {
			return apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
		}
		return apiError(KindHTTP, "Archery 返回了 HTML 页面而非 JSON")
	}
	if out == nil {
		return nil
	}
	if err := decodeJSON(body, out); err != nil {
		return apiError(KindHTTP, "无法解析 Archery 响应："+err.Error())
	}
	return nil
}

func decodeJSON(body []byte, out any) error {
	if !utf8.Valid(body) {
		body = latin1ToUTF8(body)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func latin1ToUTF8(body []byte) []byte {
	var builder strings.Builder
	for _, char := range body {
		builder.WriteRune(rune(char))
	}
	return []byte(builder.String())
}

func isHTML(resp *http.Response) bool {
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html")
}

// looksLikeLoginPage 判定响应是否是 Archery 登录页。
// 业务页面（base.html + detail.html 等）既有 {% csrf_token %} 表单，
// 也有「确认密码」模态框（type="password" id="passwd"），两者都不能作为判据；
// 只认登录页独有标记：login-form 类名、#inputUsername/#inputPassword/#btnLogin、页面标题。
func looksLikeLoginPage(body []byte) bool {
	text := strings.ToLower(string(body))
	switch {
	case strings.Contains(text, "login-form"):
		return true
	case strings.Contains(text, `id="inputusername"`), strings.Contains(text, `id="inputpassword"`):
		return true
	case strings.Contains(text, `id="btnlogin"`):
		return true
	case strings.Contains(text, "login to archery"):
		return true
	}
	return false
}

func (c *Client) csrfToken() string {
	if token := c.cookieValue("csrftoken"); token != "" {
		return token
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.csrf
}

func (c *Client) cookieValue(name string) string {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	for _, cookie := range c.http.Jar.Cookies(base) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (c *Client) setCookie(name, value string) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return
	}
	c.http.Jar.SetCookies(base, []*http.Cookie{{Name: name, Value: value, Path: "/"}})
}

func (c *Client) clearCookie(name string) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return
	}
	c.http.Jar.SetCookies(base, []*http.Cookie{{Name: name, Value: "", Path: "/", MaxAge: -1}})
}

// loadPersisted 把上次保存的 Cookie 装回 Jar，有效性留到首次请求时探测。
func (c *Client) loadPersisted() {
	if c.cfg.SessionStore == nil || !c.cfg.SessionMode() {
		return
	}
	cookies := c.cfg.SessionStore.Load(c.baseURL, c.cfg.Username)
	if cookies["sessionid"] == "" {
		// 没有 sessionid 无法恢复登录态，仅有 csrftoken 不值得探测。
		return
	}
	for name, value := range cookies {
		c.setCookie(name, value)
	}
	c.stateMu.Lock()
	c.restore = true
	c.stateMu.Unlock()
}

// savePersisted 只保存 Django 会话需要的 Cookie；未验码的临时会话不会走到这里。
func (c *Client) savePersisted() {
	if c.cfg.SessionStore == nil || !c.cfg.SessionMode() {
		return
	}
	cookies := map[string]string{}
	for _, name := range []string{"sessionid", "csrftoken"} {
		if value := c.cookieValue(name); value != "" {
			cookies[name] = value
		}
	}
	if cookies["sessionid"] == "" {
		return
	}
	c.cfg.SessionStore.Save(c.baseURL, c.cfg.Username, cookies)
}

func (c *Client) dropPersisted() {
	if c.cfg.SessionStore == nil || !c.cfg.SessionMode() {
		return
	}
	c.cfg.SessionStore.Delete(c.baseURL, c.cfg.Username)
}

// Version 读取 Archery 版本，无需登录。
func (c *Client) Version(ctx context.Context) (string, error) {
	var payload struct {
		Archery struct {
			Version string `json:"version"`
		} `json:"archery"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/api/info",
		Out:     &payload,
		NoAuth:  true,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return "", err
	}
	return payload.Archery.Version, nil
}

// DashboardHTML 抓取 Archery 控制台首页 HTML。
// 四个指标卡数字只存在于服务端模板里（/dashboard/api/ 返回的是图表 HTML 片段）。
// start_date / end_date 与 Archery 其余列表页一致，用于首页按日统计图的时间范围；
// 两个都为空时不带参数，走 Archery 自己的默认窗口。
func (c *Client) DashboardHTML(ctx context.Context, startDate, endDate string) ([]byte, error) {
	query := url.Values{}
	if startDate != "" && endDate != "" {
		query.Set("start_date", startDate)
		query.Set("end_date", endDate)
	}
	return c.rawBody(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/dashboard/",
		Query:   query,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
}

// DashboardTrends 按时间范围重新取首页两张按日统计图。
// 页面视图 /dashboard/ 不解析日期参数（整页永远是默认窗口），
// 真正按区间出数的是 /dashboard/api/?start_date=&end_date=，返回 {"chart": {line1: html, bar1: html, ...}}。
func (c *Client) DashboardTrends(ctx context.Context, startDate, endDate string) (*DashboardTrends, error) {
	query := url.Values{}
	query.Set("start_date", startDate)
	query.Set("end_date", endDate)
	body, err := c.rawBody(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/dashboard/api/",
		Query:   query,
		Accept:  "application/json",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Chart map[string]string `json:"chart"`
	}
	if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
		return nil, apiError(KindUnsupported, "Archery 未返回图表数据（/dashboard/api/ 结构不符）")
	}
	trends := ParseTrendFragments(payload.Chart, startDate, endDate)
	if trends == nil {
		return nil, apiError(KindUnsupported, "首页图表数据为空")
	}
	return trends, nil
}

// rawBody 复用 do 的重登逻辑，但直接返回响应体，供 HTML 页面解析使用。
func (c *Client) rawBody(ctx context.Context, call httpCall) ([]byte, error) {
	if !call.NoAuth {
		if err := c.EnsureSession(ctx); err != nil {
			return nil, err
		}
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return nil, err
	}
	if !call.NoAuth && c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return nil, reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return nil, err
		}
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, apiError(KindAuthRequired, "登录状态已失效，请重新登录")
	case resp.StatusCode == http.StatusForbidden:
		return nil, apiError(KindForbidden, "无权限访问 Archery 首页（需要 Dashboard 菜单权限）")
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
	case resp.StatusCode >= 400:
		return nil, statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if looksLikeLoginPage(body) {
		return nil, apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
	}
	return body, nil
}

// Instances 返回当前用户可访问的实例。
func (c *Client) Instances(ctx context.Context) ([]Instance, error) {
	if !c.cfg.SessionMode() {
		return c.jwtInstances(ctx)
	}
	data, err := c.envelope(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/group/user_all_instances/",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return mapInstances(data)
}

func (c *Client) jwtInstances(ctx context.Context) ([]Instance, error) {
	data, err := c.envelope(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/api/v1/sqlquery/instances/",
		Timeout: c.cfg.Timeout,
	})
	if err == nil {
		return mapInstances(data)
	}
	if !IsKind(err, KindNotFound) {
		return nil, err
	}
	query := url.Values{}
	query.Set("limit", "1000")
	var page struct {
		Results []struct {
			ID     int    `json:"id"`
			Name   string `json:"instance_name"`
			Type   string `json:"type"`
			DBType string `json:"db_type"`
		} `json:"results"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/api/v1/instance/",
		Query:   query,
		Out:     &page,
		Timeout: c.cfg.Timeout,
	}); err != nil {
		return nil, err
	}
	instances := make([]Instance, 0, len(page.Results))
	for _, item := range page.Results {
		instances = append(instances, Instance{ID: item.ID, Name: item.Name, Type: item.Type, DBType: item.DBType})
	}
	return instances, nil
}

func mapInstances(raw json.RawMessage) ([]Instance, error) {
	var items []struct {
		ID     int    `json:"id"`
		Name   string `json:"instance_name"`
		Type   string `json:"type"`
		DBType string `json:"db_type"`
	}
	if err := decodeJSON(raw, &items); err != nil {
		return nil, apiError(KindHTTP, "无法解析实例列表")
	}
	instances := make([]Instance, 0, len(items))
	for _, item := range items {
		instances = append(instances, Instance{ID: item.ID, Name: item.Name, Type: item.Type, DBType: item.DBType})
	}
	return instances, nil
}

// Resources 返回实例下的库/模式/表/列。
func (c *Client) Resources(ctx context.Context, ref InstanceRef, resourceType, dbName, schemaName, tbName string) ([]Resource, error) {
	var call httpCall
	if c.cfg.SessionMode() {
		query := url.Values{}
		if ref.Name != "" {
			query.Set("instance_name", ref.Name)
		} else {
			query.Set("instance_id", strconv.Itoa(ref.ID))
		}
		query.Set("resource_type", resourceType)
		if dbName != "" {
			query.Set("db_name", dbName)
		}
		if schemaName != "" {
			query.Set("schema_name", schemaName)
		}
		if tbName != "" {
			query.Set("tb_name", tbName)
		}
		call = httpCall{Method: http.MethodGet, Path: "/instance/instance_resource/", Query: query, Timeout: c.cfg.QueryTimeout}
	} else {
		query := url.Values{}
		query.Set("instance_id", strconv.Itoa(ref.ID))
		query.Set("resource_type", resourceType)
		if dbName != "" {
			query.Set("db_name", dbName)
		}
		if schemaName != "" {
			query.Set("schema_name", schemaName)
		}
		if tbName != "" {
			query.Set("tb_name", tbName)
		}
		call = httpCall{Method: http.MethodGet, Path: "/api/v1/sqlquery/resources/", Query: query, Timeout: c.cfg.QueryTimeout}
	}

	data, err := c.envelope(ctx, call)
	if err != nil && !c.cfg.SessionMode() && IsKind(err, KindNotFound) {
		query := url.Values{}
		query.Set("instance_id", strconv.Itoa(ref.ID))
		query.Set("resource_type", resourceType)
		if dbName != "" {
			query.Set("db_name", dbName)
		}
		if schemaName != "" {
			query.Set("schema_name", schemaName)
		}
		if tbName != "" {
			query.Set("tb_name", tbName)
		}
		data, err = c.envelope(ctx, httpCall{Method: http.MethodGet, Path: "/api/v1/instance/resource/", Query: query, Timeout: c.cfg.QueryTimeout})
	}
	if err != nil {
		return nil, err
	}
	return mapResources(data)
}

func mapResources(raw json.RawMessage) ([]Resource, error) {
	var rows []any
	if err := decodeJSON(raw, &rows); err != nil {
		return nil, apiError(KindHTTP, "无法解析实例资源")
	}
	resources := make([]Resource, 0, len(rows))
	for _, item := range rows {
		switch value := item.(type) {
		case []any:
			if len(value) == 0 {
				continue
			}
			row := value
			if len(row) == 1 {
				// SHOW TABLES 只有表名，补一个空注释位，调用方可以统一取 values[1]。
				row = []any{value[0], ""}
			}
			resources = append(resources, Resource{Name: toText(value[0]), Values: row})
		case map[string]any:
			// 新版 API 返回对象数组，注释字段名各版本不一致，按候选键兜底。
			name := pickText(value, "name", "NAME", "table_name", "db_name", "column_name")
			comment := pickText(value, "table_comment", "comment", "tableComment", "description", "remarks")
			resources = append(resources, Resource{Name: name, Values: []any{name, comment}})
		default:
			resources = append(resources, Resource{Name: toText(value), Values: []any{value}})
		}
	}
	return resources, nil
}

// TableDictionary 返回带注释的表清单。
//
// Archery 的 instance_resource?resource_type=table 走 SHOW TABLES，只有表名；
// 表注释在数据字典接口 /data_dictionary/table_list/ 里（按首字母分组的
// {letter: [[table_name, table_comment], ...]}）。该接口需要
// sql.menu_data_dictionary 菜单权限，无权限或旧版本时退回资源接口。
func (c *Client) TableDictionary(ctx context.Context, ref InstanceRef, dbName, dbType string) ([]DictionaryTable, bool, error) {
	if !c.cfg.SessionMode() ||
		strings.TrimSpace(ref.Name) == "" ||
		strings.TrimSpace(dbName) == "" ||
		strings.TrimSpace(dbType) == "" {
		list, err := c.Resources(ctx, ref, "table", dbName, "", "")
		return resourcesToDictionary(list), false, err
	}

	query := url.Values{}
	query.Set("instance_name", ref.Name)
	query.Set("db_name", dbName)
	query.Set("db_type", dbType)
	data, err := c.envelope(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/data_dictionary/table_list/",
		Query:   query,
		Timeout: c.cfg.QueryTimeout,
	})
	if err != nil {
		// 403（无数据字典菜单权限）或 404（版本没有该接口）都不该让页面整体失败。
		list, fallbackErr := c.Resources(ctx, ref, "table", dbName, "", "")
		if fallbackErr != nil {
			return nil, false, err
		}
		return resourcesToDictionary(list), false, nil
	}

	tables, parseErr := mapDictionaryTables(data)
	if parseErr != nil {
		list, fallbackErr := c.Resources(ctx, ref, "table", dbName, "", "")
		if fallbackErr != nil {
			return nil, false, parseErr
		}
		return resourcesToDictionary(list), false, nil
	}
	return tables, true, nil
}

// TableMetadata 返回单表元数据（表信息 / 列信息 / 索引信息 / 建表语句）。
//
// 走 Archery /data_dictionary/table_info/：meta_data 的 rows 是单行，
// desc / index 是二维行，create_sql 是 SHOW CREATE TABLE 的结果行。
func (c *Client) TableMetadata(ctx context.Context, ref InstanceRef, dbName, tbName, dbType string) (*TableMetadata, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "表元数据仅支持会话登录模式")
	}
	if strings.TrimSpace(ref.Name) == "" ||
		strings.TrimSpace(dbName) == "" ||
		strings.TrimSpace(tbName) == "" ||
		strings.TrimSpace(dbType) == "" {
		return nil, apiError(KindUnsupported, "表元数据需要 instanceName、dbType、dbName 与 tbName")
	}

	query := url.Values{}
	query.Set("instance_name", ref.Name)
	query.Set("db_name", dbName)
	query.Set("tb_name", tbName)
	query.Set("db_type", dbType)
	data, err := c.envelope(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/data_dictionary/table_info/",
		Query:   query,
		Timeout: c.cfg.QueryTimeout,
	})
	if err != nil {
		return nil, err
	}
	return mapTableMetadata(data)
}

func mapTableMetadata(raw json.RawMessage) (*TableMetadata, error) {
	var payload struct {
		MetaData  json.RawMessage `json:"meta_data"`
		Desc      json.RawMessage `json:"desc"`
		Index     json.RawMessage `json:"index"`
		CreateSQL json.RawMessage `json:"create_sql"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return nil, apiError(KindHTTP, "无法解析表元数据")
	}
	meta, err := parseTableGrid(payload.MetaData, true)
	if err != nil {
		return nil, err
	}
	desc, err := parseTableGrid(payload.Desc, false)
	if err != nil {
		return nil, err
	}
	index, err := parseTableGrid(payload.Index, false)
	if err != nil {
		return nil, err
	}
	return &TableMetadata{
		Meta:      meta,
		Desc:      desc,
		Index:     index,
		CreateSQL: extractCreateSQL(payload.CreateSQL),
	}, nil
}

// parseTableGrid 把 {"column_list": [...], "rows": ...} 规范成表格。
// singleRow 用于表信息：Archery 在那里返回单行而不是二维数组。
func parseTableGrid(raw json.RawMessage, singleRow bool) (TableGrid, error) {
	grid := TableGrid{Columns: []string{}, Rows: [][]any{}}
	if isEmptyJSON(raw) {
		return grid, nil
	}
	var payload struct {
		ColumnList []string        `json:"column_list"`
		Rows       json.RawMessage `json:"rows"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return grid, apiError(KindHTTP, "无法解析表元数据表格")
	}
	grid.Columns = payload.ColumnList
	if isEmptyJSON(payload.Rows) {
		return grid, nil
	}
	var rows []any
	if err := decodeJSON(payload.Rows, &rows); err != nil {
		return grid, apiError(KindHTTP, "无法解析表元数据行")
	}
	if len(rows) == 0 {
		return grid, nil
	}
	if singleRow {
		// 兼容 [[...]] 与 [...] 两种单行结构。
		if len(rows) == 1 {
			if values, ok := rows[0].([]any); ok {
				grid.Rows = append(grid.Rows, values)
				return grid, nil
			}
		}
		grid.Rows = append(grid.Rows, rows)
		return grid, nil
	}
	for _, row := range rows {
		if values, ok := row.([]any); ok {
			grid.Rows = append(grid.Rows, values)
			continue
		}
		grid.Rows = append(grid.Rows, []any{row})
	}
	return grid, nil
}

func isEmptyJSON(raw json.RawMessage) bool {
	text := strings.TrimSpace(string(raw))
	return text == "" || text == "null"
}

// extractCreateSQL 从 SHOW CREATE TABLE 的结果行里取出建表语句。
func extractCreateSQL(raw json.RawMessage) string {
	if isEmptyJSON(raw) {
		return ""
	}
	var rows []any
	if err := decodeJSON(raw, &rows); err != nil {
		return ""
	}
	for _, row := range rows {
		values, ok := row.([]any)
		if !ok {
			continue
		}
		for index := len(values) - 1; index >= 0; index-- {
			text := strings.TrimSpace(toText(values[index]))
			if strings.HasPrefix(strings.ToUpper(text), "CREATE ") {
				return text
			}
		}
	}
	return ""
}

// mapDictionaryTables 解析 {letter: [[表名, 注释], ...]}，按字母顺序摊平。
func mapDictionaryTables(raw json.RawMessage) ([]DictionaryTable, error) {
	var payload any
	if err := decodeJSON(raw, &payload); err != nil {
		return nil, apiError(KindHTTP, "无法解析数据字典")
	}
	tables := make([]DictionaryTable, 0)
	switch value := payload.(type) {
	case map[string]any:
		letters := make([]string, 0, len(value))
		for letter := range value {
			letters = append(letters, letter)
		}
		sort.Strings(letters)
		for _, letter := range letters {
			rows, ok := value[letter].([]any)
			if !ok {
				continue
			}
			for _, row := range rows {
				if table, ok := dictionaryTable(row); ok {
					tables = append(tables, table)
				}
			}
		}
	case []any:
		for _, row := range value {
			if table, ok := dictionaryTable(row); ok {
				tables = append(tables, table)
			}
		}
	default:
		return nil, apiError(KindHTTP, "数据字典返回结构不受支持")
	}
	return tables, nil
}

// dictionaryTable 兼容位置数组、对象和纯字符串三种行结构。
func dictionaryTable(item any) (DictionaryTable, bool) {
	switch row := item.(type) {
	case []any:
		if len(row) == 0 {
			return DictionaryTable{}, false
		}
		name := strings.TrimSpace(toText(row[0]))
		if name == "" {
			return DictionaryTable{}, false
		}
		comment := ""
		if len(row) > 1 {
			comment = strings.TrimSpace(toText(row[1]))
		}
		return DictionaryTable{Name: name, Comment: comment}, true
	case map[string]any:
		name := pickText(row, "table_name", "name", "TABLE_NAME")
		if name == "" {
			return DictionaryTable{}, false
		}
		comment := pickText(row, "table_comment", "comment", "description", "remarks", "TABLE_COMMENT")
		return DictionaryTable{Name: name, Comment: comment}, true
	case string:
		name := strings.TrimSpace(row)
		if name == "" {
			return DictionaryTable{}, false
		}
		return DictionaryTable{Name: name}, true
	}
	return DictionaryTable{}, false
}

// resourcesToDictionary 把只有表名的资源行（SHOW TABLES 回退路径）转成字典结构。
func resourcesToDictionary(list []Resource) []DictionaryTable {
	tables := make([]DictionaryTable, 0, len(list))
	for _, item := range list {
		comment := ""
		if len(item.Values) > 1 {
			comment = strings.TrimSpace(toText(item.Values[1]))
		}
		tables = append(tables, DictionaryTable{Name: item.Name, Comment: comment})
	}
	return tables
}

// pickText 按候选键取第一个非空文本，兼容不同 Archery 版本的字段命名。
func pickText(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text := strings.TrimSpace(toText(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

// Describe 返回表结构。
//
// Archery ≤1.14 的 /instance/describetable/ 视图会把引擎异常变成 HTTP 500
// （sql/instance.py 里 result["data"].get("error") 在 data 仍为 list 时二次抛错），
// 而且 schema_name 缺省是 None，MySQL 引擎 escape_string(None) 会直接抛错。
// 因此这里始终发送空的 schema_name，并在接口失败时退回查询接口执行 SHOW CREATE TABLE。
func (c *Client) Describe(ctx context.Context, ref InstanceRef, dbName, tbName, schemaName string) (*DescribeResult, error) {
	var call httpCall
	if c.cfg.SessionMode() {
		form := url.Values{}
		form.Set("instance_name", ref.Name)
		form.Set("db_name", dbName)
		form.Set("tb_name", tbName)
		form.Set("schema_name", schemaName)
		call = httpCall{Method: http.MethodPost, Path: "/instance/describetable/", Form: form, Timeout: c.cfg.QueryTimeout}
	} else {
		body := map[string]any{
			"instance_name": ref.Name,
			"db_name":       dbName,
			"tb_name":       tbName,
			"schema_name":   schemaName,
		}
		call = httpCall{Method: http.MethodPost, Path: "/api/v1/sqlquery/describetable/", JSON: body, Timeout: c.cfg.QueryTimeout}
	}
	data, err := c.envelope(ctx, call)
	if err != nil {
		if IsKind(err, KindHTTP) || IsKind(err, KindServer) || IsKind(err, KindUnsupported) {
			if fallback, fallbackErr := c.describeViaQuery(ctx, ref, dbName, tbName, schemaName); fallbackErr == nil {
				return fallback, nil
			}
		}
		return nil, err
	}
	return parseDescribe(data)
}

// describeViaQuery 用查询接口执行 SHOW CREATE TABLE，兼容 describetable 报 500 的实例。
func (c *Client) describeViaQuery(ctx context.Context, ref InstanceRef, dbName, tbName, schemaName string) (*DescribeResult, error) {
	sql := fmt.Sprintf("SHOW CREATE TABLE %s.%s", quoteIdent(dbName), quoteIdent(tbName))
	query, err := c.Query(ctx, QueryRequest{
		InstanceName: ref.Name,
		DBName:       dbName,
		SchemaName:   schemaName,
		TBName:       tbName,
		SQL:          sql,
		LimitNum:     1,
	})
	if err != nil {
		return nil, err
	}
	result := &DescribeResult{
		Columns: query.Columns,
		Types:   query.ColumnTypes,
		Rows:    query.Rows,
		Table:   tbName,
	}
	result.Table, result.DDL = extractCreateTable(query.Rows)
	if result.Table == "" {
		result.Table = tbName
	}
	return result, nil
}

// quoteIdent 用反引号包住库/表名，避免 SHOW CREATE TABLE 被标识符里的符号截断。
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func parseDescribe(raw json.RawMessage) (*DescribeResult, error) {
	var payload struct {
		ColumnList []any  `json:"column_list"`
		ColumnType []any  `json:"column_type"`
		Rows       []any  `json:"rows"`
		Error      string `json:"error"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return nil, apiError(KindHTTP, "无法解析表结构")
	}
	if payload.Error != "" {
		return nil, apiError(KindServer, payload.Error)
	}
	columns, rows := normalizeRows(payload.Rows, stringList(payload.ColumnList))
	types := stringList(payload.ColumnType)
	result := &DescribeResult{Columns: columns, Types: types, Rows: rows}
	result.Table, result.DDL = extractCreateTable(rows)
	return result, nil
}

// extractCreateTable 从 describe 结果里挑出 SHOW CREATE TABLE 的建表语句。
// MySQL/Doris 等引擎返回 [表名, DDL]；返回列清单的引擎没有 DDL，前端回退到表格。
func extractCreateTable(rows [][]any) (string, string) {
	for _, row := range rows {
		for index, cell := range row {
			text := strings.TrimSpace(toText(cell))
			if !isCreateTableStatement(text) {
				continue
			}
			table := ""
			if index > 0 {
				table = strings.TrimSpace(toText(row[0]))
			}
			return table, text
		}
	}
	return "", ""
}

func isCreateTableStatement(text string) bool {
	upper := strings.ToUpper(text)
	return strings.HasPrefix(upper, "CREATE TABLE") ||
		strings.HasPrefix(upper, "CREATE TEMPORARY TABLE")
}

// Query 执行查询。
func (c *Client) Query(ctx context.Context, req QueryRequest) (*QueryResult, error) {
	limit := req.LimitNum
	if limit < 0 {
		limit = 0
	}
	var call httpCall
	if c.cfg.SessionMode() {
		form := url.Values{}
		form.Set("instance_name", req.InstanceName)
		form.Set("db_name", req.DBName)
		form.Set("sql_content", req.SQL)
		form.Set("limit_num", strconv.Itoa(limit))
		if req.SchemaName != "" {
			form.Set("schema_name", req.SchemaName)
		}
		if req.TBName != "" {
			form.Set("tb_name", req.TBName)
		}
		call = httpCall{Method: http.MethodPost, Path: "/query/", Form: form, Timeout: c.cfg.QueryTimeout}
	} else {
		body := map[string]any{
			"instance_name": req.InstanceName,
			"db_name":       req.DBName,
			"sql_content":   req.SQL,
			"limit_num":     limit,
		}
		if req.SchemaName != "" {
			body["schema_name"] = req.SchemaName
		}
		if req.TBName != "" {
			body["tb_name"] = req.TBName
		}
		call = httpCall{Method: http.MethodPost, Path: "/api/v1/sqlquery/execute/", JSON: body, Timeout: c.cfg.QueryTimeout}
	}
	data, err := c.envelope(ctx, call)
	if IsKind(err, KindNotFound) && !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "该 Archery 版本未提供查询 API，请改用会话登录模式")
	}
	if err != nil {
		return nil, err
	}
	return parseQueryResult(data)
}

func parseQueryResult(raw json.RawMessage) (*QueryResult, error) {
	var payload struct {
		ColumnList          []any  `json:"column_list"`
		ColumnType          []any  `json:"column_type"`
		Rows                []any  `json:"rows"`
		AffectedRows        any    `json:"affected_rows"`
		QueryTime           any    `json:"query_time"`
		Masked              any    `json:"is_masked"`
		MaskRuleHit         any    `json:"mask_rule_hit"`
		Warning             any    `json:"warning"`
		Error               string `json:"error"`
		SecondsBehindMaster any    `json:"seconds_behind_master"`
	}
	if err := decodeJSON(raw, &payload); err != nil {
		return nil, apiError(KindHTTP, "无法解析查询结果")
	}
	if payload.Error != "" {
		return nil, apiError(KindServer, payload.Error)
	}
	columns, rows := normalizeRows(payload.Rows, stringList(payload.ColumnList))
	result := &QueryResult{
		Columns:             columns,
		ColumnTypes:         stringList(payload.ColumnType),
		Rows:                rows,
		RowCount:            len(rows),
		AffectedRows:        toInt64(payload.AffectedRows),
		ElapsedMS:           secondsToMillis(payload.QueryTime),
		Masked:              toBool(payload.Masked),
		MaskRuleHit:         toBool(payload.MaskRuleHit),
		Warning:             toText(payload.Warning),
		SecondsBehindMaster: toText(payload.SecondsBehindMaster),
	}
	return result, nil
}

// QueryLogs 读取查询历史。
func (c *Client) QueryLogs(ctx context.Context, query LogQuery) (*QueryLogPage, error) {
	params := url.Values{}
	if query.Limit > 0 {
		params.Set("limit", strconv.Itoa(query.Limit))
	}
	if query.Offset > 0 {
		params.Set("offset", strconv.Itoa(query.Offset))
	}
	if query.Search != "" {
		params.Set("search", query.Search)
	}
	if query.InstanceName != "" {
		params.Set("instance_name", query.InstanceName)
	}
	if query.DBName != "" {
		params.Set("db_name", query.DBName)
	}
	if query.Star {
		params.Set("star", "true")
	}
	if query.QueryLogID > 0 {
		params.Set("query_log_id", strconv.Itoa(query.QueryLogID))
	}
	if query.StartDate != "" {
		params.Set("start_date", query.StartDate)
	}
	if query.EndDate != "" {
		params.Set("end_date", query.EndDate)
	}
	path := "/query/querylog/"
	if !c.cfg.SessionMode() {
		path = "/api/v1/sqlquery/logs/"
	}
	var page struct {
		Total int   `json:"total"`
		Rows  []any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    path,
		Query:   params,
		Out:     &page,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}
	result := &QueryLogPage{Total: page.Total, Rows: make([]QueryLog, 0, len(page.Rows))}
	for _, item := range page.Rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result.Rows = append(result.Rows, QueryLog{
			ID:           toInt64(row["id"]),
			InstanceName: toText(row["instance_name"]),
			DBName:       toText(row["db_name"]),
			SQL:          toText(row["sqllog"]),
			EffectRow:    toInt64(row["effect_row"]),
			CostTime:     toText(row["cost_time"]),
			UserDisplay:  toText(row["user_display"]),
			Favorite:     toBool(row["favorite"]),
			Alias:        toText(row["alias"]),
			CreateTime:   toText(row["create_time"]),
		})
	}
	return result, nil
}

// Favorite 收藏/取消收藏一条查询历史。
func (c *Client) Favorite(ctx context.Context, id int64, star bool, alias string) error {
	starText := "false"
	if star {
		starText = "true"
	}
	var call httpCall
	if c.cfg.SessionMode() {
		form := url.Values{}
		form.Set("query_log_id", strconv.FormatInt(id, 10))
		form.Set("star", starText)
		form.Set("alias", alias)
		call = httpCall{Method: http.MethodPost, Path: "/query/favorite/", Form: form, Timeout: c.cfg.Timeout}
	} else {
		call = httpCall{
			Method: http.MethodPost,
			Path:   "/api/v1/sqlquery/favorites/",
			JSON: map[string]any{
				"query_log_id": id,
				"star":         starText,
				"alias":        alias,
			},
			Timeout: c.cfg.Timeout,
		}
	}
	_, err := c.envelope(ctx, call)
	return err
}

// envelope 解析 {status,msg,data} 响应并返回 data。
func (c *Client) envelope(ctx context.Context, call httpCall) (json.RawMessage, error) {
	var payload struct {
		Status int             `json:"status"`
		Msg    string          `json:"msg"`
		Data   json.RawMessage `json:"data"`
	}
	call.Out = &payload
	if err := c.do(ctx, call); err != nil {
		return nil, err
	}
	if payload.Status != 0 {
		return nil, apiError(KindServer, fallback(payload.Msg, "Archery 返回错误"))
	}
	return payload.Data, nil
}

func extractMessage(body []byte, fallbackText string) string {
	var payload struct {
		Detail string `json:"detail"`
		Msg    string `json:"msg"`
	}
	if err := decodeJSON(body, &payload); err == nil {
		if payload.Detail != "" {
			return payload.Detail
		}
		if payload.Msg != "" {
			return payload.Msg
		}
	}
	return fallbackText
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return strings.TrimSpace(value)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// ---- 查询权限管理（仅会话模式，Archery 1.x 的权限接口都是 Web 表单端点） ----

var (
	// 申请页的组下拉：只列当前用户所属资源组，与网页行为一致。
	privilegeGroupSelectPattern = regexp.MustCompile(`(?is)<select[^>]*id="group_name".*?</select>`)
	privilegeOptionPattern      = regexp.MustCompile(`(?is)<option[^>]*value="([^"]*)"`)
	// error.html 只有 <h4>{{errMsg}}</h4>，用它把审核失败原因带回 UI。
	errorHeadingPattern = regexp.MustCompile(`(?is)<h4[^>]*>(.*?)</h4>`)
	htmlTagPattern      = regexp.MustCompile(`(?is)<[^>]+>`)
	// 详情页区块：标题、审批流（节点链）、其他信息、清单面板、最后操作信息。
	privilegeDetailTitlePattern   = regexp.MustCompile(`(?is)工单名称[：:]\s*<span[^>]*>(.*?)</span>`)
	privilegeReviewHeadingPattern = regexp.MustCompile(`(?is)<h4[^>]*>\s*审批流\s*</h4>`)
	privilegeInfoHeadingPattern   = regexp.MustCompile(`(?is)<h4[^>]*>\s*其他信息\s*</h4>`)
	privilegeArrowPattern         = regexp.MustCompile(`(?is)<span[^>]*glyphicon-arrow-right[^>]*>\s*</span>`)
	privilegeReviewerPattern      = regexp.MustCompile(`(?is)<span[^>]*class="[^"]*text-primary[^"]*"[^>]*>(.*?)</span>`)
	privilegeTrailingParenPattern = regexp.MustCompile(`\s*[（(][^（()）]*[)）]\s*$`)
	privilegePanelPattern         = regexp.MustCompile(`(?is)<div[^>]*class="[^"]*panel-heading[^"]*"[^>]*>\s*(数据库清单|表清单)\s*</div>\s*<div[^>]*class="[^"]*panel-body[^"]*"[^>]*>(.*?)</div>`)
	privilegeOperationPattern     = regexp.MustCompile(`(?is)<th[^>]*>\s*操作信息\s*</th>.*?<td[^>]*>(.*?)</td>`)
	privilegeBreakPattern         = regexp.MustCompile(`(?is)<br\s*/?>|</p>|</li>|</div>`)
)

// PrivilegeApplies 返回查询权限申请列表。
func (c *Client) PrivilegeApplies(ctx context.Context, limit, offset int, search string) (*PrivilegeApplyPage, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "权限管理仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("limit", strconv.Itoa(limit))
	form.Set("offset", strconv.Itoa(offset))
	form.Set("search", search)

	var payload struct {
		Total int              `json:"total"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/query/applylist/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}

	rows := make([]PrivilegeApply, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		rows = append(rows, PrivilegeApply{
			ApplyID:      toInt64(row["apply_id"]),
			Title:        toText(row["title"]),
			InstanceName: toText(row["instance__instance_name"]),
			DBList:       toText(row["db_list"]),
			PrivType:     int(toInt64(row["priv_type"])),
			TableList:    toText(row["table_list"]),
			LimitNum:     int(toInt64(row["limit_num"])),
			ValidDate:    toText(row["valid_date"]),
			UserDisplay:  toText(row["user_display"]),
			Status:       int(toInt64(row["status"])),
			CreateTime:   toText(row["create_time"]),
			GroupName:    toText(row["group_name"]),
		})
	}
	return &PrivilegeApplyPage{Total: payload.Total, Rows: rows}, nil
}

// UserQueryPrivileges 返回当前账号的查询权限清单（Archery「我的权限」页）。
// Archery 的 user_query_priv 对普通账号强制按 user_name 过滤，超管与
// 有 sql.query_mgtpriv 的账号会看到组内记录，因此这里不额外传 user_display。
func (c *Client) UserQueryPrivileges(ctx context.Context, limit, offset int, search string) (*UserPrivilegePage, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "我的权限仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("limit", strconv.Itoa(limit))
	form.Set("offset", strconv.Itoa(offset))
	form.Set("search", search)

	var payload struct {
		Total int              `json:"total"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/query/userprivileges/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}

	rows := make([]UserPrivilege, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		rows = append(rows, UserPrivilege{
			PrivilegeID:  toInt64(row["privilege_id"]),
			UserDisplay:  toText(row["user_display"]),
			InstanceName: toText(row["instance__instance_name"]),
			DBName:       toText(row["db_name"]),
			PrivType:     int(toInt64(row["priv_type"])),
			TableName:    toText(row["table_name"]),
			LimitNum:     int(toInt64(row["limit_num"])),
			ValidDate:    toText(row["valid_date"]),
		})
	}
	return &UserPrivilegePage{Total: payload.Total, Rows: rows}, nil
}

// PrivilegeGroups 从申请页 HTML 里解析当前用户可申请的资源组。
func (c *Client) PrivilegeGroups(ctx context.Context) ([]string, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "权限管理仅支持会话登录模式")
	}
	body, err := c.fetchPage(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/queryapplylist/",
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return parsePrivilegeGroups(body), nil
}

// parsePrivilegeGroups 提取 <select id="group_name"> 下的 option，去重并反转义。
func parsePrivilegeGroups(body []byte) []string {
	groups := make([]string, 0, 4)
	block := privilegeGroupSelectPattern.Find(body)
	if len(block) == 0 {
		return groups
	}
	seen := map[string]struct{}{}
	for _, match := range privilegeOptionPattern.FindAllSubmatch(block, -1) {
		value := strings.TrimSpace(html.UnescapeString(string(match[1])))
		if value == "" || value == "is-empty" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		groups = append(groups, value)
	}
	return groups
}

// GroupInstances 返回资源组内标记为「支持查询」的实例。
func (c *Client) GroupInstances(ctx context.Context, groupName string) ([]GroupInstance, error) {
	return c.GroupInstancesByTag(ctx, groupName, "can_read")
}

// GroupInstancesByTag 返回资源组内带指定标签的实例：
// can_read 用于查询权限申请，can_write 用于 SQL 上线提交。
func (c *Client) GroupInstancesByTag(ctx context.Context, groupName, tagCode string) ([]GroupInstance, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "该功能仅支持会话登录模式")
	}
	if strings.TrimSpace(tagCode) == "" {
		tagCode = "can_read"
	}
	form := url.Values{}
	form.Set("group_name", groupName)
	form.Set("tag_code", tagCode)

	data, err := c.envelope(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/group/instances/",
		Form:    form,
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}

	var items []map[string]any
	if err := decodeJSON(data, &items); err != nil {
		return nil, apiError(KindHTTP, "无法解析资源组实例列表")
	}
	instances := make([]GroupInstance, 0, len(items))
	for _, item := range items {
		instances = append(instances, GroupInstance{
			ID:     int(toInt64(item["id"])),
			Name:   toText(item["instance_name"]),
			Type:   toText(item["type"]),
			DBType: toText(item["db_type"]),
		})
	}
	return instances, nil
}

// ApplyPrivilege 提交一条查询权限申请。
// Archery 的申请接口按 priv_type 区分：库级提交 db_list[]，表级提交 db_name + table_list[]。
func (c *Client) ApplyPrivilege(ctx context.Context, input PrivilegeApplyInput) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "权限管理仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("title", input.Title)
	form.Set("group_name", input.GroupName)
	form.Set("instance_name", input.InstanceName)
	form.Set("priv_type", strconv.Itoa(input.PrivType))
	if input.DBName != "" {
		form.Set("db_name", input.DBName)
	}
	for _, db := range input.DBList {
		form.Add("db_list[]", db)
	}
	for _, table := range input.TableList {
		form.Add("table_list[]", table)
	}
	form.Set("valid_date", input.ValidDate)
	form.Set("limit_num", strconv.Itoa(input.LimitNum))

	_, err := c.envelope(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/query/applyforprivileges/",
		Form:    form,
		Timeout: c.cfg.QueryTimeout,
	})
	return err
}

// PrivilegeDetail 抓取工单详情页：解析标题、审批流、库/表清单与最后操作信息，并探测审核能力。
func (c *Client) PrivilegeDetail(ctx context.Context, applyID int64) (*PrivilegeDetail, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "权限管理仅支持会话登录模式")
	}
	body, err := c.fetchPage(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/queryapplydetail/%d/", applyID),
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return parsePrivilegeDetail(body, applyID), nil
}

// parsePrivilegeDetail 按区块切分详情页，不建 DOM：
// 结构来自 v1.14 的 queryapplydetail.html + workflow_display.html，任一区块缺失都返回空值。
func parsePrivilegeDetail(body []byte, applyID int64) *PrivilegeDetail {
	detail := &PrivilegeDetail{
		ApplyID:     applyID,
		CanReview:   bytes.Contains(body, []byte(`id="btnPass"`)),
		ReviewNodes: []PrivilegeReviewNode{},
		Items:       []string{},
	}
	if match := privilegeDetailTitlePattern.FindSubmatch(body); len(match) == 2 {
		detail.Title = cleanHTMLText(match[1])
	}
	detail.ReviewNodes = parseReviewNodes(body)
	detail.Items, detail.ItemKind = parseDetailItems(body)
	if match := privilegeOperationPattern.FindSubmatch(body); len(match) == 2 {
		detail.LastOperation = cleanHTMLText(match[1])
	}
	return detail
}

// parseReviewNodes 取「审批流」到「其他信息」之间的节点链。
// workflow_display.html 用 glyphicon-arrow-right 连接节点，节点状态靠 span/u 的 class 区分。
func parseReviewNodes(body []byte) []PrivilegeReviewNode {
	nodes := make([]PrivilegeReviewNode, 0, 4)
	start := privilegeReviewHeadingPattern.FindIndex(body)
	if start == nil {
		return nodes
	}
	block := body[start[1]:]
	if end := privilegeInfoHeadingPattern.FindIndex(block); end != nil {
		block = block[:end[0]]
	}
	for _, chunk := range privilegeArrowPattern.Split(string(block), -1) {
		if node, ok := parseReviewNode(chunk); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func parseReviewNode(chunk string) (PrivilegeReviewNode, bool) {
	node := PrivilegeReviewNode{Reviewers: []string{}}
	text := cleanHTMLText([]byte(chunk))
	switch {
	case strings.Contains(chunk, "text-success") && strings.Contains(text, "无需审批"):
		// 自动通过的节点只渲染「无需审批」，没有组名。
		node.State = "auto"
		return node, true
	case strings.Contains(chunk, "text-success"):
		node.State = "passed"
	case strings.Contains(chunk, "text-danger") || strings.Contains(chunk, "<u"):
		node.State = "current"
	default:
		node.State = "pending"
	}
	if node.State == "current" {
		if match := privilegeReviewerPattern.FindStringSubmatch(chunk); len(match) == 2 {
			reviewers := cleanHTMLText([]byte(match[1]))
			node.Reviewers = splitCommaList(reviewers)
			// 渲染结果是「组名(审批人)」，括号内已单独取走。
			text = strings.TrimSpace(privilegeTrailingParenPattern.ReplaceAllString(text, ""))
		}
	}
	node.Name = text
	return node, node.Name != ""
}

// parseDetailItems 解析「数据库清单 / 表清单」面板，返回清单与类型（database / table）。
func parseDetailItems(body []byte) ([]string, string) {
	items := make([]string, 0, 4)
	match := privilegePanelPattern.FindSubmatch(body)
	if len(match) != 3 {
		return items, ""
	}
	kind := "database"
	if string(match[1]) == "表清单" {
		kind = "table"
	}
	seen := map[string]struct{}{}
	for _, block := range privilegeBreakPattern.Split(string(match[2]), -1) {
		item := cleanHTMLText([]byte(block))
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		items = append(items, item)
	}
	// 部分版本的 format_str 输出逗号串而不是分行，单块时再拆一次。
	if len(items) == 1 && strings.ContainsAny(items[0], ",，") {
		items = splitCommaList(items[0])
	}
	return items, kind
}

// cleanHTMLText 去标签、反转义并压缩连续空白。
func cleanHTMLText(raw []byte) string {
	text := html.UnescapeString(htmlTagPattern.ReplaceAllString(string(raw), " "))
	return strings.Join(strings.Fields(text), " ")
}

// splitCommaList 按中英文逗号拆分并去空。
func splitCommaList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// AuditPrivilege 审核（通过/终止）一条权限工单。
// Archery 成功时 302 回详情页，失败时渲染 error.html（HTTP 200），因此不能走 do 的解码路径。
func (c *Client) AuditPrivilege(ctx context.Context, applyID int64, status int, remark string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "权限管理仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("apply_id", strconv.FormatInt(applyID, 10))
	form.Set("audit_status", strconv.Itoa(status))
	form.Set("audit_remark", remark)
	call := httpCall{
		Method:  http.MethodPost,
		Path:    "/query/privaudit/",
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}

	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return err
	}
	if c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return err
		}
	}

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil
	case resp.StatusCode == http.StatusForbidden:
		return apiError(KindForbidden, "无权限审核该工单（需要 sql.query_review 权限）")
	case resp.StatusCode == http.StatusNotFound:
		return apiError(KindNotFound, "Archery 未提供权限审核接口（HTTP 404）")
	case resp.StatusCode >= 400:
		return statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if message := extractErrorHeading(body); message != "" {
		return apiError(KindServer, message)
	}
	return apiError(KindServer, "审核失败：Archery 未返回结果")
}

// extractErrorHeading 从 Archery error.html 中取出 <h4> 错误文案。
func extractErrorHeading(body []byte) string {
	match := errorHeadingPattern.FindSubmatch(body)
	if len(match) != 2 {
		return ""
	}
	text := htmlTagPattern.ReplaceAllString(string(match[1]), "")
	return strings.TrimSpace(html.UnescapeString(text))
}

// WorkflowLogs 返回工单操作日志。
// workflow_type 缺省为查询权限工单；SQL 上线工单传 WorkflowTypeSqlReview。
func (c *Client) WorkflowLogs(ctx context.Context, workflowID int64, workflowTypes ...int) (*WorkflowLogPage, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "工单日志仅支持会话登录模式")
	}
	workflowType := WorkflowTypeQueryPriv
	if len(workflowTypes) > 0 && workflowTypes[0] > 0 {
		workflowType = workflowTypes[0]
	}
	form := url.Values{}
	form.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	form.Set("workflow_type", strconv.Itoa(workflowType))

	var payload struct {
		Total int              `json:"total"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/workflow/log/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}

	rows := make([]WorkflowLog, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		rows = append(rows, WorkflowLog{
			OperationType: toText(row["operation_type_desc"]),
			Operator:      toText(row["operator_display"]),
			Time:          toText(row["operation_time"]),
			Info:          toText(row["operation_info"]),
		})
	}
	return &WorkflowLogPage{Total: payload.Total, Rows: rows}, nil
}

// ---- 待办列表（页面 /workflow/，数据接口 /workflow/list/） ----

// 待办详情是 302：目标路径决定工单类型与工单号。
// 用「路径末尾匹配」兼容部署在子路径下的实例（如 /archery/detail/5338/）。
var workflowTodoLocationPatterns = []struct {
	pattern      *regexp.Regexp
	workflowType int
}{
	{regexp.MustCompile(`(?:^|/)detail/(\d+)/?$`), WorkflowTypeSqlReview},
	{regexp.MustCompile(`(?:^|/)queryapplydetail/(\d+)/?$`), WorkflowTypeQueryPriv},
	{regexp.MustCompile(`(?:^|/)archive/(\d+)/?$`), WorkflowTypeArchive},
}

// WorkflowTodos 返回「待我审核」的待办列表。
// Archery 的 lists 视图已按「资源组 + 当前审核组是我的组 + 待审核」过滤，
// 因此这里只透传分页、搜索与工单类型。
func (c *Client) WorkflowTodos(ctx context.Context, query WorkflowTodoQuery) (*WorkflowTodoPage, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "待办列表仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("limit", strconv.Itoa(query.Limit))
	form.Set("offset", strconv.Itoa(query.Offset))
	form.Set("workflow_type", strconv.Itoa(query.WorkflowType))
	form.Set("search", query.Search)

	var payload struct {
		Total int              `json:"total"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/workflow/list/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}

	rows := make([]WorkflowTodo, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		rows = append(rows, WorkflowTodo{
			AuditID:      toInt64(row["audit_id"]),
			WorkflowType: int(toInt64(row["workflow_type"])),
			Title:        toText(row["workflow_title"]),
			Applicant:    toText(row["create_user_display"]),
			CreateTime:   toText(row["create_time"]),
			Status:       int(toInt64(row["current_status"])),
			GroupName:    toText(row["group_name"]),
			CurrentAudit: toText(row["current_audit"]),
			AuditGroups:  toText(row["audit_auth_groups"]),
		})
	}
	return &WorkflowTodoPage{Total: payload.Total, Rows: rows}, nil
}

// WorkflowTodoTarget 解析待办对应的业务工单号（审核入口需要）。
// 这一步不能省：待办行只有 audit_id，而详情/审核接口用的是业务工单号。
func (c *Client) WorkflowTodoTarget(ctx context.Context, auditID int64) (*WorkflowTodoTarget, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "待办列表仅支持会话登录模式")
	}
	call := httpCall{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/workflow/%d/", auditID),
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return nil, err
	}
	if c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return nil, reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return nil, err
		}
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, apiError(KindNotFound, "该待办对应的工单不存在（HTTP 404）")
	case resp.StatusCode == http.StatusForbidden:
		return nil, apiError(KindForbidden, "无权限查看该待办工单")
	case resp.StatusCode >= 400:
		return nil, statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return nil, apiError(KindHTTP, "Archery 未返回工单详情地址，无法定位待办")
	}
	parsed, parseErr := url.Parse(location)
	if parseErr != nil {
		return nil, apiError(KindHTTP, "无法解析工单详情地址")
	}
	target := &WorkflowTodoTarget{AuditID: auditID}
	for _, item := range workflowTodoLocationPatterns {
		match := item.pattern.FindStringSubmatch(parsed.Path)
		if len(match) != 2 {
			continue
		}
		target.WorkflowType = item.workflowType
		target.WorkflowID = toInt64(match[1])
		break
	}
	if target.WorkflowID <= 0 {
		return nil, apiError(KindHTTP, "无法识别待办对应的工单类型："+parsed.Path)
	}
	return target, nil
}

// fetchPage 拉取 HTML 页面并复用会话失效重登逻辑。
func (c *Client) fetchPage(ctx context.Context, call httpCall) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return nil, err
	}
	if c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return nil, reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return nil, err
		}
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, apiError(KindAuthRequired, "登录状态已失效，请重新登录")
	case resp.StatusCode == http.StatusForbidden:
		return nil, apiError(KindForbidden, "无权限访问该页面（缺少对应菜单权限）")
	case resp.StatusCode == http.StatusNotFound:
		return nil, apiError(KindNotFound, "Archery 未提供该页面（HTTP 404）")
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
	case resp.StatusCode >= 400:
		return nil, statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if looksLikeLoginPage(body) {
		return nil, apiError(KindAuthRequired, "Archery 会话已失效，请重新登录")
	}
	return body, nil
}

// ---- SQL 上线工单（会话模式；提交与 SQL 检测走 DRF /api/v1/workflow/） ----

var (
	// 列表页筛选下拉：按 id 区分实例与资源组，option 的 value 即 instance_id / group_id。
	workflowSelectPattern   = regexp.MustCompile(`(?is)<select\b[^>]*>.*?</select>`)
	workflowSelectIDPattern = regexp.MustCompile(`(?is)<select\b[^>]*\bid="([^"]*)"`)
	workflowOptionPattern   = regexp.MustCompile(`(?is)<option\b[^>]*?value="([^"]*)"[^>]*>(.*?)</option>`)
	// 提交页的组 option：value 是组名，group-id 才是提交用的 ID。
	submitGroupSelectPattern = regexp.MustCompile(`(?is)<select[^>]*id="group_name".*?</select>`)
	optionTagPattern         = regexp.MustCompile(`(?is)<option\b([^>]*)>(.*?)</option>`)
	optionValueAttrPattern   = regexp.MustCompile(`(?is)\bvalue="([^"]*)"`)
	optionGroupIDAttrPattern = regexp.MustCompile(`(?is)\bgroup-id="(\d+)"`)
	// 详情页「其他信息」：表头 th 与唯一一行 success 的 td 按顺序配对。
	infoHeadingPattern   = regexp.MustCompile(`(?is)<h4[^>]*>\s*其他信息\s*</h4>`)
	infoNextHeadingIndex = regexp.MustCompile(`(?is)<h4[^>]*>`)
	infoTablePattern     = regexp.MustCompile(`(?is)<table[^>]*>(.*?)</table>`)
	thCellPattern        = regexp.MustCompile(`(?is)<th[^>]*>(.*?)</th>`)
	successRowPattern    = regexp.MustCompile(`(?is)<tr[^>]*class="[^"]*\bsuccess\b[^"]*"[^>]*>(.*?)</tr>`)
	tdCellPattern        = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
)

// 「待我审核」的扫描窗口：审批队列与待审工单各最多取 1000 行。
// Archery 的列表接口没有「按当前审核组过滤」的参数，只能先取回再求交集；
// 个人待办通常只有几十条，1000 行足够覆盖，同时避免超大待审队列把页面拖死。
const (
	sqlWorkflowAuditPageSize  = 200
	sqlWorkflowAuditScanLimit = 1000
)

// SqlWorkflows 返回 SQL 上线工单列表；query.Audit 为真时只返回当前需要我审核的工单。
func (c *Client) SqlWorkflows(ctx context.Context, query SqlWorkflowQuery) (*SqlWorkflowPage, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	if query.Audit {
		return c.sqlWorkflowsAwaitingMe(ctx, query)
	}
	return c.sqlWorkflowListPage(ctx, query)
}

// sqlWorkflowsAwaitingMe 用 /workflow/list/ 的审批队列与待审核工单求交集。
//
// Archery v1.14 的 /sqlworkflow_list_audit/ 只是给 /sqlworkflow_list/ 加了
// sql.audit_user 权限装饰器，查询条件与普通列表完全一致，判断不出「当前审核节点
// 是否轮到我的审核组」；按 current_audit 过滤的只有 common/workflow.py::lists。
// 两边的行都没有对方的主键，因此按「工单名 + 资源组」求交集（同一资源组的同类
// 工单共用一条审批流配置），再用发起人排除同名不同人的工单。
func (c *Client) sqlWorkflowsAwaitingMe(ctx context.Context, query SqlWorkflowQuery) (*SqlWorkflowPage, error) {
	// 审批队列不传 search：/workflow/list/ 只按工单名匹配，会把按发起人搜的结果直接过滤掉；
	// 搜索统一交给 /sqlworkflow_list/（工单名 + 发起人），交集后结果自然收敛。
	audits, err := c.workflowTodosScan(ctx, WorkflowTypeSqlReview, "")
	if err != nil {
		return nil, err
	}
	page := &SqlWorkflowPage{Rows: []SqlWorkflow{}}
	if len(audits) == 0 {
		return page, nil
	}
	index := make(map[string][]WorkflowTodo, len(audits))
	for _, audit := range audits {
		key := sqlWorkflowAuditKey(audit.Title, audit.GroupName)
		index[key] = append(index[key], audit)
	}

	pending := query
	pending.Status = "workflow_manreviewing"
	workflows, err := c.sqlWorkflowListScan(ctx, pending)
	if err != nil {
		return nil, err
	}
	matched := make([]SqlWorkflow, 0, len(workflows))
	for _, workflow := range workflows {
		if workflowNeedsAudit(workflow, index) {
			matched = append(matched, workflow)
		}
	}

	page.Total = len(matched)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	limit := query.Limit
	if limit <= 0 {
		limit = sqlWorkflowAuditPageSize
	}
	if offset < len(matched) {
		end := offset + limit
		if end > len(matched) {
			end = len(matched)
		}
		page.Rows = append(page.Rows, matched[offset:end]...)
	}
	return page, nil
}

// sqlWorkflowAuditKey 是交集键：Archery 的审批流按「资源组 + 工单类型」配置，
// 同名同组即同一张审批单；发起人用来区分同名工单。
func sqlWorkflowAuditKey(title, group string) string {
	return strings.TrimSpace(title) + "\x00" + strings.TrimSpace(group)
}

// workflowNeedsAudit 判断工单是否在当前待我审核的队列里。
func workflowNeedsAudit(row SqlWorkflow, index map[string][]WorkflowTodo) bool {
	for _, audit := range index[sqlWorkflowAuditKey(row.WorkflowName, row.GroupName)] {
		// 同名同组也可能来自不同提交人，发起人可区分。
		if audit.Applicant != "" && row.EngineerDisplay != "" && audit.Applicant != row.EngineerDisplay {
			continue
		}
		return true
	}
	return false
}

// workflowTodosScan 分页拉取完整的「待我审核」队列（/workflow/list/）。
func (c *Client) workflowTodosScan(ctx context.Context, workflowType int, search string) ([]WorkflowTodo, error) {
	rows := make([]WorkflowTodo, 0, sqlWorkflowAuditPageSize)
	for offset := 0; offset < sqlWorkflowAuditScanLimit; offset += sqlWorkflowAuditPageSize {
		page, err := c.WorkflowTodos(ctx, WorkflowTodoQuery{
			Limit:        sqlWorkflowAuditPageSize,
			Offset:       offset,
			Search:       search,
			WorkflowType: workflowType,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Rows...)
		if len(page.Rows) < sqlWorkflowAuditPageSize || len(rows) >= page.Total {
			break
		}
	}
	return rows, nil
}

// sqlWorkflowListScan 分页拉取待审核工单（带页面筛选），供交集使用。
func (c *Client) sqlWorkflowListScan(ctx context.Context, query SqlWorkflowQuery) ([]SqlWorkflow, error) {
	rows := make([]SqlWorkflow, 0, sqlWorkflowAuditPageSize)
	for offset := 0; offset < sqlWorkflowAuditScanLimit; offset += sqlWorkflowAuditPageSize {
		pageQuery := query
		pageQuery.Offset = offset
		pageQuery.Limit = sqlWorkflowAuditPageSize
		page, err := c.sqlWorkflowListPage(ctx, pageQuery)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Rows...)
		if len(page.Rows) < sqlWorkflowAuditPageSize || len(rows) >= page.Total {
			break
		}
	}
	return rows, nil
}

// sqlWorkflowListPage 取 /sqlworkflow_list/ 的一页。
func (c *Client) sqlWorkflowListPage(ctx context.Context, query SqlWorkflowQuery) (*SqlWorkflowPage, error) {
	form := url.Values{}
	form.Set("limit", strconv.Itoa(query.Limit))
	form.Set("offset", strconv.Itoa(query.Offset))
	form.Set("search", query.Search)
	if query.Status != "" {
		form.Set("navStatus", query.Status)
	}
	if query.InstanceID > 0 {
		form.Set("instance_id", strconv.Itoa(query.InstanceID))
	}
	if query.GroupID > 0 {
		form.Set("group_id", strconv.Itoa(query.GroupID))
	}
	// Archery 只在起止日期同时存在时才按创建时间过滤，单边日期会被忽略。
	if query.StartDate != "" && query.EndDate != "" {
		form.Set("start_date", query.StartDate)
		form.Set("end_date", query.EndDate)
	}
	for _, syntax := range query.SyntaxTypes {
		if value := strings.TrimSpace(syntax); value != "" {
			form.Add("syntax_type[]", value)
		}
	}

	var payload struct {
		Total int              `json:"total"`
		Rows  []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/sqlworkflow_list/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}

	rows := make([]SqlWorkflow, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		rows = append(rows, SqlWorkflow{
			ID:              toInt64(row["id"]),
			WorkflowName:    toText(row["workflow_name"]),
			EngineerDisplay: toText(row["engineer_display"]),
			Status:          toText(row["status"]),
			IsBackup:        toBool(row["is_backup"]),
			CreateTime:      toText(row["create_time"]),
			InstanceName:    toText(row["instance__instance_name"]),
			DBName:          toText(row["db_name"]),
			GroupName:       toText(row["group_name"]),
			SyntaxType:      int(toInt64(row["syntax_type"])),
			ExportFormat:    toText(row["export_format"]),
		})
	}
	return &SqlWorkflowPage{Total: payload.Total, Rows: rows}, nil
}

// SqlWorkflowOptions 从 /sqlworkflow/ 页面解析筛选下拉（只列当前账号可见工单涉及的实例与组）。
func (c *Client) SqlWorkflowOptions(ctx context.Context) (*SqlWorkflowOptions, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	body, err := c.fetchPage(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/sqlworkflow/",
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	options := &SqlWorkflowOptions{
		Instances: []SqlWorkflowOption{},
		Groups:    []SqlWorkflowOption{},
	}
	for _, block := range workflowSelectPattern.FindAll(body, -1) {
		if match := workflowSelectIDPattern.FindSubmatch(block); len(match) == 2 {
			switch id := strings.ToLower(string(match[1])); {
			case strings.Contains(id, "instance"):
				options.Instances = parseWorkflowOptions(block)
			case strings.Contains(id, "group"):
				options.Groups = parseWorkflowOptions(block)
			}
		}
	}
	return options, nil
}

// parseWorkflowOptions 只收 id 为正整数的 option，跳过「全部」占位项。
func parseWorkflowOptions(block []byte) []SqlWorkflowOption {
	options := make([]SqlWorkflowOption, 0, 8)
	seen := map[int]struct{}{}
	for _, match := range workflowOptionPattern.FindAllSubmatch(block, -1) {
		id, err := strconv.Atoi(strings.TrimSpace(html.UnescapeString(string(match[1]))))
		if err != nil || id <= 0 {
			continue
		}
		name := cleanHTMLText(match[2])
		if name == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		options = append(options, SqlWorkflowOption{ID: id, Name: name})
	}
	return options
}

// GroupAuditors 返回资源组在指定工单类型下的审批流展示串。
// workflowType 取 WorkflowTypeQueryPriv（申请查询权限页）或 WorkflowTypeSqlReview（SQL 上线页），
// 同一个资源组两类工单可以配置不同的审批流，不能混用。
func (c *Client) GroupAuditors(ctx context.Context, groupName string, workflowType int) (string, string, error) {
	if !c.cfg.SessionMode() {
		return "", "", apiError(KindUnsupported, "审批流程查询仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("group_name", groupName)
	form.Set("workflow_type", strconv.Itoa(workflowType))
	var payload struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   *struct {
			Auditors        string `json:"auditors"`
			AuditorsDisplay string `json:"auditors_display"`
		} `json:"data"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/group/auditors/",
		Form:    form,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return "", "", err
	}
	if payload.Status != 0 {
		return "", "", apiError(KindServer, fallback(payload.Msg, "无法获取审批流程"))
	}
	if payload.Data == nil {
		return "", "", nil
	}
	return payload.Data.AuditorsDisplay, payload.Data.Auditors, nil
}

// SqlSubmitOptions 从 /submitsql/ 页面解析提交弹窗的数据源：
// 组 option 带 group-id 属性，是提交工单 group_id 的唯一来源；
// 备份开关只在支持备份的引擎下渲染，用 <select id="is_backup"> 是否存在判断。
func (c *Client) SqlSubmitOptions(ctx context.Context) (*SqlSubmitOptions, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	body, err := c.fetchPage(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/submitsql/",
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	options := &SqlSubmitOptions{
		Groups:        []SqlWorkflowOption{},
		BackupEnabled: strings.Contains(string(body), `id="is_backup"`),
	}
	block := submitGroupSelectPattern.Find(body)
	if len(block) == 0 {
		return options, nil
	}
	seen := map[int]struct{}{}
	for _, match := range optionTagPattern.FindAllSubmatch(block, -1) {
		name := cleanHTMLText(match[2])
		groupID := 0
		attrs := string(match[1])
		if idMatch := optionGroupIDAttrPattern.FindStringSubmatch(attrs); len(idMatch) == 2 {
			groupID, _ = strconv.Atoi(idMatch[1])
		}
		if groupID == 0 {
			if valueMatch := optionValueAttrPattern.FindStringSubmatch(attrs); len(valueMatch) == 2 {
				if parsed, parseErr := strconv.Atoi(strings.TrimSpace(html.UnescapeString(valueMatch[1]))); parseErr == nil {
					groupID = parsed
				}
			}
		}
		if groupID <= 0 || name == "" {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		options.Groups = append(options.Groups, SqlWorkflowOption{ID: groupID, Name: name})
	}
	return options, nil
}

// SqlWorkflowDetail 返回工单详情：审批流与操作信息取详情页，
// SQL 审核/执行结果取 /sqlworkflow/detail_content/。
func (c *Client) SqlWorkflowDetail(ctx context.Context, workflowID int64) (*SqlWorkflowDetail, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	body, err := c.fetchPage(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/detail/%d/", workflowID),
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	detail := &SqlWorkflowDetail{
		WorkflowID:  workflowID,
		ReviewNodes: []PrivilegeReviewNode{},
		InfoFields:  []InfoField{},
		ReviewRows:  []SqlReviewRow{},
	}
	if match := privilegeDetailTitlePattern.FindSubmatch(body); len(match) == 2 {
		detail.Title = cleanHTMLText(match[1])
	}
	detail.ReviewNodes = parseReviewNodes(body)
	detail.InfoFields = parseInfoFields(body)
	if match := privilegeOperationPattern.FindSubmatch(body); len(match) == 2 {
		detail.LastOperation = cleanHTMLText(match[1])
	}

	rows, rowsErr := c.sqlWorkflowContent(ctx, workflowID)
	switch {
	case rowsErr == nil:
		detail.ReviewRows = rows
	case IsKind(rowsErr, KindAuthRequired), IsKind(rowsErr, KindNetwork), IsKind(rowsErr, KindTimeout):
		return nil, rowsErr
	}
	// 审核内容为空的老工单会让 detail_content 返回 500，此时详情页其余区块照常展示。
	return detail, nil
}

// sqlWorkflowContent 读取工单的审核/执行结果行。
func (c *Client) sqlWorkflowContent(ctx context.Context, workflowID int64) ([]SqlReviewRow, error) {
	query := url.Values{}
	query.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	var payload struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/sqlworkflow/detail_content/",
		Query:   query,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}
	return mapReviewRows(payload.Rows), nil
}

// mapReviewRows 把 Archery ReviewResult 行转成前端结构。
func mapReviewRows(rows []map[string]any) []SqlReviewRow {
	out := make([]SqlReviewRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, SqlReviewRow{
			Sequence:     toText(row["sequence"]),
			Stage:        toText(row["stage"]),
			Status:       toText(row["stagestatus"]),
			ErrorLevel:   int(toInt64(row["errlevel"])),
			ErrorMessage: toText(row["errormessage"]),
			SQL:          toText(row["sql"]),
			AffectedRows: toInt64(row["affected_rows"]),
			BackupDBName: toText(row["backup_dbname"]),
			ExecuteTime:  toText(row["execute_time"]),
		})
	}
	return out
}

// parseInfoFields 解析详情页「其他信息」：表头 th 与唯一一行 success 的 td 按顺序配对。
// 表头与数据列数不一致时整块丢弃，避免前端错位展示。
func parseInfoFields(body []byte) []InfoField {
	start := infoHeadingPattern.FindIndex(body)
	if start == nil {
		return nil
	}
	block := body[start[1]:]
	if next := infoNextHeadingIndex.FindIndex(block); next != nil {
		block = block[:next[0]]
	}
	// 区块里可能紧跟 SQL 列表等别的表格（没有下一个 h4 分隔）：只认「其他信息」后的第一张表，
	// 否则后面的表头会一起算进列名，列数对不上会把整块信息丢掉。
	scope := block
	if table := infoTablePattern.FindSubmatch(block); len(table) == 2 {
		scope = table[1]
	}
	labels := make([]string, 0, 10)
	for _, match := range thCellPattern.FindAllSubmatch(scope, -1) {
		if label := cleanHTMLText(match[1]); label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return nil
	}
	row := successRowPattern.FindSubmatch(scope)
	if len(row) != 2 {
		return nil
	}
	values := make([]string, 0, len(labels))
	for _, match := range tdCellPattern.FindAllSubmatch(row[1], -1) {
		values = append(values, cleanHTMLText(match[1]))
	}
	if len(values) != len(labels) {
		return nil
	}
	fields := make([]InfoField, 0, len(labels))
	for index, label := range labels {
		fields = append(fields, InfoField{Label: label, Value: values[index]})
	}
	return fields
}

// RollbackSQL 读取工单回滚语句（含对应的执行语句）。
// 详情页的「查看回滚SQL」走 /rollback/ 页面异步拉取，数据源同样是 backup_sql；
// goInception 引擎返回 [执行语句, 回滚语句] 的二元数组并按执行顺序倒序。
func (c *Client) RollbackSQL(ctx context.Context, workflowID int64) ([]SqlRollbackRow, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	query := url.Values{}
	query.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	var payload struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Rows   []any  `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodGet,
		Path:    "/sqlworkflow/backup_sql/",
		Query:   query,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}
	if payload.Status != 0 {
		return nil, apiError(KindServer, fallback(payload.Msg, "无法获取回滚 SQL"))
	}
	return parseRollbackRows(payload.Rows), nil
}

// parseRollbackRows 兼容三种行：[执行语句, 回滚语句]（goInception 实际结构）、
// {sql, rollback_sql}（按字段名的版本）与纯回滚字符串（旧兜底），空行丢弃。
func parseRollbackRows(rows []any) []SqlRollbackRow {
	out := make([]SqlRollbackRow, 0, len(rows))
	for _, row := range rows {
		var item SqlRollbackRow
		switch typed := row.(type) {
		case []any:
			if len(typed) > 0 {
				item.SQL = strings.TrimSpace(toText(typed[0]))
			}
			if len(typed) > 1 {
				item.RollbackSQL = strings.TrimSpace(toText(typed[1]))
			}
		case map[string]any:
			item.SQL = strings.TrimSpace(toText(typed["sql"]))
			item.RollbackSQL = strings.TrimSpace(toText(typed["rollback_sql"]))
			if item.RollbackSQL == "" {
				item.RollbackSQL = strings.TrimSpace(toText(typed["rollbackSql"]))
			}
		case string:
			item.RollbackSQL = strings.TrimSpace(typed)
		}
		if item.SQL == "" && item.RollbackSQL == "" {
			continue
		}
		item.ID = len(out) + 1
		out = append(out, item)
	}
	return out
}

// AuditSqlWorkflow 审核 SQL 上线工单：pass → /passed/，reject → /cancel/（驳回）。
func (c *Client) AuditSqlWorkflow(ctx context.Context, workflowID int64, action, remark string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	path := "/passed/"
	if action == WorkflowActionReject {
		path = "/cancel/"
		form.Set("cancel_remark", remark)
	} else {
		form.Set("audit_remark", remark)
	}
	return c.postWorkflowAction(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    path,
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}, "无权限审核该工单（需要 sql.sql_review 权限）")
}

// ExecuteSqlWorkflow 执行工单：auto=平台排队执行，manual=确认线下执行完成。
func (c *Client) ExecuteSqlWorkflow(ctx context.Context, workflowID int64, mode string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	form.Set("mode", mode)
	return c.postWorkflowAction(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/execute/",
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.QueryTimeout,
	}, "无权限执行该工单（需要 sql.sql_execute 权限）")
}

// ScheduleSqlWorkflow 设置定时执行，runDate 形如 2026-11-05 03:00。
func (c *Client) ScheduleSqlWorkflow(ctx context.Context, workflowID int64, runDate string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	form.Set("run_date", runDate)
	return c.postWorkflowAction(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/timingtask/",
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}, "无权限设置定时执行（需要 sql.sql_execute 权限）")
}

// AlterSqlWorkflowRunDate 审核人修改可执行时间（Archery /alter_run_date/）：
// 两端留空表示清空窗口，详情页会展示「无限制」。
func (c *Client) AlterSqlWorkflowRunDate(ctx context.Context, workflowID int64, start, end string) error {
	if !c.cfg.SessionMode() {
		return apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	form := url.Values{}
	form.Set("workflow_id", strconv.FormatInt(workflowID, 10))
	form.Set("run_date_start", strings.TrimSpace(start))
	form.Set("run_date_end", strings.TrimSpace(end))
	return c.postWorkflowAction(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/alter_run_date/",
		Form:    form,
		Accept:  "text/html,application/xhtml+xml",
		Timeout: c.cfg.Timeout,
	}, "无权限修改可执行时间（仅当前审核人可操作）")
}

// SubmitSqlWorkflow 提交 SQL 上线工单，返回新工单号。
// Archery 网页表单同样以 JSON 提交到 /api/v1/workflow/。
func (c *Client) SubmitSqlWorkflow(ctx context.Context, input SqlWorkflowSubmitInput) (int64, error) {
	if !c.cfg.SessionMode() {
		return 0, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	body := map[string]any{
		"workflow": map[string]any{
			"workflow_name":     input.Title,
			"demand_url":        input.DemandURL,
			"group_id":          input.GroupID,
			"instance":          input.InstanceID,
			"db_name":           input.DBName,
			"is_backup":         input.IsBackup,
			"run_date_start":    input.RunDateStart,
			"run_date_end":      input.RunDateEnd,
			"is_offline_export": 0,
		},
		"sql_content": input.SQL,
	}
	var payload struct {
		ID       any `json:"id"`
		Workflow struct {
			ID any `json:"id"`
		} `json:"workflow"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/workflow/",
		JSON:    body,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return 0, err
	}
	workflowID := toInt64(payload.Workflow.ID)
	if workflowID == 0 {
		workflowID = toInt64(payload.ID)
	}
	if workflowID == 0 {
		return 0, apiError(KindServer, "工单已提交但未返回工单号，请到 Archery 网页确认")
	}
	return workflowID, nil
}

// CheckSqlWorkflow 提交前做 SQL 检测（Archery 网页同样要求先检测再提交）。
func (c *Client) CheckSqlWorkflow(ctx context.Context, instanceID int, dbName, sql string) (*SqlCheckSummary, error) {
	if !c.cfg.SessionMode() {
		return nil, apiError(KindUnsupported, "SQL 上线工单仅支持会话登录模式")
	}
	// v1.14 的页面脚本发 instance_id，DRF 序列化器读 instance；两个键都带以兼容两种实现。
	body := map[string]any{
		"instance":    instanceID,
		"instance_id": instanceID,
		"db_name":     dbName,
		"full_sql":    sql,
	}
	var payload struct {
		ErrorCount   *int             `json:"error_count"`
		WarningCount *int             `json:"warning_count"`
		Rows         []map[string]any `json:"rows"`
	}
	if err := c.do(ctx, httpCall{
		Method:  http.MethodPost,
		Path:    "/api/v1/workflow/sqlcheck/",
		JSON:    body,
		Out:     &payload,
		Timeout: c.cfg.QueryTimeout,
	}); err != nil {
		return nil, err
	}
	summary := &SqlCheckSummary{Rows: mapReviewRows(payload.Rows)}
	for _, row := range summary.Rows {
		switch row.ErrorLevel {
		case 2:
			summary.ErrorCount++
		case 1:
			summary.WarningCount++
		}
	}
	if payload.ErrorCount != nil {
		summary.ErrorCount = *payload.ErrorCount
	}
	if payload.WarningCount != nil {
		summary.WarningCount = *payload.WarningCount
	}
	return summary, nil
}

// postWorkflowAction 提交会重定向的工单动作：成功 3xx，失败渲染 error.html（HTTP 200）。
func (c *Client) postWorkflowAction(ctx context.Context, call httpCall, forbidden string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	epoch, authHeader := c.snapshot()
	resp, body, err := c.execute(ctx, call, authHeader)
	if err != nil {
		return err
	}
	if c.authLost(resp, body) {
		c.dropPersisted()
		if reloginErr := c.relogin(ctx, epoch); reloginErr != nil {
			return reloginErr
		}
		_, authHeader = c.snapshot()
		resp, body, err = c.execute(ctx, call, authHeader)
		if err != nil {
			return err
		}
	}

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil
	case resp.StatusCode == http.StatusForbidden:
		return apiError(KindForbidden, forbidden)
	case resp.StatusCode == http.StatusNotFound:
		return apiError(KindNotFound, "Archery 未提供该工单操作接口（HTTP 404）")
	case resp.StatusCode >= 400:
		return statusError(resp.StatusCode, fmt.Sprintf("Archery 返回 HTTP %d", resp.StatusCode))
	}
	if message := extractErrorHeading(body); message != "" {
		return apiError(KindServer, message)
	}
	return apiError(KindServer, "操作失败：Archery 未返回结果")
}
