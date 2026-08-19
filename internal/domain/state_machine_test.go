package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidMailTransition(t *testing.T) {
	err := ValidateMailTransition(MailStateRegistered, MailStateCompleted)
	require.Error(t, err)
	assert.True(t, errIs(err, ErrInvalidTransition))
}

func TestIllegalHandoverTransition(t *testing.T) {
	err := ValidateHandoverTransition(HandoverStateVoided, HandoverStateDualSigned)
	require.Error(t, err)
	var te TransitionError
	assert.True(t, errAs(err, &te))
	assert.Equal(t, HandoverStateVoided, te.From)
	assert.Equal(t, HandoverStateDualSigned, te.To)
}

func TestRejectDispositionTransition(t *testing.T) {
	err := ValidateDispositionTransition(DispositionStateCompleted, DispositionStatePending)
	require.Error(t, err)
	assert.True(t, errIs(err, ErrInvalidTransition))
}

func TestValidMailTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{MailStateRegistered, MailStateLoaded},
		{MailStateLoaded, MailStateInTransit},
		{MailStateInTransit, MailStateArrived},
		{MailStateArrived, MailStateDualSigned},
		{MailStateDualSigned, MailStateCompleted},
		{MailStateInTransit, MailStateIntercepted},
		{MailStateIntercepted, MailStateReturnedToSender},
	}
	for _, c := range cases {
		err := ValidateMailTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestValidHandoverTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{HandoverStateDraft, HandoverStateOutboundSigned},
		{HandoverStateOutboundSigned, HandoverStateDualSigned},
		{HandoverStateDraft, HandoverStateArrivalSigned},
		{HandoverStateArrivalSigned, HandoverStateDualSigned},
	}
	for _, c := range cases {
		err := ValidateHandoverTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestValidDispositionTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{DispositionStatePending, DispositionStateUnderReview},
		{DispositionStateUnderReview, DispositionStateIssued},
		{DispositionStateIssued, DispositionStateExecuting},
		{DispositionStateExecuting, DispositionStateCompleted},
		{DispositionStateIssued, DispositionStateWithdrawn},
	}
	for _, c := range cases {
		err := ValidateDispositionTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestHandoverNextStateAfterSignoff(t *testing.T) {
	assert.Equal(t, HandoverStateOutboundSigned,
		HandoverNextStateAfterSignoff(HandoverStateDraft, SignoffPartyOutbound))
	assert.Equal(t, HandoverStateDualSigned,
		HandoverNextStateAfterSignoff(HandoverStateOutboundSigned, SignoffPartyArrival))
	assert.Equal(t, HandoverStateDualSigned,
		HandoverNextStateAfterSignoff(HandoverStateArrivalSigned, SignoffPartyOutbound))
	assert.Equal(t, HandoverStateOutboundSigned,
		HandoverNextStateAfterSignoff(HandoverStateOutboundSigned, SignoffPartyOutbound))
}

func TestBatchTransitions(t *testing.T) {
	require.NoError(t, ValidateBatchTransition(BatchStatePending, BatchStateProcessing))
	require.NoError(t, ValidateBatchTransition(BatchStateProcessing, BatchStateSucceeded))
	require.NoError(t, ValidateBatchTransition(BatchStateProcessing, BatchStateRolledBack))
	require.NoError(t, ValidateBatchTransition(BatchStateRolledBack, BatchStateProcessing))
	require.Error(t, ValidateBatchTransition(BatchStateCompleted, BatchStateProcessing))
}

func TestIsDispositionActive(t *testing.T) {
	assert.True(t, IsDispositionActive(DispositionStatePending))
	assert.True(t, IsDispositionActive(DispositionStateIssued))
	assert.False(t, IsDispositionActive(DispositionStateCompleted))
	assert.False(t, IsDispositionActive(DispositionStateRejected))
}

func errIs(err, target error) bool {
	return errorIs(err, target)
}

func errAs(err error, target any) bool {
	return errorAs(err, target)
}
