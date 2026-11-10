package archery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionStore 把 Django 会话 Cookie 落到插件数据目录。
// sidecar 进程随连接开关重启，内存里的 CookieJar 会丢；落盘后行为才和网页版一致。
type SessionStore struct {
	path string
	mu   sync.Mutex
}

type sessionFile struct {
	Sessions map[string]sessionEntry `json:"sessions"`
}

type sessionEntry struct {
	BaseURL  string            `json:"baseURL"`
	Username string            `json:"username"`
	Cookies  map[string]string `json:"cookies"`
	SavedAt  time.Time         `json:"savedAt"`
}

// NewFileSessionStore 返回以 path 为存储文件的会话仓库。
func NewFileSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

// Load 返回某个账号上次保存的 Cookie，没有则返回 nil。
func (s *SessionStore) Load(baseURL, username string) map[string]string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.readLocked()
	if !ok {
		return nil
	}
	entry, ok := file.Sessions[sessionKey(baseURL, username)]
	if !ok || len(entry.Cookies) == 0 {
		return nil
	}
	cookies := make(map[string]string, len(entry.Cookies))
	for name, value := range entry.Cookies {
		cookies[name] = value
	}
	return cookies
}

// Save 覆盖保存某个账号的 Cookie。
func (s *SessionStore) Save(baseURL, username string, cookies map[string]string) {
	if s == nil || len(cookies) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.readLocked()
	if !ok || file.Sessions == nil {
		file = sessionFile{Sessions: map[string]sessionEntry{}}
	}
	file.Sessions[sessionKey(baseURL, username)] = sessionEntry{
		BaseURL:  baseURL,
		Username: username,
		Cookies:  cookies,
		SavedAt:  time.Now().UTC(),
	}
	s.writeLocked(file)
}

// Delete 丢弃某个账号的会话；服务端已判失效时调用。
func (s *SessionStore) Delete(baseURL, username string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.readLocked()
	if !ok {
		return
	}
	key := sessionKey(baseURL, username)
	if _, exists := file.Sessions[key]; !exists {
		return
	}
	delete(file.Sessions, key)
	if len(file.Sessions) == 0 {
		_ = os.Remove(s.path)
		return
	}
	s.writeLocked(file)
}

// sessionKey 用地址+账号做键：换连接 ID 或改连接名不该导致重新验证码。
func sessionKey(baseURL, username string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(baseURL) + "\x00" + strings.TrimSpace(username)))
	return hex.EncodeToString(sum[:])
}

func (s *SessionStore) readLocked() (sessionFile, bool) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return sessionFile{}, false
	}
	var file sessionFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Sessions == nil {
		// 文件损坏按空处理，不能让旧垃圾影响登录。
		return sessionFile{}, false
	}
	return file, true
}

func (s *SessionStore) writeLocked(file sessionFile) {
	payload, err := json.Marshal(file)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
	}
}
