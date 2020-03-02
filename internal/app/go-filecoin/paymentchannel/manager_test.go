package paymentchannel

import (
	"context"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	spect "github.com/filecoin-project/specs-actors/support/testing"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_CreatePaymentChannel(t *testing.T) {
	ds := datastore.NewMapDatastore()
	ctx := context.Background()
	testAPI := NewFakePaymentChannelAPI(ctx, t)

	m := NewManager(context.Background(), ds, testAPI, testAPI)

	clientAddr := spect.NewIDAddr(t, 901)
	minerAddr := spect.NewIDAddr(t, 902)
	paychIDAddr := spect.NewIDAddr(t, 999)
	paychUniqueAddr := spect.NewActorAddr(t, "paych")
	blockHeight := uint64(1234)

	testAPI.StubMessage(clientAddr, builtin.InitActorAddr, paychIDAddr, paychUniqueAddr, builtin.MethodsInit.Exec, exitcode.Ok, blockHeight)

	err := m.CreatePaymentChannel(clientAddr, minerAddr)
	require.NoError(t, err)
	testAPI.Verify()

	chinfo, err := m.GetPaymentChannelInfo(paychUniqueAddr)
	require.NoError(t, err)
	require.NotNil(t, chinfo)
	assert.Equal(t, clientAddr, chinfo.State.From)
}
func TestManager_AllocateLane(t *testing.T) {
	ds := datastore.NewMapDatastore()
	ctx := context.Background()
	testApi := NewFakePaymentChannelAPI(ctx, t)

	m := NewManager(context.Background(), ds, testApi, testApi)
	paychIDAddr := spect.NewIDAddr(t, 999)

	t.Run("saves a new lane", func(t *testing.T) {
		lane, err := m.AllocateLane(paychIDAddr)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), lane)
	})
	t.Run("updates a lane", func(t *testing.T) {})
	t.Run("errors if update lane doesn't exist", func(t *testing.T) {})
	t.Run("errors if create lane fails", func(t *testing.T) {})
}
