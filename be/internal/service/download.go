package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"
)

// DownloadService 下载服务接口，负责创建下载任务、查询进度和返回文件。
type DownloadService interface {
	Start(ctx context.Context, userID int64, scopeType string, scopeID int64, format string, layout string) (model.DownloadJobView, error)
	List(ctx context.Context, userID int64) ([]model.DownloadJobView, error)
	Status(ctx context.Context, userID int64, jobID string) (model.DownloadJobView, error)
	File(ctx context.Context, userID int64, jobID string) (string, string, []byte, error)
}

type downloadService struct {
	repositories repo.Repositories
	mu           sync.RWMutex
	jobs         map[string]*downloadJob
}

type downloadJob struct {
	ID       string
	UserID   int64
	Status   string
	Progress int
	Message  string
	Filename string
	MIME     string
	Data     []byte
	Err      string
	Created  time.Time
}

type downloadContent struct {
	Novel  model.SharedNovel
	Novels []model.SharedNovel
}

// NewDownloadService 创建下载服务。
func NewDownloadService(repositories repo.Repositories) DownloadService {
	return &downloadService{
		repositories: repositories,
		jobs:         make(map[string]*downloadJob),
	}
}

// Start 创建下载任务并异步生成文件。
func (s *downloadService) Start(ctx context.Context, userID int64, scopeType string, scopeID int64, format string, layout string) (model.DownloadJobView, error) {
	scopeType = strings.TrimSpace(scopeType)
	format = normalizeDownloadFormat(format)
	layout = normalizeDownloadLayout(layout)
	if (scopeType != "all" && scopeID <= 0) || !validDownloadScope(scopeType) {
		return model.DownloadJobView{}, ErrResourceNotFound
	}

	job := &downloadJob{
		ID:       randomJobID(),
		UserID:   userID,
		Status:   "pending",
		Progress: 1,
		Message:  "已创建下载任务",
		Created:  time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.cleanupLocked()
	s.mu.Unlock()

	go s.buildJob(context.Background(), job.ID, userID, scopeType, scopeID, format, layout)
	return job.view(), nil
}

// List 查询当前用户的内存下载任务列表。
func (s *downloadService) List(ctx context.Context, userID int64) ([]model.DownloadJobView, error) {
	s.mu.Lock()
	s.cleanupLocked()
	jobs := make([]*downloadJob, 0)
	for _, job := range s.jobs {
		if job.UserID == userID {
			jobs = append(jobs, job.snapshot())
		}
	}
	s.mu.Unlock()

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Created.After(jobs[j].Created)
	})
	views := make([]model.DownloadJobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, job.view())
	}
	return views, nil
}

// Status 查询下载任务进度。
func (s *downloadService) Status(ctx context.Context, userID int64, jobID string) (model.DownloadJobView, error) {
	job, err := s.findJob(userID, jobID)
	if err != nil {
		return model.DownloadJobView{}, wrapError("查询下载任务失败", err)
	}
	return job.view(), nil
}

// File 返回已完成下载任务的文件内容。
func (s *downloadService) File(ctx context.Context, userID int64, jobID string) (string, string, []byte, error) {
	job, err := s.findJob(userID, jobID)
	if err != nil {
		return "", "", nil, wrapError("查询下载文件任务失败", err)
	}
	if job.Status != "done" || len(job.Data) == 0 {
		return "", "", nil, ErrResourceNotFound
	}
	return job.Filename, job.MIME, append([]byte(nil), job.Data...), nil
}

// buildJob 聚合正文并生成下载文件。
func (s *downloadService) buildJob(ctx context.Context, jobID string, userID int64, scopeType string, scopeID int64, format string, layout string) {
	s.updateJob(jobID, "running", 10, "正在查询正文内容", nil)

	content, err := s.loadContent(ctx, userID, scopeType, scopeID)
	if err != nil {
		s.finishJobError(jobID, err)
		return
	}
	s.updateJob(jobID, "running", 45, "正在整理章节内容", nil)

	data, filename, mime, err := renderContentFile(content, scopeType, format, layout)
	if err != nil {
		s.finishJobError(jobID, err)
		return
	}
	s.updateJob(jobID, "done", 100, "下载文件已生成", func(job *downloadJob) {
		job.Data = data
		job.Filename = filename
		job.MIME = mime
	})
}

