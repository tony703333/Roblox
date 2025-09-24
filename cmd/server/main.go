package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"im/internal/auth"
	"im/internal/config"
	"im/internal/server"
	"im/internal/storage"
	"im/internal/ws"
)

func main() {
	cfg, err := config.Load("setting.conf")
	if err != nil {
		log.Fatalf("failed to load setting.conf: %v", err)
	}

	mysqlStore, err := storage.NewMySQL(cfg.MySQL.DSN())
	if err != nil {
		log.Fatalf("failed to connect mysql: %v", err)
	}
	defer mysqlStore.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}
	defer redisClient.Close()

	accountRepo := storage.NewAccountRepository(mysqlStore.DB)
	settingsRepo := storage.NewAgencySettingsRepository(mysqlStore.DB)
	tokenStore := auth.NewRedisTokenStore(redisClient)

	authManager, err := auth.NewManager(accountRepo, tokenStore, cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.Expiry)
	if err != nil {
		log.Fatalf("init auth manager failed: %v", err)
	}

	hub := ws.NewHub()
	srv := server.New(hub, authManager, settingsRepo, "web")
	httpServer := srv.Start(":8080")

	fmt.Println("IM(客服系統) 伺服器已啟動於 http://localhost:8080")
	fmt.Println("管理後台：/admin/ 玩家前台：/client/")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("正在關閉服務...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("關閉服務失敗: %v\n", err)
	}
}
