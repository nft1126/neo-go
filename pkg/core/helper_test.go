package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/nspcc-dev/neo-go/internal/testchain"
	"github.com/nspcc-dev/neo-go/internal/testserdes"
	"github.com/nspcc-dev/neo-go/pkg/config"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/chaindump"
	"github.com/nspcc-dev/neo-go/pkg/core/fee"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/emit"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// multisig address which possess all NEO
var neoOwner = testchain.MultisigScriptHash()

// newTestChain should be called before newBlock invocation to properly setup
// global state.
func newTestChain(t *testing.T) *Blockchain {
	return newTestChainWithCustomCfg(t, nil)
}

func newTestChainWithCustomCfg(t *testing.T, f func(*config.Config)) *Blockchain {
	chain := initTestChain(t, f)
	go chain.Run()
	return chain
}

func initTestChain(t *testing.T, f func(*config.Config)) *Blockchain {
	unitTestNetCfg, err := config.Load("../../config", testchain.Network())
	require.NoError(t, err)
	if f != nil {
		f(&unitTestNetCfg)
	}
	chain, err := NewBlockchain(storage.NewMemoryStore(), unitTestNetCfg.ProtocolConfiguration, zaptest.NewLogger(t))
	require.NoError(t, err)
	return chain
}

func (bc *Blockchain) newBlock(txs ...*transaction.Transaction) *block.Block {
	lastBlock := bc.topBlock.Load().(*block.Block)
	if bc.config.StateRootInHeader {
		sr, err := bc.GetStateRoot(bc.BlockHeight())
		if err != nil {
			panic(err)
		}
		return newBlockWithState(bc.config, lastBlock.Index+1, lastBlock.Hash(), &sr.Root, txs...)
	}
	return newBlock(bc.config, lastBlock.Index+1, lastBlock.Hash(), txs...)
}

func newBlock(cfg config.ProtocolConfiguration, index uint32, prev util.Uint256, txs ...*transaction.Transaction) *block.Block {
	return newBlockWithState(cfg, index, prev, nil, txs...)
}

func newBlockWithState(cfg config.ProtocolConfiguration, index uint32, prev util.Uint256,
	prevState *util.Uint256, txs ...*transaction.Transaction) *block.Block {
	validators, _ := validatorsFromConfig(cfg)
	valScript, _ := smartcontract.CreateDefaultMultiSigRedeemScript(validators)
	witness := transaction.Witness{
		VerificationScript: valScript,
	}
	b := &block.Block{
		Base: block.Base{
			Network:       testchain.Network(),
			Version:       0,
			PrevHash:      prev,
			Timestamp:     uint64(time.Now().UTC().Unix())*1000 + uint64(index),
			Index:         index,
			NextConsensus: witness.ScriptHash(),
			Script:        witness,
		},
		ConsensusData: block.ConsensusData{
			PrimaryIndex: 0,
			Nonce:        1111,
		},
		Transactions: txs,
	}
	if prevState != nil {
		b.StateRootEnabled = true
		b.PrevStateRoot = *prevState
	}
	b.RebuildMerkleRoot()
	b.Script.InvocationScript = testchain.Sign(b.GetSignedPart())
	return b
}

func (bc *Blockchain) genBlocks(n int) ([]*block.Block, error) {
	blocks := make([]*block.Block, n)
	lastHash := bc.topBlock.Load().(*block.Block).Hash()
	lastIndex := bc.topBlock.Load().(*block.Block).Index
	for i := 0; i < n; i++ {
		blocks[i] = newBlock(bc.config, uint32(i)+lastIndex+1, lastHash)
		if err := bc.AddBlock(blocks[i]); err != nil {
			return blocks, err
		}
		lastHash = blocks[i].Hash()
	}
	return blocks, nil
}

func getDecodedBlock(t *testing.T, i int) *block.Block {
	data, err := getBlockData(i)
	require.NoError(t, err)

	b, err := hex.DecodeString(data["raw"].(string))
	require.NoError(t, err)

	block := block.New(testchain.Network(), false)
	require.NoError(t, testserdes.DecodeBinary(b, block))

	return block
}

func getBlockData(i int) (map[string]interface{}, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf("test_data/block_%d.json", i))
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, err
}

func newDumbBlock() *block.Block {
	return &block.Block{
		Base: block.Base{
			Network:       testchain.Network(),
			Version:       0,
			PrevHash:      hash.Sha256([]byte("a")),
			MerkleRoot:    hash.Sha256([]byte("b")),
			Timestamp:     100500,
			Index:         1,
			NextConsensus: hash.Hash160([]byte("a")),
			Script: transaction.Witness{
				VerificationScript: []byte{0x51}, // PUSH1
				InvocationScript:   []byte{0x61}, // NOP
			},
		},
		ConsensusData: block.ConsensusData{
			PrimaryIndex: 0,
			Nonce:        1111,
		},
		Transactions: []*transaction.Transaction{
			transaction.New(testchain.Network(), []byte{byte(opcode.PUSH1)}, 0),
		},
	}
}