// loadContent 按下载范围读取完整内容。
func (s *downloadService) loadContent(ctx context.Context, userID int64, scopeType string, scopeID int64) (downloadContent, error) {
	var (
		content downloadContent
		err     error
	)
	switch scopeType {
	case "all":
		content.Novels, err = s.loadAllContent(ctx, userID)
	case "novel":
		content.Novel, err = s.loadNovelContent(ctx, userID, scopeID)
	case "volume":
		content.Novel, err = s.loadVolumeContent(ctx, userID, scopeID)
	case "chapter":
		content.Novel, err = s.loadChapterContent(ctx, userID, scopeID)
	default:
		err = ErrResourceNotFound
	}
	if errors.Is(err, ErrResourceNotFound) || errors.Is(err, repo.ErrNovelNotFound) || errors.Is(err, repo.ErrVolumeNotFound) || errors.Is(err, repo.ErrChapterNotFound) {
		return downloadContent{}, ErrResourceNotFound
	}
	if errors.Is(err, ErrForbidden) {
		return downloadContent{}, ErrForbidden
	}
	return content, wrapError("加载下载内容失败", err)
}

// loadAllContent 组装当前用户全部小说的下载内容。
func (s *downloadService) loadAllContent(ctx context.Context, userID int64) ([]model.SharedNovel, error) {
	novels, err := s.repositories.Novels.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]model.SharedNovel, 0, len(novels))
	for _, item := range novels {
		novel, err := sharedNovelContent(ctx, s.repositories, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, novel)
	}
	return result, nil
}

// loadNovelContent 校验小说归属后组装整本小说下载内容。
func (s *downloadService) loadNovelContent(ctx context.Context, userID int64, novelID int64) (model.SharedNovel, error) {
	if _, err := ensureNovelOwnerWithRepositories(ctx, s.repositories, userID, novelID); err != nil {
		return model.SharedNovel{}, err
	}
	return sharedNovelContent(ctx, s.repositories, novelID)
}

// loadVolumeContent 校验卷归属后组装单卷下载内容。
func (s *downloadService) loadVolumeContent(ctx context.Context, userID int64, volumeID int64) (model.SharedNovel, error) {
	if _, err := ensureVolumeOwnerWithRepositories(ctx, s.repositories, userID, volumeID); err != nil {
		return model.SharedNovel{}, err
	}
	novel, _, err := sharedVolumeContent(ctx, s.repositories, volumeID)
	return novel, err
}

// loadChapterContent 校验章节归属后组装单章下载内容。
func (s *downloadService) loadChapterContent(ctx context.Context, userID int64, chapterID int64) (model.SharedNovel, error) {
	if _, err := ensureChapterOwnerWithRepositories(ctx, s.repositories, userID, chapterID); err != nil {
		return model.SharedNovel{}, err
	}
	novel, _, _, err := sharedChapterContent(ctx, s.repositories, chapterID)
	return novel, err
}

// findJob 查找并校验任务归属。
func (s *downloadService) findJob(userID int64, jobID string) (*downloadJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job := s.jobs[strings.TrimSpace(jobID)]
	if job == nil || job.UserID != userID {
		return nil, ErrResourceNotFound
	}
	return job.snapshot(), nil
}

// updateJob 更新任务状态。
func (s *downloadService) updateJob(jobID string, status string, progress int, message string, extra func(*downloadJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	job.Status = status
	job.Progress = progress
	job.Message = message
	if extra != nil {
		extra(job)
	}
}

// finishJobError 将任务标记为失败。
func (s *downloadService) finishJobError(jobID string, err error) {
	message := "生成下载文件失败"
	if errors.Is(err, ErrResourceNotFound) {
		message = "下载内容不存在"
	}
	if errors.Is(err, ErrForbidden) {
		message = "无权下载该内容"
	}
	s.updateJob(jobID, "error", 100, message, func(job *downloadJob) {
		job.Err = err.Error()
	})
}

// cleanupLocked 清理过期下载任务。
func (s *downloadService) cleanupLocked() {
	deadline := time.Now().Add(-30 * time.Minute)
	for id, job := range s.jobs {
		if job.Created.Before(deadline) {
			delete(s.jobs, id)
		}
	}
}

// view 转换为对外任务状态。
func (j *downloadJob) view() model.DownloadJobView {
	return model.DownloadJobView{
		ID:       j.ID,
		Status:   j.Status,
		Progress: j.Progress,
		Message:  j.Message,
		Filename: j.Filename,
	}
}

// snapshot 复制任务，避免外部读取时持有锁。
func (j *downloadJob) snapshot() *downloadJob {
	cp := *j
	cp.Data = append([]byte(nil), j.Data...)
	return &cp
}

// validDownloadScope 判断下载范围是否合法。
func validDownloadScope(scopeType string) bool {
	return scopeType == "all" || scopeType == "novel" || scopeType == "volume" || scopeType == "chapter"
}

// normalizeDownloadFormat 规范化下载格式。
func normalizeDownloadFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if !strings.HasPrefix(format, ".") {
		format = "." + format
	}
	switch format {
	case ".txt", ".docx", ".pdf":
		return format
	default:
		return ".txt"
	}
}

