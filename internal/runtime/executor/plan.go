package executor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	runtimeexecutionpool "github.com/tiger/realtime-speech-pipeline/internal/runtime/executionpool"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/lanes"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/nodehost"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

// NodeSpec defines one deterministic runtime execution node.
type NodeSpec struct {
	NodeID           string
	NodeType         string
	Lane             eventabi.Lane
	Provider         *ProviderInvocationInput
	Shed             bool
	Reason           string
	AllowDegrade     bool
	AllowFallback    bool
	ConcurrencyLimit int
	FairnessKey      string
}

// EdgeSpec defines one directed edge between execution nodes.
type EdgeSpec struct {
	From string
	To   string
}

// ExecutionPlan defines a runtime execution graph for deterministic dispatch.
type ExecutionPlan struct {
	Nodes []NodeSpec
	Edges []EdgeSpec
}

// NodeExecutionResult captures one dispatched node outcome.
type NodeExecutionResult struct {
	NodeID         string
	DispatchTarget lanes.DispatchTarget
	Decision       SchedulingDecision
	Failure        *nodehost.NodeFailureResult
}

// ExecutionTrace summarizes deterministic plan execution.
type ExecutionTrace struct {
	NodeOrder      []string
	Nodes          []NodeExecutionResult
	ControlSignals []eventabi.ControlSignal
	Completed      bool
	TerminalReason string
}

// ExecutePlan runs a deterministic execution plan in topological order.
func (s Scheduler) ExecutePlan(in SchedulingInput, plan ExecutionPlan) (ExecutionTrace, error) {
	nodeByID, err := plan.validate()
	if err != nil {
		return ExecutionTrace{}, err
	}

	order, err := topologicalOrder(plan, nodeByID)
	if err != nil {
		return ExecutionTrace{}, err
	}

	if in.ResolvedTurnPlan != nil {
		return s.executePlanWithLaneScheduler(in, plan, nodeByID)
	}
	return s.executePlanInTopologicalOrder(in, nodeByID, order)
}

