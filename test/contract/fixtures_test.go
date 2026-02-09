package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

type validatorFn func([]byte) error

func TestContractFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		baseDir   string
		validator validatorFn
	}{
		{name: "event", baseDir: "fixtures/event", validator: validateEvent},
		{name: "control_signal", baseDir: "fixtures/control_signal", validator: validateControlSignal},
		{name: "turn_transition", baseDir: "fixtures/turn_transition", validator: validateTurnTransition},
		{name: "resolved_turn_plan", baseDir: "fixtures/resolved_turn_plan", validator: validateResolvedTurnPlan},
		{name: "decision_outcome", baseDir: "fixtures/decision_outcome", validator: validateDecisionOutcome},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"_valid", func(t *testing.T) {
			t.Parallel()
			runFixtures(t, filepath.Join(tc.baseDir, "valid"), true, tc.validator)
		})

		t.Run(tc.name+"_invalid", func(t *testing.T) {
			t.Parallel()
			runFixtures(t, filepath.Join(tc.baseDir, "invalid"), false, tc.validator)
		})
	}
}

func runFixtures(t *testing.T, dir string, shouldPass bool, validator validatorFn) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixtures dir %s: %v", dir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no fixtures in %s", dir)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bytes, readErr := os.ReadFile(filepath.Join(dir, name))
			if readErr != nil {
				t.Fatalf("read fixture: %v", readErr)
			}
			vErr := validator(bytes)
			if shouldPass && vErr != nil {
				t.Fatalf("expected valid fixture, got error: %v", vErr)
			}
			if !shouldPass && vErr == nil {
				t.Fatalf("expected invalid fixture to fail validation")
			}
		})
	}
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
