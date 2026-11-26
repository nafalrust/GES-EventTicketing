package photos

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

type fakePresigner struct {
	err                      error
	bucket, key, contentType string
	ttl                      time.Duration
}

func (f *fakePresigner) PutURL(_ context.Context, bucket, key, ct string, ttl time.Duration) (string, error) {
	f.bucket, f.key, f.contentType, f.ttl = bucket, key, ct, ttl
	if f.err != nil {
		return "", f.err
	}
	return "https://" + bucket + ".s3.amazonaws.com/" + key + "?sig=x", nil
}

func call(p *fakePresigner, method string, q map[string]string) (int, map[string]any) {
	a := &App{Presign: p, Bucket: "b", TTL: 5 * time.Minute, NewID: func() string { return "abc123" }}
	req := events.APIGatewayV2HTTPRequest{QueryStringParameters: q}
	req.RequestContext.HTTP.Method = method
	resp, _ := a.Handle(context.Background(), req)
	var body map[string]any
	_ = json.Unmarshal([]byte(resp.Body), &body)
	return resp.StatusCode, body
}

func TestOK(t *testing.T) {
	p := &fakePresigner{}
	code, body := call(p, "GET", map[string]string{"event_id": "EVT-2026_01", "content_type": "image/jpeg"})
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, body)
	}
	if p.key != "photos/EVT-2026_01/abc123.jpg" || p.bucket != "b" || p.contentType != "image/jpeg" || p.ttl != 5*time.Minute {
		t.Errorf("argumen presign salah: %+v", p)
	}
	if body["view_path"] != "/photos/EVT-2026_01/abc123.jpg" || body["method"] != "PUT" || body["expires_in"].(float64) != 300 {
		t.Errorf("body: %v", body)
	}
	if body["headers"].(map[string]any)["Content-Type"] != "image/jpeg" {
		t.Errorf("headers: %v", body["headers"])
	}
}

func TestExtensionFollowsContentType(t *testing.T) {
	for ct, ext := range map[string]string{"image/png": ".png", "image/webp": ".webp"} {
		p := &fakePresigner{}
		call(p, "GET", map[string]string{"event_id": "e1", "content_type": ct})
		if !strings.HasSuffix(p.key, ext) {
			t.Errorf("%s -> %s", ct, p.key)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	cases := []map[string]string{
		{"content_type": "image/jpeg"},                       // tanpa event_id
		{"event_id": "../etc", "content_type": "image/jpeg"}, // path traversal
		{"event_id": "a/b", "content_type": "image/jpeg"},    // slash
		{"event_id": strings.Repeat("a", 65), "content_type": "image/jpeg"},
		{"event_id": "e1", "content_type": "text/html"}, // bukan gambar
		{"event_id": "e1"}, // tanpa content_type
	}
	for _, q := range cases {
		p := &fakePresigner{}
		if code, _ := call(p, "GET", q); code != 400 {
			t.Errorf("%v -> %d, harusnya 400", q, code)
		}
		if p.key != "" {
			t.Errorf("presign tidak boleh dipanggil untuk input buruk: %v", q)
		}
	}
}

func TestWrongMethod(t *testing.T) {
	if code, _ := call(&fakePresigner{}, "POST", map[string]string{"event_id": "e1", "content_type": "image/png"}); code != 405 {
		t.Fatalf("code=%d", code)
	}
}

func TestPresignError(t *testing.T) {
	code, body := call(&fakePresigner{err: errors.New("boom")}, "GET", map[string]string{"event_id": "e1", "content_type": "image/png"})
	if code != 500 || strings.Contains(body["error"].(string), "boom") {
		t.Fatalf("500 tanpa membocorkan detail error: %d %v", code, body)
	}
}
