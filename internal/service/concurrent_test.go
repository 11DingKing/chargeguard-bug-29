package service

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
)

func TestConcurrentMailUpdateRace(t *testing.T) {
	store, clock, _, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	current, err := store.MailItemRepo().Get(ctx, mail.ID)
	require.NoError(t, err)
	expectedState := current.State
	expectedVersion := current.Version
	var wg sync.WaitGroup
	var success, conflict int64
	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := store.BeginTx(ctx)
			if err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			defer tx.Rollback()
			err = store.MailItemRepo().UpdateStateTx(ctx, tx, mail.ID,
				expectedState, domain.MailStateArrived, expectedVersion)
			if err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			if err := tx.Commit(); err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			atomic.AddInt64(&success, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), success, "exactly one update should succeed")
	assert.Equal(t, int64(goroutines-1), conflict, "all others should conflict")
}

func TestParallelHandoverSignoffRace(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form := registerTestHandover(t, ctx, handSvc, "F-CONC", "2026-08-19", "R001")
	form, _ = handSvc.SignoffWithLock(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "op1", Station: "JMS-01",
	})
	var wg sync.WaitGroup
	var success, fail int64
	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := handSvc.SignoffWithLock(ctx, domain.HandoverSignoffRequest{
				FormID: form.ID, Party: domain.SignoffPartyArrival, Signer: "arrival-op", Station: "JMS-02",
			})
			if err != nil {
				atomic.AddInt64(&fail, 1)
			} else {
				atomic.AddInt64(&success, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), success, "exactly one arrival signoff should succeed")
	assert.True(t, fail > 0, "others should fail")
}
