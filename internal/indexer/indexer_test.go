package indexer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDDB struct {
	items []map[string]types.AttributeValue
	err   error
}

func (f *fakeDDB) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.items = append(f.items, in.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func rec(key string, size int64) events.S3EventRecord {
	return events.S3EventRecord{
		EventTime: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC),
		S3:        events.S3Entity{Object: events.S3Object{Key: key, Size: size}},
	}
}

func s(v types.AttributeValue) string { return v.(*types.AttributeValueMemberS).Value }

func TestIndexesPhoto(t *testing.T) {
	f := &fakeDDB{}
	a := &App{DDB: f, Table: "Photos"}
	if err := a.Handle(context.Background(), events.S3Event{Records: []events.S3EventRecord{rec("photos/EVT-1/abc.jpg", 2048)}}); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 1 {
		t.Fatalf("items=%d", len(f.items))
	}
	it := f.items[0]
	if s(it["event_id"]) != "EVT-1" || s(it["photo_id"]) != "abc.jpg" || s(it["key"]) != "photos/EVT-1/abc.jpg" {
		t.Errorf("item salah: %v", it)
	}
	if it["size_bytes"].(*types.AttributeValueMemberN).Value != "2048" || s(it["uploaded_at"]) != "2026-11-26T10:00:00Z" {
		t.Errorf("size/uploaded_at salah: %v", it)
	}
}

func TestURLEncodedKey(t *testing.T) {
	f := &fakeDDB{}
	a := &App{DDB: f, Table: "Photos"}
	_ = a.Handle(context.Background(), events.S3Event{Records: []events.S3EventRecord{rec("photos/EVT%2D1/a%2Bb.jpg", 1)}})
	if len(f.items) != 1 || s(f.items[0]["event_id"]) != "EVT-1" || s(f.items[0]["photo_id"]) != "a+b.jpg" {
		t.Fatalf("kunci harus di-decode: %v", f.items)
	}
}

func TestSkipsKeysOutsidePattern(t *testing.T) {
	f := &fakeDDB{}
	a := &App{DDB: f, Table: "Photos"}
	err := a.Handle(context.Background(), events.S3Event{Records: []events.S3EventRecord{
		rec("other/EVT-1/a.jpg", 1), rec("photos/a.jpg", 1), rec("photos/EVT-1/sub/a.jpg", 1), rec("photos//a.jpg", 1),
	}})
	if err != nil || len(f.items) != 0 {
		t.Fatalf("err=%v items=%d", err, len(f.items))
	}
}

func TestPutFailureReturnsError(t *testing.T) {
	a := &App{DDB: &fakeDDB{err: errors.New("throttled")}, Table: "Photos"}
	if err := a.Handle(context.Background(), events.S3Event{Records: []events.S3EventRecord{rec("photos/EVT-1/a.jpg", 1)}}); err == nil {
		t.Fatal("harus error supaya S3 mencoba ulang")
	}
}
