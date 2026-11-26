package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/indexer"
)

func main() {
	table := os.Getenv("PHOTOS_TABLE")
	if table == "" {
		log.Fatal("PHOTOS_TABLE belum di-set")
	}
	app, err := indexer.New(context.Background(), table)
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
	}
	lambda.Start(app.Handle)
}
