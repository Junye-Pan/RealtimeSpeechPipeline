package egress

import (
	"fmt"

	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
)

// Delivery models one transport egress attempt.
type Delivery struct {
	Attempt      runtimetransport.OutputAttempt
	PayloadBytes int
}

// Broker wraps output fence behavior for transport adapters.
type Broker struct {
	fence *runtimetransport.OutputFence
}

// NewBroker creates an egress broker.
func NewBroker(fence *runtimetransport.OutputFence) Broker {
	if fence == nil {
		fence = runtimetransport.NewOutputFence()
	}
	return Broker{fence: fence}
}

// Deliver applies deterministic egress fencing and returns output decision.
func (b Broker) Deliver(in Delivery) (runtimetransport.OutputDecision, error) {
	if in.PayloadBytes < 0 {
		return runtimetransport.OutputDecision{}, fmt.Errorf("payload_bytes must be >=0")
	}
	return b.fence.EvaluateOutput(in.Attempt)
}
