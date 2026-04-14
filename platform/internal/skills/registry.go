package skills

import (
	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/tools"
)

// NewBridgeRegistry creates a tool registry with all Bridge-specific skills registered.
// Pass nil for executor or backend if they're not available yet.
func NewBridgeRegistry(executor CodeExecutor, backend llm.Backend) *tools.Registry {
	reg := tools.NewRegistry()

	reg.Register(NewTutor())
	reg.Register(NewCodeAnalyzer())

	if executor != nil {
		reg.Register(NewCodeRunner(executor))
	}
	if backend != nil {
		reg.Register(NewReportGenerator(backend))
		reg.Register(NewLessonGenerator(backend))
	}

	return reg
}
