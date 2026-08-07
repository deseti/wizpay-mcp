// Package autonomy contains the bounded, provider-neutral autonomous runtime.
// It owns authorization context and durable work identity; it never signs or
// submits a transaction.
package autonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type IntentType string

const (
	IntentPayroll IntentType = "PAYROLL"
	IntentSwap    IntentType = "SWAP"
)

func (t IntentType) Valid() bool { return t == IntentPayroll || t == IntentSwap }

type ScheduleStatus string

const (
	ScheduleActive  ScheduleStatus = "ACTIVE"
	SchedulePaused  ScheduleStatus = "PAUSED"
	ScheduleRevoked ScheduleStatus = "REVOKED"
)

func (s ScheduleStatus) Valid() bool {
	return s == ScheduleActive || s == SchedulePaused || s == ScheduleRevoked
}
func ValidScheduleTransition(from, to ScheduleStatus) bool {
	switch from {
	case ScheduleActive:
		return to == SchedulePaused || to == ScheduleRevoked
	case SchedulePaused:
		return to == ScheduleActive || to == ScheduleRevoked
	case ScheduleRevoked:
		return false
	default:
		return false
	}
}

type MissedRunPolicy string

const (
	MissedSkip      MissedRunPolicy = "SKIP"
	MissedRunLatest MissedRunPolicy = "RUN_LATEST"
)

type ConcurrencyPolicy string

const (
	ForbidOverlap ConcurrencyPolicy = "FORBID_OVERLAP"
)

type Frequency string

const (
	Once    Frequency = "ONCE"
	Daily   Frequency = "DAILY"
	Weekly  Frequency = "WEEKLY"
	Monthly Frequency = "MONTHLY"
)

type Recurrence struct {
	Frequency  Frequency
	Start      time.Time
	End        time.Time
	Weekday    time.Weekday
	DayOfMonth int
	Location   string
}

func (r Recurrence) Validate() error {
	if r.Start.IsZero() || r.Start.Location() == time.Local {
		return fmt.Errorf("recurrence start and explicit timezone are required")
	}
	if !r.Frequency.Valid() {
		return fmt.Errorf("unsupported recurrence frequency")
	}
	if !r.End.IsZero() && !r.End.After(r.Start) {
		return fmt.Errorf("recurrence end must follow start")
	}
	if r.Frequency == Weekly && (r.Weekday < time.Sunday || r.Weekday > time.Saturday) {
		return fmt.Errorf("invalid weekly day")
	}
	if r.Frequency == Monthly && (r.DayOfMonth < 1 || r.DayOfMonth > 31) {
		return fmt.Errorf("invalid monthly day")
	}
	if r.Location == "" {
		return fmt.Errorf("timezone is required")
	}
	if _, err := time.LoadLocation(r.Location); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
}
func (f Frequency) Valid() bool { return f == Once || f == Daily || f == Weekly || f == Monthly }
func (r Recurrence) Next(after time.Time) (time.Time, bool) {
	if r.Validate() != nil {
		return time.Time{}, false
	}
	loc, _ := time.LoadLocation(r.Location)
	base := r.Start.In(loc)
	after = after.In(loc)
	var next time.Time
	switch r.Frequency {
	case Once:
		if after.Before(base) {
			next = base
		}
	case Daily:
		next = base
		for !next.After(after) {
			next = next.AddDate(0, 0, 1)
		}
	case Weekly:
		next = base
		for !next.After(after) || next.Weekday() != r.Weekday {
			next = next.AddDate(0, 0, 1)
		}
	case Monthly:
		next = base
		for !next.After(after) || next.Day() != r.DayOfMonth {
			next = next.AddDate(0, 1, 0)
			next = time.Date(next.Year(), next.Month(), r.DayOfMonth, base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), loc)
		}
	}
	if next.IsZero() || (!r.End.IsZero() && !next.Before(r.End)) {
		return time.Time{}, false
	}
	return next.UTC(), true
}

