package executor

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/lanes"
)

// NodeSpec defines one deterministic runtime execution node.
type NodeSpec struct {
	NodeID   string
	NodeType string
	Lane     eventabi.Lane
	Provider *ProviderInvocationInput
	Shed     bool
	Reason   string
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
}

// ExecutionTrace summarizes deterministic plan execution.
type ExecutionTrace struct {
	NodeOrder      []string
	Nodes          []NodeExecutionResult
	ControlSignals []eventabi.ControlSignal
	Completed      bool
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
		nodeInput.Shed = node.Shed
		nodeInput.Reason = node.Reason
		nodeInput.TransportSequence = nonNegative(in.TransportSequence) + offset
		nodeInput.RuntimeSequence = nonNegative(in.RuntimeSequence) + offset
		nodeInput.AuthorityEpoch = nonNegative(in.AuthorityEpoch)
		nodeInput.RuntimeTimestampMS = nonNegative(in.RuntimeTimestampMS) + offset
		nodeInput.WallClockTimestampMS = nonNegative(in.WallClockTimestampMS) + offset
		nodeInput.ProviderInvocation = node.Provider

		decision, err := s.NodeDispatch(nodeInput)
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

		if !decision.Allowed {
			trace.Completed = false
			break
		}
	}

	trace.ControlSignals, err = runtimeeventabi.ValidateAndNormalizeControlSignals(trace.ControlSignals)
	if err != nil {
		return ExecutionTrace{}, err
	}
	return trace, nil
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

func topologicalOrder(plan ExecutionPlan, nodeByID map[string]NodeSpec) ([]string, error) {
	adj := make(map[string][]string, len(nodeByID))
	indegree := make(map[string]int, len(nodeByID))
	for _, node := range plan.Nodes {
		adj[node.NodeID] = []string{}
		indegree[node.NodeID] = 0
	}
	for _, edge := range plan.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		indegree[edge.To]++
	}

	ready := make([]string, 0, len(nodeByID))
	for _, node := range plan.Nodes {
		if indegree[node.NodeID] == 0 {
			ready = append(ready, node.NodeID)
		}
	}

	order := make([]string, 0, len(nodeByID))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
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
