package health

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandler(t *testing.T) {
	resp, err := Handler("abc123")(context.Background(), events.APIGatewayV2HTTPRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Fatalf("content-type = %q", resp.Headers["Content-Type"])
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("body bukan JSON valid: %v", err)
	}
	if body["status"] != "ok" || body["version"] != "abc123" || body["service"] != "ticketing-api" {
		t.Fatalf("body = %v", body)
	}
}
