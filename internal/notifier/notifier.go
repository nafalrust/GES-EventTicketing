// Package notifier: dipicu DynamoDB Streams (tabel Bookings) -> publish ke SNS.
// Ini bagian "event-driven" arsitektur: booking dibuat/diubah (mis. check-in)/dihapus ->
// notifikasi, tanpa kode API perlu tahu soal notifikasi sama sekali.
package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// Item dengan booking_id berawalan "__" (mis. probe) tidak memicu notifikasi.
const internalPrefix = "__"

// Interface tipis supaya bisa dites tanpa AWS.
type PublisherAPI interface {
	Publish(ctx context.Context, in *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type App struct {
	SNS      PublisherAPI
	TopicARN string
}

// New dipanggil sekali di init phase (di luar handler).
func New(ctx context.Context, topicARN string) (*App, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &App{SNS: sns.NewFromConfig(cfg), TopicARN: topicARN}, nil
}

// Handle memakai "partial batch response": kalau satu record gagal dipublish,
// record itu dan semua setelahnya dicoba ulang oleh Lambda, bukan seluruh batch.
func (a *App) Handle(ctx context.Context, ev events.DynamoDBEvent) (events.DynamoDBEventResponse, error) {
	var resp events.DynamoDBEventResponse
	for i, r := range ev.Records {
		subject, body, ok := describe(r)
		if !ok {
			continue
		}
		_, err := a.SNS.Publish(ctx, &sns.PublishInput{
			TopicArn: aws.String(a.TopicARN),
			Subject:  aws.String(subject),
			Message:  aws.String(body),
		})
		if err != nil {
			log.Printf("publish gagal untuk %q: %v", subject, err)
			for _, rest := range ev.Records[i:] {
				resp.BatchItemFailures = append(resp.BatchItemFailures,
					events.DynamoDBBatchItemFailure{ItemIdentifier: rest.Change.SequenceNumber})
			}
			return resp, nil
		}
		log.Printf("notifikasi terkirim: %s", subject)
	}
	return resp, nil
}

// describe mengubah satu record stream menjadi (subject, body). ok=false -> lewati.
func describe(r events.DynamoDBEventRecord) (subject, body string, ok bool) {
	id := strAttr(r.Change.Keys["booking_id"])
	if id == "" || strings.HasPrefix(id, internalPrefix) {
		return "", "", false
	}
	image := r.Change.NewImage
	if r.EventName == "REMOVE" {
		image = r.Change.OldImage
	}
	subject = clip(asciiOnly(fmt.Sprintf("Booking %s: %s", strings.ToLower(r.EventName), id)), 100) // batas subjek SNS
	body = fmt.Sprintf("change: %s\nbooking_id: %s\nevent_id: %s\nnim: %s\nstatus: %s\nchecked_in_at: %s\n",
		r.EventName, id, strAttr(image["event_id"]), strAttr(image["nim"]), strAttr(image["status"]), strAttr(image["checked_in_at"]))
	return subject, body, true
}

func strAttr(v events.DynamoDBAttributeValue) string {
	if v.DataType() == events.DataTypeString {
		return v.String()
	}
	return ""
}

// Subjek SNS harus ASCII tercetak.
func asciiOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, s)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