// This function generates "../rpc/testdata/testblocks.acc" file which contains data
// for RPC unit tests. It also is a nice integration test.
// To generate new "../rpc/testdata/testblocks.acc", follow the steps:
// 		1. Set saveChain down below to true
// 		2. Run tests with `$ make test`
func TestCreateBasicChain(t *testing.T) {
	const saveChain = false
	const prefix = "../rpc/server/testdata/"

	bc := newTestChain(t)
	defer bc.Close()
	initBasicChain(t, bc)

	if saveChain {
		outStream, err := os.Create(prefix + "testblocks.acc")
		require.NoError(t, err)
		defer outStream.Close()

		writer := io.NewBinWriterFromIO(outStream)
		writer.WriteU32LE(bc.BlockHeight())
		err = chaindump.Dump(bc, writer, 1, bc.BlockHeight())
		require.NoError(t, err)
	}

	priv0 := testchain.PrivateKeyByID(0)
	priv1 := testchain.PrivateKeyByID(1)
	priv0ScriptHash := priv0.GetScriptHash()
	acc0, err := wallet.NewAccountFromWIF(priv0.WIF())
	require.NoError(t, err)

	// Prepare some transaction for future submission.
	txSendRaw := newNEP17Transfer(bc.contracts.NEO.Hash, priv0ScriptHash, priv1.GetScriptHash(), int64(util.Fixed8FromInt64(1000)))
	txSendRaw.ValidUntilBlock = transaction.MaxValidUntilBlockIncrement
	txSendRaw.Nonce = 0x1234
	txSendRaw.Signers = []transaction.Signer{{
		Account:          priv0ScriptHash,
		Scopes:           transaction.CalledByEntry,
		AllowedContracts: nil,
		AllowedGroups:    nil,
	}}
	require.NoError(t, addNetworkFee(bc, txSendRaw, acc0))
	require.NoError(t, acc0.SignTx(txSendRaw))
	bw := io.NewBufBinWriter()
	txSendRaw.EncodeBinary(bw.BinWriter)
	t.Logf("sendrawtransaction: %s", hex.EncodeToString(bw.Bytes()))
}

