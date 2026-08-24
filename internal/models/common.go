package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type ID = uuid.UUID

type JSONB json.RawMessage

func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	return []byte(j), nil
}

func (j *JSONB) Scan(src interface{}) error {
	if src == nil {
		*j = JSONB("null")
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*j = JSONB(append([]byte{}, v...))
		return nil
	case string:
		*j = JSONB(v)
		return nil
	default:
		return errors.New("models: unsupported source type for JSONB.Scan")
	}
}

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSONB) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("models: JSONB.UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

type StringArray = pq.StringArray

type Timestamps struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
