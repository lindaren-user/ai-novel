package model

import "time"

const (
	ModelRunScopeMessage        int16 = 1
	ModelRunScopeNovelSetup     int16 = 2
	ModelRunScopeDraftHumanize  int16 = 3
	ModelRunScopeDraftProofread int16 = 4
)

const (
	ModelRunStatusRunning  int16 = 0
	ModelRunStatusSuccess  int16 = 1
	ModelRunStatusFailed   int16 = 2
	ModelRunStatusCanceled int16 = 3
)

type ModelRun struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"userId"`
	ScopeType    int16      `json:"scopeType"`
	ScopeID      *int64     `json:"scopeId,omitempty"`
	ModelID      int64      `json:"modelId"`
	Status       int16      `json:"status"`
	TokenCount   int64      `json:"tokenCount"`
	FinishReason string     `json:"finishReason"`
	ErrorMessage string     `json:"errorMessage"`
	StartTime    time.Time  `json:"startTime"`
	EndTime      *time.Time `json:"endTime,omitempty"`
}
