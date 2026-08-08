package model

import (
	"encoding/json"
	"time"
)

type Novel struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"userId"`
	Title             string    `json:"title"`
	PlanData          JSONMap   `json:"planData"`
	SetupOriginalText string    `json:"setupOriginalText"`
	Status            int16     `json:"status"`
	WordCount         int       `json:"wordCount"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type Volume struct {
	ID           int64     `json:"id"`
	NovelID      int64     `json:"novelId"`
	PlanData     JSONMap   `json:"planData"`
	SortOrder    int       `json:"sortOrder"`
	Status       int16     `json:"status"`
	WordCount    int       `json:"wordCount"`
	ChapterCount int       `json:"chapterCount"`
	IsDeleted    int16     `json:"isDeleted"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Chapter struct {
	ID          int64      `json:"id"`
	VolumeID    int64      `json:"volumeId"`
	PlanData    JSONMap    `json:"planData"`
	Content     string     `json:"content"`
	SortOrder   int        `json:"sortOrder"`
	Status      int16      `json:"status"`
	WordCount   int        `json:"wordCount"`
	IsDeleted   int16      `json:"isDeleted"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt"`
}

type ChatMessage struct {
	ID         int64     `json:"id"`
	SessionID  int64     `json:"sessionId"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	RenderData JSONMap   `json:"renderData"`
	DraftID    int64     `json:"draftId,omitempty"`
	IsDeleted  int16     `json:"isDeleted,omitempty"`
	Temporary  bool      `json:"temporary,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
}

type ChatSession struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	ScopeType int16     `json:"scopeType"`
	ScopeID   int64     `json:"scopeId"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ChatSessionMeta struct {
	ID        int64 `json:"id"`
	ScopeType int16 `json:"scopeType"`
	ScopeID   int64 `json:"scopeId"`
}

type ChatMessagesResponse struct {
	Messages []ChatMessage   `json:"messages"`
	Session  ChatSessionMeta `json:"session"`
}

type ChapterContentDraftRecord struct {
	ID              int64      `json:"id"`
	ChapterID       int64      `json:"chapterId"`
	SourceMessageID int64      `json:"sourceMessageId"`
	DraftType       int16      `json:"draftType"`
	OriginDraftID   int64      `json:"originDraftId"`
	DraftName       string     `json:"draftName"`
	Content         string     `json:"content"`
	Status          int16      `json:"status"`
	WordCount       int        `json:"wordCount"`
	IsDeleted       int16      `json:"isDeleted"`
	UsedAt          *time.Time `json:"usedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ChapterHumanizeResult struct {
	Content string `json:"content"`
	Report  string `json:"report"`
}

// ChapterProofreadSuggestion 表示 AI 校审返回的一条临时修改建议。
type ChapterProofreadSuggestion struct {
	OriginalText  string `json:"originalText"`
	SuggestedText string `json:"suggestedText"`
	Reason        string `json:"reason"`
}

type CreateNovelResponse struct {
	Novel   Novel        `json:"novel"`
	Message *ChatMessage `json:"message,omitempty"`
}

type NovelOverviewItem struct {
	ID                int64     `json:"id"`
	Title             string    `json:"title"`
	PlanData          JSONMap   `json:"planData"`
	SetupOriginalText string    `json:"setupOriginalText"`
	WordCount         int       `json:"wordCount"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type WorkspaceDashboard struct {
	TotalWords        int              `json:"totalWords"`
	CompletedChapters int              `json:"completedChapters"`
	VolumeCount       int              `json:"volumeCount"`
	WritingHours      float64          `json:"writingHours"`
	LastEditedAt      time.Time        `json:"lastEditedAt"`
	WordTrend         []WordTrendPoint `json:"wordTrend"`
}

type WordTrendPoint struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	Words     int    `json:"words"`
	WordLabel string `json:"wordLabel"`
}

type JSONMap map[string]any

// Scan 将 PostgreSQL JSON/JSONB 字段转换为通用 JSONMap。
func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data, _ = json.Marshal(v)
	}
	if len(data) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(data, m)
}
