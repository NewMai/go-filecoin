package paymentchannel

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	initActor "github.com/filecoin-project/specs-actors/actors/builtin/init"
	paychActor "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	xerrors "github.com/pkg/errors"

	"github.com/filecoin-project/go-filecoin/internal/pkg/block"
	"github.com/filecoin-project/go-filecoin/internal/pkg/encoding"
	"github.com/filecoin-project/go-filecoin/internal/pkg/types"
	"github.com/filecoin-project/go-filecoin/internal/pkg/vm"
)

var defaultMessageValue = types.NewAttoFILFromFIL(0)
var defaultGasPrice = types.NewAttoFILFromFIL(1)
var defaultGasLimit = types.GasUnits(300)

// Manager manages payment channel actor and the data paymentChannels operations.
type Manager struct {
	ctx             context.Context
	paymentChannels *statestore.StateStore
	sender          MsgSender
	waiter          MsgWaiter
}

// PaymentChannelStorePrefix is the prefix used in the datastore
var PaymentChannelStorePrefix = "/retrievaldeals/paymentchannel"

// MsgWaiter is an interface for waiting for a message to appear on chain
type MsgWaiter interface {
	Wait(ctx context.Context, msgCid cid.Cid, cb func(*block.Block, *types.SignedMessage, *vm.MessageReceipt) error) error
}

// MsgSender is an interface for something that can post messages on chain
type MsgSender interface {
	// Send sends a message to the chain
	Send(ctx context.Context,
		from, to address.Address,
		value types.AttoFIL,
		gasPrice types.AttoFIL,
		gasLimit types.GasUnits,
		bcast bool,
		method abi.MethodNum,
		params interface{}) (out cid.Cid, pubErrCh chan error, err error)
}

// NewManager creates and returns a new paymentchannel.Manager
func NewManager(ctx context.Context, ds datastore.Batching, waiter MsgWaiter, sender MsgSender) *Manager {
	store := statestore.New(namespace.Wrap(ds, datastore.NewKey(PaymentChannelStorePrefix)))
	return &Manager{ctx, store, sender, waiter}
}

// AllocateLane adds a new lane to a payment channel entry
func (pm *Manager) AllocateLane(paychAddr address.Address) (uint64, error) {

	return 0, nil
}

// GetPaymentChannelByAccounts looks up a payment channel via payer/payee
func (pm *Manager) GetPaymentChannelByAccounts(payer, payee address.Address) (address.Address, *ChannelInfo) {
	panic("implement me")
}

// GetPaymentChannelInfo retrieves channel info from the paymentChannels
func (pm *Manager) GetPaymentChannelInfo(paychAddr address.Address) (*ChannelInfo, error) {
	return nil, nil
}

// CreatePaymentChannel will send the message to the InitActor to create a paych.Actor.
// If successful, a new payment channel entry will be persisted to the paymentChannels via a message wait handler
func (pm *Manager) CreatePaymentChannel(clientAddress, minerAddress address.Address) error {
	execParams, err := PaychActorCtorExecParamsFor(clientAddress, minerAddress)
	if err != nil {
		return err
	}
	msgCid, _, err := pm.sender.Send(
		context.Background(),
		clientAddress,
		builtin.InitActorAddr,
		defaultMessageValue,
		defaultGasPrice,
		defaultGasLimit,
		true,
		builtin.MethodsInit.Exec,
		execParams,
	)
	if err != nil {
		return err
	}
	err = pm.waiter.Wait(pm.ctx, msgCid, pm.handleCreatePaymentChannelResult)
	if err != nil {
		return err
	}
	return nil
}

// CreateVoucher creates a signed voucher for the paymentActor returns the result
func (pm *Manager) CreateVoucher(paychAddr address.Address, voucher *paychActor.SignedVoucher) error {

	//chinfo, err := pm.paymentChannels.getChannelInfo(paychAddr)
	//if err != nil {
	//	return err
	//}
	// save voucher, secret, proof, msgCid in paymentChannels
	return nil
}

// SaveVoucher stores the voucher in the payment channel store
func (pm *Manager) SaveVoucher(paychAddr address.Address, voucher *paychActor.SignedVoucher, proof []byte, expected abi.TokenAmount) (actual abi.TokenAmount, err error) {
	panic("implement SaveVoucher")
	return abi.NewTokenAmount(0), nil
}

func (pm *Manager) handleUpdatePaymentChannelResult(b *block.Block, sm *types.SignedMessage, mr *vm.MessageReceipt) error {
	// save results in paymentChannels
	panic("implement handleUpdatePaymentChannelResult")
	return nil
}

func (pm *Manager) handleCreatePaymentChannelResult(b *block.Block, sm *types.SignedMessage, mr *vm.MessageReceipt) error {
	var res initActor.ExecReturn
	if err := encoding.Decode(mr.ReturnValue, &res); err != nil {
		return err
	}
	has, err := pm.paymentChannels.Has(res.RobustAddress)
	if err != nil {
		return err
	}
	if has {
		return xerrors.Errorf("channel exists %s", res.RobustAddress)
	}

	var msgParams initActor.ExecParams
	if err := encoding.Decode(sm.Message.Params, &msgParams); err != nil {
		return err
	}


	var ctorParams paychActor.ConstructorParams
	if err = encoding.Decode(msgParams.ConstructorParams, &ctorParams); err != nil {
		return err
	}

	chinfo := ChannelInfo{
		IDAddr: res.IDAddress,
		State: &paychActor.State{
			From:            ctorParams.From,
			To:              ctorParams.To,
			ToSend:          abi.NewTokenAmount(0),
			SettlingAt:      0,
			MinSettleHeight: b.Height + 1,
			LaneStates:      nil,
		},
		Vouchers: nil,
	}
	return pm.paymentChannels.Begin(res.RobustAddress, chinfo)
}

func updatePaymentChannelStateParamsFor(voucher *paychActor.SignedVoucher) (initActor.ExecParams, error) {
	ucp := paychActor.UpdateChannelStateParams{
		Sv: paychActor.SignedVoucher{},
		// TODO secret, proof for UpdatePaymentChanneStateParams
		//Secret: nil,
		//Proof:  nil,
	}
	encoded, err := encoding.Encode(ucp)
	if err != nil {
		return initActor.ExecParams{}, err
	}

	p := initActor.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: encoded,
	}
	return p, nil
}

func PaychActorCtorExecParamsFor(client, miner address.Address) (initActor.ExecParams, error) {
	ctorParams := paychActor.ConstructorParams{
		From: client,
		To:   miner,
	}
	marshaled, err := encoding.Encode(ctorParams)
	if err != nil {
		return initActor.ExecParams{}, err
	}

	p := initActor.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: marshaled,
	}
	return p, nil
}
