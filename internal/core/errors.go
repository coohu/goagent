package core

import "fmt"

const (
	ErrInternal         = "AGT-1001"
	ErrTimeout          = "AGT-1002"
	ErrLoopDetected     = "AGT-1003"
	ErrPlanFailed       = "AGT-2001"
	ErrPlanTooLarge     = "AGT-2002"
	ErrReplanExhausted  = "AGT-2003"
	ErrToolNotFound     = "AGT-3001"
	ErrToolForbidden    = "AGT-3002"
	ErrToolTimeout      = "AGT-3003"
	ErrSandboxFailed    = "AGT-3004"
	ErrToolExhausted    = "AGT-3005"
	ErrLLMTimeout       = "AGT-4001"
	ErrLLMRateLimit     = "AGT-4002"
	ErrLLMBudget        = "AGT-4003"
	ErrLLMCallsLimit    = "AGT-4004"
	ErrMemoryStore      = "AGT-5001"
	ErrMemorySearch     = "AGT-5002"
	ErrSessionNotFound  = "AGT-6001"
	ErrSessionFull      = "AGT-6002"
	ErrSessionCancelled = "AGT-6003"
)

type AgentError struct {
	Code    string
	Message string
	Cause   error
}

func (e *AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AgentError) Unwrap() error { return e.Cause }

func Errorf(code, msg string, cause error) *AgentError {
	return &AgentError{Code: code, Message: msg, Cause: cause}
}
