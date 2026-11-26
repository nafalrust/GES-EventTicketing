// Package httpx berisi helper kecil untuk response API Gateway HTTP API (payload v2).
package httpx

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
)

// JSON membangun response proxy yang valid. Kesalahan bentuk response
// (mis. body bukan string) adalah penyebab #1 error 502 dari API Gateway.
func JSON(status int, body any) events.APIGatewayV2HTTPResponse {
	b, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"detail":"internal error"}`,
		}
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}
}
