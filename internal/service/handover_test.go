package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
)

func TestDualPartySignoff(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form := registerTestHandover(t, ctx, handSvc, "F001", "2026-08-19", "R001")
	assert.Equal(t, domain.HandoverStateDraft, form.State)
	form, err := handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "outbound-op", Station: "JMS-01",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.HandoverStateOutboundSigned, form.State)
	form, err = handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyArrival, Signer: "arrival-op", Station: "JMS-02",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.HandoverStateDualSigned, form.State)
}

func TestSinglePartySignoffIncomplete(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form := registerTestHandover(t, ctx, handSvc, "F002", "2026-08-19", "R001")
	form, err := handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "outbound-op", Station: "JMS-01",
	})
	require.NoError(t, err)
	assert.False(t, domain.IsHandoverComplete(form.State))
	assert.Equal(t, domain.HandoverStateOutboundSigned, form.State)
}

func TestHandoverIdempotentRegister(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form1 := registerTestHandover(t, ctx, handSvc, "F003", "2026-08-19", "R001")
	assert.NotEmpty(t, form1.ID)
	_, err := handSvc.Register(ctx, RegisterHandoverRequest{
		FormNo: "F003", Date: "2026-08-19", RouteID: "R001",
		MailItems: []RegisterMailItem{{MailNo: "F003-M1"}},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAlreadyExists))
}

func TestHandoverRejectDoubleSignoff(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form := registerTestHandover(t, ctx, handSvc, "F004", "2026-08-19", "R001")
	form, _ = handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "op1", Station: "JMS-01",
	})
	form, _ = handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyArrival, Signer: "op2", Station: "JMS-02",
	})
	_, err := handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "op3", Station: "JMS-01",
	})
	require.Error(t, err)
}

func TestHandoverVoidedRejectsSignoff(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandoverService(store, clock, bus)
	form := registerTestHandover(t, ctx, handSvc, "F005", "2026-08-19", "R001")
	form.State = domain.HandoverStateVoided
	form.Version++
	require.NoError(t, store.HandoverRepo().Save(ctx, form))
	_, err := handSvc.Signoff(ctx, domain.HandoverSignoffRequest{
		FormID: form.ID, Party: domain.SignoffPartyOutbound, Signer: "op1", Station: "JMS-01",
	})
	require.Error(t, err)
}
