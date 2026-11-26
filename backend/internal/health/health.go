// Package health: handler untuk GET /health.
// Sengaja TIDAK memakai AWS SDK sama sekali -> fungsi ini tidak butuh izin IAM apa pun
// selain menulis log, dan cold start-nya sekecil mungkin.
package health

import (
	"context"
	"net/http"

	"github.com/aws/aws-lambda-go/events"

	"ticketing-serverless/internal/httpx"
)

type Func = func(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

func Handler(version string) Func {
	return func(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		return httpx.JSON(http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "ticketing-api",
			"version": version,
		}), nil
	}
}