type ScheduleSpec struct {
	Recurrence     Recurrence
	Missed         MissedRunPolicy
	Concurrency    ConcurrencyPolicy
	MaxRecipients  int
	Intent         IntentType
	TemplateDigest string
}

func (s ScheduleSpec) Validate() error {
	if err := s.Recurrence.Validate(); err != nil {
		return err
	}
	if !s.Missed.Valid() {
		return fmt.Errorf("invalid missed-run policy")
	}
	if s.Concurrency != ForbidOverlap {
		return fmt.Errorf("unsupported concurrency policy")
	}
	if !s.Intent.Valid() {
		return fmt.Errorf("unsupported intent type")
	}
	if s.TemplateDigest == "" || len(s.TemplateDigest) > 128 {
		return fmt.Errorf("typed intent template digest is required")
	}
	if s.MaxRecipients < 1 || s.MaxRecipients > 500 {
		return fmt.Errorf("recipient bound is invalid")
	}
	return nil
}
func (p MissedRunPolicy) Valid() bool { return p == MissedSkip || p == MissedRunLatest }

type Principal struct{ TenantID, UserID, AgentID string }

func (p Principal) Validate() error {
	for n, v := range map[string]string{"tenant": p.TenantID, "user": p.UserID, "agent": p.AgentID} {
		if strings.TrimSpace(v) == "" || len(v) > 256 {
			return fmt.Errorf("%s principal field is invalid", n)
		}
	}
	return nil
}

type Delegation struct {
	ID                       string
	Version                  uint64
	PrincipalUserID, AgentID string
	Capabilities             []IntentType
	ExpiresAt                time.Time
	Revoked                  bool
	NonTransitive            bool
}

func (d Delegation) ValidateStructure() error {
	if d.ID == "" || len(d.ID) > 256 || d.Version == 0 || d.PrincipalUserID == "" || len(d.PrincipalUserID) > 256 || d.AgentID == "" || len(d.AgentID) > 256 || d.ExpiresAt.IsZero() {
		return fmt.Errorf("delegation identity is invalid")
	}
	if !d.NonTransitive {
		return fmt.Errorf("delegation must be explicitly non-transitive")
	}
	if len(d.Capabilities) == 0 || len(d.Capabilities) > 2 {
		return fmt.Errorf("delegation capabilities are required and bounded")
	}
	seen := map[IntentType]bool{}
	for _, capability := range d.Capabilities {
		if !capability.Valid() || seen[capability] {
			return fmt.Errorf("delegation capability is invalid")
		}
		seen[capability] = true
	}
	return nil
}

func (d Delegation) Validate(at time.Time, intent IntentType) error {
	if err := d.ValidateStructure(); err != nil {
		return err
	}
	if d.Revoked || !at.Before(d.ExpiresAt) {
		return fmt.Errorf("delegation is not active")
	}
	for _, c := range d.Capabilities {
		if c == intent {
			return nil
		}
	}
	return fmt.Errorf("delegation capability denied")
}

type Grant struct {
	ID                                                                   string
	Version                                                              uint64
	PrincipalUserID, WalletBindingID                                     string
	Intent                                                               IntentType
	ScheduleID                                                           string
	ExpiresAt                                                            time.Time
	Paused, Revoked                                                      bool
	PerActionBaseUnits, AggregateCapBaseUnits, RollingWindowCapBaseUnits string
	RollingWindow                                                        time.Duration
	StepUpAboveBaseUnits                                                 string
	AllowedRecipients, AllowedTokens, AllowedChains                      []string
}

