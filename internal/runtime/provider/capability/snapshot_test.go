package capability

import "testing"

func TestFreezeAndFingerprintDeterminism(t *testing.T) {
	t.Parallel()

	in := FreezeInput{
		SnapshotRef:  "provider-health/v1",
		CapturedAtMS: 123,
		Providers: map[string]ProviderState{
			"stt-a": {Healthy: true, AvailabilityScore: 98, PriceMicros: 30},
			"stt-b": {Healthy: false, AvailabilityScore: 20, PriceMicros: 10},
		},
	}
	snapshot, err := Freeze(in)
	if err != nil {
		t.Fatalf("freeze snapshot: %v", err)
	}
	fp1, err := snapshot.Fingerprint()
	if err != nil {
		t.Fatalf("fingerprint1: %v", err)
	}
	fp2, err := snapshot.Fingerprint()
	if err != nil {
		t.Fatalf("fingerprint2: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("expected deterministic fingerprint, got %q and %q", fp1, fp2)
	}
}

func TestFreezeValidation(t *testing.T) {
	t.Parallel()

	_, err := Freeze(FreezeInput{
		SnapshotRef:  "provider-health/v1",
		CapturedAtMS: 1,
		Providers: map[string]ProviderState{
			"": {Healthy: true},
		},
	})
	if err == nil {
		t.Fatalf("expected empty provider id to fail validation")
	}

	_, err = Freeze(FreezeInput{
		SnapshotRef:  "provider-health/v1",
		CapturedAtMS: 1,
		Providers: map[string]ProviderState{
			"stt-a": {Healthy: true, AvailabilityScore: 200},
		},
	})
	if err == nil {
		t.Fatalf("expected invalid availability score to fail validation")
	}
}

func TestFreezeDefaultSnapshotRef(t *testing.T) {
	t.Parallel()

	snapshot, err := Freeze(FreezeInput{CapturedAtMS: 7})
	if err != nil {
		t.Fatalf("freeze snapshot: %v", err)
	}
	if snapshot.SnapshotRef != defaultSnapshotRef {
		t.Fatalf("expected default snapshot ref %q, got %q", defaultSnapshotRef, snapshot.SnapshotRef)
	}
}
