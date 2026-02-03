package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Token          string
	Zones          []ZoneConfig
	Interval       time.Duration
	HTTPTimeout    time.Duration
	RequestTimeout time.Duration
	RetryAttempts  int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	PreserveRecords bool
	UserAgent      string
	LogLevel       slog.Level
	LogFormat      string
}

type ZoneConfig struct {
	Name          string
	Records       []RecordConfig
	RecordType    string
	IPProviderURL string
	TTL           *int
}

type RecordConfig struct {
	Name string
	TTL  *int
}

func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv("HETZNER_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("HETZNER_TOKEN is required")
	}

	interval, err := parseInterval()
	if err != nil {
		return Config{}, err
	}
	if interval <= 0 {
		return Config{}, fmt.Errorf("INTERVAL must be greater than zero")
	}

	httpTimeout, err := parseDuration("HTTP_TIMEOUT", "10s")
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := parseDuration("REQUEST_TIMEOUT", "20s")
	if err != nil {
		return Config{}, err
	}

	defaultIPProvider := strings.TrimSpace(getEnv("IP_PROVIDER", "https://api.ipify.org"))
	defaultRecordType, err := parseRecordType(getEnv("RECORD_TYPE", "A"))
	if err != nil {
		return Config{}, err
	}

	defaultTTL, err := parseTTL("TTL")
	if err != nil {
		return Config{}, err
	}

	retryAttempts, err := parseInt("RETRY_ATTEMPTS", 3, 1, 10)
	if err != nil {
		return Config{}, err
	}
	retryBaseDelay, err := parseDuration("RETRY_BASE_DELAY", "500ms")
	if err != nil {
		return Config{}, err
	}
	retryMaxDelay, err := parseDuration("RETRY_MAX_DELAY", "5s")
	if err != nil {
		return Config{}, err
	}
	if retryMaxDelay < retryBaseDelay {
		return Config{}, fmt.Errorf("RETRY_MAX_DELAY must be >= RETRY_BASE_DELAY")
	}

	preserveRecords, err := parseBool("PRESERVE_EXISTING_RECORDS", "true")
	if err != nil {
		return Config{}, err
	}

	userAgent := strings.TrimSpace(getEnv("USER_AGENT", "hetzner-ddns/1.0"))

	logLevel, err := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	logFormat := strings.ToLower(strings.TrimSpace(getEnv("LOG_FORMAT", "text")))
	if logFormat != "text" && logFormat != "json" {
		return Config{}, fmt.Errorf("LOG_FORMAT must be text or json")
	}

	zones, err := parseZones(defaultRecordType, defaultIPProvider, defaultTTL)
	if err != nil {
		return Config{}, err
	}
	if len(zones) == 0 {
		return Config{}, fmt.Errorf("no zones configured; use ZONE_NAME or ZONE_<N>_NAME")
	}

	return Config{
		Token:           token,
		Zones:           zones,
		Interval:        interval,
		HTTPTimeout:     httpTimeout,
		RequestTimeout:  requestTimeout,
		RetryAttempts:   retryAttempts,
		RetryBaseDelay:  retryBaseDelay,
		RetryMaxDelay:   retryMaxDelay,
		PreserveRecords: preserveRecords,
		UserAgent:       userAgent,
		LogLevel:        logLevel,
		LogFormat:       logFormat,
	}, nil
}

func parseInterval() (time.Duration, error) {
	intervalStr := strings.TrimSpace(os.Getenv("INTERVAL"))
	if intervalStr != "" {
		return time.ParseDuration(intervalStr)
	}

	intervalSeconds := strings.TrimSpace(os.Getenv("INTERVAL_SECONDS"))
	if intervalSeconds == "" {
		return time.ParseDuration("5m")
	}
	seconds, err := strconv.Atoi(intervalSeconds)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("INTERVAL_SECONDS must be a positive integer")
	}
	return time.Duration(seconds) * time.Second, nil
}

func parseDuration(envKey, fallback string) (time.Duration, error) {
	value := strings.TrimSpace(getEnv(envKey, fallback))
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", envKey, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", envKey)
	}
	return d, nil
}

func parseInt(envKey string, fallback, min, max int) (int, error) {
	raw := strings.TrimSpace(getEnv(envKey, strconv.Itoa(fallback)))
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", envKey)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", envKey, min, max)
	}
	return value, nil
}

func parseBool(envKey, fallback string) (bool, error) {
	raw := strings.TrimSpace(getEnv(envKey, fallback))
	switch strings.ToLower(raw) {
	case "true", "1", "yes", "y", "on":
		return true, nil
	case "false", "0", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", envKey)
	}
}

