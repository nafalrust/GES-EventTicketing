package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/photos"
)

func main() {
	bucket := os.Getenv("PHOTOS_BUCKET")
	if bucket == "" {
		log.Fatal("PHOTOS_BUCKET belum di-set")
	}
	app, err := photos.New(context.Background(), bucket)
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
	}
	lambda.Start(app.Handle)
}
