package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"taskservice/internal/bootstrap"
	"taskservice/internal/config"
	"time"
)

func main() {
	configPath := flag.String("config", "config.yaml", "config path")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdown, err := bootstrap.Build(ctx, cfg, log)
	if err != nil {
		log.Error("build", "error", err)
		os.Exit(1)
	}
	<-ctx.Done()
	sdCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := shutdown(sdCtx); err != nil {
		log.Error("shutdown", "error", err)
	}
}
