// Package config 汇总后端全部可配置项：命令行 flag 优先，其次环境变量，最后内置默认值。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Addr          string // HTTP 监听地址
	DataDir       string // 数据目录（db.json、日志、密钥）
	FrontendDist  string // 前端静态资源目录；为空则只提供 API
	MasterKey     []byte // 令牌静态加密主密钥（32 字节）
	TrustProxy    bool   // 是否信任 X-Forwarded-For（反代后必须开启才能记录真实 IP）
	SingleSession bool   // 单点登录：新登录吊销旧会话
	SessionTTL    time.Duration
	CookieSecure  bool

	AdminUsername string // 首次启动引导管理员
	AdminPassword string

	MockUpstream bool // 开发联调：上游返回模拟数据，绝不用于生产

	DefaultDayLimit  int // 首次启动播种的全局默认规则
	DefaultWeekLimit int
}

func Load() (*Config, error) {
	cfg := &Config{}
	flag.StringVar(&cfg.Addr, "addr", envOr("LISTEN_ADDR", "127.0.0.1:8787"), "HTTP 监听地址")
	flag.StringVar(&cfg.DataDir, "data", envOr("DATA_DIR", "data"), "数据目录")
	flag.StringVar(&cfg.FrontendDist, "frontend", envOr("FRONTEND_DIST", "frontend/dist"), "前端静态资源目录（空字符串禁用）")
	flag.BoolVar(&cfg.TrustProxy, "trust-proxy", envBool("TRUST_PROXY", false), "信任 X-Forwarded-For 头")
	flag.BoolVar(&cfg.SingleSession, "single-session", envBool("SINGLE_SESSION", true), "单点登录：新登录吊销旧会话")
	flag.BoolVar(&cfg.CookieSecure, "cookie-secure", envBool("COOKIE_SECURE", false), "Cookie 附加 Secure 属性（HTTPS 部署时开启）")
	flag.BoolVar(&cfg.MockUpstream, "mock-upstream", envBool("MOCK_UPSTREAM", false), "使用模拟上游（仅开发/测试）")
	flag.IntVar(&cfg.DefaultDayLimit, "default-day-limit", envInt("DEFAULT_DAY_LIMIT", 1), "首次启动播种的全局每日重置上限")
	flag.IntVar(&cfg.DefaultWeekLimit, "default-week-limit", envInt("DEFAULT_WEEK_LIMIT", 2), "首次启动播种的全局每周重置上限")
	flag.Parse()

	// 会话有效期单位为小时。
	ttlHours := envInt("SESSION_TTL_HOURS", 24*7)
	if ttlHours <= 0 {
		ttlHours = 168
	}
	cfg.SessionTTL = time.Duration(ttlHours) * time.Hour

	cfg.AdminUsername = envOr("ADMIN_USERNAME", "admin")
	cfg.AdminPassword = os.Getenv("ADMIN_PASSWORD")

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	key, err := loadOrCreateMasterKey(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cfg.MasterKey = key
	return cfg, nil
}

// loadOrCreateMasterKey 优先取 MASTER_KEY 环境变量（64 位 hex）；
// 缺省时生成并写入 data/secret.key，文件权限仅当前用户可读。
func loadOrCreateMasterKey(dataDir string) ([]byte, error) {
	if hexKey := strings.TrimSpace(os.Getenv("MASTER_KEY")); hexKey != "" {
		key, err := hex.DecodeString(hexKey)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("MASTER_KEY 必须是 64 个 hex 字符（32 字节）")
		}
		return key, nil
	}

	keyFile := filepath.Join(dataDir, "secret.key")
	if raw, err := os.ReadFile(keyFile); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("secret.key 内容损坏，请删除后重启（注意：已加密令牌将无法解密）")
		}
		return key, nil
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成主密钥失败: %w", err)
	}
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("写入主密钥失败: %w", err)
	}
	return key, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
		return fallback
	}
	return n
}
