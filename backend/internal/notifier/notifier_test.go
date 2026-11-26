package notifier

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type fakeSNS struct {
	sent    []*sns.PublishInput
	failAt  int // 1-based; 0 = tidak pernah gagal
	callNum int
}

func (f *fakeSNS) Publish(_ context.Context, in *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.callNum++
	if f.failAt != 0 && f.callNum == f.failAt {
		return nil, errors.New("sns down")
	}
	f.sent = append(f.sent, in)
	return &sns.PublishOutput{}, nil
}

func rec(name, id, title, seq string) events.DynamoDBEventRecord {
	img := map[string]events.DynamoDBAttributeValue{
		"nim":      events.NewStringAttribute(title),
		"event_id": events.NewStringAttribute("EVT-1"),
		"status":   events.NewStringAttribute("open"),
	}
	r := events.DynamoDBEventRecord{EventName: name}
	r.Change.Keys = map[string]events.DynamoDBAttributeValue{"booking_id": events.NewStringAttribute(id)}
	r.Change.SequenceNumber = seq
	if name == "REMOVE" {
		r.Change.OldImage = img
	} else {
		r.Change.NewImage = img
	}
	return r
}

func TestInsertPublishes(t *testing.T) {
	f := &fakeSNS{}
	a := &App{SNS: f, TopicARN: "arn:topic"}
	resp, err := a.Handle(context.Background(), events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{rec("INSERT", "t-1", "Wifi mati", "1")}})
	if err != nil || len(resp.BatchItemFailures) != 0 {
		t.Fatalf("err=%v failures=%v", err, resp.BatchItemFailures)
	}
	if len(f.sent) != 1 {
		t.Fatalf("sent=%d", len(f.sent))
	}
	if got := *f.sent[0].Subject; got != "Booking insert: t-1" {
		t.Errorf("subject=%q", got)
	}
	if !strings.Contains(*f.sent[0].Message, "nim: Wifi mati") || *f.sent[0].TopicArn != "arn:topic" {
		t.Errorf("message/topic salah: %q %q", *f.sent[0].Message, *f.sent[0].TopicArn)
	}
}

func TestRemoveUsesOldImage(t *testing.T) {
	f := &fakeSNS{}
	a := &App{SNS: f, TopicARN: "arn:topic"}
	_, _ = a.Handle(context.Background(), events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{rec("REMOVE", "t-2", "Printer", "1")}})
	if len(f.sent) != 1 || !strings.Contains(*f.sent[0].Message, "nim: Printer") {
		t.Fatalf("REMOVE harus membaca OldImage: %+v", f.sent)
	}
}

func TestProbeItemIgnored(t *testing.T) {
	f := &fakeSNS{}
	a := &App{SNS: f, TopicARN: "arn:topic"}
	_, _ = a.Handle(context.Background(), events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
		rec("INSERT", "__healthcheck__", "x", "1"), rec("REMOVE", "__healthcheck__", "x", "2"),
	}})
	if len(f.sent) != 0 {
		t.Fatalf("probe health check tidak boleh memicu notifikasi, sent=%d", len(f.sent))
	}
}

func TestPartialBatchFailure(t *testing.T) {
	f := &fakeSNS{failAt: 2}
	a := &App{SNS: f, TopicARN: "arn:topic"}
	resp, err := a.Handle(context.Background(), events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
		rec("INSERT", "a", "A", "100"), rec("INSERT", "b", "B", "101"), rec("INSERT", "c", "C", "102"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.BatchItemFailures) != 2 || resp.BatchItemFailures[0].ItemIdentifier != "101" || resp.BatchItemFailures[1].ItemIdentifier != "102" {
		t.Fatalf("harus melaporkan record gagal + sisanya, got %+v", resp.BatchItemFailures)
	}
}

func TestSubjectSanitized(t *testing.T) {
	_, subj, ok := describe(rec("INSERT", "t-é"+strings.Repeat("x", 200), "T", "1"))
	_ = subj
	s, _, _ := describe(rec("INSERT", "t-é"+strings.Repeat("x", 200), "T", "1"))
	if !ok || len(s) != 100 || strings.ContainsRune(s, 'é') {
		t.Fatalf("subject harus ASCII dan <=100 char, len=%d", len(s))
	}
}