func initBasicChain(t *testing.T, bc *Blockchain) {
	const prefix = "../rpc/server/testdata/"
	// Increase in case if you need more blocks
	const validUntilBlock = 1200

	// To be incremented after each created transaction to keep chain constant.
	var testNonce uint32 = 1

	// Use as nonce when new transaction is created to avoid random data in tests.
	getNextNonce := func() uint32 {
		testNonce++
		return testNonce
	}

	const neoAmount = 99999000

	gasHash := bc.contracts.GAS.Hash
	neoHash := bc.contracts.NEO.Hash
	policyHash := bc.contracts.Policy.Hash
	t.Logf("native GAS hash: %v", gasHash)
	t.Logf("native NEO hash: %v", neoHash)
	t.Logf("native Policy hash: %v", policyHash)

	priv0 := testchain.PrivateKeyByID(0)
	priv0ScriptHash := priv0.GetScriptHash()

	require.Equal(t, big.NewInt(5000_0000), bc.GetUtilityTokenBalance(priv0ScriptHash)) // gas bounty
	// Move some NEO to one simple account.
	txMoveNeo, err := testchain.NewTransferFromOwner(bc, neoHash, priv0ScriptHash, neoAmount, getNextNonce(), validUntilBlock)
	require.NoError(t, err)
	// Move some GAS to one simple account.
	txMoveGas, err := testchain.NewTransferFromOwner(bc, gasHash, priv0ScriptHash, int64(util.Fixed8FromInt64(1000)),
		getNextNonce(), validUntilBlock)
	require.NoError(t, err)

	b := bc.newBlock(txMoveNeo, txMoveGas)
	require.NoError(t, bc.AddBlock(b))
	t.Logf("Block1 hash: %s", b.Hash().StringLE())
	bw := io.NewBufBinWriter()
	b.EncodeBinary(bw.BinWriter)
	require.NoError(t, bw.Err)
	t.Logf("Block1 hex: %s", hex.EncodeToString(bw.Bytes()))
	t.Logf("txMoveNeo hash: %s", txMoveNeo.Hash().StringLE())
	t.Logf("txMoveNeo hex: %s", hex.EncodeToString(txMoveNeo.Bytes()))
	t.Logf("txMoveGas hash: %s", txMoveGas.Hash().StringLE())

	require.True(t, bc.GetUtilityTokenBalance(priv0ScriptHash).Cmp(big.NewInt(1000*native.GASFactor)) >= 0)
	// info for getblockheader rpc tests
	t.Logf("header hash: %s", b.Hash().StringLE())
	buf := io.NewBufBinWriter()
	b.Header().EncodeBinary(buf.BinWriter)
	t.Logf("header: %s", hex.EncodeToString(buf.Bytes()))

	acc0, err := wallet.NewAccountFromWIF(priv0.WIF())
	require.NoError(t, err)

	// Push some contract into the chain.
	txDeploy, cHash := newDeployTx(t, priv0ScriptHash, prefix+"test_contract.go", "Rubl")
	txDeploy.Nonce = getNextNonce()
	txDeploy.ValidUntilBlock = validUntilBlock
	require.NoError(t, addNetworkFee(bc, txDeploy, acc0))
	require.NoError(t, acc0.SignTx(txDeploy))
	b = bc.newBlock(txDeploy)
	require.NoError(t, bc.AddBlock(b))
	t.Logf("txDeploy: %s", txDeploy.Hash().StringLE())
	t.Logf("Block2 hash: %s", b.Hash().StringLE())

	// Now invoke this contract.
	script := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(script.BinWriter, cHash, "putValue", "testkey", "testvalue")

	txInv := transaction.New(testchain.Network(), script.Bytes(), 1*native.GASFactor)
	txInv.Nonce = getNextNonce()
	txInv.ValidUntilBlock = validUntilBlock
	txInv.Signers = []transaction.Signer{{Account: priv0ScriptHash}}
	require.NoError(t, addNetworkFee(bc, txInv, acc0))
	require.NoError(t, acc0.SignTx(txInv))
	b = bc.newBlock(txInv)
	require.NoError(t, bc.AddBlock(b))
	t.Logf("txInv: %s", txInv.Hash().StringLE())

	priv1 := testchain.PrivateKeyByID(1)
	txNeo0to1 := newNEP17Transfer(neoHash, priv0ScriptHash, priv1.GetScriptHash(), 1000)
	txNeo0to1.Nonce = getNextNonce()
	txNeo0to1.ValidUntilBlock = validUntilBlock
	txNeo0to1.Signers = []transaction.Signer{
		{
			Account:          priv0ScriptHash,
			Scopes:           transaction.CalledByEntry,
			AllowedContracts: nil,
			AllowedGroups:    nil,
		},
	}
	require.NoError(t, addNetworkFee(bc, txNeo0to1, acc0))
	require.NoError(t, acc0.SignTx(txNeo0to1))
	b = bc.newBlock(txNeo0to1)
	require.NoError(t, bc.AddBlock(b))

	w := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(w.BinWriter, cHash, "init")
	initTx := transaction.New(testchain.Network(), w.Bytes(), 1*native.GASFactor)
	initTx.Nonce = getNextNonce()
	initTx.ValidUntilBlock = validUntilBlock
	initTx.Signers = []transaction.Signer{{Account: priv0ScriptHash}}
	require.NoError(t, addNetworkFee(bc, initTx, acc0))
	require.NoError(t, acc0.SignTx(initTx))
	transferTx := newNEP17Transfer(cHash, cHash, priv0.GetScriptHash(), 1000)
	transferTx.Nonce = getNextNonce()
	transferTx.ValidUntilBlock = validUntilBlock
	transferTx.Signers = []transaction.Signer{
		{
			Account:          priv0ScriptHash,
			Scopes:           transaction.CalledByEntry,
			AllowedContracts: nil,
			AllowedGroups:    nil,
		},
	}
	require.NoError(t, addNetworkFee(bc, transferTx, acc0))
	require.NoError(t, acc0.SignTx(transferTx))

	b = bc.newBlock(initTx, transferTx)
	require.NoError(t, bc.AddBlock(b))
	t.Logf("recieveRublesTx: %v", transferTx.Hash().StringLE())

	transferTx = newNEP17Transfer(cHash, priv0.GetScriptHash(), priv1.GetScriptHash(), 123)
	transferTx.Nonce = getNextNonce()
	transferTx.ValidUntilBlock = validUntilBlock
	transferTx.Signers = []transaction.Signer{
		{
			Account:          priv0ScriptHash,
			Scopes:           transaction.CalledByEntry,
			AllowedContracts: nil,
			AllowedGroups:    nil,
		},
	}
	require.NoError(t, addNetworkFee(bc, transferTx, acc0))
	require.NoError(t, acc0.SignTx(transferTx))

	b = bc.newBlock(transferTx)
	require.NoError(t, bc.AddBlock(b))
	t.Logf("sendRublesTx: %v", transferTx.Hash().StringLE())

	// Push verification contract into the chain.
	txDeploy2, _ := newDeployTx(t, priv0ScriptHash, prefix+"verification_contract.go", "Verify")
	txDeploy2.Nonce = getNextNonce()
	txDeploy2.ValidUntilBlock = validUntilBlock
	require.NoError(t, addNetworkFee(bc, txDeploy2, acc0))
	require.NoError(t, acc0.SignTx(txDeploy2))
	b = bc.newBlock(txDeploy2)
	require.NoError(t, bc.AddBlock(b))
}

