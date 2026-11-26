package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/notifier"
)

func main() {
	topic := os.Getenv("TOPIC_ARN")
	if topic == "" {
		log.Fatal("TOPIC_ARN belum di-set")
	}
	app, err := notifier.New(context.Background(), topic)
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
	}
	lambda.Start(app.Handle)
}
