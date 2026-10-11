// icebreaker 现场抽奖后端：单进程单二进制 —— HTTP + SQLite + 内嵌前端。
//
// 用法：./icebreaker -config config.yaml
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"icebreaker"
	"icebreaker/internal/api"
	"icebreaker/internal/auth"
	"icebreaker/internal/config"
	"icebreaker/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[icebreaker] ")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	if len(cfg.Auth.AdminSubs) == 0 {
		// fail closed：没白名单就没人能管理，但要让运维一眼看见原因。
		log.Printf("警告：auth.admin_subs 为空，任何人都进不了管理台。填上管理员的 Keycloak sub 再重启。")
	}

	st, err := store.Open(cfg.DB.Path)
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}

	// go:embed all:dist 的根是整个模块目录，静态路径要落在 dist/ 之下。
	ui, err := fs.Sub(icebreaker.Frontend, "dist")
	if err != nil {
		log.Fatalf("启动失败: 取内置前端目录: %v", err)
	}

	a := auth.New(cfg)
	srv := api.New(cfg, a, st, ui, log.Default())

	httpSrv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// 签到与大屏轮询都是短请求，不做长连接。
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("监听 %s（前端已内嵌进二进制，数据库 %s）", cfg.Server.Addr, cfg.DB.Path)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务退出: %v", err)
		}
	}()

	// 后台预热 OIDC 发现文档：Keycloak 不可达时也只是记一行日志，服务照常开。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Warmup(ctx)
	}()

	// SIGTERM / SIGINT：优雅收尾，让 SQLite 把 WAL checkpoint 回主库，
	// 之后只拷 icebreaker.db 就是完整数据。
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("收到退出信号，正在收尾")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("HTTP 关闭超时: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("关闭数据库失败: %v", err)
	}
	log.Printf("已退出")
}
