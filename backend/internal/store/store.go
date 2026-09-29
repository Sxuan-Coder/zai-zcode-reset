// Package store 提供基于 JSON 文件 + JSONL 追加日志的持久化实现。
// 数据规模小（数十用户），RWMutex + 全量原子写足够；接口化设计便于将来替换 SQLite。
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
)

type dbFile struct {
	Version  int                       `json:"version"`
	Users    []model.User              `json:"users"`
	Sessions []model.Session           `json:"sessions"`
	Rules    []model.Rule              `json:"rules"`
	Accounts []model.UpstreamAccount   `json:"accounts"`
	Counters map[string]int            `json:"counters"`
	Seed     map[string]map[string]int `json:"seed"` // 首次播种标记
}

type Store struct {
	mu   sync.RWMutex
	path string

	logMu     sync.Mutex
	loginFile *os.File
	resetFile *os.File
	auditFile *os.File
	loginPath string
	resetPath string
	auditPath string

	db *dbFile
}

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

func Open(dataDir string) (*Store, error) {
	s := &Store{
		path: filepath.Join(dataDir, "db.json"),
		db: &dbFile{
			Version:  1,
			Counters: map[string]int{},
			Seed:     map[string]map[string]int{},
		},
	}
	if raw, err := os.ReadFile(s.path); err == nil {
		if err := json.Unmarshal(raw, s.db); err != nil {
			return nil, err
		}
		if s.db.Counters == nil {
			s.db.Counters = map[string]int{}
		}
		if s.db.Seed == nil {
			s.db.Seed = map[string]map[string]int{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	for name, pair := range map[string][2]interface{}{
		"login.jsonl": {&s.loginFile, &s.loginPath},
		"reset.jsonl": {&s.resetFile, &s.resetPath},
		"audit.jsonl": {&s.auditFile, &s.auditPath},
	} {
		full := filepath.Join(dataDir, name)
		f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		*(pair[0].(**os.File)) = f
		*(pair[1].(*string)) = full
	}
	return s, nil
}

// saveLocked 原子落盘：写临时文件后 rename 覆盖，进程崩溃时不产生半份 db.json。
func (s *Store) saveLocked() error {
	raw, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	id, _ := cryptoutil.RandomToken(8)
	return id
}

// ---------- 用户 ----------

func (s *Store) CreateUser(u model.User) (model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, exist := range s.db.Users {
		if exist.Username == u.Username {
			return model.User{}, ErrConflict
		}
	}
	u.ID = newID()
	u.CreatedAt = time.Now()
	s.db.Users = append(s.db.Users, u)
	return u, s.saveLocked()
}

func (s *Store) UpdateUser(id string, mutate func(*model.User) error) (model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Users {
		if s.db.Users[i].ID == id {
			before := s.db.Users[i]
			if err := mutate(&s.db.Users[i]); err != nil {
				s.db.Users[i] = before
				return model.User{}, err
			}
			out := s.db.Users[i]
			return out, s.saveLocked()
		}
	}
	return model.User{}, ErrNotFound
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i := range s.db.Users {
		if s.db.Users[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	s.db.Users = append(s.db.Users[:idx], s.db.Users[idx+1:]...)
	// 同步清理该用户的会话与规则，避免悬空引用。
	s.db.Sessions = filterSessions(s.db.Sessions, func(x model.Session) bool { return x.UserID != id })
	s.db.Rules = filterRules(s.db.Rules, func(x model.Rule) bool { return x.UserID != id })
	return s.saveLocked()
}

func (s *Store) GetUser(id string) (model.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.db.Users {
		if u.ID == id {
			return u, true
		}
	}
	return model.User{}, false
}

func (s *Store) GetUserByUsername(username string) (model.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.db.Users {
		if u.Username == username {
			return u, true
		}
	}
	return model.User{}, false
}

func (s *Store) ListUsers() []model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.User, len(s.db.Users))
	copy(out, s.db.Users)
	return out
}

// ---------- 会话 ----------

func (s *Store) CreateSession(sess model.Session) (model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.ID = newID()
	sess.CreatedAt = time.Now()
	sess.LastSeenAt = sess.CreatedAt
	s.db.Sessions = append(s.db.Sessions, sess)
	return sess, s.saveLocked()
}

func (s *Store) TouchSession(id string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Sessions {
		if s.db.Sessions[i].ID == id {
			s.db.Sessions[i].LastSeenAt = time.Now()
			s.db.Sessions[i].ExpiresAt = time.Now().Add(ttl)
			_ = s.saveLocked()
			return
		}
	}
}

func (s *Store) RevokeSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := range s.db.Sessions {
		if s.db.Sessions[i].ID == id && s.db.Sessions[i].RevokedAt == nil {
			s.db.Sessions[i].RevokedAt = &now
		}
	}
	return s.saveLocked()
}

func (s *Store) RevokeUserSessions(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	n := 0
	for i := range s.db.Sessions {
		if s.db.Sessions[i].UserID == userID && s.db.Sessions[i].RevokedAt == nil {
			s.db.Sessions[i].RevokedAt = &now
			n++
		}
	}
	_ = s.saveLocked()
	return n
}

// LookupSession 按 token 哈希取会话并校验有效性（含用户启用状态由调用方判断）。
func (s *Store) LookupSession(tokenHash string) (model.Session, model.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for _, sess := range s.db.Sessions {
		if sess.TokenHash != tokenHash {
			continue
		}
		if sess.RevokedAt != nil || !now.Before(sess.ExpiresAt) {
			return model.Session{}, model.User{}, false
		}
		for _, u := range s.db.Users {
			if u.ID == sess.UserID {
				return sess, u, true
			}
		}
		return model.Session{}, model.User{}, false
	}
	return model.Session{}, model.User{}, false
}

func (s *Store) ListActiveSessions() []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var out []model.Session
	for _, sess := range s.db.Sessions {
		if sess.RevokedAt == nil && now.Before(sess.ExpiresAt) {
			out = append(out, sess)
		}
	}
	return out
}

// ---------- 规则 ----------

func (s *Store) ListRules() []model.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Rule, len(s.db.Rules))
	copy(out, s.db.Rules)
	return out
}

