package routing

import "testing"

func TestRoutingClientApplyAndStaleGuards(t *testing.T) {
	t.Parallel()

	client := NewClient()
	changed, err := client.Apply(View{Version: 1, ActiveEndpoint: "runtime-a", LeaseEpoch: 5})
	if err != nil {
		t.Fatalf("apply initial view: %v", err)
	}
	if !changed {
		t.Fatalf("expected first apply to report change")
	}

	if _, err := client.Apply(View{Version: 0, ActiveEndpoint: "runtime-b", LeaseEpoch: 5}); err == nil {
		t.Fatalf("expected stale version to be rejected")
	}
	if _, err := client.Apply(View{Version: 2, ActiveEndpoint: "runtime-b", LeaseEpoch: 4}); err == nil {
		t.Fatalf("expected stale lease epoch to be rejected")
	}

	changed, err = client.Apply(View{Version: 2, ActiveEndpoint: "runtime-b", LeaseEpoch: 6})
	if err != nil {
		t.Fatalf("apply newer view: %v", err)
	}
	if !changed {
		t.Fatalf("expected newer view to report change")
	}

	current, ok := client.Current()
	if !ok {
		t.Fatalf("expected current view to be available")
	}
	if current.ActiveEndpoint != "runtime-b" || current.LeaseEpoch != 6 {
		t.Fatalf("unexpected current view %+v", current)
	}
}
