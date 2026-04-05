package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/eventbus"
	"github.com/coohu/goagent/internal/executor"
	"github.com/coohu/goagent/internal/fsm"
	"github.com/coohu/goagent/internal/pipeline"
	"github.com/coohu/goagent/internal/planner"
)

type Runner struct {
	fsm      *fsm.Engine
	bus      *eventbus.Bus
	planner  *planner.Planner
	executor *executor.Executor
	memory   core.MemoryManager
	toolRun  ToolRunner
}

type ToolRunner interface {
	Run(ctx context.Context, call *core.ToolCall, reg core.ToolRegistry) (*core.ToolResult, error)
}

func NewRunner(
	engine *fsm.Engine,
	bus *eventbus.Bus,
	pl *planner.Planner,
	ex *executor.Executor,
	mem core.MemoryManager,
	toolRunner ToolRunner,
) *Runner {
	r := &Runner{fsm: engine, bus: bus, planner: pl, executor: ex, memory: mem, toolRun: toolRunner}
	r.registerHandlers()
	return r
}

func (r *Runner) Run(parentCtx context.Context, session *core.AgentSession) error {
	ctx, cancel := context.WithTimeout(parentCtx, session.Config.MaxRuntime)
	defer cancel()

	session.SetState(core.StateIdle)

	unsub := r.bus.SubscribeSession(session.ID, func(_ context.Context, ev core.Event) {
		select {
		case session.EventChan <- ev:
		default:
		}
	})
	defer unsub()

	r.emit(ctx, session, core.EventStart, nil)

	for {
		select {
		case <-ctx.Done():
			r.emit(ctx, session, core.EventTimeout, nil)
			session.SetState(core.StateTimeout)
			return ctx.Err()

		case ev, ok := <-session.EventChan:
			if !ok {
				return nil
			}
			if exceeded, reason := session.ExceedsLimits(); exceeded {
				r.emit(ctx, session, core.EventCostExceeded, map[string]any{"reason": reason})
				session.SetState(core.StateError)
				return fmt.Errorf("session limits exceeded: %s", reason)
			}

			_, newEvents, err := r.fsm.Transition(ctx, session, ev)
			if err != nil {
				session.SetState(core.StateError)
				return err
			}
			for _, ne := range newEvents {
				ne.SessionID = session.ID
				r.bus.Emit(ctx, ne)
			}

			st := session.GetState()
			if st == core.StateDone || st == core.StateError ||
				st == core.StateCancelled || st == core.StateTimeout {
				return nil
			}
		}
	}
}

func (r *Runner) registerHandlers() {
	r.fsm.RegisterHandler(core.StatePlanning, r.handlePlanning)
	r.fsm.RegisterHandler(core.StateExecuting, r.handleExecuting)
	r.fsm.RegisterHandler(core.StateWaitTool, r.handleWaitTool)
	r.fsm.RegisterHandler(core.StateProcessResult, r.handleProcessResult)
	r.fsm.RegisterHandler(core.StateUpdateMemory, r.handleUpdateMemory)
	r.fsm.RegisterHandler(core.StateReflecting, r.handleReflecting)
	r.fsm.RegisterHandler(core.StateReplanning, r.handleReplanning)
	r.fsm.RegisterHandler(core.StateNextStep, r.handleNextStep)
	r.fsm.RegisterHandler(core.StateBuildContext, r.handleBuildContext)
	r.fsm.RegisterHandler(core.StateDone, r.handleDone)
}

func (r *Runner) handlePlanning(ctx context.Context, session *core.AgentSession, _ core.Event) ([]core.Event, error) {
	plan, err := r.planner.CreatePlan(ctx, session.Goal, session.AgentCtx)
	if err != nil {
		return []core.Event{makeEvent(core.EventStepFailed, map[string]any{"error": err.Error()})}, nil
	}
	session.Plan = plan
	session.AgentCtx.Plan = plan
	return []core.Event{makeEvent(core.EventPlanCreated, nil)}, nil
}

func (r *Runner) handleExecuting(ctx context.Context, session *core.AgentSession, _ core.Event) ([]core.Event, error) {
	if session.Plan == nil || session.Plan.CurrentStep >= len(session.Plan.Steps) {
		return []core.Event{makeEvent(core.EventAllDone, nil)}, nil
	}

	step := &session.Plan.Steps[session.Plan.CurrentStep]
	step.Status = core.StepRunning
	session.AgentCtx.Scratchpad = &core.Scratchpad{MaxTokens: session.Config.ScratchpadMaxTokens}

	tc, err := r.executor.ExecuteStep(ctx, session)
	if err != nil {
		return []core.Event{makeEvent(core.EventToolError, map[string]any{"error": err.Error()})}, nil
	}

	return []core.Event{makeEvent(core.EventToolCall, map[string]any{
		"tool_name": tc.ToolName,
		"input":     tc.Input,
		"step_id":   tc.StepID,
	})}, nil
}

func (r *Runner) handleWaitTool(ctx context.Context, session *core.AgentSession, ev core.Event) ([]core.Event, error) {
	toolName, _ := ev.Payload["tool_name"].(string)
	input, _ := ev.Payload["input"].(map[string]any)
	stepID, _ := ev.Payload["step_id"].(int)

	session.IncrTool()

	tc := &core.ToolCall{ToolName: toolName, Input: input, SessionID: session.ID, StepID: stepID}
	result, err := r.toolRun.Run(ctx, tc, nil)

	if err != nil || (result != nil && !result.Success) {
		payload := map[string]any{"tool_name": toolName, "step_id": stepID}
		if result != nil {
			payload["result"] = result
			payload["stderr"] = result.Stderr
		}
		if err != nil {
			payload["error"] = err.Error()
		}
		session.Plan.Steps[session.Plan.CurrentStep].RetryCount++
		return []core.Event{makeEvent(core.EventToolError, payload)}, nil
	}

	return []core.Event{makeEvent(core.EventToolResult, map[string]any{
		"tool_name": toolName,
		"result":    result,
		"step_id":   stepID,
	})}, nil
}

