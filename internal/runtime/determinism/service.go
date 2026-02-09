package determinism

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

// Service issues deterministic runtime determinism contexts.
type Service struct{}

// NewService returns a deterministic context issuer.
func NewService() Service {
	return Service{}
}

// IssueContext produces a deterministic context for a plan hash and runtime sequence marker.
func (Service) IssueContext(planHash string, runtimeSequence int64) (controlplane.Determinism, error) {
	if planHash == "" {
		return controlplane.Determinism{}, fmt.Errorf("plan_hash is required")
	}
	seed := deriveSeed(planHash, runtimeSequence)
	ctx := controlplane.Determinism{
		Seed:             seed,
		OrderingMarkers:  []string{"runtime_sequence", "event_id"},
		MergeRuleID:      "default-merge-rule",
		MergeRuleVersion: "v1.0.0",
		NondeterministicInputs: []controlplane.NondeterministicInput{
			{
				InputKey:    "plan_hash",
				Source:      "other",
				EvidenceRef: planHash,
			},
		},
	}
	if err := ctx.Validate(); err != nil {
		return controlplane.Determinism{}, err
	}
	return ctx, nil
}

// ValidateContext validates a previously issued context.
func (Service) ValidateContext(ctx controlplane.Determinism) error {
	return ctx.Validate()
}

func deriveSeed(planHash string, runtimeSequence int64) int64 {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", planHash, runtimeSequence)))
	value := int64(binary.BigEndian.Uint64(sum[:8]))
	if value < 0 {
		return -value
	}
	return value
}
