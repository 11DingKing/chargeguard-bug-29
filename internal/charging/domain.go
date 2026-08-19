package charging

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("charging record not found")
	ErrInvalidState    = errors.New("invalid charging state transition")
	ErrAlreadyClosed   = errors.New("hazard already closed")
	ErrMissingEvidence = errors.New("inspection evidence is required")
)

type StationStatus string

const (
	StationActive    StationStatus = "active"
	StationSuspended StationStatus = "suspended"
)

type HazardState string

const (
	HazardOpen      HazardState = "open"
	HazardAssigned  HazardState = "assigned"
	HazardRectified HazardState = "rectified"
	HazardVerified  HazardState = "verified"
	HazardRejected  HazardState = "rejected"
)

type Station struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	County     string        `json:"county"`
	OperatorID string        `json:"operator_id"`
	Status     StationStatus `json:"status"`
	Latitude   float64       `json:"latitude"`
	Longitude  float64       `json:"longitude"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Version    int           `json:"version"`
}

type Hazard struct {
	ID          string      `json:"id"`
	StationID   string      `json:"station_id"`
	Kind        string      `json:"kind"`
	Severity    string      `json:"severity"`
	Description string      `json:"description"`
	State       HazardState `json:"state"`
	ReportedBy  string      `json:"reported_by"`
	AssignedTo  string      `json:"assigned_to,omitempty"`
	DueAt       time.Time   `json:"due_at"`
	RectifiedAt *time.Time  `json:"rectified_at,omitempty"`
	VerifiedAt  *time.Time  `json:"verified_at,omitempty"`
	Evidence    string      `json:"evidence,omitempty"`
	Version     int         `json:"version"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type Inspection struct {
	ID                 string    `json:"id"`
	StationID          string    `json:"station_id"`
	InspectorID        string    `json:"inspector_id"`
	CheckedAt          time.Time `json:"checked_at"`
	ExtinguishersOK    bool      `json:"extinguishers_ok"`
	ExtinguisherExpiry time.Time `json:"extinguisher_expiry"`
	CrashBarrierOK     bool      `json:"crash_barrier_ok"`
	SignageOK          bool      `json:"signage_ok"`
	Notes              string    `json:"notes"`
}

func (h Hazard) CanTransition(next HazardState) bool {
	if h.State == next {
		return true
	}
	switch h.State {
	case HazardOpen:
		return next == HazardAssigned
	case HazardAssigned:
		return next == HazardRectified
	case HazardRectified:
		return next == HazardVerified || next == HazardRejected
	case HazardRejected:
		return next == HazardAssigned
	default:
		return false
	}
}
