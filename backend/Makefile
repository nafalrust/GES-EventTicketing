VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
STACK   ?= campus-events

# Role eksekusi Lambda: default LabRole yang sudah ada (policy lab tidak mengizinkan iam:CreateRole).
# Kosongkan (make deploy ROLE_ARN=) kalau akunmu boleh membuat role -> SAM membuat role per fungsi.
ROLE_ARN    ?= $(shell aws iam get-role --role-name LabRole --query Role.Arn --output text 2>/dev/null)
ALERT_EMAIL ?=

# Ambil output stack (dipakai target smoke/web-deploy/destroy/run-aws)
out = $(shell aws cloudformation describe-stacks --stack-name $(STACK) --query "Stacks[0].Outputs[?OutputKey=='$(1)'].OutputValue" --output text)

.PHONY: test run run-aws validate build deploy web-deploy smoke logs-health logs-aws logs-photourl logs-indexer logs-notifier destroy

# ---- dipanggil oleh `sam build` (nama target = "build-" + logical ID di template.yaml) ----
build-HealthFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/health

build-AwsCheckFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/awscheck

build-PhotoUrlFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/photourl

build-PhotoIndexerFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/indexer

build-NotifierFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/notifier

build-JanitorFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags "$(LDFLAGS)" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/janitor

# ---- perintah harian ----
test:
	gofmt -l . | (! grep .)
	go vet ./...
	go test ./...

run:
	go run ./cmd/local

# Server lokal yang terhubung ke resource AWS hasil deploy (memakai kredensial laptopmu)
run-aws:
	EVENTS_TABLE=$(call out,EventsTableName) BOOKINGS_TABLE=$(call out,BookingsTableName) PHOTOS_TABLE=$(call out,PhotosTableName) PHOTOS_BUCKET=$(call out,PhotosBucketName) go run ./cmd/local

validate:
	sam validate --lint --region us-east-1

build:
	sam build

deploy: build
	@test -n "$(ROLE_ARN)" || echo "PERINGATAN: ROLE_ARN kosong -> SAM akan membuat role sendiri (butuh iam:CreateRole)"
	sam deploy --no-confirm-changeset --role-arn "$(ROLE_ARN)" --parameter-overrides "LambdaRoleArn=$(ROLE_ARN)" $(if $(ALERT_EMAIL),"AlertEmail=$(ALERT_EMAIL)")

# Unggah frontend ke S3 lalu bersihkan cache CloudFront
web-deploy:
	aws s3 sync ../frontend/ s3://$(call out,WebBucketName)/ --delete
	aws cloudfront create-invalidation --distribution-id $(call out,DistributionId) --paths "/*" >/dev/null
	@echo "Buka: $(call out,SiteUrl)"

smoke:
	./scripts/smoke.sh $(call out,SiteUrl)

logs-health:
	sam logs --stack-name $(STACK) --name HealthFunction --tail

logs-aws:
	sam logs --stack-name $(STACK) --name AwsCheckFunction --tail

logs-photourl:
	sam logs --stack-name $(STACK) --name PhotoUrlFunction --tail

logs-indexer:
	sam logs --stack-name $(STACK) --name PhotoIndexerFunction --tail

logs-notifier:
	sam logs --stack-name $(STACK) --name NotifierFunction --tail

# Kedua bucket harus kosong dulu, kalau tidak CloudFormation gagal menghapusnya.
destroy:
	aws s3 rm s3://$(call out,WebBucketName)/ --recursive
	aws s3 rm s3://$(call out,PhotosBucketName)/ --recursive
	sam delete --stack-name $(STACK)
