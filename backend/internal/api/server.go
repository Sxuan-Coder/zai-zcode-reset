// Package api 组装 HTTP 路由、认证中间件与前端静态资源托管。
package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zai-zcode-reset/internal/config"
	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/quota"
	"zai-zcode-reset/internal/store"
)

const sessionCookie = "zsr_session"

var errNotFound = errors.New("not found")

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

// CurrentUser 从请求上下文取认证中间件注入的用户与会话。
func CurrentUser(ctx context.Context) (model.User, model.Session, bool) {
	u, uok := ctx.Value(ctxUser).(model.User)
	s, sok := ctx.Value(ctxSession).(model.Session)
	return u, s, uok && sok
}

type Server struct {
	cfg   *config.Config
	store *store.Store
	quota *quota.Manager
	box   *cryptoutil.SecretBox
	mux   *http.ServeMux
	guard *loginGuard
}

func NewServer(cfg *config.Config, st *store.Store, q *quota.Manager, box *cryptoutil.SecretBox) *Server {
	s := &Server{
		cfg:   cfg,
		store: st,
		quota: q,
		box:   box,
		mux:   http.NewServeMux(),
		guard: newLoginGuard(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "time": time.Now().UnixMilli()})
	})

	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	m.HandleFunc("GET /api/auth/me", s.requireAuth(s.handleMe))

	m.HandleFunc("GET /api/reset/status", s.requireAuth(s.handleResetStatus))
	m.HandleFunc("GET /api/reset/history", s.requireAuth(s.handleResetHistory))
	m.HandleFunc("POST /api/reset/execute", s.requireAuth(s.handleResetExecute))

	a := s.requireAdmin
	m.HandleFunc("GET /api/admin/overview", a(s.handleAdminOverview))
	m.HandleFunc("GET /api/admin/users", a(s.handleListUsers))
	m.HandleFunc("POST /api/admin/users", a(s.handleCreateUser))
	m.HandleFunc("PATCH /api/admin/users/{id}", a(s.handlePatchUser))
	m.HandleFunc("DELETE /api/admin/users/{id}", a(s.handleDeleteUser))
	m.HandleFunc("POST /api/admin/users/{id}/revoke-sessions", a(s.handleRevokeUserSessions))
	m.HandleFunc("GET /api/admin/sessions", a(s.handleListSessions))
	m.HandleFunc("DELETE /api/admin/sessions/{id}", a(s.handleRevokeSession))
	m.HandleFunc("GET /api/admin/rules", a(s.handleListRules))
	m.HandleFunc("POST /api/admin/rules", a(s.handleCreateRule))
	m.HandleFunc("PATCH /api/admin/rules/{id}", a(s.handlePatchRule))
	m.HandleFunc("DELETE /api/admin/rules/{id}", a(s.handleDeleteRule))
	m.HandleFunc("GET /api/admin/accounts", a(s.handleListAccounts))
	m.HandleFunc("POST /api/admin/accounts", a(s.handleCreateAccount))
	m.HandleFunc("PATCH /api/admin/accounts/{id}", a(s.handlePatchAccount))
	m.HandleFunc("DELETE /api/admin/accounts/{id}", a(s.handleDeleteAccount))
	m.HandleFunc("POST /api/admin/accounts/{id}/test", a(s.handleTestAccount))
	m.HandleFunc("GET /api/admin/logs/{kind}", a(s.handleListLogs))

	m.HandleFunc("/", s.serveSPA)
}

// ---------- 认证中间件 ----------

type handlerFunc func(http.ResponseWriter, *http.Request)

func (s *Server) requireAuth(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "未登录或会话已过期")
			return
		}
		tokenHash := cryptoutil.SHA256Hex(cookie.Value)
		sess, user, ok := s.store.LookupSession(tokenHash)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "会话已失效，请重新登录")
			return
		}
		if user.Status != model.StatusActive {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "账号已被停用")
			return
		}
		// 滑动过期；节流避免每个轮询请求都落盘。
		if time.Since(sess.LastSeenAt) > 5*time.Minute {
			s.store.TouchSession(sess.ID, s.cfg.SessionTTL)
		}
		ctx := context.WithValue(r.Context(), ctxUser, user)
		ctx = context.WithValue(ctx, ctxSession, sess)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) requireAdmin(next handlerFunc) handlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := CurrentUser(r.Context())
		if !ok || user.Role != model.RoleAdmin {
			writeErr(w, http.StatusForbidden, "forbidden", "需要管理员权限")
			return
		}
		next(w, r)
	})
}

// ---------- 登录防爆破 ----------

type loginAttempt struct {
	fails     int
	lockUntil time.Time
}

type loginGuard struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

func newLoginGuard() *loginGuard {
	return &loginGuard{attempts: map[string]*loginAttempt{}}
}

const (
	loginMaxFails = 5
	loginLockTTL  = 15 * time.Minute
)

func (g *loginGuard) allow(key string) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a := g.attempts[key]
	if a == nil {
		return 0, true
	}
	if remain := time.Until(a.lockUntil); remain > 0 {
		return remain, false
	}
	if len(g.attempts) > 10_000 { // 防内存膨胀
		g.attempts = map[string]*loginAttempt{}
	}
	return 0, true
}

func (g *loginGuard) fail(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a := g.attempts[key]
	if a == nil {
		a = &loginAttempt{}
		g.attempts[key] = a
	}
	a.fails++
	if a.fails >= loginMaxFails {
		a.lockUntil = time.Now().Add(loginLockTTL)
		a.fails = 0
	}
}

func (g *loginGuard) reset(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, key)
}

// ---------- 前端静态资源 ----------

// serveSPA 托管构建产物并做 SPA 回退（无后缀路由回 index.html）。
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	dist := s.cfg.FrontendDist
	if strings.HasPrefix(r.URL.Path, "/api/") || dist == "" {
		writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	clean, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "路径非法")
		return
	}
	rel := filepath.FromSlash(strings.TrimPrefix(filepath.Clean("/"+clean), "/"))
	abs := filepath.Join(dist, rel)
	// 防目录穿越：解析后的路径必须仍在 dist 内。
	if !strings.HasPrefix(abs, filepath.Clean(dist)+string(os.PathSeparator)) && abs != filepath.Clean(dist) {
		writeErr(w, http.StatusBadRequest, "bad_request", "路径非法")
		return
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		if strings.HasSuffix(abs, "index.html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, abs)
		return
	}
	// SPA 回退。
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(dist, "index.html"))
}

func init() {
	// Go 默认 log 输出到 stderr，带时间戳即可。
	log.SetFlags(log.LstdFlags)
}
