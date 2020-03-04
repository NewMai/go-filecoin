package paymentchannel

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
)

//go:generate cbor-gen-for ChannelInfo VoucherInfo

// The key for the store is the payment channel's "robust" or "unique" address

// ChannelInfo is the primary payment channel record
type ChannelInfo struct {
	IDAddr   address.Address // internal ID address for payment channel actor
	State    *paych.State
	Vouchers []*VoucherInfo // All vouchers submitted for this channel
}

// VoucherInfo is a record of a voucher submitted for a payment channel
type VoucherInfo struct {
	Voucher *paych.SignedVoucher
	Proof   []byte
}
