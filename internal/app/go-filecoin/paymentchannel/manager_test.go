package paymentchannel_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	spect "github.com/filecoin-project/specs-actors/support/testing"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/filecoin-project/go-filecoin/internal/app/go-filecoin/paymentchannel"
	"github.com/filecoin-project/go-filecoin/internal/app/go-filecoin/plumbing/cst"
	"github.com/filecoin-project/go-filecoin/internal/pkg/block"
	"github.com/filecoin-project/go-filecoin/internal/pkg/crypto"
)

func TestManager_CreatePaymentChannel(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	ctx := context.Background()
	testAPI := NewFakePaymentChannelAPI(ctx, t)
	root := shared_testutil.GenerateCids(1)[0]
	viewer := makeStateViewer(t, root, nil)
	m := NewManager(context.Background(), ds, testAPI, testAPI, viewer, &cst.ChainStateReadWriter{})

	t.Run("happy path", func(t *testing.T) {
		clientAddr, minerAddr, paychIDAddr, paychUniqueAddr, _ := requireSetupPaymentChannel(t, testAPI, m)
		exists, err := m.ChannelExists(paychUniqueAddr)
		require.NoError(t, err)
		assert.True(t, exists)

		chinfo, err := m.GetPaymentChannelInfo(paychUniqueAddr)
		require.NoError(t, err)
		require.NotNil(t, chinfo)
		expectedChinfo := ChannelInfo{
			LastLane: 0,
			From:     clientAddr,
			To:       minerAddr,
			IDAddr:   paychIDAddr,
		}
		assert.Equal(t, expectedChinfo, *chinfo)
	})
	testCases := []struct {
		name             string
		waitErr, sendErr error
		expErr           string
	}{
		{name: "returns err and does nto create channel if Send fails", sendErr: errors.New("sendboom"), expErr: "sendboom"},
		{name: "returns err and does nto create channel if Wait fails", waitErr: errors.New("waitboom"), expErr: "waitboom"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testAPI.MsgSendErr = tc.sendErr
			testAPI.MsgWaitErr = tc.waitErr
			clientAddr := spect.NewIDAddr(t, 901)
			minerAddr := spect.NewIDAddr(t, 902)
			paychIDAddr := spect.NewIDAddr(t, 999)
			paychUniqueAddr := spect.NewActorAddr(t, "paych")
			blockHeight := uint64(1234)

			testAPI.StubCreatePaychActorMessage(clientAddr, minerAddr, paychIDAddr, paychUniqueAddr, builtin.MethodsInit.Exec, exitcode.Ok, blockHeight)

			err := m.CreatePaymentChannel(clientAddr, minerAddr)
			assert.EqualError(t, err, tc.expErr)
		})
	}
}

func TestManager_AllocateLane(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	ctx := context.Background()
	testAPI := NewFakePaymentChannelAPI(ctx, t)

	root := shared_testutil.GenerateCids(1)[0]
	viewer := makeStateViewer(t, root, nil)
	m := NewManager(context.Background(), ds, testAPI, testAPI, viewer, &cst.ChainStateReadWriter{})
	clientAddr, minerAddr, paychIDAddr, paychUniqueAddr, _ := requireSetupPaymentChannel(t, testAPI, m)

	t.Run("saves a new lane", func(t *testing.T) {
		lane, err := m.AllocateLane(paychUniqueAddr)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), lane)

		chinfo, err := m.GetPaymentChannelInfo(paychUniqueAddr)
		require.NoError(t, err)
		require.NotNil(t, chinfo)
		expectedChinfo := ChannelInfo{
			LastLane: 1,
			From:     clientAddr,
			To:       minerAddr,
			IDAddr:   paychIDAddr,
		}

		assert.Equal(t, expectedChinfo, *chinfo)
	})

	t.Run("errors if update lane doesn't exist", func(t *testing.T) {
		badAddr := spect.NewActorAddr(t, "nonexistent")
		lane, err := m.AllocateLane(badAddr)
		expErr := fmt.Sprintf("No state for /%s", badAddr.String())
		assert.EqualError(t, err, expErr)
		assert.Zero(t, lane)
	})
}

// CreateVoucher is called by a retrieval client
func TestManager_CreateVoucher(t *testing.T) {

}

// SaveVoucher is called by a retrieval provider
func TestManager_SaveVoucher(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	ctx := context.Background()
	testAPI := NewFakePaymentChannelAPI(ctx, t)

	paychUniqueAddr := spect.NewActorAddr(t, "abcd123")
	clientAddr := spect.NewIDAddr(t, 99)
	minerAddr := spect.NewIDAddr(t, 100)
	root := shared_testutil.GenerateCids(1)[0]
	cr := NewFakeChainReader(root, block.NewTipSetKey(root))

	viewer := makeStateViewer(t, root, nil)
	viewer.Views[root].AddActorWithState(paychUniqueAddr, clientAddr, minerAddr)

	m := NewManager(context.Background(), ds, testAPI, testAPI, viewer, cr)

	// SaveVoucher is called by a provider
	t.Run("happy path", func(t *testing.T) {
		amt := big.NewInt(300)
		sig := crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: []byte("doesntmatter")}
		v := paych.SignedVoucher{
			Nonce:       2,
			TimeLockMax: abi.ChainEpoch(12345),
			TimeLockMin: abi.ChainEpoch(12346),
			Lane:        2,
			Amount:      amt,
			Signature:   &sig,
		}
		proof := []byte("some proof")
		resAmt, err := m.SaveVoucher(paychUniqueAddr, &v, proof, amt)
		require.NoError(t, err)
		assert.Equal(t, amt, resAmt)

		has, err := m.ChannelExists(paychUniqueAddr)
		require.NoError(t, err)
		assert.True(t, has)
		//chinfo, err := m.GetPaymentChannelInfo(paychUniqueAddr)
		//require.NoError(t, err)
		//require.NotNil(t, chinfo)
		//assert.Len(t, chinfo.Vouchers, 1)
		//assert.Equal(t, v, chinfo.Vouchers[0])
	})

}

func requireSetupPaymentChannel(t *testing.T, testAPI *FakePaymentChannelAPI, m *Manager) (address.Address, address.Address, address.Address, address.Address, uint64) {
	clientAddr := spect.NewIDAddr(t, 901)
	minerAddr := spect.NewIDAddr(t, 902)
	paychIDAddr := spect.NewIDAddr(t, 999)
	paychUniqueAddr := spect.NewActorAddr(t, "abcd123")
	blockHeight := uint64(1234)

	testAPI.StubCreatePaychActorMessage(clientAddr, minerAddr, paychIDAddr, paychUniqueAddr, builtin.MethodsInit.Exec, exitcode.Ok, blockHeight)

	err := m.CreatePaymentChannel(clientAddr, minerAddr)
	require.NoError(t, err)
	testAPI.Verify()
	return clientAddr, minerAddr, paychIDAddr, paychUniqueAddr, blockHeight
}

func makeStateViewer(t *testing.T, stateRoot cid.Cid, viewErr error) *FakeStateViewer {
	return &FakeStateViewer{
		Views: map[cid.Cid]*FakeStateView{stateRoot: NewFakeStateView(t, viewErr)}}
}
