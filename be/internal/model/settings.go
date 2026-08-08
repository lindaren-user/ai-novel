package model

import (
	"encoding/json"
	"time"
)

type SettingsRequest struct {
	Settings json.RawMessage `json:"settings"`
}

type SettingsResponse struct {
	Settings  json.RawMessage `json:"settings"`
	UpdatedAt time.Time       `json:"updatedAt"`
}
