package main

import (
	"github.com/aws/aws-lambda-go/lambda"

	"ticketing-serverless/internal/janitor"
)

func main() { lambda.Start(janitor.Handle) }
