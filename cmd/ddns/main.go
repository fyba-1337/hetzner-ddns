package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hetzner-ddns/internal/config"
	"hetzner-ddns/internal/ddns"
	"hetzner-ddns/internal/ip"
	"hetzner-ddns/internal/logging"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "CRITICAL: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	client := hcloud.NewClient(hcloud.WithToken(cfg.Token))
	ipFetcher := ip.NewFetcher(cfg.HTTPTimeout, cfg.UserAgent)
	service := ddns.NewService(client, ipFetcher, logger, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	zoneNames := make([]string, 0, len(cfg.Zones))
	for _, zone := range cfg.Zones {
		zoneNames = append(zoneNames, zone.Name)
	}

	logger.Info("DDNS service starting",
		"zones", zoneNames,
		"zone_count", len(cfg.Zones),
		"interval", cfg.Interval.String(),
		"preserve_records", cfg.PreserveRecords,
		"retry_attempts", cfg.RetryAttempts,
		"retry_base_delay", cfg.RetryBaseDelay.String(),
		"retry_max_delay", cfg.RetryMaxDelay.String(),
		"http_timeout", cfg.HTTPTimeout.String(),
		"request_timeout", cfg.RequestTimeout.String(),
		"log_format", cfg.LogFormat,
	)

	if err := service.Run(ctx); err != nil {
		logger.Error("DDNS service stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("DDNS service stopped")
}
