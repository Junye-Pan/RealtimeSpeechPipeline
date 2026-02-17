package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

const (
	// PipelineSpecSchemaVersionV1 is the canonical MVP schema version for pipeline spec files.
	PipelineSpecSchemaVersionV1 = "rspp.pipeline.spec.v1"
	// PolicyBundleSchemaVersionV1 is the canonical MVP schema version for policy bundle files.
	PolicyBundleSchemaVersionV1 = "rspp.policy.bundle.v1"
)

// ValidationMode controls strictness for tooling validation commands.
type ValidationMode string

const (
	ValidationModeStrict  ValidationMode = "strict"
	ValidationModeRelaxed ValidationMode = "relaxed"
)

type pipelineSpecDocument struct {
	SchemaVersion      string             `json:"schema_version"`
	SpecID             string             `json:"spec_id"`
	PipelineVersion    string             `json:"pipeline_version"`
	GraphDefinitionRef string             `json:"graph_definition_ref"`
	Nodes              []pipelineSpecNode `json:"nodes"`
	Edges              []pipelineSpecEdge `json:"edges"`
}

type pipelineSpecNode struct {
	NodeID   string `json:"node_id"`
	Kind     string `json:"kind"`
	Modality string `json:"modality,omitempty"`
}

type pipelineSpecEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type policyBundleDocument struct {
	SchemaVersion            string                       `json:"schema_version"`
	PolicyResolutionSnapshot string                       `json:"policy_resolution_snapshot"`
	AllowedAdaptiveActions   []string                     `json:"allowed_adaptive_actions"`
	ProviderBindings         map[string]string            `json:"provider_bindings"`
	RecordingPolicy          controlplane.RecordingPolicy `json:"recording_policy"`
}

// ParseValidationMode normalizes command mode input.
func ParseValidationMode(raw string) (ValidationMode, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ValidationModeStrict, nil
	}
	switch ValidationMode(trimmed) {
	case ValidationModeStrict, ValidationModeRelaxed:
		return ValidationMode(trimmed), nil
	default:
		return "", fmt.Errorf("unsupported validation mode %q (expected strict|relaxed)", raw)
	}
}

// ValidatePipelineSpecFile validates a pipeline spec file in strict or relaxed mode.
func ValidatePipelineSpecFile(path string, mode string) error {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return fmt.Errorf("spec_path is required")
	}
	raw, err := os.ReadFile(normalizedPath)
	if err != nil {
		return fmt.Errorf("read spec file %s: %w", normalizedPath, err)
	}
	if err := ValidatePipelineSpec(raw, mode); err != nil {
		return fmt.Errorf("validate spec %s: %w", normalizedPath, err)
	}
	return nil
}

// ValidatePolicyBundleFile validates a policy bundle file in strict or relaxed mode.
func ValidatePolicyBundleFile(path string, mode string) error {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return fmt.Errorf("policy_path is required")
	}
	raw, err := os.ReadFile(normalizedPath)
	if err != nil {
		return fmt.Errorf("read policy file %s: %w", normalizedPath, err)
	}
	if err := ValidatePolicyBundle(raw, mode); err != nil {
		return fmt.Errorf("validate policy %s: %w", normalizedPath, err)
	}
	return nil
}

// ValidatePipelineSpec validates pipeline spec JSON payload in strict or relaxed mode.
func ValidatePipelineSpec(raw []byte, mode string) error {
	parsedMode, err := ParseValidationMode(mode)
	if err != nil {
		return err
	}
	doc := pipelineSpecDocument{}
	if err := decodeJSON(raw, &doc, parsedMode == ValidationModeStrict); err != nil {
		return err
	}
	return validatePipelineSpec(doc, parsedMode)
}

// ValidatePolicyBundle validates policy bundle JSON payload in strict or relaxed mode.
func ValidatePolicyBundle(raw []byte, mode string) error {
	parsedMode, err := ParseValidationMode(mode)
	if err != nil {
		return err
	}
	doc := policyBundleDocument{}
	if err := decodeJSON(raw, &doc, parsedMode == ValidationModeStrict); err != nil {
		return err
	}
	return validatePolicyBundle(doc, parsedMode)
}

