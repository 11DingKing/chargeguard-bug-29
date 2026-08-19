package sla

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
)

func newMail(routeID string, inTransitAt time.Time) *domain.MailItem {
	return &domain.MailItem{
		ID: "m-" + routeID, MailNo: "MN-" + routeID, RouteID: routeID,
		State: domain.MailStateInTransit, InTransitAt: inTransitAt,
		ShardID: domain.ShardIDFor("2026-08-19", routeID), DataVersion: 1,
	}
}

func TestSLAClassifyOnTime(t *testing.T) {
	rs := NewRuleSet(48)
	rs.Add(&Rule{RouteID: "R001", MaxTransitHours: 24})
	require.NoError(t, rs.Activate("R001"))
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	now := start.Add(20 * time.Hour)
	assert.Equal(t, ClassOnTime, rs.Classify(newMail("R001", start), now))
}

func TestSLAClassifyOverdueAndCritical(t *testing.T) {
	rs := NewRuleSet(48)
	rs.Add(&Rule{RouteID: "R002", MaxTransitHours: 12})
	require.NoError(t, rs.Activate("R002"))
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	overdueNow := start.Add(18 * time.Hour)  // 12 < 18 <= 24
	criticalNow := start.Add(30 * time.Hour) // 30 > 24
	assert.Equal(t, ClassOverdue, rs.Classify(newMail("R002", start), overdueNow))
	assert.Equal(t, ClassCritical, rs.Classify(newMail("R002", start), criticalNow))
}

func TestSLAClassifyBoundary(t *testing.T) {
	rs := NewRuleSet(0)
	rs.Add(&Rule{RouteID: "RB", MaxTransitHours: 10})
	require.NoError(t, rs.Activate("RB"))
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	// exactly at the ceiling -> on_time (closed lower bucket)
	assert.Equal(t, ClassOnTime, rs.Classify(newMail("RB", start), start.Add(10*time.Hour)))
	// one tick over the ceiling -> overdue
	assert.Equal(t, ClassOverdue, rs.Classify(newMail("RB", start), start.Add(10*time.Hour+time.Second)))
	// exactly at twice the ceiling -> overdue (not critical)
	assert.Equal(t, ClassOverdue, rs.Classify(newMail("RB", start), start.Add(20*time.Hour)))
	// one tick over twice the ceiling -> critical
	assert.Equal(t, ClassCritical, rs.Classify(newMail("RB", start), start.Add(20*time.Hour+time.Second)))
}

func TestSLAUnknownRouteFallbacksToDefault(t *testing.T) {
	rs := NewRuleSet(6) // no rules registered; default applies
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	assert.Equal(t, ClassOnTime, rs.Classify(newMail("RX", start), start.Add(5*time.Hour)))
	assert.Equal(t, ClassOverdue, rs.Classify(newMail("RX", start), start.Add(8*time.Hour)))
	assert.Equal(t, ClassCritical, rs.Classify(newMail("RX", start), start.Add(20*time.Hour)))
}

func TestSLAClassifyNotInTransit(t *testing.T) {
	rs := NewRuleSet(24)
	rs.Add(&Rule{RouteID: "R003", MaxTransitHours: 24})
	require.NoError(t, rs.Activate("R003"))
	mail := newMail("R003", time.Now())
	mail.State = domain.MailStateArrived
	assert.Equal(t, ClassNotInTransit, rs.Classify(mail, time.Now()))
	// nil mail is safe
	assert.Equal(t, ClassNotInTransit, rs.Classify(nil, time.Now()))
	// in-transit but no recorded start time
	mail2 := newMail("R003", time.Time{})
	mail2.State = domain.MailStateInTransit
	assert.Equal(t, ClassUnknownRoute, rs.Classify(mail2, time.Now()))
}

func TestSLARuleStateMachineValidTransitions(t *testing.T) {
	rs := NewRuleSet(48)
	rs.Add(&Rule{RouteID: "R004", MaxTransitHours: 24})
	require.NoError(t, rs.Activate("R004"))
	require.NoError(t, rs.Deprecate("R004"))
	// deprecated rule stops governing classification
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	assert.Equal(t, ClassOnTime, rs.Classify(newMail("R004", start), start.Add(5*time.Hour)))   // default 48h
	require.NoError(t, rs.Activate("R004"))                                                     // re-activate
	assert.Equal(t, ClassOverdue, rs.Classify(newMail("R004", start), start.Add(30*time.Hour))) // 24h rule again
}

func TestSLARuleStateMachineRejectsInvalidTransition(t *testing.T) {
	rs := NewRuleSet(48)
	rs.Add(&Rule{RouteID: "R005", MaxTransitHours: 24})
	// draft -> deprecated is not a legal edge
	err := rs.Deprecate("R005")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidTransition), "expected illegal transition, got: %v", err)
	// rule remains draft and un-activated, so default applies
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	assert.Equal(t, ClassOnTime, rs.Classify(newMail("R005", start), start.Add(10*time.Hour)))
}

func TestSLAActivateUnknownRouteReturnsNotFound(t *testing.T) {
	rs := NewRuleSet(48)
	err := rs.Activate("no-such-route")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "expected not-found, got: %v", err)
}

func TestSLAConcurrentAddAndClassify(t *testing.T) {
	rs := NewRuleSet(12)
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			routeID := fmt.Sprintf("RC%02d", i%10)
			rs.Add(&Rule{RouteID: routeID, MaxTransitHours: 6 + (i % 5)})
			_ = rs.Activate(routeID)
			_ = rs.Classify(newMail(routeID, start), start.Add(time.Duration(i)*time.Hour))
		}(i)
	}
	wg.Wait()
	// after concurrent churn, classification must be deterministic & non-fatal
	class := rs.Classify(newMail("RC00", start), start.Add(2*time.Hour))
	assert.Contains(t, []string{ClassOnTime, ClassOverdue}, class)
}

func TestSLASummary(t *testing.T) {
	rs := NewRuleSet(48)
	rs.Add(&Rule{RouteID: "RS", MaxTransitHours: 10})
	require.NoError(t, rs.Activate("RS"))
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	mails := []*domain.MailItem{
		newMail("RS", start),                    // on-time at +5h
		newMail("RS", start.Add(-20*time.Hour)), // critical
		newMail("RX", start),                    // default 48h, on-time
	}
	sum := rs.Summary(mails, start.Add(5*time.Hour))
	assert.Equal(t, 2, sum[ClassOnTime])
	assert.Equal(t, 1, sum[ClassCritical])
}
