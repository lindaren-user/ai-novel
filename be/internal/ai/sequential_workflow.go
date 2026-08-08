package ai

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/compose"
)

// Workflow 定义 AI 层顺序工作流，只负责编排输入、事件和输出。
type Workflow[I any, O any] interface {
	Run(ctx context.Context, input I, sink WorkflowEventSink) (O, error)
}

// WorkflowStep 定义工作流中的单个步骤，步骤内部负责处理当前状态并返回新状态。
type WorkflowStep[S any] interface {
	Name() string
	Run(ctx context.Context, state S) (S, error)
}

// WorkflowEventSink 接收工作流执行过程中的阶段事件。
type WorkflowEventSink interface {
	Step(ctx context.Context, text string)
}

// SequentialWorkflowConfig 定义顺序工作流的初始化、步骤和收尾逻辑。
type SequentialWorkflowConfig[I any, S any, O any] struct {
	Name     string
	Init     func(I) S
	Steps    []WorkflowStep[S]
	Finalize func(context.Context, I, S) (O, error)
}

// SequentialWorkflow 保存已编译的 Eino Chain，运行时只负责 Invoke。
type SequentialWorkflow[I any, O any] struct {
	runner compose.Runnable[sequentialWorkflowRunInput[I], O]
	trace  workflowDebugTrace
}

type sequentialWorkflowRunInput[I any] struct {
	input I
	sink  WorkflowEventSink
}

type sequentialWorkflowState[I any, S any] struct {
	input I
	sink  WorkflowEventSink
	state S
}

// NewSequentialWorkflow 构建并编译顺序工作流，后续 Run 复用同一个 Eino runner。
func NewSequentialWorkflow[I any, S any, O any](ctx context.Context, config SequentialWorkflowConfig[I, S, O]) (*SequentialWorkflow[I, O], error) {
	var zero O
	trace := newWorkflowDebugTrace(config.Name, len(config.Steps))
	if config.Init == nil {
		return nil, errors.New("workflow init is nil")
	}
	if config.Finalize == nil {
		return nil, errors.New("workflow finalize is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trace.compileStart()
	chain := compose.NewChain[sequentialWorkflowRunInput[I], O]()
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, input sequentialWorkflowRunInput[I]) (sequentialWorkflowState[I, S], error) {
		if err := ctx.Err(); err != nil {
			return sequentialWorkflowState[I, S]{}, err
		}
		return sequentialWorkflowState[I, S]{
			input: input.input,
			sink:  input.sink,
			state: config.Init(input.input),
		}, nil
	}))
	for _, step := range config.Steps {
		if step == nil {
			return nil, errors.New("workflow step is nil")
		}
		chain.AppendLambda(sequentialWorkflowStepLambda[I](trace, step))
	}
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, current sequentialWorkflowState[I, S]) (O, error) {
		trace.finalizeStart()
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		output, err := config.Finalize(ctx, current.input, current.state)
		if err != nil {
			trace.error("finalize", err)
			return zero, err
		}
		trace.finalizeDone()
		return output, nil
	}))
	runner, err := chain.Compile(ctx)
	if err != nil {
		trace.error("compile", err)
		return nil, err
	}
	trace.compileDone()
	return &SequentialWorkflow[I, O]{runner: runner, trace: trace}, nil
}

// sequentialWorkflowStepLambda 将单个业务步骤包装成 Eino Chain 可追加的 Lambda 节点。
func sequentialWorkflowStepLambda[I any, S any](trace workflowDebugTrace, step WorkflowStep[S]) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, current sequentialWorkflowState[I, S]) (sequentialWorkflowState[I, S], error) {
		trace.stepStart(step.Name())
		if current.sink != nil {
			current.sink.Step(ctx, step.Name())
		}
		if err := ctx.Err(); err != nil {
			return current, err
		}
		next, err := step.Run(ctx, current.state)
		if err != nil {
			if ctx.Err() == nil {
				trace.stepError(step.Name(), err)
			}
			return current, err
		}
		current.state = next
		if err := ctx.Err(); err != nil {
			return current, err
		}
		trace.stepDone(step.Name())
		return current, nil
	})
}

// Run 执行已编译的顺序工作流，任一步骤失败或上下文取消都会立即停止。
func (w *SequentialWorkflow[I, O]) Run(ctx context.Context, input I, sink WorkflowEventSink) (O, error) {
	w.trace.runStart()
	output, err := w.runner.Invoke(ctx, sequentialWorkflowRunInput[I]{
		input: input,
		sink:  sink,
	})
	if err != nil {
		if ctx.Err() == nil {
			w.trace.error("run", err)
		}
		return output, err
	}
	w.trace.runDone()
	return output, nil
}