// normalizeDownloadLayout 规范化下载组织方式。
func normalizeDownloadLayout(layout string) string {
	switch strings.ToLower(strings.TrimSpace(layout)) {
	case "chapter", "by_chapter", "按章":
		return "chapter"
	default:
		return "volume"
	}
}

// downloadTitle 根据下载范围推断文件标题。
func downloadTitle(novel model.SharedNovel, scopeType string) string {
	if scopeType == "volume" && len(novel.Volumes) > 0 {
		return sharedVolumeTitle(novel.Volumes[0])
	}
	if scopeType == "chapter" && len(novel.Volumes) > 0 && len(novel.Volumes[0].Chapters) > 0 {
		return sharedChapterTitle(novel.Volumes[0].Chapters[0])
	}
	return novel.Title
}

// plainDownloadText 将完整内容树整理为纯文本。
func plainDownloadText(novel model.SharedNovel, layout string) string {
	var b strings.Builder
	b.WriteString("【小说】")
	b.WriteString(novel.Title)
	b.WriteString("\n\n")
	volIndex := 0
	for _, volume := range novel.Volumes {
		volIndex++
		if layout != "chapter" {
			b.WriteString("【")
			if title := sharedVolumeTitle(volume); title != "" {
				b.WriteString(title)
			} else {
				fmt.Fprintf(&b, "第%d卷", volIndex)
			}
			b.WriteString("】\n\n")
		}
		chIndex := 0
		for _, chapter := range volume.Chapters {
			chIndex++
			b.WriteString("【")
			if title := sharedChapterTitle(chapter); title != "" {
				b.WriteString(title)
			} else {
				fmt.Fprintf(&b, "第%d章", chIndex)
			}
			b.WriteString("】\n\n")
			content := strings.TrimSpace(chapter.Content)
			if content != "" {
				paras := splitParagraphs(content)
				for _, para := range paras {
					para = strings.TrimSpace(para)
					if para == "" {
						continue
					}
					b.WriteString("　　")
					b.WriteString(para)
					b.WriteString("\n\n")
				}
			} else {
				b.WriteString("本章正文尚未生成。\n\n")
			}
		}
	}
	return b.String()
}

// renderDownloadFile 根据格式生成下载文件。
func renderDownloadFile(title string, text string, format string) ([]byte, string, string, error) {
	filename := safeDownloadFilename(title) + format
	switch format {
	case ".docx":
		data, err := buildDocx(title, text)
		return data, filename, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", err
	case ".pdf":
		return buildPDF(title, text), filename, "application/pdf", nil
	default:
		return []byte(text), filename, "text/plain; charset=utf-8", nil
	}
}

// renderContentFile 根据下载范围选择普通文件或全量小说压缩包。
func renderContentFile(content downloadContent, scopeType string, format string, layout string) ([]byte, string, string, error) {
	if scopeType == "all" {
		return renderAllNovelsZip(content.Novels, format, layout)
	}
	title := downloadTitle(content.Novel, scopeType)
	text := plainDownloadText(content.Novel, layout)
	return renderDownloadFile(title, text, format)
}

// renderAllNovelsZip 将全部小说打包为 zip，压缩包内每本小说一个文件。
func renderAllNovelsZip(novels []model.SharedNovel, format string, layout string) ([]byte, string, string, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, novel := range novels {
		title := strings.TrimSpace(novel.Title)
		if title == "" {
			title = fmt.Sprintf("未命名小说_%d", i+1)
		}
		text := plainDownloadText(novel, layout)
		data, filename, _, err := renderDownloadFile(title, text, format)
		if err != nil {
			_ = zw.Close()
			return nil, "", "", err
		}
		filename = uniqueZipFilename(filename, i+1)
		w, err := zw.Create(filename)
		if err != nil {
			_ = zw.Close()
			return nil, "", "", err
		}
		if _, err := w.Write(data); err != nil {
			_ = zw.Close()
			return nil, "", "", wrapError("写入全量下载压缩包失败", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", "", wrapError("关闭全量下载压缩包失败", err)
	}
	return buf.Bytes(), "全部创作数据.zip", "application/zip", nil
}

// uniqueZipFilename 为压缩包内文件添加序号，避免同名小说互相覆盖。
func uniqueZipFilename(filename string, index int) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return fmt.Sprintf("%03d_未命名小说.txt", index)
	}
	return fmt.Sprintf("%03d_%s", index, filename)
}

