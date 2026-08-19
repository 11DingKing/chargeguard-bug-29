package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
	"chargeguard/internal/storage"
)

func setupTestMailState(t *testing.T, ctx context.Context, store storage.Store, clock *domain.FakeClock, state string) *domain.MailItem {
	t.Helper()
	mail := &domain.MailItem{
		ID: "mail-disp-" + clock.Now().Format("150405"), MailNo: "MND" + clock.Now().Format("150405"),
		RouteID: "R001", VehicleNo: "V001", State: state,
		OriginStation: "JMS-01", DestStation: "JMS-02",
		SenderName: "S", ReceiverName: "R", Responsible: "test",
		RegisteredAt: clock.Now(), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.MailItemRepo().Save(ctx, mail))
	return mail
}

func TestDispositionConflictResolution(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewDispositionService(store, clock, bus)
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	disp1, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
		MailID: mail.ID, Type: domain.DispositionTypeIntercept, SubmittedBy: "station-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStatePending, disp1.State)
	disp2, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
		MailID: mail.ID, Type: domain.DispositionTypeReturn, SubmittedBy: "another-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStateLost, disp2.State)
	assert.NotEmpty(t, disp2.ConflictReason)
}

func TestConcurrentDispositionRace(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewDispositionService(store, clock, bus)
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	var wg sync.WaitGroup
	var pending, lost int64
	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			disp, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
				MailID: mail.ID, Type: domain.DispositionTypeIntercept,
				SubmittedBy: "concurrent-op",
				RequestNo:   "REQ-" + itoa(idx),
			})
			if err != nil {
				return
			}
			if disp.State == domain.DispositionStatePending {
				atomic.AddInt64(&pending, 1)
			} else if disp.State == domain.DispositionStateLost {
				atomic.AddInt64(&lost, 1)
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int64(1), pending, "exactly one disposition should be pending")
	assert.Equal(t, int64(goroutines-1), lost, "all others should be lost")
}

func TestAdjudicationWorkflow(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewDispositionService(store, clock, bus)
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	disp, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
		MailID: mail.ID, Type: domain.DispositionTypeRedirect,
		TargetAddress: "new address", SubmittedBy: "station-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStatePending, disp.State)
	disp, err = dispSvc.Review(ctx, ReviewRequest{
		DispositionID: disp.ID, Reviewer: "adjudicator", Decision: "approve", Note: "approved",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStateIssued, disp.State)
	disp, err = dispSvc.Execute(ctx, disp.ID, "system")
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStateCompleted, disp.State)
	updated, _ := store.MailItemRepo().Get(ctx, mail.ID)
	assert.Equal(t, domain.MailStateRedirected, updated.State)
	assert.Equal(t, "new address", updated.ReceiverAddr)
}

func TestWithdrawAndRetry(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewDispositionService(store, clock, bus)
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	disp, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
		MailID: mail.ID, Type: domain.DispositionTypeIntercept, SubmittedBy: "station-op",
	})
	require.NoError(t, err)
	disp, err = dispSvc.Review(ctx, ReviewRequest{
		DispositionID: disp.ID, Reviewer: "adjudicator", Decision: "approve", Note: "ok",
	})
	require.NoError(t, err)
	disp, err = dispSvc.Withdraw(ctx, WithdrawRequest{
		DispositionID: disp.ID, WithdrawnBy: "adjudicator", Reason: "wrong request",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionStateWithdrawn, disp.State)
	active, err := dispSvc.GetActiveByMail(ctx, mail.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, len(active), "no active dispositions after withdrawal")
}

func TestRejectDispositionInvalidDecision(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewDispositionService(store, clock, bus)
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	disp, err := dispSvc.Submit(ctx, SubmitDispositionRequest{
		MailID: mail.ID, Type: domain.DispositionTypeIntercept, SubmittedBy: "op",
	})
	require.NoError(t, err)
	_, err = dispSvc.Review(ctx, ReviewRequest{
		DispositionID: disp.ID, Reviewer: "adjudicator", Decision: "invalid",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrValidation))
}
