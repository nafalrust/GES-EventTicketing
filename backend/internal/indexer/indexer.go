// Package indexer: dipicu event S3 (ObjectCreated di prefix photos/).
// Mencatat metadata foto ke tabel Photos supaya foto bisa didaftar per event
// tanpa harus meng-list isi bucket.
//
// Kunci objek: photos/<event_id>/<photo_id>.<ext>  ->  Photos: PK event_id, SK photo_id
package indexer

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type PutAPI interface {
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type App struct {
	DDB   PutAPI
	Table string
}

func New(ctx context.Context, table string) (*App, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &App{DDB: dynamodb.NewFromConfig(cfg), Table: table}, nil
}

// Mengembalikan error bila PutItem gagal -> S3 (invokasi async) mencoba ulang otomatis.
// Kunci yang tidak sesuai pola dilewati (bukan error), supaya tidak diulang terus.
func (a *App) Handle(ctx context.Context, ev events.S3Event) error {
	for _, r := range ev.Records {
		key, err := url.QueryUnescape(r.S3.Object.Key) // kunci di event S3 ter-URL-encode
		if err != nil {
			log.Printf("kunci tidak valid, dilewati: %q", r.S3.Object.Key)
			continue
		}
		eventID, photoID, ok := parseKey(key)
		if !ok {
			log.Printf("kunci di luar pola photos/<event>/<file>, dilewati: %q", key)
			continue
		}
		_, err = a.DDB.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(a.Table),
			Item: map[string]types.AttributeValue{
				"event_id":    &types.AttributeValueMemberS{Value: eventID},
				"photo_id":    &types.AttributeValueMemberS{Value: photoID},
				"key":         &types.AttributeValueMemberS{Value: key},
				"size_bytes":  &types.AttributeValueMemberN{Value: strconv.FormatInt(r.S3.Object.Size, 10)},
				"uploaded_at": &types.AttributeValueMemberS{Value: r.EventTime.UTC().Format(time.RFC3339)},
			},
		})
		if err != nil {
			return fmt.Errorf("simpan metadata %s: %w", key, err)
		}
		log.Printf("foto terindeks: event=%s photo=%s size=%d", eventID, photoID, r.S3.Object.Size)
	}
	return nil
}

func parseKey(key string) (eventID, photoID string, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "photos" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}
