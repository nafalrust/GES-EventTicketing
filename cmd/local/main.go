// Server HTTP lokal untuk development: menjalankan handler Lambda yang SAMA
// tanpa deploy, plus frontend statis dari folder web/. Routing path ditiru di sini
// (di AWS dilakukan CloudFront: /api/health -> HealthFunction, dst).
//
//	go run ./cmd/local                 # buka http://localhost:8000
//	curl localhost:8000/api/health
//
// Untuk /api/health/aws dan upload foto, isi nama resource hasil deploy:
//
//	EVENTS_TABLE=.. BOOKINGS_TABLE=.. PHOTOS_TABLE=.. PHOTOS_BUCKET=.. go run ./cmd/local
//
// /health/aws memakai KREDENSIAL AWS DI MESINMU (bukan role Lambda), jadi ini tes
// koneksi laptop -> AWS. Tes role Lambda yang sebenarnya terjadi setelah deploy.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"

	"ticketing-serverless/internal/awscheck"
	"ticketing-serverless/internal/health"
	"ticketing-serverless/internal/photos"
)

type lambdaFunc = func(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

func adapt(h lambdaFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query := map[string]string{}
		for k := range r.URL.Query() {
			query[k] = r.URL.Query().Get(k)
		}
		req := events.APIGatewayV2HTTPRequest{
			RawPath:               r.URL.Path,
			QueryStringParameters: query,
			Body:                  string(body),
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: r.Method,
					Path:   r.URL.Path,
				},
			},
		}
		resp, err := h(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.WriteString(w, resp.Body)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()
	bucket := os.Getenv("PHOTOS_BUCKET")
	check, err := awscheck.New(ctx, awscheck.Config{
		EventsTable:   env("EVENTS_TABLE", "Events"),
		BookingsTable: env("BOOKINGS_TABLE", "Bookings"),
		PhotosTable:   env("PHOTOS_TABLE", "Photos"),
		PhotosBucket:  bucket,
	})
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", adapt(health.Handler("local")))
	mux.HandleFunc("GET /api/health/aws", adapt(check.Handle))
	if bucket != "" {
		up, err := photos.New(ctx, bucket)
		if err != nil {
			log.Fatalf("gagal memuat konfigurasi AWS: %v", err)
		}
		mux.HandleFunc("GET /api/photos/upload-url", adapt(up.Handle))
	} else {
		log.Print("PHOTOS_BUCKET kosong: /api/photos/upload-url tidak aktif")
	}
	mux.Handle("/", http.FileServer(http.Dir("web"))) // frontend

	port := env("PORT", "8000")
	log.Printf("mode lokal listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
