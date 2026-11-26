// Package photos: GET /api/photos/upload-url
//
// Membuat presigned URL (berlaku singkat) supaya browser mengunggah foto LANGSUNG ke S3.
// Lambda tidak pernah memegang isi foto -> tidak kena batas payload Lambda (~6 MB),
// dan tidak kena syarat header SHA-256 untuk POST/PUT lewat CloudFront OAC (permintaan ini GET).
//
// Alur: (1) browser GET /api/photos/upload-url?event_id=..&content_type=image/jpeg
//
//	(2) browser PUT file ke upload_url dengan header Content-Type yang SAMA
//	(3) S3 memicu PhotoIndexerFunction yang mencatat metadata ke tabel Photos
//	(4) foto dilihat lewat CloudFront: /photos/<event_id>/<file>
package photos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"ticketing-serverless/internal/httpx"
)

const KeyPrefix = "photos/"

var (
	eventIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// Hanya format gambar umum; ekstensi ditentukan server, bukan input pengguna.
	extByType = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
)

// Interface tipis supaya bisa dites tanpa AWS.
type Presigner interface {
	PutURL(ctx context.Context, bucket, key, contentType string, ttl time.Duration) (string, error)
}

type App struct {
	Presign Presigner
	Bucket  string
	TTL     time.Duration
	NewID   func() string
}

type s3Presigner struct{ c *s3.PresignClient }

func (p s3Presigner) PutURL(ctx context.Context, bucket, key, contentType string, ttl time.Duration) (string, error) {
	r, err := p.c.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType), // ikut ditandatangani: klien wajib memakai Content-Type yang sama
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return r.URL, nil
}

// New dipanggil sekali di init phase (di luar handler).
func New(ctx context.Context, bucket string) (*App, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &App{
		Presign: s3Presigner{c: s3.NewPresignClient(s3.NewFromConfig(cfg))},
		Bucket:  bucket,
		TTL:     5 * time.Minute,
		NewID:   randomID,
	}, nil
}

func (a *App) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	if m := req.RequestContext.HTTP.Method; m != "" && m != http.MethodGet {
		return httpx.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "gunakan GET"}), nil
	}
	eventID := req.QueryStringParameters["event_id"]
	ctype := req.QueryStringParameters["content_type"]

	if !eventIDRe.MatchString(eventID) {
		return httpx.JSON(http.StatusBadRequest, map[string]string{"error": "event_id wajib: 1-64 karakter huruf/angka/_/-"}), nil
	}
	ext, ok := extByType[ctype]
	if !ok {
		return httpx.JSON(http.StatusBadRequest, map[string]string{"error": "content_type harus image/jpeg, image/png, atau image/webp"}), nil
	}

	key := KeyPrefix + eventID + "/" + a.NewID() + ext
	url, err := a.Presign.PutURL(ctx, a.Bucket, key, ctype, a.TTL)
	if err != nil {
		return httpx.JSON(http.StatusInternalServerError, map[string]string{"error": "gagal membuat upload URL"}), nil
	}
	return httpx.JSON(http.StatusOK, map[string]any{
		"upload_url": url,
		"method":     "PUT",
		"headers":    map[string]string{"Content-Type": ctype},
		"key":        key,
		"view_path":  "/" + key, // dilayani CloudFront
		"expires_in": int(a.TTL.Seconds()),
	}), nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand tidak pernah gagal di Linux modern
	return hex.EncodeToString(b)
}