func parseRecordType(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "A":
		return "A", nil
	case "AAAA":
		return "AAAA", nil
	default:
		return "", fmt.Errorf("RECORD_TYPE must be A or AAAA")
	}
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
}

func parseRecords(value, fallback string) ([]RecordConfig, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		raw = fallback
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]RecordConfig, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		var ttl *int
		if strings.Contains(part, ":") {
			split := strings.SplitN(part, ":", 2)
			name = strings.TrimSpace(split[0])
			ttlStr := strings.TrimSpace(split[1])
			if ttlStr == "" {
				return nil, fmt.Errorf("record %q has empty ttl", part)
			}
			val, err := strconv.Atoi(ttlStr)
			if err != nil || val <= 0 {
				return nil, fmt.Errorf("record %q has invalid ttl", part)
			}
			ttl = &val
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, RecordConfig{
			Name: name,
			TTL:  ttl,
		})
	}
	return out, nil
}

func parseZones(defaultRecordType string, defaultIPProvider string, defaultTTL *int) ([]ZoneConfig, error) {
	indexes := zoneIndexesFromEnv()
	if len(indexes) == 0 {
		zoneName := strings.TrimSpace(os.Getenv("ZONE_NAME"))
		if zoneName == "" {
			return nil, nil
		}
		records, err := parseRecords(os.Getenv("RECORDS"), "@")
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("RECORDS resolved to empty list")
		}
		return []ZoneConfig{
			{
				Name:          zoneName,
				Records:       records,
				RecordType:    defaultRecordType,
				IPProviderURL: defaultIPProvider,
				TTL:           defaultTTL,
			},
		}, nil
	}

	if strings.TrimSpace(os.Getenv("ZONE_NAME")) != "" {
		return nil, fmt.Errorf("cannot mix ZONE_NAME with ZONE_<N>_NAME")
	}

	zones := make([]ZoneConfig, 0, len(indexes))
	for _, index := range indexes {
		prefix := fmt.Sprintf("ZONE_%d_", index)
		zoneName := strings.TrimSpace(os.Getenv(prefix + "NAME"))
		if zoneName == "" {
			return nil, fmt.Errorf("%sNAME is required", prefix)
		}
		records, err := parseRecords(os.Getenv(prefix+"RECORDS"), "@")
		if err != nil {
			return nil, fmt.Errorf("%sRECORDS invalid: %w", prefix, err)
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("%sRECORDS resolved to empty list", prefix)
		}
		recordTypeValue := getEnv(prefix+"RECORD_TYPE", "")
		recordType := defaultRecordType
		if strings.TrimSpace(recordTypeValue) != "" {
			parsed, err := parseRecordType(recordTypeValue)
			if err != nil {
				return nil, fmt.Errorf("%sRECORD_TYPE invalid: %w", prefix, err)
			}
			recordType = parsed
		}
		ttl, err := parseTTL(prefix + "TTL")
		if err != nil {
			return nil, err
		}
		if ttl == nil {
			ttl = defaultTTL
		}
		ipProvider := strings.TrimSpace(getEnv(prefix+"IP_PROVIDER", ""))
		if ipProvider == "" {
			ipProvider = defaultIPProvider
		}
		zones = append(zones, ZoneConfig{
			Name:          zoneName,
			Records:       records,
			RecordType:    recordType,
			IPProviderURL: ipProvider,
			TTL:           ttl,
		})
	}

	return zones, nil
}

func zoneIndexesFromEnv() []int {
	var indexes []int
	seen := make(map[int]struct{})
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 0 {
			continue
		}
		key := parts[0]
		if !strings.HasPrefix(key, "ZONE_") || !strings.HasSuffix(key, "_NAME") {
			continue
		}
		indexStr := strings.TrimSuffix(strings.TrimPrefix(key, "ZONE_"), "_NAME")
		index, err := strconv.Atoi(indexStr)
		if err != nil || index <= 0 {
			continue
		}
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	if len(indexes) <= 1 {
		return indexes
	}
	for i := 0; i < len(indexes)-1; i++ {
		for j := i + 1; j < len(indexes); j++ {
			if indexes[j] < indexes[i] {
				indexes[i], indexes[j] = indexes[j], indexes[i]
			}
		}
	}
	return indexes
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseTTL(envKey string) (*int, error) {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be an integer (seconds)", envKey)
	}
	if value <= 0 {
		return nil, fmt.Errorf("%s must be greater than zero", envKey)
	}
	return &value, nil
}
