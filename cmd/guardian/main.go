package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/guardian"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/Will-Luck/Docker-Guardian/internal/notify"
)

func main() {
	cfg := config.Load()

	log := logging.New(cfg.LogJSON)

	log.Info("Docker-Guardian (Go rewrite)")
	log.Info("=============================================")
	cfg.Print(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	client, err := docker.NewClient(cfg.DockerSock)
	if err != nil {
		log.Error("failed to create Docker client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	dispatcher := notify.NewDispatcher(cfg, log)
	log.Info("notifications", "services", dispatcher.ConfiguredServices(), "events", cfg.NotifyEvents)

	g := guardian.New(cfg, client, dispatcher, log)

	if cfg.StartPeriod > 0 {
		log.Info("monitoring containers", "delay", fmt.Sprintf("%ds", cfg.StartPeriod))
		select {
		case <-time.After(time.Duration(cfg.StartPeriod) * time.Second):
		case <-ctx.Done():
			return
		}
	}

	dispatcher.Startup(fmt.Sprintf("Docker-Guardian started. Monitoring active. Services: %s",
		dispatcher.ConfiguredServices()))

	if err := g.Run(ctx); err != nil {
		log.Error("guardian exited with error", "error", err)
		os.Exit(1)
	}
}
