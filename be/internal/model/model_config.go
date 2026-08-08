package model

import "time"

const (
	ModelConfigStatusEnabled  int16 = 1 // 模型启用，可参与用户选择和调用。
	ModelConfigStatusDisabled int16 = 2 // 模型禁用，不再参与用户选择和调用。
)

type ModelConfig struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	ModelID     string    `json:"modelId"`
	APIURL      string    `json:"apiUrl"`
	APIKey      string    `json:"apiKey"`
	TopP        float64   `json:"topP"`
	Temperature float64   `json:"temperature"`
	Status      int16     `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateModelRequest struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ModelID     string  `json:"modelId"`
	APIURL      string  `json:"apiUrl"`
	APIKey      string  `json:"apiKey"`
	TopP        float64 `json:"topP"`
	Temperature float64 `json:"temperature"`
	Status      int16   `json:"status"`
}

type TestModelResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}
