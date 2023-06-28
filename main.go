package main

import (
	"flag"
	"log"
	"os"
)

const KEY_LENGTH = 5

func main() {
	log.Println("File Cloud starting up...")
	var (
		bucket string
		secret string
		key    string
		cdn    string

		user      string
		pass      string
		port      string // https://twitter.com/keith_duncan/status/638582305917833217
		plausible string
	)

	flag.StringVar(&bucket, "bucket", LookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&key, "key", LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&cdn, "cdn", LookupEnvDefault("CDN", ""), "CDN URL to use for with object keys. Leave blank to use presigned S3 URLs")

	flag.StringVar(&port, "port", LookupEnvDefault("PORT", "8080"), "Port to listen on")
	flag.StringVar(&user, "username", LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&pass, "password", LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.StringVar(&plausible, "plausible", LookupEnvDefault("PLAUSIBLE", ""), "The domain setup for Plausible. Leave blank to disable")
	flag.Parse()

	client, err := NewAWSClient(bucket, secret, key, cdn)
	if err != nil {
		log.Fatal(err)
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