func (s Scheduler) executePlanInTopologicalOrder(
	in SchedulingInput,
	nodeByID map[string]NodeSpec,
	order []string,
) (ExecutionTrace, error) {
	router := s.router
	if router == nil {
		defaultRouter := lanes.NewDefaultRouter()
		router = defaultRouter
	}

	trace := ExecutionTrace{
		NodeOrder:      append([]string(nil), order...),
		Nodes:          make([]NodeExecutionResult, 0, len(order)),
		ControlSignals: make([]eventabi.ControlSignal, 0),
		Completed:      true,
	}

	baseEventID := in.EventID
	if baseEventID == "" {
		baseEventID = "evt-execution-plan"
	}

	for idx, nodeID := range order {
		node := nodeByID[nodeID]
		dispatchTarget, err := router.Resolve(node.NodeType, node.Lane)
		if err != nil {
			return ExecutionTrace{}, err
		}

		offset := int64(idx)
		nodeInput := in
		nodeInput.EventID = fmt.Sprintf("%s-%s", baseEventID, node.NodeID)
		nodeInput.NodeID = node.NodeID
		nodeInput.NodeType = node.NodeType
		nodeInput.EdgeID = fmt.Sprintf("lane/%s", node.Lane)
		nodeInput.Shed = node.Shed
		nodeInput.Reason = node.Reason
		nodeInput.TransportSequence = nonNegative(in.TransportSequence) + offset
		nodeInput.RuntimeSequence = nonNegative(in.RuntimeSequence) + offset
		nodeInput.AuthorityEpoch = nonNegative(in.AuthorityEpoch)
		nodeInput.RuntimeTimestampMS = nonNegative(in.RuntimeTimestampMS) + offset
		nodeInput.WallClockTimestampMS = nonNegative(in.WallClockTimestampMS) + offset
		nodeInput.ProviderInvocation = node.Provider

		decision, err := s.dispatchNode(node, nodeInput)
		if err != nil {
			return ExecutionTrace{}, err
		}
		trace.Nodes = append(trace.Nodes, NodeExecutionResult{
			NodeID:         node.NodeID,
			DispatchTarget: dispatchTarget,
			Decision:       decision,
		})

		if decision.ControlSignal != nil {
			trace.ControlSignals = append(trace.ControlSignals, *decision.ControlSignal)
		}
		if decision.Provider != nil && len(decision.Provider.Signals) > 0 {
			trace.ControlSignals = append(trace.ControlSignals, decision.Provider.Signals...)
		}

		allowContinue := decision.Allowed
		if shouldShapeNodeFailure(decision) {
			failureResult, err := nodehost.HandleFailure(nodehost.NodeFailureInput{
				SessionID:            nodeInput.SessionID,
				TurnID:               nodeInput.TurnID,
				PipelineVersion:      defaultPipelineVersion(nodeInput.PipelineVersion),
				EventID:              nodeInput.EventID + "-node-failure",
				TransportSequence:    nodeInput.TransportSequence,
				RuntimeSequence:      nodeInput.RuntimeSequence,
				AuthorityEpoch:       nodeInput.AuthorityEpoch,
				RuntimeTimestampMS:   nodeInput.RuntimeTimestampMS,
				WallClockTimestampMS: nodeInput.WallClockTimestampMS,
				AllowDegrade:         node.AllowDegrade,
				AllowFallback:        node.AllowFallback,
			})
			if err != nil {
				return ExecutionTrace{}, err
			}
			trace.ControlSignals = append(trace.ControlSignals, failureResult.Signals...)
			last := len(trace.Nodes) - 1
			trace.Nodes[last].Failure = &failureResult
			allowContinue = !failureResult.Terminal
			if failureResult.Terminal && trace.TerminalReason == "" {
				trace.TerminalReason = failureResult.TerminalReason
			}
		}

		if !allowContinue {
			trace.Completed = false
			if trace.TerminalReason == "" {
				trace.TerminalReason = terminalReasonFromDeniedDecision(decision)
			}
			break
		}
	}

	trace.ControlSignals = stabilizeControlSignals(trace.ControlSignals)
	normalizedSignals, err := runtimeeventabi.ValidateAndNormalizeControlSignals(trace.ControlSignals)
	if err != nil {
		return ExecutionTrace{}, err
	}
	trace.ControlSignals = normalizedSignals
	return trace, nil
}

