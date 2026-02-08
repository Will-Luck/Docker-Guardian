package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Will-Luck/Docker-Guardian/internal/config"
	"github.com/Will-Luck/Docker-Guardian/internal/docker"
	"github.com/Will-Luck/Docker-Guardian/internal/guardian"
	"github.com/Will-Luck/Docker-Guardian/internal/logging"
	"github.com/Will-Luck/Docker-Guardian/internal/notify"
)

func main() {
	// Accept "autoheal" arg for backward compat with shell version's CMD ["autoheal"]
	if len(os.Args) > 1 && os.Args[1] != "autoheal" {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

	cfg := config.Load()
	log := logging.New(cfg.LogJSON)

	// Banner: plain stdout for acceptance test compatibility
	fmt.Println("Docker-Guardian (Go rewrite)")
	fmt.Println("=============================================")
	cfg.PrintBanner()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	client, err := docker.NewClient(cfg.DockerSock)
	if err != nil {
		log.Error("failed to create Docker client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	dispatcher := notify.NewDispatcher(cfg, log)

	// Notification banner: tests grep for "NOTIFICATIONS=.*gotify" and "NOTIFY_EVENTS=..."
	fmt.Println("NOTIFICATIONS=" + dispatcher.ConfiguredServices())
	resolved := cfg.ResolvedNotifyEvents()
	fmt.Printf("NOTIFY_EVENTS=%s (resolved: %s)\n", cfg.NotifyEvents, strings.Join(resolved, ","))

	g := guardian.New(cfg, client, dispatcher, log)

	if cfg.StartPeriod > 0 {
		fmt.Printf("Monitoring containers in %d second(s)\n", cfg.StartPeriod)
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