func validatePipelineSpec(doc pipelineSpecDocument, mode ValidationMode) error {
	schemaVersion := strings.TrimSpace(doc.SchemaVersion)
	if schemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if mode == ValidationModeStrict && schemaVersion != PipelineSpecSchemaVersionV1 {
		return fmt.Errorf("schema_version must equal %q in strict mode", PipelineSpecSchemaVersionV1)
	}
	if mode == ValidationModeRelaxed && !strings.HasPrefix(schemaVersion, "rspp.pipeline.spec.") {
		return fmt.Errorf("schema_version must start with %q in relaxed mode", "rspp.pipeline.spec.")
	}

	if strings.TrimSpace(doc.SpecID) == "" {
		return fmt.Errorf("spec_id is required")
	}
	if strings.TrimSpace(doc.PipelineVersion) == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if strings.TrimSpace(doc.GraphDefinitionRef) == "" {
		return fmt.Errorf("graph_definition_ref is required")
	}
	if len(doc.Nodes) == 0 {
		return fmt.Errorf("nodes requires at least one entry")
	}

	nodeIDs := make(map[string]struct{}, len(doc.Nodes))
	for idx, node := range doc.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			return fmt.Errorf("nodes[%d].node_id is required", idx)
		}
		if _, exists := nodeIDs[nodeID]; exists {
			return fmt.Errorf("nodes[%d].node_id duplicates %q", idx, nodeID)
		}
		nodeIDs[nodeID] = struct{}{}
		if strings.TrimSpace(node.Kind) == "" {
			return fmt.Errorf("nodes[%d].kind is required", idx)
		}
		if strings.EqualFold(strings.TrimSpace(node.Kind), "provider") {
			modality := strings.ToLower(strings.TrimSpace(node.Modality))
			if modality == "" {
				return fmt.Errorf("nodes[%d].modality is required for provider kind", idx)
			}
			if modality != "stt" && modality != "llm" && modality != "tts" {
				return fmt.Errorf("nodes[%d].modality %q must be one of stt|llm|tts", idx, node.Modality)
			}
		}
	}

	if len(doc.Edges) == 0 {
		return fmt.Errorf("edges requires at least one entry")
	}
	seenEdges := make(map[string]struct{}, len(doc.Edges))
	for idx, edge := range doc.Edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" {
			return fmt.Errorf("edges[%d].from is required", idx)
		}
		if to == "" {
			return fmt.Errorf("edges[%d].to is required", idx)
		}
		if _, ok := nodeIDs[from]; !ok {
			return fmt.Errorf("edges[%d].from %q does not reference a declared node_id", idx, from)
		}
		if _, ok := nodeIDs[to]; !ok {
			return fmt.Errorf("edges[%d].to %q does not reference a declared node_id", idx, to)
		}
		edgeKey := from + "->" + to
		if _, exists := seenEdges[edgeKey]; exists {
			return fmt.Errorf("edges[%d] duplicates edge %q", idx, edgeKey)
		}
		seenEdges[edgeKey] = struct{}{}
	}

	return nil
}

func validatePolicyBundle(doc policyBundleDocument, mode ValidationMode) error {
	schemaVersion := strings.TrimSpace(doc.SchemaVersion)
	if schemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if mode == ValidationModeStrict && schemaVersion != PolicyBundleSchemaVersionV1 {
		return fmt.Errorf("schema_version must equal %q in strict mode", PolicyBundleSchemaVersionV1)
	}
	if mode == ValidationModeRelaxed && !strings.HasPrefix(schemaVersion, "rspp.policy.bundle.") {
		return fmt.Errorf("schema_version must start with %q in relaxed mode", "rspp.policy.bundle.")
	}
	if strings.TrimSpace(doc.PolicyResolutionSnapshot) == "" {
		return fmt.Errorf("policy_resolution_snapshot is required")
	}
	if len(doc.AllowedAdaptiveActions) == 0 {
		return fmt.Errorf("allowed_adaptive_actions requires at least one action")
	}
	seenActions := make(map[string]struct{}, len(doc.AllowedAdaptiveActions))
	for idx, action := range doc.AllowedAdaptiveActions {
		normalized := strings.ToLower(strings.TrimSpace(action))
		if normalized == "" {
			return fmt.Errorf("allowed_adaptive_actions[%d] must be non-empty", idx)
		}
		switch normalized {
		case "retry", "provider_switch", "degrade", "fallback":
		default:
			return fmt.Errorf("allowed_adaptive_actions[%d] %q is unsupported", idx, action)
		}
		if _, exists := seenActions[normalized]; exists {
			return fmt.Errorf("allowed_adaptive_actions[%d] duplicates %q", idx, normalized)
		}
		seenActions[normalized] = struct{}{}
	}
	if len(doc.ProviderBindings) == 0 {
		return fmt.Errorf("provider_bindings must be non-empty")
	}
	for modality, providerID := range doc.ProviderBindings {
		if strings.TrimSpace(modality) == "" {
			return fmt.Errorf("provider_bindings contains an empty modality key")
		}
		if strings.TrimSpace(providerID) == "" {
			return fmt.Errorf("provider_bindings[%q] must be non-empty", modality)
		}
	}
	if err := validateRecordingPolicy(doc.RecordingPolicy); err != nil {
		return fmt.Errorf("recording_policy: %w", err)
	}
	return nil
}

func validateRecordingPolicy(policy controlplane.RecordingPolicy) error {
	level := strings.TrimSpace(policy.RecordingLevel)
	if level != "L0" && level != "L1" && level != "L2" {
		return fmt.Errorf("invalid recording_level")
	}
	if len(policy.AllowedReplayModes) < 1 {
		return fmt.Errorf("allowed_replay_modes requires at least one mode")
	}
	seen := make(map[string]struct{}, len(policy.AllowedReplayModes))
	for idx, rawMode := range policy.AllowedReplayModes {
		mode := strings.TrimSpace(rawMode)
		if mode == "" {
			return fmt.Errorf("allowed_replay_modes[%d] cannot be empty", idx)
		}
		// Preserve compatibility with existing policy bundle fixtures that still use
		// legacy aliases while newer control-plane snapshots use canonical replay modes.
		isLegacyAlias := mode == "audit" || mode == "regression"
		if !obs.IsReplayMode(mode) && !isLegacyAlias {
			return fmt.Errorf("invalid replay mode: %s", mode)
		}
		if _, exists := seen[mode]; exists {
			return fmt.Errorf("duplicate replay mode: %s", mode)
		}
		seen[mode] = struct{}{}
	}
	return nil
}

func decodeJSON(raw []byte, out any, strict bool) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON payload")
}
