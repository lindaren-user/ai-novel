package service

import (
	"context"
	"sync"

	"ai-novel-ide/be/internal/ai"
)

type streamHub struct {
	mu           sync.Mutex
	wg           sync.WaitGroup       // 可以认为是正在进行的任务的总数
	jobs         map[int64]*streamJob // key: sessionID
	shuttingDown bool
}

type streamCancelReason uint8

const (
	streamCancelNone streamCancelReason = iota
	streamCancelUser
	streamCancelServerShutdown
)

type streamJob struct {
	userID       int64
	cancel       context.CancelFunc
	cancelReason streamCancelReason
	subscriber   chan ai.StreamEvent
}

// newStreamHub 创建内存流式任务中心，每个会话只保留一个当前 SSE 订阅者。
func newStreamHub() *streamHub {
	return &streamHub{jobs: make(map[int64]*streamJob)}
}

// startJob 注册一个新的后台 AI 任务，并按用户会员权益限制并发数。
func (h *streamHub) startJob(sessionID int64, userID int64, maxConcurrent int) (context.Context, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.shuttingDown {
		return nil, false, ErrServiceShuttingDown
	}
	if _, exists := h.jobs[sessionID]; exists {
		return nil, false, nil
	}
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrentStreams
	}
	count := 0
	for _, job := range h.jobs {
		if job.userID == userID {
			count++
		}
	}
	if count >= maxConcurrent {
		return nil, false, ErrConcurrentStreamLimitExceeded
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	h.jobs[sessionID] = &streamJob{userID: userID, cancel: cancel}
	h.wg.Add(1)
	return jobCtx, true, nil
}

// beginShutdown 立即禁止新任务，但让已有任务继续运行到统一取消阶段。
func (h *streamHub) beginShutdown() {
	h.mu.Lock()
	h.shuttingDown = true
	h.mu.Unlock()
}

// subscribe 订阅指定会话的后台 AI 任务；新订阅会替换旧订阅。
func (h *streamHub) subscribe(ctx context.Context, sessionID int64) (<-chan ai.StreamEvent, bool) {
	h.mu.Lock()
	job, ok := h.jobs[sessionID]
	if !ok {
		h.mu.Unlock()
		return nil, false
	}
	ch := make(chan ai.StreamEvent, 64)
	if job.subscriber != nil {
		close(job.subscriber)
	}
	job.subscriber = ch
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.removeSubscriber(sessionID, ch)
	}()

	return ch, true
}

// removeSubscriber 移除当前断开的 SSE 订阅者。
func (h *streamHub) removeSubscriber(sessionID int64, ch chan ai.StreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	job, ok := h.jobs[sessionID]
	if !ok {
		return
	}
	if job.subscriber == ch {
		job.subscriber = nil
		close(ch)
	}
}

// push 向当前订阅者推送事件，慢订阅者不会阻塞后台 AI 任务。
func (h *streamHub) push(sessionID int64, event ai.StreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	job, ok := h.jobs[sessionID]
	if !ok {
		return
	}
	if job.subscriber != nil {
		select {
		case job.subscriber <- event:
		default:
		}
	}
}

// cancelJob 标记并取消指定会话的后台 AI 任务。
func (h *streamHub) cancelJob(sessionID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	job, ok := h.jobs[sessionID]
	if !ok {
		return false
	}
	if job.cancelReason == streamCancelNone {
		job.cancelReason = streamCancelUser
	}
	if job.cancel != nil {
		job.cancel()
	}
	return true
}

// cancelReason 返回指定任务的取消原因。
func (h *streamHub) cancelReason(sessionID int64) streamCancelReason {
	h.mu.Lock()
	defer h.mu.Unlock()
	job, ok := h.jobs[sessionID]
	if !ok {
		return streamCancelNone
	}
	return job.cancelReason
}

// finishJob 结束后台任务并关闭当前订阅者。
func (h *streamHub) finishJob(sessionID int64) {
	h.mu.Lock()
	job, ok := h.jobs[sessionID]
	if !ok {
		h.mu.Unlock()
		return
	}
	if job.subscriber != nil {
		close(job.subscriber)
	}
	delete(h.jobs, sessionID)
	h.mu.Unlock()
	h.wg.Done()
}

// Shutdown 标记任务中心关闭，取消所有后台任务，并等待任务完成。
func (h *streamHub) Shutdown(ctx context.Context) error {
	h.beginShutdown()
	h.mu.Lock()
	for _, job := range h.jobs {
		job.cancelReason = streamCancelServerShutdown
		if job.cancel != nil {
			job.cancel()
		}
	}
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done(): // 超时取消，兜底机制，防止长时间阻塞
		return ctx.Err()
	case <-done:
		return nil
	}
}
