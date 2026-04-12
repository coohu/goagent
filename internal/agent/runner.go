package agent

import (
	"context"
	"fmt"
	"log/slog"
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
	ctx, cancel := context.WithTimeout(parentCtx, session.Config.MaxRuntime.Duration)
	defer cancel()

	session.SetState(core.StateIdle)
	queue := []core.Event{makeEvent(core.EventStart, nil)}
	slog.Info("agent started", "session", session.ID, "goal", session.Goal)
	for {
		select {
		case <-ctx.Done():
			slog.Info("agent timeout", "session", session.ID)
			session.SetState(core.StateTimeout)
			r.broadcast(session, core.EventTimeout, nil)
			return ctx.Err()
		default:
		}

		// Also drain any external events (cancel, approve, etc.) from the channel.
		drain:
		for {
			select {
			case extEv, ok := <-session.EventChan:
				if !ok {
					return nil
				}
				queue = append(queue, extEv)
			default:
				break drain
			}
		}

		if len(queue) == 0 {
			// Nothing to process — wait for an external event or timeout.
			select {
			case <-ctx.Done():
				slog.Info("agent timeout", "session", session.ID)
				session.SetState(core.StateTimeout)
				r.broadcast(session, core.EventTimeout, nil)
				return ctx.Err()
			case extEv, ok := <-session.EventChan:
				if !ok {
					return nil
				}
				queue = append(queue, extEv)
			}
		}
		ev := queue[0]
		queue = queue[1:]

		slog.Debug("processing event", "session", session.ID, "event", ev.Type, "state", session.GetState())
		if exceeded, reason := session.ExceedsLimits(); exceeded {
			slog.Info("agent limits exceeded", "session", session.ID, "reason", reason)
			session.SetState(core.StateError)
			r.broadcast(session, core.EventCostExceeded, map[string]any{"reason": reason})
			return fmt.Errorf("limits exceeded: %s", reason)
		}
		r.broadcast(session, ev.Type, ev.Payload)

		// Drive the FSM.
		_, newEvents, err := r.fsm.Transition(ctx, session, ev)
		if err != nil {
			slog.Error("fsm transition error", "session", session.ID, "err", err)
			session.SetState(core.StateError)
			return err
		}

		// Enqueue new events at the front so the state machine progresses immediately.
		queue = append(newEvents, queue...)
		st := session.GetState()
		slog.Debug("state after transition", "session", session.ID, "state", st)

		if st == core.StateDone || st == core.StateError ||
			st == core.StateCancelled || st == core.StateTimeout {
			slog.Info("agent finished", "session", session.ID, "state", st)
			return nil
		}
	}
}

func (r *Runner) broadcast(session *core.AgentSession, t core.EventType, payload map[string]any) {
	ev := makeEvent(t, payload)
	ev.SessionID = session.ID
	_ = r.bus.Emit(context.Background(), ev)
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
	slog.Info("planning", "session", session.ID, "model", session.Config.Models.Planning)
	plan, err := r.planner.CreatePlan(ctx, session.Goal, session.AgentCtx, &session.Config.Models)
	if err != nil {
		slog.Error("planning failed", "session", session.ID, "err", err)
		return []core.Event{makeEvent(core.EventStepFailed, map[string]any{"error": err.Error()})}, nil
	}
	session.Plan = plan
	session.AgentCtx.Plan = plan
	slog.Info("plan created", "session", session.ID, "steps", len(plan.Steps))
	return []core.Event{makeEvent(core.EventPlanCreated, nil)}, nil
}

func (r *Runner) handleExecuting(ctx context.Context, session *core.AgentSession, _ core.Event) ([]core.Event, error) {
	if session.Plan == nil || session.Plan.CurrentStep >= len(session.Plan.Steps) {
		return []core.Event{makeEvent(core.EventAllDone, nil)}, nil
	}

	step := &session.Plan.Steps[session.Plan.CurrentStep]
	step.Status = core.StepRunning
	session.AgentCtx.Scratchpad = &core.Scratchpad{MaxTokens: session.Config.ScratchpadMaxTokens}

	slog.Info("executing step", "session", session.ID, "step", step.Name, "tool", step.Tool)

	tc, err := r.executor.ExecuteStep(ctx, session)
	if err != nil {
		slog.Error("executor failed", "session", session.ID, "err", err)
		return []core.Event{makeEvent(core.EventStepFailed, map[string]any{"error": err.Error()})}, nil
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

	slog.Info("calling tool", "session", session.ID, "tool", toolName)

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

	slog.Info("reflection", "session", session.ID, "action", rc.Result.Action, "reason", rc.Result.Reason)

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

	newPlan, err := r.planner.Replan(ctx, session.Plan, reason, session.AgentCtx, &session.Config.Models)
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