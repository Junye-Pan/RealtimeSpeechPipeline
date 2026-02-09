package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// ContractValidationSummary reports fixture validation totals.
type ContractValidationSummary struct {
	Total    int
	Failed   int
	Failures []string
}

// ValidateContractFixtures validates valid/invalid fixture sets for core artifacts.
func ValidateContractFixtures(root string) (ContractValidationSummary, error) {
	return ValidateContractFixturesWithSchema(filepath.Join("docs", "ContractArtifacts.schema.json"), root)
}

// ValidateContractFixturesWithSchema validates fixture sets using typed validators and JSON schema.
func ValidateContractFixturesWithSchema(schemaPath, root string) (ContractValidationSummary, error) {
	validators := []struct {
		name      string
		validator func([]byte) error
	}{
		{name: "event", validator: validateEvent},
		{name: "control_signal", validator: validateControlSignal},
		{name: "turn_transition", validator: validateTurnTransition},
		{name: "resolved_turn_plan", validator: validateResolvedTurnPlan},
		{name: "decision_outcome", validator: validateDecisionOutcome},
	}

	summary := ContractValidationSummary{}
	compiled, err := compileSchema(schemaPath)
	if err != nil {
		return summary, err
	}

	for _, entry := range validators {
		for _, validity := range []struct {
			dir        string
			shouldPass bool
		}{
			{dir: "valid", shouldPass: true},
			{dir: "invalid", shouldPass: false},
		} {
			dir := filepath.Join(root, entry.name, validity.dir)
			items, err := os.ReadDir(dir)
			if err != nil {
				return summary, fmt.Errorf("read fixtures %s: %w", dir, err)
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				if !item.IsDir() {
					names = append(names, item.Name())
				}
			}
			sort.Strings(names)
			for _, name := range names {
				summary.Total++
				filePath := filepath.Join(dir, name)
				raw, readErr := os.ReadFile(filePath)
				if readErr != nil {
					summary.Failed++
					summary.Failures = append(summary.Failures, fmt.Sprintf("%s: read error: %v", filePath, readErr))
					continue
				}

				typedErr := entry.validator(raw)
				schemaErr := validateAgainstSchema(compiled, raw)

				if validity.shouldPass {
					if typedErr != nil || schemaErr != nil {
						summary.Failed++
						summary.Failures = append(summary.Failures, fmt.Sprintf("%s: expected valid, typed_err=%v schema_err=%v", filePath, typedErr, schemaErr))
					}
					continue
				}

				if typedErr == nil || schemaErr == nil {
					summary.Failed++
					summary.Failures = append(summary.Failures, fmt.Sprintf("%s: expected invalid by both validators, typed_err=%v schema_err=%v", filePath, typedErr, schemaErr))
				}
			}
		}
	}

	return summary, nil
}

func compileSchema(schemaPath string) (*jsonschema.Schema, error) {
	absSchemaPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("resolve schema path: %w", err)
	}
	if _, err := os.Stat(absSchemaPath); err != nil {
		return nil, fmt.Errorf("schema file unavailable at %s: %w", absSchemaPath, err)
	}

	compiler := jsonschema.NewCompiler()
	f, err := os.Open(absSchemaPath)
	if err != nil {
		return nil, fmt.Errorf("open schema file: %w", err)
	}
	defer f.Close()
	if err := compiler.AddResource(absSchemaPath, f); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(absSchemaPath)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

func validateAgainstSchema(schema *jsonschema.Schema, raw []byte) error {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	return schema.Validate(payload)
}

func RenderSummary(summary ContractValidationSummary) string {
	lines := []string{fmt.Sprintf("contract fixtures: total=%d failed=%d", summary.Total, summary.Failed)}
	if len(summary.Failures) > 0 {
		lines = append(lines, "failures:")
		for _, f := range summary.Failures {
			lines = append(lines, "- "+f)
		}
	}
	return strings.Join(lines, "\n")
}

func validateEvent(data []byte) error {
	var e eventabi.EventRecord
	if err := strictUnmarshal(data, &e); err != nil {
		return err
	}
	return e.Validate()
}

func validateControlSignal(data []byte) error {
	var s eventabi.ControlSignal
	if err := strictUnmarshal(data, &s); err != nil {
		return err
	}
	return s.Validate()
}

func validateTurnTransition(data []byte) error {
	var tr controlplane.TurnTransition
	if err := strictUnmarshal(data, &tr); err != nil {
		return err
	}
	return tr.Validate()
}

func validateResolvedTurnPlan(data []byte) error {
	var p controlplane.ResolvedTurnPlan
	if err := strictUnmarshal(data, &p); err != nil {
		return err
	}
	return p.Validate()
}

func validateDecisionOutcome(data []byte) error {
	var d controlplane.DecisionOutcome
	if err := strictUnmarshal(data, &d); err != nil {
		return err
	}
	return d.Validate()
}

func strictUnmarshal(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON payload")
	}
	return nil
}
