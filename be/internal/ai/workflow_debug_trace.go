package ai

import "go.uber.org/zap"

// workflowDebugTrace 负责在 debug 模式下输出简洁的 workflow 执行轨迹。
type workflowDebugTrace struct {
	enabled bool
	name    string
	steps   int
}

// newWorkflowDebugTrace 根据全局日志级别创建 workflow 调试轨迹；非 debug 级别时所有方法都是空操作。
func newWorkflowDebugTrace(name string, steps int) workflowDebugTrace {
	return workflowDebugTrace{
		enabled: zap.L().Core().Enabled(zap.DebugLevel),
		name:    firstNonEmpty(name, "workflow"),
		steps:   steps,
	}
}

// compileStart 输出 workflow 编译开始信息。
func (t workflowDebugTrace) compileStart() {
	t.event("workflow compile", "compile_start", zap.Int("steps", t.steps))
}

// compileDone 输出 workflow 编译完成信息。
func (t workflowDebugTrace) compileDone() {
	t.event("workflow compile", "compile_done", zap.Int("steps", t.steps))
}

// runStart 输出 workflow 单次运行开始信息。
func (t workflowDebugTrace) runStart() {
	t.event("workflow trace", "run_start", zap.Int("steps", t.steps))
}

// runDone 输出 workflow 单次运行完成信息。
func (t workflowDebugTrace) runDone() {
	t.event("workflow trace", "run_done", zap.Int("steps", t.steps))
}

// stepStart 输出 workflow 单个步骤开始信息。
func (t workflowDebugTrace) stepStart(step string) {
	t.event("workflow trace", "step_start", zap.String("step", step))
}

// stepDone 输出 workflow 单个步骤完成信息。
func (t workflowDebugTrace) stepDone(step string) {
	t.event("workflow trace", "step_done", zap.String("step", step))
}

// stepError 输出 workflow 单个步骤错误信息。
func (t workflowDebugTrace) stepError(step string, err error) {
	if err == nil {
		return
	}
	t.event("workflow trace", "step_error", zap.String("step", step), zap.String("error", err.Error()))
}

// finalizeStart 输出 workflow 收尾阶段开始信息。
func (t workflowDebugTrace) finalizeStart() {
	t.event("workflow trace", "finalize_start")
}

// finalizeDone 输出 workflow 收尾阶段完成信息。
func (t workflowDebugTrace) finalizeDone() {
	t.event("workflow trace", "finalize_done")
}

// error 输出 workflow 通用阶段错误信息。
func (t workflowDebugTrace) error(stage string, err error) {
	if err == nil {
		return
	}
	t.event("workflow trace", "error", zap.String("stage", stage), zap.String("error", err.Error()))
}

// event 按统一字段输出一条 workflow 调试日志。
func (t workflowDebugTrace) event(message string, event string, fields ...zap.Field) {
	if !t.enabled {
		return
	}
	base := []zap.Field{
		zap.String("event", event),
		zap.String("workflow", t.name),
	}
	zap.L().Debug(message, append(base, fields...)...)
}
