package fsm

import "github.com/coohu/goagent/internal/core"

type transitionKey struct {
	State core.AgentState
	Event core.EventType
}

var transitions = map[transitionKey]core.AgentState{
	{core.StateIdle, core.EventStart}:                core.StatePlanning,
	{core.StatePlanning, core.EventPlanCreated}:      core.StateExecuting,
	{core.StatePlanning, core.EventStepFailed}:       core.StateError,
	{core.StateExecuting, core.EventToolCall}:        core.StateWaitTool,
	{core.StateExecuting, core.EventLLMRequest}:      core.StateLLMThinking,
	{core.StateExecuting, core.EventStepFailed}:      core.StateError,
	{core.StateLLMThinking, core.EventLLMResponse}:   core.StateExecuting,
	{core.StateLLMThinking, core.EventLLMError}:      core.StateReflecting,
	{core.StateWaitTool, core.EventToolResult}:       core.StateProcessResult,
	{core.StateWaitTool, core.EventToolError}:        core.StateProcessResult,
	{core.StateWaitTool, core.EventTimeout}:          core.StateReflecting,
	{core.StateProcessResult, core.EventProcessed}:   core.StateUpdateMemory,
	{core.StateUpdateMemory, core.EventMemoryUpdated}: core.StateReflecting,
	{core.StateReflecting, core.EventStepDone}:       core.StateNextStep,
	{core.StateReflecting, core.EventReplanNeeded}:   core.StateReplanning,
	{core.StateNextStep, core.EventHasNextStep}:      core.StateBuildContext,
	{core.StateNextStep, core.EventAllDone}:          core.StateDone,
	{core.StateBuildContext, core.EventContextBuilt}: core.StateExecuting,
	{core.StateReplanning, core.EventPlanUpdated}:    core.StateBuildContext,
	{core.StateReplanning, core.EventStepFailed}:     core.StateError,
	{core.StateExecuting, core.EventApprovalNeeded}:  core.StateWaitUser,
	{core.StateWaitUser, core.EventApproved}:         core.StateExecuting,
}

var anyStateTransitions = map[core.EventType]core.AgentState{
	core.EventCancel:       core.StateCancelled,
	core.EventLoopDetected: core.StateError,
	core.EventCostExceeded: core.StateError,
	core.EventTimeout:      core.StateTimeout,
}

func nextState(current core.AgentState, event core.EventType) (core.AgentState, bool) {
	if next, ok := anyStateTransitions[event]; ok {
		return next, true
	}
	next, ok := transitions[transitionKey{current, event}]
	return next, ok
}
