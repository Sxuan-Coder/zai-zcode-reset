// zai-zcode-reset 后端入口：装配配置、存储、配额与 HTTP 服务。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zai-zcode-reset/internal/api"
	"zai-zcode-reset/internal/config"
	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/quota"
	"zai-zcode-reset/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("打开存储失败: %v", err)
	}
	box, err := cryptoutil.NewSecretBox(cfg.MasterKey)
	if err != nil {
		log.Fatalf("初始化令牌保险箱失败: %v", err)
	}

	bootstrapAdmin(st, cfg)
	seedRules(st, cfg)

	q := quota.NewManager(st)
	srv := api.NewServer(cfg, st, q, box)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if cfg.MockUpstream {
		log.Printf("⚠️  MOCK_UPSTREAM 已开启：所有上游交互均为模拟数据，仅供开发联调！")
	}
	log.Printf("zai-zcode-reset 已启动: http://%s (数据目录: %s)", cfg.Addr, cfg.DataDir)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("正在关闭……")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	log.Println("已退出")
}

// bootstrapAdmin 首次启动（无任何用户）时创建管理员；
// 密码优先取 ADMIN_PASSWORD，否则生成随机密码并只打印一次。
func bootstrapAdmin(st *store.Store, cfg *config.Config) {
	if len(st.ListUsers()) > 0 {
		return
	}
	password := cfg.AdminPassword
	generated := false
	if password == "" {
		pw, err := cryptoutil.RandomPassword(16)
		if err != nil {
			log.Fatalf("生成管理员密码失败: %v", err)
		}
		password = pw
		generated = true
	}
	hash, err := cryptoutil.HashPassword(password)
	if err != nil {
		log.Fatalf("管理员密码哈希失败: %v", err)
	}
	if _, err := st.CreateUser(model.User{
		Username:     cfg.AdminUsername,
		PasswordHash: hash,
		Role:         model.RoleAdmin,
		Status:       model.StatusActive,
	}); err != nil {
		log.Fatalf("创建管理员失败: %v", err)
	}
	if generated {
		log.Printf("════════════════════════════════════════════════")
		log.Printf(" 已创建管理员账号: %s", cfg.AdminUsername)
		log.Printf(" 初始密码（仅显示此一次）: %s", password)
		log.Printf("════════════════════════════════════════════════")
	} else {
		log.Printf("已按 ADMIN_PASSWORD 创建管理员账号: %s", cfg.AdminUsername)
	}
}

// seedRules 幂等播种全局默认配额规则。
func seedRules(st *store.Store, cfg *config.Config) {
	if err := st.SeedDefaultRule(model.PeriodDay, cfg.DefaultDayLimit); err != nil {
		log.Printf("播种每日默认规则失败: %v", err)
	}
	if err := st.SeedDefaultRule(model.PeriodWeek, cfg.DefaultWeekLimit); err != nil {
		log.Printf("播种每周默认规则失败: %v", err)
	}
}
