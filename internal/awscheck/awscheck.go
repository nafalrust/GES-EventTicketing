// Package awscheck: handler untuk GET /api/health/aws.
//
// Membuktikan dari DALAM Lambda (jadi memakai execution role fungsi ini):
//  1. kredensial/role valid            -> STS GetCallerIdentity
//  2. ketiga tabel DynamoDB terjangkau -> DescribeTable (Events, Bookings, Photos)
//  3. izin baca-tulis ke tabel Events  -> put/get/delete item probe
//  4. bucket foto S3 terjangkau        -> HeadBucket
package awscheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"ticketing-serverless/internal/httpx"
)

const probeID = "__healthcheck__"

// Interface tipis supaya bisa dites tanpa AWS (lihat awscheck_test.go).
type DynamoAPI interface {
	DescribeTable(ctx context.Context, in *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type STSAPI interface {
	GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type S3API interface {
	HeadBucket(ctx context.Context, in *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// Config: nama resource, diisi dari environment variable di cmd/awscheck.
type Config struct {
	EventsTable   string // PK: event_id
	BookingsTable string // PK: booking_id
	PhotosTable   string // PK: event_id, SK: photo_id
	PhotosBucket  string
}

type App struct {
	DDB    DynamoAPI
	STS    STSAPI
	S3     S3API
	Cfg    Config
	Region string
}

type check struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// New dipanggil SEKALI di init phase (di luar handler), sehingga client SDK
// dipakai ulang oleh semua invocation di environment yang sama (warm start).
func New(ctx context.Context, c Config) (*App, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1" // hanya relevan untuk mode lokal; di Lambda AWS_REGION selalu terisi
	}
	return &App{
		DDB:    dynamodb.NewFromConfig(cfg),
		STS:    sts.NewFromConfig(cfg),
		S3:     s3.NewFromConfig(cfg),
		Cfg:    c,
		Region: cfg.Region,
	}, nil
}

func (a *App) Handle(parent context.Context, _ events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()

	checks := map[string]check{}
	checks["sts"] = a.checkSTS(ctx)
	checks["dynamodb_tables"] = a.checkTables(ctx)
	if checks["dynamodb_tables"].OK {
		checks["dynamodb_rw"] = a.checkReadWrite(ctx)
	}
	checks["s3_bucket"] = a.checkBucket(ctx)

	status, label := http.StatusOK, "ok"
	for _, c := range checks {
		if !c.OK {
			status, label = http.StatusServiceUnavailable, "degraded"
			break
		}
	}
	return httpx.JSON(status, map[string]any{
		"status": label,
		"region": a.Region,
		"checks": checks,
	}), nil
}

func (a *App) checkSTS(ctx context.Context) check {
	out, err := a.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return check{Error: shortErr(err)}
	}
	return check{OK: true, Detail: roleFromARN(aws.ToString(out.Arn))}
}

func (a *App) checkTables(ctx context.Context) check {
	named := []struct{ label, name string }{
		{"events", a.Cfg.EventsTable},
		{"bookings", a.Cfg.BookingsTable},
		{"photos", a.Cfg.PhotosTable},
	}
	var details, failures []string
	for _, t := range named {
		out, err := a.DDB.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(t.name)})
		switch {
		case err != nil:
			failures = append(failures, t.label+": "+shortErr(err))
		case out.Table == nil:
			failures = append(failures, t.label+": empty DescribeTable response")
		default:
			details = append(details, fmt.Sprintf("%s=%s", t.label, out.Table.TableStatus))
		}
	}
	if len(failures) > 0 {
		return check{Error: strings.Join(failures, "; ")}
	}
	return check{OK: true, Detail: strings.Join(details, " ")}
}

// Probe baca-tulis ke tabel Events (key: event_id). Item "__healthcheck__" selalu dihapus lagi.
func (a *App) checkReadWrite(ctx context.Context) check {
	table := aws.String(a.Cfg.EventsTable)
	key := map[string]types.AttributeValue{"event_id": &types.AttributeValueMemberS{Value: probeID}}
	item := map[string]types.AttributeValue{
		"event_id":   &types.AttributeValueMemberS{Value: probeID},
		"checked_at": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}

	if _, err := a.DDB.PutItem(ctx, &dynamodb.PutItemInput{TableName: table, Item: item}); err != nil {
		return check{Error: "put: " + shortErr(err)}
	}
	got, err := a.DDB.GetItem(ctx, &dynamodb.GetItemInput{TableName: table, Key: key, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return check{Error: "get: " + shortErr(err)}
	}
	if len(got.Item) == 0 {
		return check{Error: "get: item not found after put"}
	}
	if _, err := a.DDB.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: table, Key: key}); err != nil {
		return check{Error: "delete: " + shortErr(err)}
	}
	return check{OK: true, Detail: "put/get/delete ok (events)"}
}

func (a *App) checkBucket(ctx context.Context) check {
	if _, err := a.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(a.Cfg.PhotosBucket)}); err != nil {
		return check{Error: shortErr(err)}
	}
	return check{OK: true, Detail: "bucket foto terjangkau"}
}

// "arn:aws:sts::123456789012:assumed-role/MyRole/sesi" -> "assumed-role/MyRole"
// (account ID sengaja tidak ikut ditampilkan di response)
func roleFromARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return "unknown"
	}
	segs := strings.Split(parts[5], "/")
	if len(segs) >= 2 {
		return segs[0] + "/" + segs[1]
	}
	return parts[5]
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
