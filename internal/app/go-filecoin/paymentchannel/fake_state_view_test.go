package paymentchannel_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-filecoin/internal/app/go-filecoin/paymentchannel"
	"github.com/filecoin-project/go-filecoin/internal/pkg/block"
)

type FakeChainReader struct {
	root cid.Cid
	tsk  block.TipSetKey
}

func (f FakeChainReader) GetTipSetStateRoot(context.Context, block.TipSetKey) (cid.Cid, error) {
	return f.root, nil
}

func (f FakeChainReader) Head() block.TipSetKey {
	return f.tsk
}

func NewFakeChainReader(root cid.Cid, tsk block.TipSetKey) *FakeChainReader {
	return &FakeChainReader{root: root, tsk: tsk}
}

var _ paymentchannel.ChainReader = &FakeChainReader{}

// FakeStateViewer mocks a state viewer for payment channel actor testing
type FakeStateViewer struct {
	Views map[cid.Cid]*FakeStateView
}

// StateView mocks fetching a real state view
func (f *FakeStateViewer) StateView(root cid.Cid) paymentchannel.PaychActorStateView {
	return f.Views[root]
}

// FakeStateView is a mock version of a state view for payment channel actors
type FakeStateView struct {
	t                    *testing.T
	actors               map[address.Address]*FakePaychActorState
	PaychActorPartiesErr error
}

func NewFakeStateView(t *testing.T, viewErr error) *FakeStateView {
	return &FakeStateView{
		t:                    t,
		actors:               make(map[address.Address]*FakePaychActorState),
		PaychActorPartiesErr: viewErr,
	}
}

// PaychActorParties mocks returning the From and To addrs of a paych.Actor
func (f *FakeStateView) PaychActorParties(_ context.Context, paychAddr address.Address) (from, to address.Address, err error) {
	st, ok := f.actors[paychAddr]
	if !ok {
		f.t.Fatalf("actor does not exist %s", paychAddr.String())
	}
	return st.From, st.To, f.PaychActorPartiesErr
}

func (f *FakeStateView) AddActorWithState(actorAddr, from, to address.Address) {
	f.actors[actorAddr] = &FakePaychActorState{to, from}
}

var _ paymentchannel.PaychActorStateView = &FakeStateView{}

// FakePaychActorState is a mock actor state containing test info
type FakePaychActorState struct {
	To, From address.Address
}
