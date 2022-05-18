package main

import (
	"flag"
	"log"
	"os"
)

var awsClient AWSClient
var web WebServer

const KEY_LENGTH = 5

func main() {
	log.Println("File Cloud starting up...")
	flag.StringVar(&awsClient.Bucket, "bucket", LookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&awsClient.Secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&awsClient.Key, "key", LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&awsClient.CDN, "cdn", LookupEnvDefault("CDN", "https://acab123.cloudfront.net"), "CDN URL to use for with object keys. Leave blank to use presigned S3 URLs")

	flag.StringVar(&web.Port, "port", LookupEnvDefault("PORT", "8080"), "Port to listen on")
	flag.StringVar(&web.User, "username", LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&web.Pass, "password", LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.StringVar(&web.Plausible, "plausible", LookupEnvDefault("PLAUSIBLE", ""), "The domain setup for Plausible. Leave blank to disable")
	flag.Parse()

	awsClient.init()
	web.init()
}

func LookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
