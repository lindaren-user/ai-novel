package model

import "time"

type SharedNovel struct {
	ID       int64          `json:"id"`
	Title    string         `json:"title"`
	PlanData JSONMap        `json:"planData"`
	Volumes  []SharedVolume `json:"volumes"`
}

type SharedVolume struct {
	ID        int64           `json:"id"`
	NovelID   int64           `json:"novelId"`
	PlanData  JSONMap         `json:"planData"`
	SortOrder int             `json:"sortOrder"`
	Chapters  []SharedChapter `json:"chapters"`
}

type SharedChapter struct {
	ID        int64      `json:"id"`
	VolumeID  int64      `json:"volumeId"`
	PlanData  JSONMap    `json:"planData"`
	Content   string     `json:"content"`
	SortOrder int        `json:"sortOrder"`
	WordCount int        `json:"wordCount"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Completed *time.Time `json:"completedAt"`
}

type SharedContent struct {
	Type              string      `json:"type"`
	RequiresPassword  bool        `json:"requiresPassword"`
	Novel             SharedNovel `json:"novel"`
	SelectedVolumeID  int64       `json:"selectedVolumeId"`
	SelectedChapterID int64       `json:"selectedChapterId"`
}
