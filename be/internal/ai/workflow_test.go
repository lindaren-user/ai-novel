package ai

import (
	"context"
	"errors"
	"testing"
)

type testWorkflowStep struct {
	name string
	run  func(context.Context, int) (int, error)
}

// Name 返回测试步骤名称。
func (s testWorkflowStep) Name() string {
	return s.name
}

// Run 执行测试步骤中注入的处理逻辑。
func (s testWorkflowStep) Run(ctx context.Context, state int) (int, error) {
	return s.run(ctx, state)
}

type testWorkflowSink struct {
	events []string
}

// Step 记录测试工作流发出的阶段事件。
func (s *testWorkflowSink) Step(_ context.Context, text string) {
	s.events = append(s.events, text)
}

// TestSequentialWorkflowRunInOrder 校验顺序工作流按步骤顺序执行并最终汇总输出。
func TestSequentialWorkflowRunInOrder(t *testing.T) {
	sink := &testWorkflowSink{}
	workflow, err := NewSequentialWorkflow(context.Background(), SequentialWorkflowConfig[int, int, int]{
		Init: func(input int) int { return input },
		Steps: []WorkflowStep[int]{
			testWorkflowStep{name: "第一步", run: func(_ context.Context, state int) (int, error) { return state + 1, nil }},
			testWorkflowStep{name: "第二步", run: func(_ context.Context, state int) (int, error) { return state * 3, nil }},
		},
		Finalize: func(_ context.Context, _ int, state int) (int, error) { return state + 2, nil },
	})
	if err != nil {
		t.Fatalf("NewSequentialWorkflow returned error: %v", err)
	}

	got, err := workflow.Run(context.Background(), 2, sink)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got != 11 {
		t.Fatalf("Run result = %d, want 11", got)
	}
	if len(sink.events) != 2 || sink.events[0] != "第一步" || sink.events[1] != "第二步" {
		t.Fatalf("events = %#v, want ordered step names", sink.events)
	}
}

// TestSequentialWorkflowStopsOnStepError 校验步骤失败时不会继续执行后续步骤。
func TestSequentialWorkflowStopsOnStepError(t *testing.T) {
	stepErr := errors.New("step failed")
	ranSecond := false
	workflow, err := NewSequentialWorkflow(context.Background(), SequentialWorkflowConfig[int, int, int]{
		Init: func(input int) int { return input },
		Steps: []WorkflowStep[int]{
			testWorkflowStep{name: "失败步骤", run: func(_ context.Context, state int) (int, error) { return state, stepErr }},
			testWorkflowStep{name: "不应执行", run: func(_ context.Context, state int) (int, error) {
				ranSecond = true
				return state, nil
			}},
		},
		Finalize: func(_ context.Context, _ int, state int) (int, error) { return state, nil },
	})
	if err != nil {
		t.Fatalf("NewSequentialWorkflow returned error: %v", err)
	}

	_, err = workflow.Run(context.Background(), 1, nil)
	if !errors.Is(err, stepErr) {
		t.Fatalf("Run error = %v, want %v", err, stepErr)
	}
	if ranSecond {
		t.Fatal("second step ran after previous step failed")
	}
}

// TestSequentialWorkflowStopsOnContextCancel 校验上下文取消后工作流停止并返回取消错误。
func TestSequentialWorkflowStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	workflow, err := NewSequentialWorkflow(context.Background(), SequentialWorkflowConfig[int, int, int]{
		Init: func(input int) int { return input },
		Steps: []WorkflowStep[int]{
			testWorkflowStep{name: "取消步骤", run: func(_ context.Context, state int) (int, error) {
				cancel()
				return state, nil
			}},
			testWorkflowStep{name: "不应执行", run: func(_ context.Context, state int) (int, error) { return state + 1, nil }},
		},
		Finalize: func(_ context.Context, _ int, state int) (int, error) { return state, nil },
	})
	if err != nil {
		t.Fatalf("NewSequentialWorkflow returned error: %v", err)
	}

	_, err = workflow.Run(ctx, 1, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}
