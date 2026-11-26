package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/awscheck"
)

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s belum di-set", k)
	}
	return v
}

func main() {
	app, err := awscheck.New(context.Background(), awscheck.Config{ // init phase: sekali per environment
		EventsTable:   mustEnv("EVENTS_TABLE"),
		BookingsTable: mustEnv("BOOKINGS_TABLE"),
		PhotosTable:   mustEnv("PHOTOS_TABLE"),
		PhotosBucket:  mustEnv("PHOTOS_BUCKET"),
	})
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
	}
	lambda.Start(app.Handle)
}
