package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
)

func TestBatchAllSucceedCommit(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	var mailIDs []string
	for i := 0; i < 3; i++ {
		mail := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
		mailIDs = append(mailIDs, mail.ID)
		clock.Advance(1e9)
	}
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		VehicleNo: "V001", Date: "2026-08-19", RouteID: "R001",
		MailIDs: mailIDs,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStatePending, batch.State)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateCompleted, batch.State)
	assert.Equal(t, 3, batch.SucceededCount)
}

func TestBatchRollbackCompensation(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	mail1 := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	clock.Advance(1e9)
	mail2 := setupTestMailState(t, ctx, store, clock, domain.MailStateRegistered)
	clock.Advance(1e9)
	mail3 := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		VehicleNo: "V001", Date: "2026-08-19", RouteID: "R001",
		MailIDs: []string{mail1.ID, mail2.ID, mail3.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateRolledBack, batch.State)
	assert.True(t, batch.FailedCount > 0)
	compensated1, _ := store.MailItemRepo().Get(ctx, mail1.ID)
	assert.Equal(t, domain.MailStateInTransit, compensated1.State, "compensated back to in_transit")
	compensated3, _ := store.MailItemRepo().Get(ctx, mail3.ID)
	assert.Equal(t, domain.MailStateInTransit, compensated3.State, "compensated back to in_transit")
}

func TestBatchRetryIdempotentNotReapplied(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	mail1 := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	clock.Advance(1e9)
	mail2 := setupTestMailState(t, ctx, store, clock, domain.MailStateRegistered)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		VehicleNo: "V001", Date: "2026-08-19", RouteID: "R001",
		MailIDs: []string{mail1.ID, mail2.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateRolledBack, batch.State)
	mail2.State = domain.MailStateInTransit
	mail2.Version++
	require.NoError(t, store.MailItemRepo().Save(ctx, mail2))
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateCompleted, batch.State)
}

func TestBatchCompensationIdempotentDuplicate(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	mail1 := setupTestMailState(t, ctx, store, clock, domain.MailStateInTransit)
	clock.Advance(1e9)
	mail2 := setupTestMailState(t, ctx, store, clock, domain.MailStateRegistered)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		VehicleNo: "V001", Date: "2026-08-19", RouteID: "R001",
		MailIDs: []string{mail1.ID, mail2.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	require.Equal(t, domain.BatchStateRolledBack, batch.State)
	items, _ := batchSvc.ListItems(ctx, batch.ID)
	var rolledBackItem *domain.BatchItem
	for _, item := range items {
		if item.State == domain.BatchItemStateRolledBack {
			rolledBackItem = item
			break
		}
	}
	require.NotNil(t, rolledBackItem)
	batchSvc.CompensateItem(ctx, rolledBackItem)
	mailAfter1, _ := store.MailItemRepo().Get(ctx, mail1.ID)
	assert.Equal(t, domain.MailStateInTransit, mailAfter1.State)
	batchSvc.CompensateItem(ctx, rolledBackItem)
	mailAfter2, _ := store.MailItemRepo().Get(ctx, mail1.ID)
	assert.Equal(t, domain.MailStateInTransit, mailAfter2.State, "second compensation is idempotent")
}
