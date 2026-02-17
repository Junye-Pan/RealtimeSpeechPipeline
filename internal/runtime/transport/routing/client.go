package routing

import "fmt"

// View is the transport-facing routing snapshot.
type View struct {
	Version           int64
	ActiveEndpoint    string
	CandidateEndpoint string
	LeaseEpoch        int64
}

// Validate enforces routing view invariants.
func (v View) Validate() error {
	if v.Version < 1 {
		return fmt.Errorf("routing view version must be >=1")
	}
	if v.ActiveEndpoint == "" {
		return fmt.Errorf("routing view active_endpoint is required")
	}
	if v.LeaseEpoch < 0 {
		return fmt.Errorf("routing view lease_epoch must be >=0")
	}
	return nil
}

// Client tracks the latest routing view with stale-update protection.
type Client struct {
	current     View
	initialized bool
}

// NewClient creates a routing view client.
func NewClient() *Client {
	return &Client{}
}

// Apply sets a newer routing view and rejects stale updates.
func (c *Client) Apply(view View) (bool, error) {
	if err := view.Validate(); err != nil {
		return false, err
	}
	if !c.initialized {
		c.current = view
		c.initialized = true
		return true, nil
	}
	if view.Version < c.current.Version {
		return false, fmt.Errorf("stale routing view version: %d < %d", view.Version, c.current.Version)
	}
	if view.LeaseEpoch < c.current.LeaseEpoch {
		return false, fmt.Errorf("stale routing lease_epoch: %d < %d", view.LeaseEpoch, c.current.LeaseEpoch)
	}
	if view.Version == c.current.Version && view.LeaseEpoch == c.current.LeaseEpoch && view.ActiveEndpoint == c.current.ActiveEndpoint && view.CandidateEndpoint == c.current.CandidateEndpoint {
		return false, nil
	}
	c.current = view
	return true, nil
}

// Current returns the last applied routing view.
func (c *Client) Current() (View, bool) {
	if !c.initialized {
		return View{}, false
	}
	return c.current, true
}
