package timebase

import (
	"fmt"
	"strings"
	"sync"
)

// Calibration defines the anchor that maps monotonic, wall-clock, and media time.
type Calibration struct {
	MonotonicMS int64
	WallClockMS int64
	MediaTimeMS int64
}

// Projection is a mapped timestamp tuple returned by the service.
type Projection struct {
	MonotonicMS int64
	WallClockMS int64
	MediaTimeMS int64
	SkewMS      int64
	Rebased     bool
}

type sessionClock struct {
	anchor        Calibration
	lastMonotonic int64
	lastWallClock int64
	lastMediaTime int64
}

// Service provides deterministic runtime-local timebase mapping.
type Service struct {
	mu       sync.Mutex
	sessions map[string]*sessionClock
}

// NewService constructs an empty timebase mapping service.
func NewService() *Service {
	return &Service{
		sessions: make(map[string]*sessionClock),
	}
}

// Calibrate sets or updates the session anchor.
func (s *Service) Calibrate(sessionID string, c Calibration) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if c.MonotonicMS < 0 || c.WallClockMS < 0 || c.MediaTimeMS < 0 {
		return fmt.Errorf("calibration values must be >= 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sessions[sessionID]; ok {
		// Re-calibration is allowed only at or ahead of previous monotonic anchor.
		if c.MonotonicMS < existing.anchor.MonotonicMS {
			return fmt.Errorf("calibration monotonic_ms must be >= previous anchor")
		}
	}

	s.sessions[sessionID] = &sessionClock{
		anchor:        c,
		lastMonotonic: c.MonotonicMS,
		lastWallClock: c.WallClockMS,
		lastMediaTime: c.MediaTimeMS,
	}
	return nil
}

// Project maps a monotonic timestamp to wall-clock and media timestamps.
func (s *Service) Project(sessionID string, monotonicMS int64) (Projection, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Projection{}, fmt.Errorf("session_id is required")
	}
	if monotonicMS < 0 {
		return Projection{}, fmt.Errorf("monotonic_ms must be >= 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	clock, ok := s.sessions[sessionID]
	if !ok {
		return Projection{}, fmt.Errorf("session %q is not calibrated", sessionID)
	}
	if monotonicMS < clock.lastMonotonic {
		return Projection{}, fmt.Errorf("monotonic_ms must be non-decreasing")
	}

	delta := monotonicMS - clock.anchor.MonotonicMS
	projection := Projection{
		MonotonicMS: monotonicMS,
		WallClockMS: clock.anchor.WallClockMS + delta,
		MediaTimeMS: clock.anchor.MediaTimeMS + delta,
		SkewMS:      0,
	}

	clock.lastMonotonic = projection.MonotonicMS
	clock.lastWallClock = projection.WallClockMS
	clock.lastMediaTime = projection.MediaTimeMS
	return projection, nil
}

// Observe compares observed wall/media timestamps at a monotonic point and rebases if skew exceeds tolerance.
func (s *Service) Observe(sessionID string, monotonicMS int64, observedWallMS int64, observedMediaMS int64, maxSkewMS int64) (Projection, error) {
	if observedWallMS < 0 || observedMediaMS < 0 {
		return Projection{}, fmt.Errorf("observed values must be >= 0")
	}
	if maxSkewMS < 0 {
		return Projection{}, fmt.Errorf("max_skew_ms must be >= 0")
	}

	projected, err := s.Project(sessionID, monotonicMS)
	if err != nil {
		return Projection{}, err
	}
	skew := abs64(observedWallMS - projected.WallClockMS)
	mediaSkew := abs64(observedMediaMS - projected.MediaTimeMS)
	if mediaSkew > skew {
		skew = mediaSkew
	}
	projected.SkewMS = skew

	if skew > maxSkewMS {
		if err := s.Calibrate(sessionID, Calibration{
			MonotonicMS: monotonicMS,
			WallClockMS: observedWallMS,
			MediaTimeMS: observedMediaMS,
		}); err != nil {
			return Projection{}, err
		}
		projected.WallClockMS = observedWallMS
		projected.MediaTimeMS = observedMediaMS
		projected.Rebased = true
	}

	return projected, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