func (g Grant) Validate() error {
	if g.ID == "" || g.Version == 0 || g.PrincipalUserID == "" || g.WalletBindingID == "" || !g.Intent.Valid() || g.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid policy grant")
	}
	for n, v := range map[string]string{"per action": g.PerActionBaseUnits, "aggregate": g.AggregateCapBaseUnits, "rolling": g.RollingWindowCapBaseUnits} {
		if v != "" {
			z, ok := new(big.Int).SetString(v, 10)
			if !ok || z.Sign() <= 0 {
				return fmt.Errorf("%s limit must be positive", n)
			}
		}
	}
	if g.RollingWindowCapBaseUnits != "" && (g.RollingWindow <= 0) {
		return fmt.Errorf("rolling window is required")
	}
	for name, values := range map[string][]string{"recipient": g.AllowedRecipients, "token": g.AllowedTokens, "chain": g.AllowedChains} {
		if len(values) > 500 {
			return fmt.Errorf("%s allowlist is too large", name)
		}
		for i, value := range values {
			if value == "" || len(value) > 256 || strings.IndexFunc(value, func(r rune) bool { return r < ' ' }) >= 0 {
				return fmt.Errorf("%s allowlist value is invalid", name)
			}
			if i > 0 && values[i-1] >= value {
				return fmt.Errorf("%s allowlist must be sorted and unique", name)
			}
		}
	}
	return nil
}
func allowed(list []string, value string) bool {
	if len(list) == 0 {
		return true
	}
	i := sort.SearchStrings(list, value)
	return i < len(list) && list[i] == value
}
func (g Grant) Active(at time.Time) bool {
	return g.Validate() == nil && !g.Paused && !g.Revoked && at.Before(g.ExpiresAt)
}

type Schedule struct {
	ID                   string
	Version              uint64
	Principal            Principal
	WalletBindingID      string
	WalletBindingVersion uint64
	GrantID              string
	GrantVersion         uint64
	DelegationID         string
	DelegationVersion    uint64
	CreatedAt, UpdatedAt time.Time
	Status               ScheduleStatus
	Spec                 ScheduleSpec
	Digest               string
}

func (s Schedule) Validate() error {
	if s.ID == "" || s.Version == 0 || s.WalletBindingID == "" || s.WalletBindingVersion == 0 || s.GrantID == "" || s.GrantVersion == 0 || s.DelegationID == "" || s.DelegationVersion == 0 || s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		return fmt.Errorf("invalid schedule identity")
	}
	if err := s.Principal.Validate(); err != nil {
		return err
	}
	if err := s.Spec.Validate(); err != nil {
		return err
	}
	if s.Status != ScheduleActive && s.Status != SchedulePaused && s.Status != ScheduleRevoked {
		return fmt.Errorf("invalid schedule status")
	}
	if s.Digest == "" {
		return fmt.Errorf("schedule digest is required")
	}
	if s.Digest != s.ComputeDigest() {
		return fmt.Errorf("schedule digest does not match immutable schedule material")
	}
	return nil
}
func (s Schedule) ComputeDigest() string {
	r := s.Spec.Recurrence
	material := fmt.Sprintf("%s|%d|%s|%s|%s|%d|%s|%d|%s|%d|%s|%s|%s|%s|%d|%d|%s|%s|%s", s.ID, s.Version, s.Principal.TenantID, s.Principal.UserID, s.WalletBindingID, s.WalletBindingVersion, s.GrantID, s.GrantVersion, s.DelegationID, s.DelegationVersion, s.Spec.Intent, r.Frequency, r.Start.UTC().Format(time.RFC3339Nano), r.End.UTC().Format(time.RFC3339Nano), r.Weekday, r.DayOfMonth, r.Location, s.Spec.Missed, s.Spec.Concurrency)
	material += fmt.Sprintf("|%d|%s", s.Spec.MaxRecipients, s.Spec.TemplateDigest)
	h := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(h[:])
}

type OccurrenceStatus string

const (
	OccurrenceDue              OccurrenceStatus = "DUE"
	OccurrenceClaimed          OccurrenceStatus = "CLAIMED"
	OccurrenceBlocked          OccurrenceStatus = "BLOCKED"
	OccurrenceApprovalRequired OccurrenceStatus = "APPROVAL_REQUIRED"
	OccurrenceDispatched       OccurrenceStatus = "DISPATCHED"
	OccurrenceReconcile        OccurrenceStatus = "RECONCILIATION_ONLY"
	OccurrenceCompleted        OccurrenceStatus = "COMPLETED"
	OccurrenceSkipped          OccurrenceStatus = "SKIPPED"
)