func (s *Store) CreateRule(r model.Rule) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = newID()
	r.CreatedAt = time.Now()
	s.db.Rules = append(s.db.Rules, r)
	return r, s.saveLocked()
}

func (s *Store) UpdateRule(id string, mutate func(*model.Rule) error) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Rules {
		if s.db.Rules[i].ID == id {
			before := s.db.Rules[i]
			if err := mutate(&s.db.Rules[i]); err != nil {
				s.db.Rules[i] = before
				return model.Rule{}, err
			}
			return s.db.Rules[i], s.saveLocked()
		}
	}
	return model.Rule{}, ErrNotFound
}

func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Rules {
		if s.db.Rules[i].ID == id {
			s.db.Rules = append(s.db.Rules[:i], s.db.Rules[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// SeedDefaultRule 幂等播种全局默认规则（仅首次启动写入一次）。
func (s *Store) SeedDefaultRule(period string, maxCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db.Seed["rules"] == nil {
		s.db.Seed["rules"] = map[string]int{}
	}
	if _, ok := s.db.Seed["rules"][period]; ok {
		return nil
	}
	s.db.Seed["rules"][period] = 1
	s.db.Rules = append(s.db.Rules, model.Rule{
		ID:        newID(),
		ResetType: model.ResetAll,
		Period:    period,
		MaxCount:  maxCount,
		Enabled:   true,
		CreatedAt: time.Now(),
	})
	return s.saveLocked()
}

// ---------- 上游账号 ----------

func (s *Store) ListAccounts() []model.UpstreamAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.UpstreamAccount, len(s.db.Accounts))
	copy(out, s.db.Accounts)
	return out
}

func (s *Store) GetAccount(id string) (model.UpstreamAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.db.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return model.UpstreamAccount{}, false
}

// ResolveAccount 计算某用户实际使用的上游账号：用户绑定优先，否则取默认账号。
func (s *Store) ResolveAccount(userID string) (model.UpstreamAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var user model.User
	found := false
	for _, u := range s.db.Users {
		if u.ID == userID {
			user = u
			found = true
			break
		}
	}
	if found && user.UpstreamAccountID != "" {
		for _, a := range s.db.Accounts {
			if a.ID == user.UpstreamAccountID && a.Enabled {
				return a, true
			}
		}
	}
	for _, a := range s.db.Accounts {
		if a.IsDefault && a.Enabled {
			return a, true
		}
	}
	// 没有显式默认时退而取第一个启用的账号。
	for _, a := range s.db.Accounts {
		if a.Enabled {
			return a, true
		}
	}
	return model.UpstreamAccount{}, false
}

func (s *Store) CreateAccount(a model.UpstreamAccount) (model.UpstreamAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.IsDefault {
		for i := range s.db.Accounts {
			s.db.Accounts[i].IsDefault = false
		}
	}
	a.ID = newID()
	now := time.Now()
	a.CreatedAt, a.UpdatedAt = now, now
	s.db.Accounts = append(s.db.Accounts, a)
	return a, s.saveLocked()
}

func (s *Store) UpdateAccount(id string, mutate func(*model.UpstreamAccount) error) (model.UpstreamAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Accounts {
		if s.db.Accounts[i].ID == id {
			before := s.db.Accounts[i]
			if err := mutate(&s.db.Accounts[i]); err != nil {
				s.db.Accounts[i] = before
				return model.UpstreamAccount{}, err
			}
			s.db.Accounts[i].UpdatedAt = time.Now()
			if s.db.Accounts[i].IsDefault {
				for j := range s.db.Accounts {
					if s.db.Accounts[j].ID != id {
						s.db.Accounts[j].IsDefault = false
					}
				}
			}
			return s.db.Accounts[i], s.saveLocked()
		}
	}
	return model.UpstreamAccount{}, ErrNotFound
}

func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Accounts {
		if s.db.Accounts[i].ID == id {
			s.db.Accounts = append(s.db.Accounts[:i], s.db.Accounts[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// ---------- 重置计数 ----------

func (s *Store) GetCounter(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db.Counters[key]
}

func (s *Store) AdjustCounter(key string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Counters[key] += delta
	if s.db.Counters[key] <= 0 {
		delete(s.db.Counters, key)
	}
	return s.saveLocked()
}

// ---------- 日志（JSONL 追加） ----------

func appendJSONL(f *os.File, v interface{}) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = f.Write(raw)
	return err
}

func (s *Store) AppendLoginLog(l model.LoginLog) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return appendJSONL(s.loginFile, l)
}

func (s *Store) AppendResetLog(l model.ResetLog) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return appendJSONL(s.resetFile, l)
}

func (s *Store) AppendAuditLog(l model.AuditLog) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return appendJSONL(s.auditFile, l)
}

// readJSONLTail 从文件末尾往前取最后 limit 行。追加句柄是只写的，
// 这里按路径重新以只读方式打开，读完即关。
func readJSONLTail(path string, limit int, out interface{ push(line []byte) error }) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	type entry = []byte
	var lines []entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, append(entry(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for _, line := range lines {
		if err := out.push(line); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReadLoginLogs(limit int) ([]model.LoginLog, error) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	var items []model.LoginLog
	err := readJSONLTail(s.loginPath, limit, &loginSink{&items})
	return items, err
}

type loginSink struct{ items *[]model.LoginLog }

func (p *loginSink) push(line []byte) error {
	var l model.LoginLog
	if err := json.Unmarshal(line, &l); err != nil {
		return nil // 跳过损坏行
	}
	*p.items = append(*p.items, l)
	return nil
}

func (s *Store) ReadResetLogs(limit int) ([]model.ResetLog, error) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	var items []model.ResetLog
	err := readJSONLTail(s.resetPath, limit, &resetSink{&items})
	return items, err
}

type resetSink struct{ items *[]model.ResetLog }

func (p *resetSink) push(line []byte) error {
	var l model.ResetLog
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}
	*p.items = append(*p.items, l)
	return nil
}

func (s *Store) ReadAuditLogs(limit int) ([]model.AuditLog, error) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	var items []model.AuditLog
	err := readJSONLTail(s.auditPath, limit, &auditSink{&items})
	return items, err
}

type auditSink struct{ items *[]model.AuditLog }

func (p *auditSink) push(line []byte) error {
	var l model.AuditLog
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}
	*p.items = append(*p.items, l)
	return nil
}

// ---------- 工具 ----------

func filterSessions(in []model.Session, keep func(model.Session) bool) []model.Session {
	out := in[:0]
	for _, x := range in {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func filterRules(in []model.Rule, keep func(model.Rule) bool) []model.Rule {
	out := in[:0]
	for _, x := range in {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}
