// Package janitor: dipicu EventBridge rule berjadwal (cron).
// Tahap inisiasi: hanya membuktikan jadwal -> Lambda tersambung. Logika
// sebenarnya (mis. auto-close tiket lama) ditambahkan di tahap berikutnya.
package janitor

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
)

func Handle(_ context.Context, ev events.CloudWatchEvent) (string, error) {
	log.Printf("janitor jalan: source=%s time=%s", ev.Source, ev.Time.Format("2006-01-02T15:04:05Z07:00"))
	return "ok", nil
}
