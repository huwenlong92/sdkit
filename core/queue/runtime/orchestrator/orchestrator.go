package orchestrator

import "github.com/huwenlong92/sdkit/core/queue"

type Orchestrator = queue.Orchestrator
type Option = queue.OrchestratorOption
type Execution = queue.RuntimeExecution

var New = queue.NewOrchestrator
var WithEventPublisher = queue.WithEventPublisher
var WithObserver = queue.WithObserver