func (s Scheduler) executePlanWithLaneScheduler(
	in SchedulingInput,
	plan ExecutionPlan,
	nodeByID map[string]NodeSpec,
) (ExecutionTrace, error) {
	router := s.router
	if router == nil {
		defaultRouter := lanes.NewDefaultRouter()
		router = defaultRouter
	}

	planCfg, err := lanes.ConfigFromResolvedTurnPlan(in.SessionID, in.TurnID, *in.ResolvedTurnPlan)
	if err != nil {
		return ExecutionTrace{}, err
	}
	laneScheduler, err := lanes.NewPriorityScheduler(s.admission, planCfg)
	if err != nil {
		return ExecutionTrace{}, err
	}

	adjacency, indegree := buildExecutionGraph(plan)
	ready := make([]string, 0, len(nodeByID))
	for _, node := range plan.Nodes {
		if indegree[node.NodeID] == 0 {
			ready = append(ready, node.NodeID)
		}
	}

	trace := ExecutionTrace{
		NodeOrder:      make([]string, 0, len(nodeByID)),
		Nodes:          make([]NodeExecutionResult, 0, len(nodeByID)),
		ControlSignals: make([]eventabi.ControlSignal, 0),
		Completed:      true,
	}

	baseEventID := in.EventID
	if baseEventID == "" {
		baseEventID = "evt-execution-plan"
	}

	queued := make(map[string]struct{}, len(nodeByID))
	processed := make(map[string]struct{}, len(nodeByID))
	enqueueOffset := int64(0)

	for len(processed) < len(nodeByID) {
		for _, nodeID := range ready {
			if _, done := processed[nodeID]; done {
				continue
			}
			if _, alreadyQueued := queued[nodeID]; alreadyQueued {
				continue
			}

			node := nodeByID[nodeID]
			enqueueEventID := fmt.Sprintf("%s-%s-enqueue", baseEventID, nodeID)
			enqueueResult, err := laneScheduler.Enqueue(lanes.EnqueueInput{
				ItemID:               nodeID,
				Lane:                 node.Lane,
				EventID:              enqueueEventID,
				TransportSequence:    nonNegative(in.TransportSequence) + enqueueOffset,
				RuntimeSequence:      nonNegative(in.RuntimeSequence) + enqueueOffset,
				RuntimeTimestampMS:   nonNegative(in.RuntimeTimestampMS) + enqueueOffset,
				WallClockTimestampMS: nonNegative(in.WallClockTimestampMS) + enqueueOffset,
				PayloadClass:         eventabi.PayloadMetadata,
			})
			if err != nil {
				return ExecutionTrace{}, err
			}
			enqueueOffset++

			trace.ControlSignals = append(trace.ControlSignals, enqueueResult.Signals...)
			if enqueueResult.Accepted {
				queued[nodeID] = struct{}{}
				continue
			}

			dispatchTarget, err := router.Resolve(node.NodeType, node.Lane)
			if err != nil {
				return ExecutionTrace{}, err
			}
			droppedDecision := decisionFromLaneOverflow(node.Lane, enqueueResult)
			trace.NodeOrder = append(trace.NodeOrder, nodeID)
			trace.Nodes = append(trace.Nodes, NodeExecutionResult{
				NodeID:         nodeID,
				DispatchTarget: dispatchTarget,
				Decision:       droppedDecision,
			})
			processed[nodeID] = struct{}{}
			for _, next := range adjacency[nodeID] {
				indegree[next]--
				if indegree[next] == 0 {
					ready = append(ready, next)
				}
			}

			if droppedDecision.Allowed {
				continue
			}
			trace.Completed = false
			trace.TerminalReason = terminalReasonFromDroppedDecision(droppedDecision)
			trace.ControlSignals = stabilizeControlSignals(trace.ControlSignals)
			trace.ControlSignals, err = runtimeeventabi.ValidateAndNormalizeControlSignals(trace.ControlSignals)
			if err != nil {
				return ExecutionTrace{}, err
			}
			return trace, nil
		}

		dispatch, ok, err := laneScheduler.NextDispatch()
		if err != nil {
			return ExecutionTrace{}, err
		}
		if !ok {
			return ExecutionTrace{}, fmt.Errorf("execution plan scheduling deadlock")
		}

		node, ok := nodeByID[dispatch.Item.ItemID]
		if !ok {
			return ExecutionTrace{}, fmt.Errorf("lane dispatch referenced unknown node_id: %s", dispatch.Item.ItemID)
		}
		delete(queued, dispatch.Item.ItemID)
		processed[dispatch.Item.ItemID] = struct{}{}
		trace.ControlSignals = append(trace.ControlSignals, dispatch.Signals...)
		trace.NodeOrder = append(trace.NodeOrder, dispatch.Item.ItemID)

		dispatchTarget, err := router.Resolve(node.NodeType, node.Lane)
		if err != nil {
			return ExecutionTrace{}, err
		}

		nodeInput := in
		nodeInput.EventID = fmt.Sprintf("%s-%s", baseEventID, node.NodeID)
		nodeInput.NodeID = node.NodeID
		nodeInput.NodeType = node.NodeType
		nodeInput.EdgeID = fmt.Sprintf("lane/%s", node.Lane)
		nodeInput.Shed = node.Shed
		nodeInput.Reason = node.Reason
		nodeInput.TransportSequence = dispatch.Item.TransportSequence
		nodeInput.RuntimeSequence = dispatch.Item.RuntimeSequence
		nodeInput.AuthorityEpoch = nonNegative(in.AuthorityEpoch)
		nodeInput.RuntimeTimestampMS = dispatch.Item.RuntimeTimestampMS
		nodeInput.WallClockTimestampMS = dispatch.Item.WallClockTimestampMS
		nodeInput.ProviderInvocation = node.Provider

		decision, err := s.dispatchNode(node, nodeInput)
		if err != nil {
			return ExecutionTrace{}, err
		}
		trace.Nodes = append(trace.Nodes, NodeExecutionResult{
			NodeID:         node.NodeID,
			DispatchTarget: dispatchTarget,
			Decision:       decision,
		})

		if decision.ControlSignal != nil {
			trace.ControlSignals = append(trace.ControlSignals, *decision.ControlSignal)
		}
		if decision.Provider != nil && len(decision.Provider.Signals) > 0 {
			trace.ControlSignals = append(trace.ControlSignals, decision.Provider.Signals...)
		}

		allowContinue := decision.Allowed
		if shouldShapeNodeFailure(decision) {
			failureResult, err := nodehost.HandleFailure(nodehost.NodeFailureInput{
				SessionID:            nodeInput.SessionID,
				TurnID:               nodeInput.TurnID,
				PipelineVersion:      defaultPipelineVersion(nodeInput.PipelineVersion),
				EventID:              nodeInput.EventID + "-node-failure",
				TransportSequence:    nodeInput.TransportSequence,
				RuntimeSequence:      nodeInput.RuntimeSequence,
				AuthorityEpoch:       nodeInput.AuthorityEpoch,
				RuntimeTimestampMS:   nodeInput.RuntimeTimestampMS,
				WallClockTimestampMS: nodeInput.WallClockTimestampMS,
				AllowDegrade:         node.AllowDegrade,
				AllowFallback:        node.AllowFallback,
			})
			if err != nil {
				return ExecutionTrace{}, err
			}
			trace.ControlSignals = append(trace.ControlSignals, failureResult.Signals...)
			last := len(trace.Nodes) - 1
			trace.Nodes[last].Failure = &failureResult
			allowContinue = !failureResult.Terminal
			if failureResult.Terminal && trace.TerminalReason == "" {
				trace.TerminalReason = failureResult.TerminalReason
			}
		}

		if !allowContinue {
			trace.Completed = false
			if trace.TerminalReason == "" {
				trace.TerminalReason = terminalReasonFromDeniedDecision(decision)
			}
			break
		}

		for _, next := range adjacency[node.NodeID] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	trace.ControlSignals = stabilizeControlSignals(trace.ControlSignals)
	trace.ControlSignals, err = runtimeeventabi.ValidateAndNormalizeControlSignals(trace.ControlSignals)
	if err != nil {
		return ExecutionTrace{}, err
	}
	return trace, nil
}

func shouldShapeNodeFailure(decision SchedulingDecision) bool {
	if decision.Provider == nil {
		return false
	}
	if decision.Provider.OutcomeClass == contracts.OutcomeSuccess || decision.Provider.OutcomeClass == contracts.OutcomeCancelled {
		return false
	}
	return true
}

func (s Scheduler) dispatchNode(node NodeSpec, in SchedulingInput) (SchedulingDecision, error) {
	node = applyNodeExecutionPolicy(node, in)

	if s.executionPool == nil {
		return s.NodeDispatch(in)
	}
	resultCh := make(chan struct {
		decision SchedulingDecision
		err      error
	}, 1)
	if err := s.executionPool.Submit(runtimeexecutionpool.Task{
		ID:             node.NodeID,
		FairnessKey:    node.FairnessKey,
		MaxOutstanding: node.ConcurrencyLimit,
		Run: func() error {
			decision, dispatchErr := s.NodeDispatch(in)
			resultCh <- struct {
				decision SchedulingDecision
				err      error
			}{
				decision: decision,
				err:      dispatchErr,
			}
			return dispatchErr
		},
	}); err != nil {
		if errors.Is(err, runtimeexecutionpool.ErrQueueFull) ||
			errors.Is(err, runtimeexecutionpool.ErrClosed) ||
			errors.Is(err, runtimeexecutionpool.ErrNodeConcurrencyExceeded) {
			overloadInput := in
			overloadInput.Shed = true
			if overloadInput.Reason == "" {
				if errors.Is(err, runtimeexecutionpool.ErrQueueFull) {
					overloadInput.Reason = "execution_pool_saturated"
				} else if errors.Is(err, runtimeexecutionpool.ErrNodeConcurrencyExceeded) {
					overloadInput.Reason = "node_concurrency_limited"
				} else {
					overloadInput.Reason = "execution_pool_closed"
				}
			}
			decision, overloadErr := s.NodeDispatch(overloadInput)
			if overloadErr != nil {
				return SchedulingDecision{}, overloadErr
			}
			// Telemetry overload remains best-effort; shed is reported but execution continues.
			if node.Lane == eventabi.LaneTelemetry {
				decision.Allowed = true
				decision.Outcome = nil
			}
			return decision, nil
		}
		return SchedulingDecision{}, err
	}
	result := <-resultCh
	return result.decision, result.err
}

func applyNodeExecutionPolicy(node NodeSpec, in SchedulingInput) NodeSpec {
	plan := in.ResolvedTurnPlan
	if plan == nil {
		return node
	}

	if policy, ok := plan.NodeExecutionPolicies[node.NodeID]; ok {
		if node.ConcurrencyLimit == 0 {
			node.ConcurrencyLimit = policy.ConcurrencyLimit
		}
		if strings.TrimSpace(node.FairnessKey) == "" && strings.TrimSpace(policy.FairnessKey) != "" {
			node.FairnessKey = policy.FairnessKey
		}
	}

	if strings.TrimSpace(node.FairnessKey) == "" {
		if defaultEdgePolicy, ok := plan.EdgeBufferPolicies["default"]; ok && strings.TrimSpace(defaultEdgePolicy.FairnessKey) != "" {
			node.FairnessKey = defaultEdgePolicy.FairnessKey
		}
	}

	return node
}

func (p ExecutionPlan) validate() (map[string]NodeSpec, error) {
	if len(p.Nodes) == 0 {
		return nil, fmt.Errorf("execution plan requires at least one node")
	}

	nodeByID := make(map[string]NodeSpec, len(p.Nodes))
	for _, node := range p.Nodes {
		if node.NodeID == "" {
			return nil, fmt.Errorf("execution plan node_id is required")
		}
		if _, exists := nodeByID[node.NodeID]; exists {
			return nil, fmt.Errorf("duplicate execution plan node_id: %s", node.NodeID)
		}
		if node.NodeType == "" {
			return nil, fmt.Errorf("execution plan node_type is required for node %s", node.NodeID)
		}
		switch node.Lane {
		case eventabi.LaneData, eventabi.LaneControl, eventabi.LaneTelemetry:
		default:
			return nil, fmt.Errorf("execution plan node %s has invalid lane %q", node.NodeID, node.Lane)
		}
		if node.Provider != nil {
			if err := node.Provider.Modality.Validate(); err != nil {
				return nil, err
			}
		}
		if node.ConcurrencyLimit < 0 {
			return nil, fmt.Errorf("execution plan node %s concurrency_limit must be >=0", node.NodeID)
		}
		nodeByID[node.NodeID] = node
	}

	for _, edge := range p.Edges {
		if edge.From == "" || edge.To == "" {
			return nil, fmt.Errorf("execution plan edge from/to are required")
		}
		if edge.From == edge.To {
			return nil, fmt.Errorf("execution plan self-cycle edge is not allowed: %s", edge.From)
		}
		if _, ok := nodeByID[edge.From]; !ok {
			return nil, fmt.Errorf("execution plan edge from references unknown node: %s", edge.From)
		}
		if _, ok := nodeByID[edge.To]; !ok {
			return nil, fmt.Errorf("execution plan edge to references unknown node: %s", edge.To)
		}
	}
	return nodeByID, nil
}

func buildExecutionGraph(plan ExecutionPlan) (map[string][]string, map[string]int) {
	adj := make(map[string][]string, len(plan.Nodes))
	indegree := make(map[string]int, len(plan.Nodes))
	for _, node := range plan.Nodes {
		adj[node.NodeID] = []string{}
		indegree[node.NodeID] = 0
	}
	for _, edge := range plan.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		indegree[edge.To]++
	}
	return adj, indegree
}

