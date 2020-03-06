package paymentchannel

import (
	"context"
	"math/rand"

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
	stateViewer     MgrStateViewer
	cr              ChainReader
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

type MgrStateViewer interface {
	StateView(root cid.Cid) PaychActorStateView
}

type PaychActorStateView interface {
	PaychActorParties(ctx context.Context, paychAddr address.Address) (from, to address.Address, err error)
}

type ChainReader interface {
	GetTipSetStateRoot(context.Context, block.TipSetKey) (cid.Cid, error)
	Head() block.TipSetKey
}

// NewManager creates and returns a new paymentchannel.Manager
func NewManager(ctx context.Context, ds datastore.Batching, waiter MsgWaiter, sender MsgSender, viewer MgrStateViewer, cr ChainReader) *Manager {
	store := statestore.New(namespace.Wrap(ds, datastore.NewKey(PaymentChannelStorePrefix)))
	return &Manager{ctx, store, sender, waiter, viewer, cr}
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
// TODO: make this return chinfo + err with a paymentchannels.Get after the Wait completes
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
func (pm *Manager) SaveVoucher(paychAddr address.Address, voucher *paychActor.SignedVoucher, proof []byte, expected abi.TokenAmount) (abi.TokenAmount, error) {
	has, hasErr := pm.paymentChannels.Has(paychAddr)
	if hasErr != nil {
		return zeroAmt, hasErr
	}

	if !has {
		from, to, err := pm.getPaychActorAddrs(paychAddr)
		if err != nil {
			return zeroAmt, err
		}

		tmpAddr, err := address.NewIDAddress(rand.Uint64())
		if err != nil {
			panic("should never happen, temporary")
		}
		chinfo := ChannelInfo{
			From:     from,
			To:       to,
			IDAddr:   tmpAddr, // TODO: look up actor id address
			Vouchers: []*VoucherInfo{{Voucher: voucher, Proof: proof}},
		}
		return voucher.Amount, pm.paymentChannels.Begin(paychAddr, &chinfo)
	}

	var chinfo ChannelInfo
	st := pm.paymentChannels.Get(paychAddr)
	hasErr = st.Get(&chinfo)
	if hasErr != nil {
		return zeroAmt, hasErr
	}
	if pm.hasVoucher(&chinfo, voucher) {
		return zeroAmt, xerrors.Errorf("voucher already saved %v", voucher)
	}
	hasErr = pm.paymentChannels.Get(paychAddr).Mutate(func(info *ChannelInfo) error {
		info.Vouchers = append(info.Vouchers, &VoucherInfo{
			Voucher: voucher,
			Proof:   proof,
		})
		return nil
	})
	return voucher.Amount, hasErr
}

// ChannelExists returns whether paychAddr has a store entry, + error
func (pm *Manager) ChannelExists(paychAddr address.Address) (bool, error) {
	return pm.paymentChannels.Has(paychAddr)
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

// hasVoucher returns true if the voucher is already stored in the channelinfo
func (pm *Manager) hasVoucher(info *ChannelInfo, voucher *paychActor.SignedVoucher) bool {
	for _, v := range info.Vouchers {
		if v.Voucher == voucher {
			return true
		}
	}
	return false
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

// getPaychActorAddrs fetches the payment channel's from and to addresses at the current
// tipset head
func (pm *Manager) getPaychActorAddrs(paychAddr address.Address) (from, to address.Address, err error) {
	root, err := pm.cr.GetTipSetStateRoot(pm.ctx, pm.cr.Head())
	if err != nil {
		return address.Undef, address.Undef, err
	}
	sv := pm.stateViewer.StateView(root)
	return sv.PaychActorParties(pm.ctx, paychAddr)
}
