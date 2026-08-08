package model

// ScopeType 对话范围类型
type ScopeType = int16

const (
	ScopeTypeNovel   ScopeType = 1 // 小说级对话
	ScopeTypeVolume  ScopeType = 2 // 卷级对话
	ScopeTypeChapter ScopeType = 3 // 章级对话
)

const (
	ChapterContentStatusDraft  int16 = 1 // 草稿正文
	ChapterContentStatusActive int16 = 2 // 当前使用正文
)

const (
	ChapterDraftTypeAI       int16 = 1 // AI 原始草稿，不直接编辑
	ChapterDraftTypeEditable int16 = 2 // 用户可编辑草稿
)
