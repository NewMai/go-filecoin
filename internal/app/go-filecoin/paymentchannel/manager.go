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

// the default number of blocks after which the payment channel can be settled
var defaultSettleIncrement = abi.ChainEpoch(1)

var defaultMessageValue = types.NewAttoFILFromFIL(0) // default value for messages sent from this manager
var defaultGasPrice = types.NewAttoFILFromFIL(1)     // default gas price for messages sent from this manager
var defaultGasLimit = types.GasUnits(300)            // default gas limit for messages sent from this manager
var zeroAmt = abi.NewTokenAmount(0)

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
func (pm *Manager) AllocateLane(paychAddr address.Address) (laneID uint64, err error) {
	err = pm.paymentChannels.
		Get(paychAddr).
		Mutate(func(chinfo *ChannelInfo) error {
			laneID = chinfo.LastLane
			chinfo.LastLane++
			return nil
		})
	return laneID, err
}

// GetPaymentChannelByAccounts looks up a payment channel via payer/payee
func (pm *Manager) GetPaymentChannelByAccounts(payer, payee address.Address) (address.Address, *ChannelInfo) {
	panic("implement me")
}

// GetPaymentChannelInfo retrieves channel info from the paymentChannels
func (pm *Manager) GetPaymentChannelInfo(paychAddr address.Address) (*ChannelInfo, error) {
	ss := pm.paymentChannels.Get(paychAddr)
	if ss == nil {
		return nil, nil
	}
	var chinfo ChannelInfo
	if err := ss.Get(&chinfo); err != nil {
		return nil, err
	}
	return &chinfo, nil
}

// CreatePaymentChannel will send the message to the InitActor to create a paych.Actor.
// If successful, a new payment channel entry will be persisted to the
// paymentChannels via a message wait handler
func (pm *Manager) CreatePaymentChannel(clientAddress, minerAddress address.Address) error {
	execParams, err := PaychActorCtorExecParamsFor(clientAddress, minerAddress)
	if err != nil {
		return err
	}
	msgCid, _, err := pm.sender.Send(
		pm.ctx,
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
	err = pm.waiter.Wait(pm.ctx, msgCid, pm.handlePaychActorCtorMessageResult)
	if err != nil {
		return err
	}
	return nil
}

// saveNewChannelInfo creates a new ChannelInfo entry in the paymentchannel store.
// It assumes that a paychActor exists for paymentChannel, or returns an error
func (pm *Manager) saveNewChannelInfo(paymentChannel address.Address) (*ChannelInfo, error) {
	return nil, nil
}

// CreateVoucher creates a signed voucher for the paymentActor, saves to store and
// returns any error. Called by the retrieval market client
func (pm *Manager) CreateVoucher(paychAddr address.Address, voucher *paychActor.SignedVoucher) error {

	//chinfo, err := pm.paymentChannels.getChannelInfo(paychAddr)
	//if err != nil {
	//	return err
	//}
	// save voucher, secret, proof, msgCid in paymentChannels
	return nil
}

// SaveVoucher saves voucher to the store
// called by the retrieval market provider, when it has received a new voucher from the client
// Expects that a payment channel record has already been saved to store.
func (pm *Manager) SaveVoucher(paychAddr address.Address, voucher *paychActor.SignedVoucher, proof []byte, expected abi.TokenAmount) (actual abi.TokenAmount, err error) {
	has, err := pm.paymentChannels.Has(paychAddr)
	if err != nil {
		return zeroAmt, err
	}

	var chinfo *ChannelInfo
	if !has {
		chinfo, err = pm.saveNewChannelInfo(paychAddr)
		if err != nil {
			return zeroAmt, err
		}
	} else {
		st := pm.paymentChannels.Get(paychAddr)
		err = st.Get(chinfo)
		if err != nil {
			return zeroAmt, err
		}
	}

	if pm.hasVoucher(info, voucher) {
		return zeroAmt, xerrors.Errorf("voucher already saved %v", voucher)
	}
	err = pm.paymentChannels.Get(paychAddr).Mutate(func(chinfo *ChannelInfo) error {
		chinfo.Vouchers = append(chinfo.Vouchers, &VoucherInfo{
			Voucher: &ucp.Sv,
			Proof:   ucp.Proof,
		})
		return nil
	})

	return voucher.Amount, err
}

func (pm *Manager) hasVoucher(info *ChannelInfo, voucher *paychActor.SignedVoucher) bool {
	for _, v := range info.Vouchers {
		if v.Voucher == voucher {
			return true
		}
	}
	return false
}

// ChannelExists returns whether paychAddr has a store entry, + error
func (pm *Manager) ChannelExists(paychAddr address.Address) (bool, error) {
	return pm.paymentChannels.Has(paychAddr)
}

// handlePaychActorCtorMessageResult creates a payment channel record in the store if
// the paychActor constructor message was successfully applied
func (pm *Manager) handlePaychActorCtorMessageResult(b *block.Block, sm *types.SignedMessage, mr *vm.MessageReceipt) error {
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

	var ctorParams *paychActor.ConstructorParams
	if err = encoding.Decode(msgParams.ConstructorParams, &ctorParams); err != nil {
		return err
	}

	chinfo := ChannelInfo{IDAddr: res.IDAddress, From: ctorParams.From, To: ctorParams.To}
	return pm.paymentChannels.Begin(res.RobustAddress, &chinfo)
}

// PaychActorCtorExecParamsFor constructs parameters to send a message to InitActor
// To construct a paychActor
func PaychActorCtorExecParamsFor(client, miner address.Address) (initActor.ExecParams, error) {

	ctorParams := paychActor.ConstructorParams{From: client, To: miner}
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
