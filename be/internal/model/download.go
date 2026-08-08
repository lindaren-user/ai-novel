package model

// DownloadContent 下载聚合内容，包含小说、卷、章的完整正文树。
type DownloadContent struct {
	Novel SharedNovel
}

// DownloadJobView 下载任务对外状态。
type DownloadJobView struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
	Filename string `json:"filename"`
}