func topologicalOrder(plan ExecutionPlan, nodeByID map[string]NodeSpec) ([]string, error) {
	adj, indegree := buildExecutionGraph(plan)

	ready := make([]string, 0, len(nodeByID))
	for _, node := range plan.Nodes {
		if indegree[node.NodeID] == 0 {
			ready = append(ready, node.NodeID)
		}
	}

	order := make([]string, 0, len(nodeByID))
	for len(ready) > 0 {
		idx := selectReadyNodeIndexByLanePriority(ready, nodeByID)
		current := ready[idx]
		ready = append(ready[:idx], ready[idx+1:]...)
		order = append(order, current)

		for _, next := range adj[current] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	if len(order) != len(nodeByID) {
		return nil, fmt.Errorf("execution plan contains cycle")
	}
	return order, nil
}

func selectReadyNodeIndexByLanePriority(ready []string, nodeByID map[string]NodeSpec) int {
	bestIdx := 0
	bestPriority := lanePriority(nodeByID[ready[0]].Lane)
	for idx := 1; idx < len(ready); idx++ {
		candidatePriority := lanePriority(nodeByID[ready[idx]].Lane)
		if candidatePriority < bestPriority {
			bestIdx = idx
			bestPriority = candidatePriority
		}
	}
	return bestIdx
}

func lanePriority(lane eventabi.Lane) int {
	switch lane {
	case eventabi.LaneControl:
		return 0
	case eventabi.LaneData:
		return 1
	default:
		return 2
	}
}

func decisionFromLaneOverflow(lane eventabi.Lane, enqueueResult lanes.EnqueueResult) SchedulingDecision {
	decision := SchedulingDecision{
		Allowed: lane == eventabi.LaneTelemetry,
	}
	if enqueueResult.ShedOutcome != nil {
		decision.Outcome = enqueueResult.ShedOutcome
		decision.Allowed = false
	}
	for _, signal := range enqueueResult.Signals {
		if signal.Signal != "shed" {
			continue
		}
		copied := signal
		decision.ControlSignal = &copied
		break
	}
	return decision
}

func terminalReasonFromDroppedDecision(decision SchedulingDecision) string {
	if decision.Outcome != nil && decision.Outcome.Reason != "" {
		return decision.Outcome.Reason
	}
	return "lane_queue_capacity_exceeded"
}

func terminalReasonFromDeniedDecision(decision SchedulingDecision) string {
	if decision.Outcome != nil && decision.Outcome.Reason != "" {
		return decision.Outcome.Reason
	}
	return "execution_plan_denied"
}

func stabilizeControlSignals(in []eventabi.ControlSignal) []eventabi.ControlSignal {
	if len(in) < 2 {
		return in
	}

	out := make([]eventabi.ControlSignal, len(in))
	copy(out, in)

	lastRuntimeSequence := int64(-1)
	lastRuntimeTimestampMS := int64(-1)
	lastWallClockMS := int64(-1)
	lastTransportSequence := int64(-1)

	for idx := range out {
		signal := out[idx]
		if signal.RuntimeSequence < lastRuntimeSequence {
			signal.RuntimeSequence = lastRuntimeSequence + 1
		}
		lastRuntimeSequence = signal.RuntimeSequence

		if signal.RuntimeTimestampMS < lastRuntimeTimestampMS {
			signal.RuntimeTimestampMS = lastRuntimeTimestampMS + 1
		}
		lastRuntimeTimestampMS = signal.RuntimeTimestampMS

		if signal.WallClockMS < lastWallClockMS {
			signal.WallClockMS = lastWallClockMS + 1
		}
		lastWallClockMS = signal.WallClockMS

		if signal.TransportSequence != nil {
			transportSequence := *signal.TransportSequence
			if transportSequence < lastTransportSequence {
				transportSequence = lastTransportSequence + 1
				signal.TransportSequence = &transportSequence
			}
			lastTransportSequence = *signal.TransportSequence
		}

		out[idx] = signal
	}
	return out
}