func sharedVolumeTitle(volume model.SharedVolume) string {
	if title, ok := volume.PlanData["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if volume.SortOrder > 0 {
		return fmt.Sprintf("第%d卷", volume.SortOrder)
	}
	return ""
}

func sharedChapterTitle(chapter model.SharedChapter) string {
	if title, ok := chapter.PlanData["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if chapter.SortOrder > 0 {
		return fmt.Sprintf("第%d章", chapter.SortOrder)
	}
	return ""
}

// buildDocx 使用标准库生成最小 DOCX 文件。
func buildDocx(title string, text string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   docxDocumentXML(title, text),
	}
	order := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			return nil, wrapError("写入 DOCX 内容失败", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, wrapError("关闭 DOCX 压缩包失败", err)
	}
	return buf.Bytes(), nil
}

// docxDocumentXML 生成 Word 主文档 XML。
func docxDocumentXML(title string, text string) string {
	lines := append([]string{title, ""}, strings.Split(text, "\n")...)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, line := range lines {
		b.WriteString(`<w:p><w:r><w:t xml:space="preserve">`)
		b.WriteString(html.EscapeString(line))
		b.WriteString(`</w:t></w:r></w:p>`)
	}
	b.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
	return b.String()
}

// buildPDF 生成使用中文 CID 字体的基础 PDF。
func buildPDF(title string, text string) []byte {
	lines := wrapPDFLines(append([]string{title, ""}, strings.Split(text, "\n")...))
	pages := chunkPDFLines(lines, 48)
	fontObjectID := 3 + len(pages)*2
	cidObjectID := fontObjectID + 1
	fontDescriptorID := fontObjectID + 2
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", ""}
	pageRefs := make([]string, 0, len(pages))
	for i, pageLines := range pages {
		pageObjectID := 3 + i*2
		contentObjectID := pageObjectID + 1
		pageRefs = append(pageRefs, fmt.Sprintf("%d 0 R", pageObjectID))
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObjectID, contentObjectID),
			pdfContentStream(pageLines),
		)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), len(pageRefs))
	objects = append(objects,
		fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [%d 0 R] >>", cidObjectID),
		fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> /FontDescriptor %d 0 R >>", fontDescriptorID),
		"<< /Type /FontDescriptor /FontName /STSong-Light /Flags 4 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 700 /StemV 80 >>",
	)
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objects)+1, xref)
	return buf.Bytes()
}

// pdfContentStream 生成单页 PDF 文本流。
func pdfContentStream(lines []string) string {
	var stream strings.Builder
	stream.WriteString("BT\n/F1 12 Tf\n16 TL\n72 780 Td\n")
	for i, line := range lines {
		if i > 0 {
			stream.WriteString("T* ")
		}
		stream.WriteString(utf16BEHex(line))
		stream.WriteString(" Tj\n")
	}
	stream.WriteString("ET")
	content := stream.String()
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len([]byte(content)), content)
}

// wrapPDFLines 将文本切成 PDF 可显示的短行。
func wrapPDFLines(lines []string) []string {
	out := make([]string, 0)
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		runes := []rune(line)
		for len(runes) > 0 {
			n := 34
			if len(runes) < n {
				n = len(runes)
			}
			out = append(out, string(runes[:n]))
			runes = runes[n:]
		}
	}
	return out
}

// chunkPDFLines 按页拆分 PDF 文本行。
func chunkPDFLines(lines []string, pageSize int) [][]string {
	if len(lines) == 0 {
		return [][]string{{""}}
	}
	chunks := make([][]string, 0, (len(lines)+pageSize-1)/pageSize)
	for len(lines) > 0 {
		n := pageSize
		if len(lines) < n {
			n = len(lines)
		}
		chunks = append(chunks, lines[:n])
		lines = lines[n:]
	}
	return chunks
}

// utf16BEHex 将字符串编码为 PDF 可用的 UTF-16BE 十六进制文本。
func utf16BEHex(value string) string {
	encoded := utf16.Encode([]rune(value))
	var b strings.Builder
	b.WriteString("<")
	for _, item := range encoded {
		fmt.Fprintf(&b, "%04X", item)
	}
	b.WriteString(">")
	return b.String()
}

// splitParagraphs 按空行将正文切分为段落。
func splitParagraphs(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	blocks := strings.Split(content, "\n\n")
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block != "" {
			result = append(result, block)
		}
	}
	return result
}

// safeDownloadFilename 清理下载文件名中的非法字符。
func safeDownloadFilename(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`[\\/:*?"<>|]+`).ReplaceAllString(value, "_")
	if value == "" {
		return "download"
	}
	return value
}

// randomJobID 生成下载任务 ID。
func randomJobID() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
