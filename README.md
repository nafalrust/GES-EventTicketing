# GES Event Ticketing (Track C: Serverless, Go)

Tiket event kampus: CloudFront + Lambda Function URL + DynamoDB + S3 (SAM).

```
backend/    cmd/ internal/ events/ scripts/ template.yaml samconfig.toml Makefile
frontend/   halaman statis (diunggah ke S3, dilayani CloudFront)
.github/    CI: format, vet, test, cross-compile arm64, validasi template
```

Semua perintah dijalankan dari `backend/`:

```bash
make test        # gofmt + vet + unit test
make run         # server lokal + frontend di http://localhost:8000
make deploy      # sam build + deploy (LabRole sebagai service role)
make web-deploy  # unggah ../frontend ke S3 + invalidasi CloudFront
make smoke       # tes end-to-end lewat CloudFront
```