type Occurrence struct {
	ID, Key, ScheduleID, ScheduleDigest, GrantID string
	GrantVersion                                 uint64
	ScheduleVersion                              uint64
	ScheduledAt, CreatedAt                       time.Time
	Status                                       OccurrenceStatus
	LeaseOwner                                   string
	LeaseUntil                                   time.Time
	Fence                                        uint64
	IntentID, ApprovalID, ExecutionID            string
	Reason                                       ReasonCode
}

func (o Occurrence) Validate() error {
	if o.ID == "" || o.Key == "" || o.ScheduleID == "" || o.ScheduleVersion == 0 || o.ScheduleDigest == "" || o.GrantID == "" || o.GrantVersion == 0 || o.ScheduledAt.IsZero() || o.CreatedAt.IsZero() || o.Fence == 0 {
		return fmt.Errorf("occurrence identity is invalid")
	}
	valid := map[OccurrenceStatus]bool{OccurrenceDue: true, OccurrenceClaimed: true, OccurrenceBlocked: true, OccurrenceApprovalRequired: true, OccurrenceDispatched: true, OccurrenceReconcile: true, OccurrenceCompleted: true, OccurrenceSkipped: true}
	if !valid[o.Status] {
		return fmt.Errorf("occurrence status is invalid")
	}
	expectedKey := fmt.Sprintf("%s/%d/%s", o.ScheduleID, o.ScheduleVersion, o.ScheduledAt.UTC().Format(time.RFC3339Nano))
	if o.Key != expectedKey {
		return fmt.Errorf("occurrence key is invalid")
	}
	h := sha256.Sum256([]byte(expectedKey))
	if o.ID != "occ_"+hex.EncodeToString(h[:16]) {
		return fmt.Errorf("occurrence ID is invalid")
	}
	if o.Reason != "" && !o.Reason.Valid() {
		return fmt.Errorf("occurrence reason is invalid")
	}
	return nil
}

func NewOccurrence(s Schedule, at time.Time) Occurrence {
	key := fmt.Sprintf("%s/%d/%s", s.ID, s.Version, at.UTC().Format(time.RFC3339Nano))
	h := sha256.Sum256([]byte(key))
	return Occurrence{ID: "occ_" + hex.EncodeToString(h[:16]), Key: key, ScheduleID: s.ID, ScheduleDigest: s.Digest, ScheduleVersion: s.Version, GrantID: s.GrantID, GrantVersion: s.GrantVersion, ScheduledAt: at.UTC(), CreatedAt: time.Now().UTC(), Status: OccurrenceDue, Fence: 1}
}

type ReasonCode string

func (r ReasonCode) Valid() bool {
	switch r {
	case ReasonEligible, ReasonRuntimeDisabled, ReasonSchedulePaused, ReasonGrantDenied, ReasonDelegationDenied, ReasonEmergencyStop, ReasonStepUp, ReasonOverlap, ReasonProviderUnavailable, ReasonAlreadySubmitted:
		return true
	default:
		return false
	}
}

const (
	ReasonEligible            ReasonCode = "ELIGIBLE"
	ReasonRuntimeDisabled     ReasonCode = "RUNTIME_DISABLED"
	ReasonSchedulePaused      ReasonCode = "SCHEDULE_PAUSED"
	ReasonGrantDenied         ReasonCode = "GRANT_DENIED"
	ReasonDelegationDenied    ReasonCode = "DELEGATION_DENIED"
	ReasonEmergencyStop       ReasonCode = "EMERGENCY_STOP"
	ReasonStepUp              ReasonCode = "STEP_UP_REQUIRED"
	ReasonOverlap             ReasonCode = "OVERLAP_FORBIDDEN"
	ReasonProviderUnavailable ReasonCode = "CAPABILITY_UNAVAILABLE"
	ReasonAlreadySubmitted    ReasonCode = "ALREADY_SUBMITTED_RECONCILE"
)
