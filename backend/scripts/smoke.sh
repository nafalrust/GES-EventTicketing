#!/bin/bash
# Tes end-to-end lewat CloudFront. Pemakaian: ./scripts/smoke.sh https://dxxxx.cloudfront.net
# Cocok untuk screenshot bukti.
set -uo pipefail
BASE="${1:?Usage: smoke.sh <site-url>}"
BASE="${BASE%/}"
fail=0

echo "== GET /  (frontend statis dari S3 via CloudFront)"
code=$(curl -sS -o /dev/null -w "%{http_code}" "$BASE/") || fail=1
echo "HTTP $code"; [ "$code" = "200" ] || fail=1

echo
echo "== GET /api/health  (CloudFront -> Function URL -> HealthFunction)"
code=$(curl -sS -o /tmp/health.json -w "%{http_code}" "$BASE/api/health") || fail=1
cat /tmp/health.json; echo
echo "HTTP $code"; [ "$code" = "200" ] || fail=1

echo
echo "== GET /api/health/aws  (STS + 3 tabel DynamoDB + read/write + bucket S3)"
code=$(curl -sS -o /tmp/health_aws.json -w "%{http_code}" "$BASE/api/health/aws") || fail=1
cat /tmp/health_aws.json; echo
echo "HTTP $code"; [ "$code" = "200" ] || fail=1

echo
echo "== GET /api/photos/upload-url  (presigned URL; harus 200 + upload_url)"
code=$(curl -sS -o /tmp/upload.json -w "%{http_code}" "$BASE/api/photos/upload-url?event_id=smoke-test&content_type=image/jpeg") || fail=1
sed 's/"upload_url":"[^"]*"/"upload_url":"<dipersingkat>"/' /tmp/upload.json; echo
echo "HTTP $code"; [ "$code" = "200" ] || fail=1
grep -q '"upload_url"' /tmp/upload.json || fail=1

echo
echo "== GET /api/photos/upload-url tanpa parameter (harus 400)"
code=$(curl -sS -o /dev/null -w "%{http_code}" "$BASE/api/photos/upload-url") || true
echo "HTTP $code"; [ "$code" = "400" ] || fail=1

echo
echo "== GET /api/nope  (tidak ada route API -> harus BUKAN 200)"
code=$(curl -sS -o /dev/null -w "%{http_code}" "$BASE/api/nope") || true
echo "HTTP $code"; [ "$code" != "200" ] || fail=1

if [ "$fail" = 0 ]; then echo; echo "SEMUA CEK LULUS"; else echo; echo "ADA CEK GAGAL"; exit 1; fi
