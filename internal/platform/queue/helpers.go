package queue

import (
	"time"

	"github.com/google/uuid"
)

func newTradeID() string {
	return uuid.New().String()
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
