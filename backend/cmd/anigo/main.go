package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/greenhats/anigo/internal/cloud"
	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/httpapi"
	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/service"
	"github.com/greenhats/anigo/internal/store"
	"github.com/greenhats/anigo/internal/task"
)

func main() {
	dir, err := resolveDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析配置目录失败:", err)
		os.Exit(1)
	}

	// 1. 底层适配器
	st := store.NewJSONStore(dir)
	cache := store.NewTTLCache()
	logger := log.New(256)
	logService := service.NewLogService(logger)

	// 2. 业务服务（构造器注入）
	cfgService, err := service.NewConfigService(st, cache)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}
	rssService := service.NewRssService(cfgService, logger)
	metaService := service.NewMetadataService(cfgService, cache)
	notifyService := service.NewNotifyService(cfgService, func(msg string) {
		logger.Info("notification", msg)
	})
	aniService := service.NewAniService(cfgService, rssService, metaService)
	cloudReg := cloud.NewRegistry()
	downloadService := service.NewDownloadService(cfgService, rssService, cloudReg, cache, metaService, notifyService, logger)
	// 订阅添加成功后立即异步触发一轮下载
	aniService.SetOnAdded(func(ani *domain.Ani) { downloadService.DownloadAni(ani) })
	statusService := service.NewStatusService(cfgService, rssService, downloadService, cache)

	// 3. 后台任务
	taskMgr := task.NewTaskManager(cfgService, downloadService, logger)
	taskMgr.Start()

	// 4. HTTP 层
	srv := httpapi.NewServer(cfgService, aniService, rssService, downloadService, metaService, notifyService, logService, statusService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "7789"
	}
	httpSrv := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: srv.Handler(),
	}

	go func() {
		logger.Info("main", fmt.Sprintf("ANI-RSS 服务已启动, 监听端口 %s, 配置目录 %s", port, dir))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("main", "HTTP 服务启动失败: "+err.Error())
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("正在关闭服务...")
	taskMgr.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

// resolveDir 镜像遗留配置目录解析逻辑：
//  1. 环境变量 CONFIG
//  2. ./config（若存在）
//  3. 否则 ./config（按需创建）
func resolveDir() (string, error) {
	if d := os.Getenv("CONFIG"); d != "" {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "config"), nil
}