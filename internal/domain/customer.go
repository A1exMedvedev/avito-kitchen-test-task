package domain

import (
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID          uuid.UUID
	DisplayName string
	Phone       string
	CreatedAt   time.Time
}
