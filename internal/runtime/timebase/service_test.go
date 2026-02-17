package timebase

import "testing"

func TestProjectMaintainsMonotonicMapping(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.Calibrate("sess-timebase-1", Calibration{
		MonotonicMS: 1000,
		WallClockMS: 2000,
		MediaTimeMS: 1500,
	}); err != nil {
		t.Fatalf("calibrate: %v", err)
	}

	first, err := svc.Project("sess-timebase-1", 1010)
	if err != nil {
		t.Fatalf("project first: %v", err)
	}
	second, err := svc.Project("sess-timebase-1", 1025)
	if err != nil {
		t.Fatalf("project second: %v", err)
	}

	if first.WallClockMS != 2010 || first.MediaTimeMS != 1510 {
		t.Fatalf("unexpected first projection: %+v", first)
	}
	if second.WallClockMS != 2025 || second.MediaTimeMS != 1525 {
		t.Fatalf("unexpected second projection: %+v", second)
	}
	if second.WallClockMS <= first.WallClockMS || second.MediaTimeMS <= first.MediaTimeMS {
		t.Fatalf("expected non-decreasing projections, first=%+v second=%+v", first, second)
	}
}

func TestProjectRejectsNonMonotonicInput(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.Calibrate("sess-timebase-2", Calibration{
		MonotonicMS: 10,
		WallClockMS: 20,
		MediaTimeMS: 15,
	}); err != nil {
		t.Fatalf("calibrate: %v", err)
	}
	if _, err := svc.Project("sess-timebase-2", 12); err != nil {
		t.Fatalf("project monotonic step: %v", err)
	}
	if _, err := svc.Project("sess-timebase-2", 11); err == nil {
		t.Fatalf("expected non-monotonic projection to fail")
	}
}

func TestObserveRebasesOnExcessSkew(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.Calibrate("sess-timebase-3", Calibration{
		MonotonicMS: 100,
		WallClockMS: 1000,
		MediaTimeMS: 500,
	}); err != nil {
		t.Fatalf("calibrate: %v", err)
	}

	projected, err := svc.Observe("sess-timebase-3", 120, 1020, 520, 5)
	if err != nil {
		t.Fatalf("observe no skew: %v", err)
	}
	if projected.Rebased {
		t.Fatalf("expected no rebase under skew threshold, got %+v", projected)
	}

	rebased, err := svc.Observe("sess-timebase-3", 140, 1250, 650, 5)
	if err != nil {
		t.Fatalf("observe with skew: %v", err)
	}
	if !rebased.Rebased {
		t.Fatalf("expected rebase on high skew, got %+v", rebased)
	}
	if rebased.WallClockMS != 1250 || rebased.MediaTimeMS != 650 {
		t.Fatalf("expected rebase to observed values, got %+v", rebased)
	}

	next, err := svc.Project("sess-timebase-3", 150)
	if err != nil {
		t.Fatalf("project after rebase: %v", err)
	}
	if next.WallClockMS != 1260 || next.MediaTimeMS != 660 {
		t.Fatalf("expected projection to continue from rebased anchor, got %+v", next)
	}
}
