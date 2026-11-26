package janitor

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandle(t *testing.T) {
	out, err := Handle(context.Background(), events.CloudWatchEvent{Source: "aws.events"})
	if err != nil || out != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
