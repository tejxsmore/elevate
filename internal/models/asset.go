package models

import (
	"time"

	"github.com/google/uuid"
)

type Asset struct {
	ID              uuid.UUID `db:"id" json:"id"`
	Name            string    `db:"name" json:"name"`
	AssetType       string    `db:"asset_type" json:"asset_type"`
	StorageProvider string    `db:"storage_provider" json:"storage_provider"`
	StoragePath     string    `db:"storage_path" json:"storage_path"`
	MimeType        *string   `db:"mime_type" json:"mime_type,omitempty"`
	SizeBytes       *int64    `db:"size_bytes" json:"size_bytes,omitempty"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}