func (r *Runner) handleProcessResult(_ context.Context, session *core.AgentSession, ev core.Event) ([]core.Event, error) {
	if result, ok := ev.Payload["result"].(*core.ToolResult); ok {
		session.AgentCtx.RecentToolResults = append(session.AgentCtx.RecentToolResults, &core.ToolMemory{
			ToolName:  fmt.Sprintf("%v", ev.Payload["tool_name"]),
			Summary:   truncateStr(result.RawOutput, 500),
			SessionID: session.ID,
		})
	}
	return []core.Event{makeEvent(core.EventProcessed, ev.Payload)}, nil
}

func (r *Runner) handleUpdateMemory(ctx context.Context, session *core.AgentSession, ev core.Event) ([]core.Event, error) {
	if result, ok := ev.Payload["result"].(*core.ToolResult); ok && result != nil {
		_ = r.memory.SaveToolMemory(ctx, &core.ToolMemory{
			ID:        fmt.Sprintf("%s-step%d", session.ID, session.Plan.CurrentStep),
			SessionID: session.ID,
			StepID:    session.Plan.CurrentStep,
			ToolName:  fmt.Sprintf("%v", ev.Payload["tool_name"]),
			RawOutput: truncateStr(result.RawOutput, 5000),
			Summary:   truncateStr(result.Stdout, 500),
		})
	}
	return []core.Event{makeEvent(core.EventMemoryUpdated, nil)}, nil
}

func (r *Runner) handleReflecting(_ context.Context, session *core.AgentSession, ev core.Event) ([]core.Event, error) {
	rc := &pipeline.ReflectionCtx{
		AgentContext: *session.AgentCtx,
		StepName:     currentStepName(session),
		ToolName:     fmt.Sprintf("%v", ev.Payload["tool_name"]),
		RetryCount:   currentStep(session).RetryCount,
		ReplanCount:  session.Metrics.ReplanCount,
	}
	if result, ok := ev.Payload["result"].(*core.ToolResult); ok {
		rc.ToolResult = result
	}

	p := pipeline.New(
		pipeline.EvaluateToolResultMiddleware(),
		pipeline.GuardLimitsMiddleware(),
	)
	_ = p.Run(context.Background(), rc)

	if rc.Result == nil {
		rc.Result = &pipeline.ReflectionResult{Success: true, Action: "continue"}
	}

	switch rc.Result.Action {
	case "replan":
		return []core.Event{makeEvent(core.EventReplanNeeded, map[string]any{"reason": rc.Result.Reason})}, nil
	case "abort":
		return []core.Event{makeEvent(core.EventStepFailed, map[string]any{"reason": rc.Result.Reason})}, nil
	default:
		session.Plan.Steps[session.Plan.CurrentStep].Status = core.StepDone
		return []core.Event{makeEvent(core.EventStepDone, nil)}, nil
	}
}

func (r *Runner) handleReplanning(ctx context.Context, session *core.AgentSession, ev core.Event) ([]core.Event, error) {
	reason, _ := ev.Payload["reason"].(string)
	session.IncrReplan()

	newPlan, err := r.planner.Replan(ctx, session.Plan, reason, session.AgentCtx)
	if err != nil {
		return []core.Event{makeEvent(core.EventStepFailed, map[string]any{"error": err.Error()})}, nil
	}
	session.Plan = newPlan
	session.AgentCtx.Plan = newPlan
	return []core.Event{makeEvent(core.EventPlanUpdated, nil)}, nil
}

func (r *Runner) handleNextStep(_ context.Context, session *core.AgentSession, _ core.Event) ([]core.Event, error) {
	session.Plan.CurrentStep++
	session.Metrics.StepCount++
	if session.Plan.CurrentStep >= len(session.Plan.Steps) {
		return []core.Event{makeEvent(core.EventAllDone, nil)}, nil
	}
	return []core.Event{makeEvent(core.EventHasNextStep, nil)}, nil
}

func (r *Runner) handleBuildContext(_ context.Context, _ *core.AgentSession, _ core.Event) ([]core.Event, error) {
	return []core.Event{makeEvent(core.EventContextBuilt, nil)}, nil
}

func (r *Runner) handleDone(ctx context.Context, session *core.AgentSession, _ core.Event) ([]core.Event, error) {
	now := time.Now()
	session.FinishedAt = &now
	_ = r.memory.SaveEpisode(ctx, &core.Episode{
		ID:      session.ID,
		Goal:    session.Goal,
		Success: true,
		Summary: fmt.Sprintf("Completed %d steps in %s", session.Metrics.StepCount, time.Since(session.Metrics.StartTime).Round(time.Second)),
	})
	return nil, nil
}

func (r *Runner) emit(ctx context.Context, session *core.AgentSession, t core.EventType, payload map[string]any) {
	ev := makeEvent(t, payload)
	ev.SessionID = session.ID
	_ = r.bus.Emit(ctx, ev)
}

func makeEvent(t core.EventType, payload map[string]any) core.Event {
	if payload == nil {
		payload = map[string]any{}
	}
	return core.Event{Type: t, Payload: payload, CreatedAt: time.Now()}
}

func currentStep(session *core.AgentSession) *core.Step {
	if session.Plan == nil || session.Plan.CurrentStep >= len(session.Plan.Steps) {
		return &core.Step{}
	}
	return &session.Plan.Steps[session.Plan.CurrentStep]
}

func currentStepName(session *core.AgentSession) string {
	return currentStep(session).Name
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
