package ddns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"hetzner-ddns/internal/config"
	"hetzner-ddns/internal/ip"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Service struct {
	client    *hcloud.Client
	ipFetcher *ip.Fetcher
	logger    *slog.Logger
	cfg       config.Config
}

func NewService(client *hcloud.Client, ipFetcher *ip.Fetcher, logger *slog.Logger, cfg config.Config) *Service {
	return &Service{
		client:    client,
		ipFetcher: ipFetcher,
		logger:    logger,
		cfg:       cfg,
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.syncOnce(ctx); err != nil {
		s.logger.Warn("Initial sync failed", "error", err)
	}

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.syncOnce(ctx); err != nil {
				s.logger.Warn("Sync failed", "error", err)
			}
		}
	}
}

func (s *Service) syncOnce(ctx context.Context) error {
	var errs []error
	ipCache := make(map[string]net.IP)
	for _, zoneCfg := range s.cfg.Zones {
		ipAddr, ok := ipCache[zoneCfg.IPProviderURL]
		if !ok {
			var fetched net.IP
			s.logger.Info("Fetching current IP", "zone", zoneCfg.Name, "provider", zoneCfg.IPProviderURL, "record_type", zoneCfg.RecordType)
			err := s.withTimeout(ctx, func(opCtx context.Context) error {
				var fetchErr error
				fetched, fetchErr = s.ipFetcher.Fetch(opCtx, zoneCfg.IPProviderURL)
				return fetchErr
			})
			if err != nil {
				s.logger.Error("IP fetch failed", "zone", zoneCfg.Name, "provider", zoneCfg.IPProviderURL, "error", err)
				errs = append(errs, fmt.Errorf("zone %s ip fetch: %w", zoneCfg.Name, err))
				continue
			}
			s.logger.Info("Fetched current IP", "zone", zoneCfg.Name, "provider", zoneCfg.IPProviderURL, "ip", fetched.String())
			ipCache[zoneCfg.IPProviderURL] = fetched
			ipAddr = fetched
		}

		ipStr, err := s.normalizeIP(zoneCfg.RecordType, ipAddr)
		if err != nil {
			s.logger.Error("IP validation failed", "zone", zoneCfg.Name, "record_type", zoneCfg.RecordType, "error", err)
			errs = append(errs, fmt.Errorf("zone %s ip validation: %w", zoneCfg.Name, err))
			continue
		}
		s.logger.Debug("Normalized IP", "zone", zoneCfg.Name, "record_type", zoneCfg.RecordType, "ip", ipStr)

		s.logger.Info("Looking up zone", "zone", zoneCfg.Name)
		zone, err := s.getZone(ctx, zoneCfg.Name)
		if err != nil {
			s.logger.Error("Zone lookup failed", "zone", zoneCfg.Name, "error", err)
			errs = append(errs, fmt.Errorf("zone %s lookup: %w", zoneCfg.Name, err))
			continue
		}
		s.logger.Debug("Zone resolved", "zone", zoneCfg.Name, "zone_id", zone.ID)

		for _, record := range zoneCfg.Records {
			ttl := record.TTL
			if ttl == nil {
				ttl = zoneCfg.TTL
			}
			s.logger.Info("Checking record", "zone", zoneCfg.Name, "record", record.Name, "record_type", zoneCfg.RecordType, "ip", ipStr, "ttl", ttlValue(ttl))
			if err := s.updateRecord(ctx, zone, zoneCfg.RecordType, record.Name, ipStr, ttl); err != nil {
				s.logger.Error("Record update failed", "zone", zoneCfg.Name, "record", record.Name, "error", err)
				errs = append(errs, fmt.Errorf("zone %s record %s: %w", zoneCfg.Name, record.Name, err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sync completed with %d error(s)", len(errs))
	}
	return nil
}

func (s *Service) normalizeIP(recordType string, ipAddr net.IP) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(recordType)) {
	case "A":
		ipv4 := ipAddr.To4()
		if ipv4 == nil {
			return "", fmt.Errorf("IP provider returned non-IPv4 address for A record")
		}
		return ipv4.String(), nil
	case "AAAA":
		if ipAddr.To4() != nil {
			return "", fmt.Errorf("IP provider returned IPv4 address for AAAA record")
		}
		return ipAddr.String(), nil
	default:
		return "", fmt.Errorf("unsupported record type: %s", recordType)
	}
}

func (s *Service) getZone(ctx context.Context, name string) (*hcloud.Zone, error) {
	var zone *hcloud.Zone
	err := s.withRetry(ctx, "get zone", func(opCtx context.Context) error {
		s.logger.Debug("API request: get zone", "zone", name)
		var getErr error
		zone, _, getErr = s.client.Zone.GetByName(opCtx, name)
		if getErr != nil {
			return getErr
		}
		if zone == nil {
			return fmt.Errorf("zone not found: %s", name)
		}
		return nil
	})
	return zone, err
}

func (s *Service) updateRecord(ctx context.Context, zone *hcloud.Zone, recordType string, name, ip string, ttl *int) error {
	rrType := hcloud.ZoneRRSetType(strings.ToUpper(strings.TrimSpace(recordType)))

	var rrset *hcloud.ZoneRRSet
	err := s.withRetry(ctx, "get rrset", func(opCtx context.Context) error {
		s.logger.Debug("API request: get rrset", "zone", zone.Name, "record", name, "record_type", rrType)
		var getErr error
		rrset, _, getErr = s.client.Zone.GetRRSetByNameAndType(opCtx, zone, name, rrType)
		return getErr
	})
	if err != nil {
		return fmt.Errorf("get rrset %s/%s: %w", name, rrType, err)
	}

	if rrset == nil {
		s.logger.Info("Record missing; will create", "zone", zone.Name, "record", name, "record_type", rrType, "ip", ip, "ttl", ttlValue(ttl))
		err := s.withRetry(ctx, "create rrset", func(opCtx context.Context) error {
			s.logger.Debug("API request: create rrset", "zone", zone.Name, "record", name, "record_type", rrType, "ttl", ttlValue(ttl))
			_, _, createErr := s.client.Zone.CreateRRSet(opCtx, zone, hcloud.ZoneRRSetCreateOpts{
				Name: name,
				Type: rrType,
				TTL:  ttl,
				Records: []hcloud.ZoneRRSetRecord{
					{Value: ip},
				},
			})
			return createErr
		})
		if err != nil {
			return fmt.Errorf("create rrset %s/%s: %w", name, rrType, err)
		}
		s.logger.Info("Record created", "zone", zone.Name, "record", name, "ip", ip)
		return nil
	}

	if rrsetHasValue(rrset, ip) {
		// If we're preserving, any existing match is enough. If not preserving,
		// only short-circuit when the RRSet is already a single matching record.
		if !s.cfg.PreserveRecords && len(rrset.Records) > 1 {
			// fall through to update
		} else {
		s.logger.Info("Record already up to date", "zone", zone.Name, "record", name, "ip", ip)
		if err := s.ensureTTL(ctx, zone.Name, rrset, ttl); err != nil {
			return err
		}
		return nil
		}
	}

	if s.cfg.PreserveRecords && len(rrset.Records) > 1 {
		s.logger.Info("Record will append", "zone", zone.Name, "record", name, "record_type", rrType, "ip", ip, "ttl", ttlValue(ttl), "current_values", rrsetValues(rrset))
		err = s.withRetry(ctx, "add rrset record", func(opCtx context.Context) error {
			s.logger.Debug("API request: add rrset record", "zone", zone.Name, "record", name, "record_type", rrType, "ttl", ttlValue(ttl))
			_, _, addErr := s.client.Zone.AddRRSetRecords(opCtx, rrset, hcloud.ZoneRRSetAddRecordsOpts{
				Records: []hcloud.ZoneRRSetRecord{{Value: ip}},
				TTL:     ttl,
			})
			return addErr
		})
		if err != nil {
			return fmt.Errorf("add rrset record %s/%s: %w", name, rrType, err)
		}
		s.logger.Info("Record appended", "zone", zone.Name, "record", name, "ip", ip)
		if err := s.ensureTTL(ctx, zone.Name, rrset, ttl); err != nil {
			return err
		}
		return nil
	}

	s.logger.Info("Record will update", "zone", zone.Name, "record", name, "record_type", rrType, "ip", ip, "ttl", ttlValue(ttl), "current_values", rrsetValues(rrset))
	err = s.withRetry(ctx, "set rrset records", func(opCtx context.Context) error {
		s.logger.Debug("API request: set rrset records", "zone", zone.Name, "record", name, "record_type", rrType)
		_, _, setErr := s.client.Zone.SetRRSetRecords(opCtx, rrset, hcloud.ZoneRRSetSetRecordsOpts{
			Records: []hcloud.ZoneRRSetRecord{{Value: ip}},
		})
		return setErr
	})
	if err != nil {
		return fmt.Errorf("set rrset records %s/%s: %w", name, rrType, err)
	}
	s.logger.Info("Record updated", "zone", zone.Name, "record", name, "ip", ip, "preserve", s.cfg.PreserveRecords)
	if err := s.ensureTTL(ctx, zone.Name, rrset, ttl); err != nil {
		return err
	}
	return nil
}

func rrsetHasValue(rrset *hcloud.ZoneRRSet, ip string) bool {
	for _, record := range rrset.Records {
		if strings.TrimSpace(record.Value) == ip {
			return true
		}
	}
	return false
}

func rrsetValues(rrset *hcloud.ZoneRRSet) []string {
	values := make([]string, 0, len(rrset.Records))
	for _, record := range rrset.Records {
		value := strings.TrimSpace(record.Value)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
}

func ttlValue(ttl *int) any {
	if ttl == nil {
		return "default"
	}
	return *ttl
}

func (s *Service) ensureTTL(ctx context.Context, zoneName string, rrset *hcloud.ZoneRRSet, ttl *int) error {
	if ttl == nil {
		return nil
	}
	if rrset.TTL != nil && *rrset.TTL == *ttl {
		return nil
	}
	s.logger.Info("Record TTL will change", "zone", zoneName, "record", rrset.Name, "current_ttl", ttlValue(rrset.TTL), "target_ttl", *ttl)
	err := s.withRetry(ctx, "change rrset ttl", func(opCtx context.Context) error {
		s.logger.Debug("API request: change rrset ttl", "zone", zoneName, "record", rrset.Name, "ttl", *ttl)
		_, _, changeErr := s.client.Zone.ChangeRRSetTTL(opCtx, rrset, hcloud.ZoneRRSetChangeTTLOpts{
			TTL: ttl,
		})
		return changeErr
	})
	if err != nil {
		return fmt.Errorf("change rrset ttl: %w", err)
	}
	s.logger.Info("Record TTL updated", "zone", zoneName, "record", rrset.Name, "ttl", *ttl)
	return nil
}

func (s *Service) withTimeout(ctx context.Context, fn func(context.Context) error) error {
	opCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()
	return fn(opCtx)
}

func (s *Service) withRetry(ctx context.Context, label string, fn func(context.Context) error) error {
	return retry(ctx, s.cfg.RetryAttempts, s.cfg.RetryBaseDelay, s.cfg.RetryMaxDelay, func(opCtx context.Context, attempt int) error {
		err := s.withTimeout(opCtx, fn)
		if err != nil {
			s.logger.Warn("Operation failed", "op", label, "attempt", attempt, "error", err)
			return err
		}
		return nil
	})
}
