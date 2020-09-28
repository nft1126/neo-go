package oracle

import (
	"encoding/hex"
	"errors"
	gio "io"

	"github.com/nspcc-dev/neo-go/pkg/core/fee"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/emit"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"go.uber.org/zap"
)

var oracleInvoc []byte

func init() {
	w := io.NewBufBinWriter()
	emit.Int(w.BinWriter, 0)
	emit.Opcodes(w.BinWriter, opcode.PACK)
	emit.String(w.BinWriter, manifest.MethodVerify)
	if w.Err != nil {
		panic(w.Err)
	}
	oracleInvoc = w.Bytes()
}

func (o *Oracle) getResponse(reqID uint64) *incompleteTx {
	o.respMtx.Lock()
	defer o.respMtx.Unlock()
	incTx, ok := o.responses[reqID]
	if !ok {
		incTx = newIncompleteTx()
		o.responses[reqID] = incTx
	}
	return incTx
}

// AddResponse processes oracle response from node pub.
// sig is response transaction signature.
func (o *Oracle) AddResponse(pub *keys.PublicKey, reqID uint64, txSig []byte) {
	incTx := o.getResponse(reqID)

	incTx.Lock()
	isBackup := false
	if incTx.tx != nil {
		ok := pub.Verify(txSig, incTx.tx.GetSignedHash().BytesBE())
		if !ok {
			ok = pub.Verify(txSig, incTx.backupTx.GetSignedHash().BytesBE())
			if !ok {
				o.Log.Debug("invalid response signature",
					zap.String("pub", hex.EncodeToString(pub.Bytes())))
				incTx.Unlock()
				return
			}
			isBackup = true
		}
	}
	incTx.addResponse(pub, txSig, isBackup)
	readyTx, ready := incTx.finalize(o.getOracleNodes())
	if ready {
		ready = !incTx.isSent
		incTx.isSent = true
	}
	incTx.Unlock()

	if ready {
		o.getOnTransaction()(readyTx)
	}
}

// ErrResponseTooLarge is returned when response exceeds max allowed size.
var ErrResponseTooLarge = errors.New("too big response")

func readResponse(rc gio.ReadCloser, limit int) ([]byte, error) {
	defer rc.Close()

	buf := make([]byte, limit+1)
	n, err := gio.ReadFull(rc, buf)
	if err == gio.ErrUnexpectedEOF && n <= limit {
		return buf[:n], nil
	}
	if err == nil || n > limit {
		return nil, ErrResponseTooLarge
	}
	return nil, err
}

// CreateResponseTx creates unsigned oracle response transaction.
func (o *Oracle) CreateResponseTx(gasForResponse int64, height uint32, resp *transaction.OracleResponse) (*transaction.Transaction, error) {
	tx := transaction.New(o.Network, native.GetOracleResponseScript(), 0)
	tx.Nonce = 0
	tx.ValidUntilBlock = height + transaction.MaxValidUntilBlockIncrement
	tx.Attributes = []transaction.Attribute{{
		Type:  transaction.OracleResponseT,
		Value: resp,
	}}

	oracleSignContract := o.getOracleSignContract()
	tx.Signers = []transaction.Signer{
		{
			Account: o.oracleHash,
			Scopes:  transaction.None,
		},
		{
			Account:          hash.Hash160(oracleSignContract),
			Scopes:           transaction.None,
			AllowedContracts: []util.Uint160{o.oracleHash},
		},
	}
	tx.Scripts = []transaction.Witness{
		{
			InvocationScript: oracleInvoc,
		},
		{
			VerificationScript: oracleSignContract,
		},
	}

	// Calculate network fee.
	size := io.GetVarSize(tx)

	// FIXME (?) Verification interop context.
	v := o.Chain.GetTestVM(tx, nil)
	v.LoadScript(o.OracleScript)
	v.LoadScript(tx.Scripts[0].InvocationScript)
	if !isVerifyOk(v) {
		return nil, errors.New("can't verify transaction")
	}
	tx.NetworkFee += v.GasConsumed()
	size += io.GetVarSize(tx.Scripts[0].InvocationScript) +
		io.GetVarSize(tx.Scripts[0].VerificationScript)

	netFee, sizeDelta := fee.Calculate(tx.Scripts[1].VerificationScript)
	tx.NetworkFee += netFee
	size += sizeDelta

	currNetFee := tx.NetworkFee + int64(size)*o.Chain.FeePerByte()
	if currNetFee > gasForResponse {
		attrSize := io.GetVarSize(tx.Attributes)
		resp.Code = transaction.InsufficientFunds
		resp.Result = nil
		size = size - attrSize + io.GetVarSize(tx.Attributes)
	}
	tx.NetworkFee += int64(size) * o.Chain.FeePerByte()

	// Calculate system fee.
	tx.SystemFee = gasForResponse - tx.NetworkFee
	return tx, nil
}

func isVerifyOk(v *vm.VM) bool {
	if err := v.Run(); err != nil {
		return false
	}
	if v.Estack().Len() != 1 {
		return false
	}
	ok, err := v.Estack().Pop().Item().TryBool()
	return err == nil && ok
}
