package client

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/fee"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/fixedn"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/rpc/request"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response/result"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

var errNetworkNotInitialized = errors.New("RPC client network is not initialized")

// GetApplicationLog returns the contract log based on the specified txid.
func (c *Client) GetApplicationLog(hash util.Uint256, trig *trigger.Type) (*result.ApplicationLog, error) {
	var (
		params = request.NewRawParams(hash.StringLE())
		resp   = new(result.ApplicationLog)
	)
	if trig != nil {
		params.Values = append(params.Values, trig.String())
	}
	if err := c.performRequest("getapplicationlog", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetBestBlockHash returns the hash of the tallest block in the main chain.
func (c *Client) GetBestBlockHash() (util.Uint256, error) {
	var resp = util.Uint256{}
	if err := c.performRequest("getbestblockhash", request.NewRawParams(), &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetBlockCount returns the number of blocks in the main chain.
func (c *Client) GetBlockCount() (uint32, error) {
	var resp uint32
	if err := c.performRequest("getblockcount", request.NewRawParams(), &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetBlockByIndex returns a block by its height. You should initialize network magic
// with Init before calling GetBlockByIndex.
func (c *Client) GetBlockByIndex(index uint32) (*block.Block, error) {
	return c.getBlock(request.NewRawParams(index))
}

// GetBlockByHash returns a block by its hash. You should initialize network magic
// with Init before calling GetBlockByHash.
func (c *Client) GetBlockByHash(hash util.Uint256) (*block.Block, error) {
	return c.getBlock(request.NewRawParams(hash.StringLE()))
}

func (c *Client) getBlock(params request.RawParams) (*block.Block, error) {
	var (
		resp []byte
		err  error
		b    *block.Block
	)
	if !c.initDone {
		return nil, errNetworkNotInitialized
	}
	if err = c.performRequest("getblock", params, &resp); err != nil {
		return nil, err
	}
	r := io.NewBinReaderFromBuf(resp)
	b = block.New(c.GetNetwork(), c.StateRootInHeader())
	b.DecodeBinary(r)
	if r.Err != nil {
		return nil, r.Err
	}
	return b, nil
}

// GetBlockByIndexVerbose returns a block wrapper with additional metadata by
// its height. You should initialize network magic with Init before calling GetBlockByIndexVerbose.
// NOTE: to get transaction.ID and transaction.Size, use t.Hash() and io.GetVarSize(t) respectively.
func (c *Client) GetBlockByIndexVerbose(index uint32) (*result.Block, error) {
	return c.getBlockVerbose(request.NewRawParams(index, 1))
}

// GetBlockByHashVerbose returns a block wrapper with additional metadata by
// its hash. You should initialize network magic with Init before calling GetBlockByHashVerbose.
func (c *Client) GetBlockByHashVerbose(hash util.Uint256) (*result.Block, error) {
	return c.getBlockVerbose(request.NewRawParams(hash.StringLE(), 1))
}

func (c *Client) getBlockVerbose(params request.RawParams) (*result.Block, error) {
	var (
		resp = &result.Block{}
		err  error
	)
	if !c.initDone {
		return nil, errNetworkNotInitialized
	}
	resp.Network = c.GetNetwork()
	if err = c.performRequest("getblock", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetBlockHash returns the hash value of the corresponding block, based on the specified index.
func (c *Client) GetBlockHash(index uint32) (util.Uint256, error) {
	var (
		params = request.NewRawParams(index)
		resp   = util.Uint256{}
	)
	if err := c.performRequest("getblockhash", params, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetBlockHeader returns the corresponding block header information from serialized hex string
// according to the specified script hash. You should initialize network magic
// // with Init before calling GetBlockHeader.
func (c *Client) GetBlockHeader(hash util.Uint256) (*block.Header, error) {
	var (
		params = request.NewRawParams(hash.StringLE())
		resp   []byte
		h      *block.Header
	)
	if !c.initDone {
		return nil, errNetworkNotInitialized
	}
	if err := c.performRequest("getblockheader", params, &resp); err != nil {
		return nil, err
	}
	r := io.NewBinReaderFromBuf(resp)
	h = new(block.Header)
	h.Network = c.GetNetwork()
	h.DecodeBinary(r)
	if r.Err != nil {
		return nil, r.Err
	}
	return h, nil
}

// GetBlockHeaderVerbose returns the corresponding block header information from Json format string
// according to the specified script hash.
func (c *Client) GetBlockHeaderVerbose(hash util.Uint256) (*result.Header, error) {
	var (
		params = request.NewRawParams(hash.StringLE(), 1)
		resp   = &result.Header{}
	)
	if err := c.performRequest("getblockheader", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetBlockSysFee returns the system fees of the block, based on the specified index.
func (c *Client) GetBlockSysFee(index uint32) (fixedn.Fixed8, error) {
	var (
		params = request.NewRawParams(index)
		resp   fixedn.Fixed8
	)
	if err := c.performRequest("getblocksysfee", params, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetConnectionCount returns the current number of connections for the node.
func (c *Client) GetConnectionCount() (int, error) {
	var (
		params = request.NewRawParams()
		resp   int
	)
	if err := c.performRequest("getconnectioncount", params, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetCommittee returns the current public keys of NEO nodes in committee.
func (c *Client) GetCommittee() (keys.PublicKeys, error) {
	var (
		params = request.NewRawParams()
		resp   = new(keys.PublicKeys)
	)
	if err := c.performRequest("getcommittee", params, resp); err != nil {
		return nil, err
	}
	return *resp, nil
}

// GetContractStateByHash queries contract information, according to the contract script hash.
func (c *Client) GetContractStateByHash(hash util.Uint160) (*state.Contract, error) {
	return c.getContractState(hash.StringLE())
}

// GetContractStateByAddressOrName queries contract information, according to the contract address or name.
func (c *Client) GetContractStateByAddressOrName(addressOrName string) (*state.Contract, error) {
	return c.getContractState(addressOrName)
}

// GetContractStateByID queries contract information, according to the contract ID.
func (c *Client) GetContractStateByID(id int32) (*state.Contract, error) {
	return c.getContractState(id)
}

// getContractState is an internal representation of GetContractStateBy* methods.
func (c *Client) getContractState(param interface{}) (*state.Contract, error) {
	var (
		params = request.NewRawParams(param)
		resp   = &state.Contract{}
	)
	if err := c.performRequest("getcontractstate", params, resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetNEP17Balances is a wrapper for getnep17balances RPC.
func (c *Client) GetNEP17Balances(address util.Uint160) (*result.NEP17Balances, error) {
	params := request.NewRawParams(address.StringLE())
	resp := new(result.NEP17Balances)
	if err := c.performRequest("getnep17balances", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetNEP17Transfers is a wrapper for getnep17transfers RPC. Address parameter
// is mandatory, while all the others are optional. Start and stop parameters
// are supported since neo-go 0.77.0 and limit and page since neo-go 0.78.0.
// These parameters are positional in the JSON-RPC call, you can't specify limit
// and not specify start/stop for example.
func (c *Client) GetNEP17Transfers(address string, start, stop *uint32, limit, page *int) (*result.NEP17Transfers, error) {
	params := request.NewRawParams(address)
	if start != nil {
		params.Values = append(params.Values, *start)
		if stop != nil {
			params.Values = append(params.Values, *stop)
			if limit != nil {
				params.Values = append(params.Values, *limit)
				if page != nil {
					params.Values = append(params.Values, *page)
				}
			} else if page != nil {
				return nil, errors.New("bad parameters")
			}
		} else if limit != nil || page != nil {
			return nil, errors.New("bad parameters")
		}
	} else if stop != nil || limit != nil || page != nil {
		return nil, errors.New("bad parameters")
	}
	resp := new(result.NEP17Transfers)
	if err := c.performRequest("getnep17transfers", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPeers returns the list of nodes that the node is currently connected/disconnected from.
func (c *Client) GetPeers() (*result.GetPeers, error) {
	var (
		params = request.NewRawParams()
		resp   = &result.GetPeers{}
	)
	if err := c.performRequest("getpeers", params, resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetRawMemPool returns the list of unconfirmed transactions in memory.
func (c *Client) GetRawMemPool() ([]util.Uint256, error) {
	var (
		params = request.NewRawParams()
		resp   = new([]util.Uint256)
	)
	if err := c.performRequest("getrawmempool", params, resp); err != nil {
		return *resp, err
	}
	return *resp, nil
}

// GetRawTransaction returns a transaction by hash. You should initialize network magic
// with Init before calling GetRawTransaction.
func (c *Client) GetRawTransaction(hash util.Uint256) (*transaction.Transaction, error) {
	var (
		params = request.NewRawParams(hash.StringLE())
		resp   []byte
		err    error
	)
	if !c.initDone {
		return nil, errNetworkNotInitialized
	}
	if err = c.performRequest("getrawtransaction", params, &resp); err != nil {
		return nil, err
	}
	tx, err := transaction.NewTransactionFromBytes(c.GetNetwork(), resp)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// GetRawTransactionVerbose returns a transaction wrapper with additional
// metadata by transaction's hash. You should initialize network magic
// with Init before calling GetRawTransactionVerbose.
// NOTE: to get transaction.ID and transaction.Size, use t.Hash() and io.GetVarSize(t) respectively.
func (c *Client) GetRawTransactionVerbose(hash util.Uint256) (*result.TransactionOutputRaw, error) {
	var (
		params = request.NewRawParams(hash.StringLE(), 1)
		resp   = &result.TransactionOutputRaw{}
		err    error
	)
	if !c.initDone {
		return nil, errNetworkNotInitialized
	}
	resp.Network = c.GetNetwork()
	if err = c.performRequest("getrawtransaction", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetStorageByID returns the stored value, according to the contract ID and the stored key.
func (c *Client) GetStorageByID(id int32, key []byte) ([]byte, error) {
	return c.getStorage(request.NewRawParams(id, hex.EncodeToString(key)))
}

// GetStorageByHash returns the stored value, according to the contract script hash and the stored key.
func (c *Client) GetStorageByHash(hash util.Uint160, key []byte) ([]byte, error) {
	return c.getStorage(request.NewRawParams(hash.StringLE(), hex.EncodeToString(key)))
}

func (c *Client) getStorage(params request.RawParams) ([]byte, error) {
	var resp string
	if err := c.performRequest("getstorage", params, &resp); err != nil {
		return nil, err
	}
	res, err := hex.DecodeString(resp)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetTransactionHeight returns the block index in which the transaction is found.
func (c *Client) GetTransactionHeight(hash util.Uint256) (uint32, error) {
	var (
		params = request.NewRawParams(hash.StringLE())
		resp   uint32
	)
	if err := c.performRequest("gettransactionheight", params, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetUnclaimedGas returns unclaimed GAS amount for the specified address.
func (c *Client) GetUnclaimedGas(address string) (result.UnclaimedGas, error) {
	var (
		params = request.NewRawParams(address)
		resp   result.UnclaimedGas
	)
	if err := c.performRequest("getunclaimedgas", params, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// GetNextBlockValidators returns the current NEO consensus nodes information and voting status.
func (c *Client) GetNextBlockValidators() ([]result.Validator, error) {
	var (
		params = request.NewRawParams()
		resp   = new([]result.Validator)
	)
	if err := c.performRequest("getnextblockvalidators", params, resp); err != nil {
		return nil, err
	}
	return *resp, nil
}

// GetVersion returns the version information about the queried node.
func (c *Client) GetVersion() (*result.Version, error) {
	var (
		params = request.NewRawParams()
		resp   = &result.Version{}
	)
	if err := c.performRequest("getversion", params, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// InvokeScript returns the result of the given script after running it true the VM.
// NOTE: This is a test invoke and will not affect the blockchain.
func (c *Client) InvokeScript(script []byte, signers []transaction.Signer) (*result.Invoke, error) {
	var p = request.NewRawParams(script)
	return c.invokeSomething("invokescript", p, signers)
}

// InvokeFunction returns the results after calling the smart contract scripthash
// with the given operation and parameters.
// NOTE: this is test invoke and will not affect the blockchain.
func (c *Client) InvokeFunction(contract util.Uint160, operation string, params []smartcontract.Parameter, signers []transaction.Signer) (*result.Invoke, error) {
	var p = request.NewRawParams(contract.StringLE(), operation, params)
	return c.invokeSomething("invokefunction", p, signers)
}

// invokeSomething is an inner wrapper for Invoke* functions
func (c *Client) invokeSomething(method string, p request.RawParams, signers []transaction.Signer) (*result.Invoke, error) {
	var resp = new(result.Invoke)
	if signers != nil {
		p.Values = append(p.Values, signers)
	}
	if err := c.performRequest(method, p, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SendRawTransaction broadcasts a transaction over the NEO network.
// The given hex string needs to be signed with a keypair.
// When the result of the response object is true, the TX has successfully
// been broadcasted to the network.
func (c *Client) SendRawTransaction(rawTX *transaction.Transaction) (util.Uint256, error) {
	var (
		params = request.NewRawParams(rawTX.Bytes())
		resp   = new(result.RelayResult)
	)
	if err := c.performRequest("sendrawtransaction", params, resp); err != nil {
		return util.Uint256{}, err
	}
	return resp.Hash, nil
}

// SubmitBlock broadcasts a raw block over the NEO network.
func (c *Client) SubmitBlock(b block.Block) (util.Uint256, error) {
	var (
		params request.RawParams
		resp   = new(result.RelayResult)
	)
	buf := io.NewBufBinWriter()
	b.EncodeBinary(buf.BinWriter)
	if err := buf.Err; err != nil {
		return util.Uint256{}, err
	}
	params = request.NewRawParams(buf.Bytes())

	if err := c.performRequest("submitblock", params, resp); err != nil {
		return util.Uint256{}, err
	}
	return resp.Hash, nil
}

// SubmitRawOracleResponse submits raw oracle response to the oracle node.
// Raw params are used to avoid excessive marshalling.
func (c *Client) SubmitRawOracleResponse(ps request.RawParams) error {
	return c.performRequest("submitoracleresponse", ps, new(result.RelayResult))
}

// SignAndPushInvocationTx signs and pushes given script as an invocation
// transaction  using given wif to sign it and spending the amount of gas
// specified. It returns a hash of the invocation transaction and an error.
func (c *Client) SignAndPushInvocationTx(script []byte, acc *wallet.Account, sysfee int64, netfee fixedn.Fixed8, cosigners []transaction.Signer) (util.Uint256, error) {
	var txHash util.Uint256
	var err error

	tx, err := c.CreateTxFromScript(script, acc, sysfee, int64(netfee), cosigners...)
	if err != nil {
		return txHash, fmt.Errorf("failed to create tx: %w", err)
	}
	if err = acc.SignTx(tx); err != nil {
		return txHash, fmt.Errorf("failed to sign tx: %w", err)
	}
	txHash = tx.Hash()
	actualHash, err := c.SendRawTransaction(tx)
	if err != nil {
		return txHash, fmt.Errorf("failed to send tx: %w", err)
	}
	if !actualHash.Equals(txHash) {
		return actualHash, fmt.Errorf("sent and actual tx hashes mismatch:\n\tsent: %v\n\tactual: %v", txHash.StringLE(), actualHash.StringLE())
	}
	return txHash, nil
}

// getSigners returns an array of transaction signers from given sender and cosigners.
// If cosigners list already contains sender, the sender will be placed at the start of
// the list.
func getSigners(sender util.Uint160, cosigners []transaction.Signer) []transaction.Signer {
	s := transaction.Signer{
		Account: sender,
		Scopes:  transaction.None,
	}
	for i, c := range cosigners {
		if c.Account == sender {
			if i == 0 {
				return cosigners
			}
			s.Scopes = c.Scopes
			cosigners = append(cosigners[:i], cosigners[i+1:]...)
			break
		}
	}
	return append([]transaction.Signer{s}, cosigners...)
}

// ValidateAddress verifies that the address is a correct NEO address.
func (c *Client) ValidateAddress(address string) error {
	var (
		params = request.NewRawParams(address)
		resp   = &result.ValidateAddress{}
	)

	if err := c.performRequest("validateaddress", params, resp); err != nil {
		return err
	}
	if !resp.IsValid {
		return errors.New("validateaddress returned false")
	}
	return nil
}

// CalculateValidUntilBlock calculates ValidUntilBlock field for tx as
// current blockchain height + number of validators. Number of validators
// is the length of blockchain validators list got from GetNextBlockValidators()
// method. Validators count is being cached and updated every 100 blocks.
func (c *Client) CalculateValidUntilBlock() (uint32, error) {
	var (
		result          uint32
		validatorsCount uint32
	)
	blockCount, err := c.GetBlockCount()
	if err != nil {
		return result, fmt.Errorf("can't get block count: %w", err)
	}

	if c.cache.calculateValidUntilBlock.expiresAt > blockCount {
		validatorsCount = c.cache.calculateValidUntilBlock.validatorsCount
	} else {
		validators, err := c.GetNextBlockValidators()
		if err != nil {
			return result, fmt.Errorf("can't get validators: %w", err)
		}
		validatorsCount = uint32(len(validators))
		c.cache.calculateValidUntilBlock = calculateValidUntilBlockCache{
			validatorsCount: validatorsCount,
			expiresAt:       blockCount + cacheTimeout,
		}
	}
	return blockCount + validatorsCount + 1, nil
}

// AddNetworkFee adds network fee for each witness script and optional extra
// network fee to transaction. `accs` is an array signer's accounts.
func (c *Client) AddNetworkFee(tx *transaction.Transaction, extraFee int64, accs ...*wallet.Account) error {
	if len(tx.Signers) != len(accs) {
		return errors.New("number of signers must match number of scripts")
	}
	size := io.GetVarSize(tx)
	var ef int64
	for i, cosigner := range tx.Signers {
		if accs[i].Contract.Deployed {
			res, err := c.InvokeFunction(cosigner.Account, manifest.MethodVerify, []smartcontract.Parameter{}, tx.Signers)
			if err != nil {
				return fmt.Errorf("failed to invoke verify: %w", err)
			}
			if res.State != "HALT" {
				return fmt.Errorf("invalid VM state %s due to an error: %s", res.State, res.FaultException)
			}
			if l := len(res.Stack); l != 1 {
				return fmt.Errorf("result stack length should be equal to 1, got %d", l)
			}
			r, err := topIntFromStack(res.Stack)
			if err != nil || r == 0 {
				return core.ErrVerificationFailed
			}
			tx.NetworkFee += res.GasConsumed
			size += io.GetVarSize([]byte{}) * 2 // both scripts are empty
			continue
		}

		if ef == 0 {
			var err error
			ef, err = c.GetExecFeeFactor()
			if err != nil {
				return fmt.Errorf("can't get `ExecFeeFactor`: %w", err)
			}
		}
		netFee, sizeDelta := fee.Calculate(ef, accs[i].Contract.Script)
		tx.NetworkFee += netFee
		size += sizeDelta
	}
	fee, err := c.GetFeePerByte()
	if err != nil {
		return err
	}
	tx.NetworkFee += int64(size)*fee + extraFee
	return nil
}

// GetNetwork returns the network magic of the RPC node client connected to.
func (c *Client) GetNetwork() netmode.Magic {
	return c.network
}

// StateRootInHeader returns true if state root is contained in block header.
func (c *Client) StateRootInHeader() bool {
	return c.stateRootInHeader
}

// GetNativeContractHash returns native contract hash by its name.
func (c *Client) GetNativeContractHash(name string) (util.Uint160, error) {
	hash, ok := c.cache.nativeHashes[name]
	if ok {
		return hash, nil
	}
	cs, err := c.GetContractStateByAddressOrName(name)
	if err != nil {
		return util.Uint160{}, err
	}
	c.cache.nativeHashes[name] = cs.Hash
	return cs.Hash, nil
}
