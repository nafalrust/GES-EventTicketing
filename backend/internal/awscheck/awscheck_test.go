package awscheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ---- fake AWS ----

type fakeDDB struct {
	describeErr map[string]error // per nama tabel
	putErr      error
	items       map[string]bool
}

func newFakeDDB() *fakeDDB {
	return &fakeDDB{items: map[string]bool{}, describeErr: map[string]error{}}
}

func (f *fakeDDB) DescribeTable(_ context.Context, in *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	if err := f.describeErr[*in.TableName]; err != nil {
		return nil, err
	}
	return &dynamodb.DescribeTableOutput{Table: &types.TableDescription{TableStatus: types.TableStatusActive}}, nil
}

func (f *fakeDDB) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	f.items[in.Item["event_id"].(*types.AttributeValueMemberS).Value] = true
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeDDB) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	id := in.Key["event_id"].(*types.AttributeValueMemberS).Value
	if !f.items[id] {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"event_id": in.Key["event_id"]}}, nil
}

func (f *fakeDDB) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	delete(f.items, in.Key["event_id"].(*types.AttributeValueMemberS).Value)
	return &dynamodb.DeleteItemOutput{}, nil
}

type fakeS3 struct{ err error }

func (f fakeS3) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &s3.HeadBucketOutput{}, nil
}

type fakeSTS struct{ err error }

func (f fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	arn := "arn:aws:sts::123456789012:assumed-role/AwsCheckRole/test-session"
	return &sts.GetCallerIdentityOutput{Arn: &arn}, nil
}

// ---- helpers ----

func run(d *fakeDDB, s fakeSTS, b fakeS3) (int, string, map[string]any) {
	app := &App{DDB: d, STS: s, S3: b, Region: "us-east-1",
		Cfg: Config{EventsTable: "Events", BookingsTable: "Bookings", PhotosTable: "Photos", PhotosBucket: "photos-bucket"}}
	resp, err := app.Handle(context.Background(), events.APIGatewayV2HTTPRequest{})
	if err != nil {
		panic(err)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(resp.Body), &body)
	return resp.StatusCode, resp.Body, body
}

// ---- tests ----

func TestAllChecksOK(t *testing.T) {
	d := newFakeDDB()
	code, _, body := run(d, fakeSTS{}, fakeS3{})
	if code != 200 || body["status"] != "ok" {
		t.Fatalf("got %d %v", code, body)
	}
	checks := body["checks"].(map[string]any)
	if checks["sts"].(map[string]any)["detail"] != "assumed-role/AwsCheckRole" {
		t.Fatalf("role detail: %v", checks["sts"])
	}
	if len(d.items) != 0 {
		t.Fatalf("item probe harus dihapus lagi, tersisa %v", d.items)
	}
}

func TestNoAccountIDLeak(t *testing.T) {
	_, raw, _ := run(newFakeDDB(), fakeSTS{}, fakeS3{})
	if strings.Contains(raw, "123456789012") {
		t.Fatalf("account ID bocor: %s", raw)
	}
}

func TestTableMissing(t *testing.T) {
	d := newFakeDDB()
	d.describeErr["Bookings"] = errors.New("ResourceNotFoundException: Requested resource not found")
	code, _, body := run(d, fakeSTS{}, fakeS3{})
	if code != 503 || body["status"] != "degraded" {
		t.Fatalf("got %d %v", code, body)
	}
	if _, ok := body["checks"].(map[string]any)["dynamodb_rw"]; ok {
		t.Fatal("rw check tidak boleh jalan kalau tabel tidak ada")
	}
}

func TestWriteDenied(t *testing.T) {
	d := newFakeDDB()
	d.putErr = errors.New("AccessDeniedException: not authorized to perform dynamodb:PutItem")
	code, _, body := run(d, fakeSTS{}, fakeS3{})
	rw := body["checks"].(map[string]any)["dynamodb_rw"].(map[string]any)
	if code != 503 || rw["ok"] != false {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestSTSFails(t *testing.T) {
	code, _, body := run(newFakeDDB(), fakeSTS{err: errors.New("ExpiredToken")}, fakeS3{})
	if code != 503 || body["checks"].(map[string]any)["sts"].(map[string]any)["ok"] != false {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestRoleFromARN(t *testing.T) {
	cases := map[string]string{
		"arn:aws:sts::123456789012:assumed-role/MyRole/s1": "assumed-role/MyRole",
		"arn:aws:iam::123456789012:user/nafal":             "user/nafal",
		"garbage":                                          "unknown",
	}
	for in, want := range cases {
		if got := roleFromARN(in); got != want {
			t.Errorf("roleFromARN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOneTableMissingReportsWhich(t *testing.T) {
	d := newFakeDDB()
	d.describeErr["Photos"] = errors.New("ResourceNotFoundException")
	_, raw, _ := run(d, fakeSTS{}, fakeS3{})
	if !strings.Contains(raw, "photos: ResourceNotFoundException") {
		t.Fatalf("pesan harus menyebut tabel yang gagal: %s", raw)
	}
}

func TestBucketUnreachable(t *testing.T) {
	code, _, body := run(newFakeDDB(), fakeSTS{}, fakeS3{err: errors.New("AccessDenied")})
	s3c := body["checks"].(map[string]any)["s3_bucket"].(map[string]any)
	if code != 503 || s3c["ok"] != false {
		t.Fatalf("got %d %v", code, body)
	}
}
