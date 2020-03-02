package paymentchannel

import (
	"bytes"
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	initActor "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-filecoin/internal/pkg/block"
	"github.com/filecoin-project/go-filecoin/internal/pkg/types"
	"github.com/filecoin-project/go-filecoin/internal/pkg/vm"
)

type FakePaymentChannelAPI struct {
	t   *testing.T
	ctx context.Context

	ActualWaitCid  cid.Cid
	ExpectedMsgCid cid.Cid
	ExpectedMsg    MsgReceipts
	ActualMsg      MsgReceipts

	MsgSendErr error
	MsgWaitErr error
}

type MsgReceipts struct {
	Block  *block.Block
	Msg    *types.SignedMessage
	MsgCid cid.Cid
	Rcpt   *vm.MessageReceipt
}

var msgRcptsUndef = MsgReceipts{}

func NewFakePaymentChannelAPI(ctx context.Context, t *testing.T) *FakePaymentChannelAPI {
	return &FakePaymentChannelAPI{
		t:   t,
		ctx: ctx,
	}
}

// API methods

// Wait mocks waiting for a message to be mined
func (f *FakePaymentChannelAPI) Wait(_ context.Context, msgCid cid.Cid, cb func(*block.Block, *types.SignedMessage, *vm.MessageReceipt) error) error {
	if f.MsgWaitErr != nil {
		return f.MsgWaitErr
	}
	f.ActualWaitCid = msgCid
	return cb(f.ActualMsg.Block, f.ExpectedMsg.Msg, f.ActualMsg.Rcpt)
}

// Send mocks sending a message on chain
func (f *FakePaymentChannelAPI) Send(ctx context.Context,
	from, to address.Address,
	value types.AttoFIL,
	gasPrice types.AttoFIL,
	gasLimit types.GasUnits,
	bcast bool,
	method abi.MethodNum,
	params interface{}) (out cid.Cid, pubErrCh chan error, err error) {

	if f.MsgSendErr != nil {
		return cid.Undef, nil, f.MsgSendErr
	}
	if f.ExpectedMsg == msgRcptsUndef || f.ExpectedMsgCid == cid.Undef {
		f.t.Fatal("no message or no cid registered")
	}

	unsigned := types.NewUnsignedMessage(from, to, 1, value, method, []byte{})
	unsigned.GasPrice = gasPrice
	unsigned.GasLimit = gasLimit
	f.ActualMsg = f.ExpectedMsg
	require.Equal(f.t, *unsigned, f.ExpectedMsg.Msg.Message)

	f.ActualMsg.Msg.Message = *unsigned
	return f.ExpectedMsgCid, nil, nil
}

// testing methods

// StubMessage sets up a message response, with desired exit code and block height
func (f *FakePaymentChannelAPI) StubMessage(from, to, idAddr, uniqueAddr address.Address, method abi.MethodNum, code exitcode.ExitCode, height uint64) {
	newcid := shared_testutil.GenerateCids(1)[0]
	msg := types.NewUnsignedMessage(from, to, 1, types.ZeroAttoFIL, method, []byte{})
	msg.GasPrice = defaultGasPrice
	msg.GasLimit = defaultGasLimit
	f.ExpectedMsgCid = newcid

	res := initActor.ExecReturn{IDAddress: idAddr, RobustAddress: uniqueAddr}
	var buf bytes.Buffer
	if err := res.MarshalCBOR(&buf); err != nil {
		f.t.Fatal(err.Error())
	}

	f.ExpectedMsg = MsgReceipts{
		Block:  &block.Block{Height: abi.ChainEpoch(height)},
		Msg:    &types.SignedMessage{Message: *msg},
		MsgCid: newcid,
		Rcpt:   &vm.MessageReceipt{ExitCode: code, ReturnValue: buf.Bytes()},
	}
}

// Verify compares expected and actual results
func (f *FakePaymentChannelAPI) Verify() {
	assert.True(f.t, f.ExpectedMsgCid.Equals(f.ActualWaitCid))
	assert.True(f.t, f.ExpectedMsg.Msg.Equals(f.ActualMsg.Msg))
}

var _ MsgSender = &FakePaymentChannelAPI{}
var _ MsgWaiter = &FakePaymentChannelAPI{}
