package main

import (
	"context"
	"os/signal"
	"syscall"

	"mangoduck/internal/bot"
	"mangoduck/internal/config"
	"mangoduck/internal/db"
	"mangoduck/internal/logging"

	"go.uber.org/zap"
)

func main() {
	logger, err := logging.New()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}
	logger.Debug("config loaded")

	conn, err := db.Open(cfg.DatabasePath)
	if err != nil {
		logger.Fatal("failed to open database", zap.Error(err))
	}
	defer func() {
		_ = conn.Close()
	}()
	logger.Debug("database connection opened")

	b, err := bot.New(cfg, conn, logger)
	if err != nil {
		logger.Fatal("failed to initialize bot runtime", zap.Error(err))
	}
	logger.Debug("bot runtime initialized")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Debug("bot process started")
	bot.Run(ctx, b)
	logger.Debug("bot process stopped")
}
