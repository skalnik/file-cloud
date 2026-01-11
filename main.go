package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
)

const keyLength = 5

func main() {
	var (
		bucket   string
		secret   string
		key      string
		cdn      string
		logLevel string

		user      string
		pass      string
		port      string // https://twitter.com/keith_duncan/status/638582305917833217
		plausible string
	)

	flag.StringVar(&bucket, "bucket", LookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&key, "key", LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&cdn, "cdn", LookupEnvDefault("CDN", ""), "CDN URL to use for with object keys. Leave blank to use presigned S3 URLs")
	flag.StringVar(&logLevel, "log-level", LookupEnvDefault("LOG_LEVEL", "debug"), "Log level (debug, info, warn, error)")

	flag.StringVar(&port, "port", LookupEnvDefault("PORT", "8080"), "Port to listen on")
	flag.StringVar(&user, "username", LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&pass, "password", LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.StringVar(&plausible, "plausible", LookupEnvDefault("PLAUSIBLE", ""), "The domain setup for Plausible. Leave blank to disable")
	flag.Parse()

	setupLogger(logLevel)
	slog.Info("File Cloud starting up...")

	if err := ValidateConfig(bucket, secret, key, cdn, port, user, pass); err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	client, err := NewAWSClient(bucket, secret, key, cdn)
	if err != nil {
		slog.Error("Failed to create AWS client", "error", err)
		os.Exit(1)
	}

	web := *NewWebServer(user, pass, port, plausible, client)
	web.Start()
}

func LookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}

func setupLogger(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

func ValidateConfig(bucket, secret, key, cdn, port, user, pass string) error {
	if bucket == "" {
		return fmt.Errorf("bucket is required")
	}

	if secret == "" {
		return fmt.Errorf("AWS secret is required")
	}

	if key == "" {
		return fmt.Errorf("AWS key is required")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("port must be a number: %w", err)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if cdn != "" {
		parsedURL, err := url.Parse(cdn)
		if err != nil {
			return fmt.Errorf("CDN must be a valid URL: %w", err)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("CDN URL must use http or https scheme")
		}
	}

	if (user == "") != (pass == "") {
		return fmt.Errorf("both username and password must be provided, or neither")
	}

	return nil
}
