package api

import (
	"net/http"
	"time"

	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
)

type publicUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func toPublicUser(u model.User) publicUser {
	return publicUser{ID: u.ID, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt.Format(time.RFC3339)}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ip := clientIP(r, s.cfg.TrustProxy)
	ua := r.UserAgent()
	guardKey := req.Username + "|" + ip

	if remain, ok := s.guard.allow(guardKey); !ok {
		_ = s.store.AppendLoginLog(model.LoginLog{
			At: time.Now(), Username: req.Username, IP: ip, UserAgent: ua,
			Success: false, Reason: "locked",
		})
		writeErrExtra(w, http.StatusTooManyRequests, "login_locked",
			"失败次数过多，请稍后再试", map[string]interface{}{
				"retry_after_seconds": int(remain.Seconds()) + 1,
			})
		return
	}

	user, ok := s.store.GetUserByUsername(req.Username)
	if !ok || !cryptoutil.VerifyPassword(req.Password, user.PasswordHash) {
		s.guard.fail(guardKey)
		reason := "invalid_credentials"
		if !ok {
			reason = "unknown_user"
		}
		_ = s.store.AppendLoginLog(model.LoginLog{
			At: time.Now(), Username: req.Username, UserID: user.ID, IP: ip, UserAgent: ua,
			Success: false, Reason: reason,
		})
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	if user.Status != model.StatusActive {
		_ = s.store.AppendLoginLog(model.LoginLog{
			At: time.Now(), Username: req.Username, UserID: user.ID, IP: ip, UserAgent: ua,
			Success: false, Reason: "disabled",
		})
		writeErr(w, http.StatusForbidden, "account_disabled", "账号已被停用，请联系管理员")
		return
	}

	s.guard.reset(guardKey)

	// 单点登录：新登录吊销该用户全部旧会话。
	if s.cfg.SingleSession {
		s.store.RevokeUserSessions(user.ID)
	}

	token, err := cryptoutil.RandomToken(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "生成会话失败")
		return
	}
	sess, err := s.store.CreateSession(model.Session{
		UserID:    user.ID,
		TokenHash: cryptoutil.SHA256Hex(token),
		IP:        ip,
		UserAgent: ua,
		ExpiresAt: time.Now().Add(s.cfg.SessionTTL),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "创建会话失败")
		return
	}
	_, _ = s.store.UpdateUser(user.ID, func(u *model.User) error {
		u.LastLoginAt = time.Now()
		return nil
	})
	_ = s.store.AppendLoginLog(model.LoginLog{
		At: time.Now(), Username: user.Username, UserID: user.ID, IP: ip, UserAgent: ua,
		Success: true,
	})

	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": toPublicUser(user),
		"session": map[string]interface{}{
			"id":         sess.ID,
			"ip":         sess.IP,
			"login_at":   sess.CreatedAt.Format(time.RFC3339),
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	_, sess, _ := CurrentUser(r.Context())
	_ = s.store.RevokeSession(sess.ID)
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := CurrentUser(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": toPublicUser(user),
		"session": map[string]interface{}{
			"id":         sess.ID,
			"ip":         sess.IP,
			"user_agent": sess.UserAgent,
			"login_at":   sess.CreatedAt.Format(time.RFC3339),
			"last_seen":  sess.LastSeenAt.Format(time.RFC3339),
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		},
	})
}