func newNEP17Transfer(sc, from, to util.Uint160, amount int64, additionalArgs ...interface{}) *transaction.Transaction {
	w := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(w.BinWriter, sc, "transfer", from, to, amount, additionalArgs)
	emit.Opcodes(w.BinWriter, opcode.ASSERT)

	script := w.Bytes()
	return transaction.New(testchain.Network(), script, 10000000)
}

func newDeployTx(t *testing.T, sender util.Uint160, name, ctrName string) (*transaction.Transaction, util.Uint160) {
	c, err := ioutil.ReadFile(name)
	require.NoError(t, err)
	tx, h, err := testchain.NewDeployTx(ctrName, sender, bytes.NewReader(c))
	require.NoError(t, err)
	t.Logf("contractHash (%s): %s", name, h.StringLE())
	return tx, h
}

func addSigners(txs ...*transaction.Transaction) {
	for _, tx := range txs {
		tx.Signers = []transaction.Signer{{
			Account:          neoOwner,
			Scopes:           transaction.CalledByEntry,
			AllowedContracts: nil,
			AllowedGroups:    nil,
		}}
	}
}

func addNetworkFee(bc *Blockchain, tx *transaction.Transaction, sender *wallet.Account) error {
	size := io.GetVarSize(tx)
	netFee, sizeDelta := fee.Calculate(sender.Contract.Script)
	tx.NetworkFee += netFee
	size += sizeDelta
	for _, cosigner := range tx.Signers {
		contract := bc.GetContractState(cosigner.Account)
		if contract != nil {
			netFee, sizeDelta = fee.Calculate(contract.Script)
			tx.NetworkFee += netFee
			size += sizeDelta
		}
	}
	tx.NetworkFee += int64(size) * bc.FeePerByte()
	return nil
}

func invokeContractMethod(chain *Blockchain, sysfee int64, hash util.Uint160, method string, args ...interface{}) (*state.AppExecResult, error) {
	w := io.NewBufBinWriter()
	emit.AppCallWithOperationAndArgs(w.BinWriter, hash, method, args...)
	if w.Err != nil {
		return nil, w.Err
	}
	script := w.Bytes()
	tx := transaction.New(chain.GetConfig().Magic, script, sysfee)
	tx.ValidUntilBlock = chain.blockHeight + 1
	addSigners(tx)
	err := testchain.SignTx(chain, tx)
	if err != nil {
		return nil, err
	}
	b := chain.newBlock(tx)
	err = chain.AddBlock(b)
	if err != nil {
		return nil, err
	}

	res, err := chain.GetAppExecResults(tx.Hash(), trigger.Application)
	if err != nil {
		return nil, err
	}
	return &res[0], nil
}

func transferTokenFromMultisigAccount(t *testing.T, chain *Blockchain, to, tokenHash util.Uint160, amount int64, additionalArgs ...interface{}) *transaction.Transaction {
	transferTx := newNEP17Transfer(tokenHash, testchain.MultisigScriptHash(), to, amount, additionalArgs...)
	transferTx.SystemFee = 100000000
	transferTx.ValidUntilBlock = chain.BlockHeight() + 1
	addSigners(transferTx)
	require.NoError(t, testchain.SignTx(chain, transferTx))
	b := chain.newBlock(transferTx)
	require.NoError(t, chain.AddBlock(b))
	return transferTx
}

func checkResult(t *testing.T, result *state.AppExecResult, expected stackitem.Item) {
	require.Equal(t, vm.HaltState, result.VMState)
	require.Equal(t, 1, len(result.Stack))
	require.Equal(t, expected, result.Stack[0])
}

func checkFAULTState(t *testing.T, result *state.AppExecResult) {
	require.Equal(t, vm.FaultState, result.VMState)
}

func checkBalanceOf(t *testing.T, chain *Blockchain, addr util.Uint160, expected int) {
	balance := chain.GetNEP17Balances(addr).Trackers[chain.contracts.GAS.ContractID]
	require.Equal(t, int64(expected), balance.Balance.Int64())
}
