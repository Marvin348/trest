package publishing

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

func ListenOutbox(
	ctx context.Context,
	databaseURL string,
	wake chan<- struct{},
) {
	for ctx.Err() == nil {
		conn, err := pgx.Connect(ctx, databaseURL)
		if err == nil {
			_, err = conn.Exec(ctx, "LISTEN trest_outbox")
		}

		if err == nil {
			log.Println("listening for outbox notifications")

			for ctx.Err() == nil {
				_, err := conn.WaitForNotification(ctx)
				if err != nil {
					break
				}

				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}

		if conn != nil {
			_ = conn.Close(context.Background())
		}

		if ctx.Err() != nil {
			return
		}

		log.Printf("outbox listener disconnected: %v", err)

		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return
		}
	}
}
