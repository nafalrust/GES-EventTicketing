package main

import (
	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/health"
)

// Diisi saat build: -ldflags "-X main.version=<git-sha>"
var version = "dev"

func main() {
	lambda.Start(health.Handler(version))
}
