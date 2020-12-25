package server

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/blockchainer"
	"github.com/nspcc-dev/neo-go/pkg/core/mpt"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/network"
	"github.com/nspcc-dev/neo-go/pkg/rpc"
	"github.com/nspcc-dev/neo-go/pkg/rpc/request"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response/result"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"go.uber.org/zap"
)

type (
	// Server represents the JSON-RPC 2.0 server.
	Server struct {
		*http.Server
		chain            blockchainer.Blockchainer
		config           rpc.Config
		network          netmode.Magic
		stateRootEnabled bool
		coreServer       *network.Server
		log              *zap.Logger
		https            *http.Server
		shutdown         chan struct{}

		subsLock         sync.RWMutex
		subscribers      map[*subscriber]bool
		subsGroup        sync.WaitGroup
		blockSubs        int
		executionSubs    int
		notificationSubs int
		transactionSubs  int
		blockCh          chan *block.Block
		executionCh      chan *state.AppExecResult
		notificationCh   chan *state.NotificationEvent
		transactionCh    chan *transaction.Transaction
	}
)

const (
	// Message limit for receiving side.
	wsReadLimit = 4096

	// Disconnection timeout.
	wsPongLimit = 60 * time.Second

	// Ping period for connection liveness check.
	wsPingPeriod = wsPongLimit / 2

	// Write deadline.
	wsWriteLimit = wsPingPeriod / 2

	// Maximum number of subscribers per Server. Each websocket client is
	// treated like subscriber, so technically it's a limit on websocket
	// connections.
	maxSubscribers = 64

	// Maximum number of elements for get*transfers requests.
	maxTransfersLimit = 1000
)

var rpcHandlers = map[string]func(*Server, request.Params) (interface{}, *response.Error){
	"getapplicationlog":      (*Server).getApplicationLog,
	"getbestblockhash":       (*Server).getBestBlockHash,
	"getblock":               (*Server).getBlock,
	"getblockcount":          (*Server).getBlockCount,
	"getblockhash":           (*Server).getBlockHash,
	"getblockheader":         (*Server).getBlockHeader,
	"getblocksysfee":         (*Server).getBlockSysFee,
	"getcommittee":           (*Server).getCommittee,
	"getconnectioncount":     (*Server).getConnectionCount,
	"getcontractstate":       (*Server).getContractState,
	"getnep17balances":       (*Server).getNEP17Balances,
	"getnep17transfers":      (*Server).getNEP17Transfers,
	"getpeers":               (*Server).getPeers,
	"getproof":               (*Server).getProof,
	"getrawmempool":          (*Server).getRawMempool,
	"getrawtransaction":      (*Server).getrawtransaction,
	"getstateheight":         (*Server).getStateHeight,
	"getstateroot":           (*Server).getStateRoot,
	"getstorage":             (*Server).getStorage,
	"gettransactionheight":   (*Server).getTransactionHeight,
	"getunclaimedgas":        (*Server).getUnclaimedGas,
	"getnextblockvalidators": (*Server).getNextBlockValidators,
	"getversion":             (*Server).getVersion,
	"invokefunction":         (*Server).invokeFunction,
	"invokescript":           (*Server).invokescript,
	"invokecontractverify":   (*Server).invokeContractVerify,
	"sendrawtransaction":     (*Server).sendrawtransaction,
	"submitblock":            (*Server).submitBlock,
	"validateaddress":        (*Server).validateAddress,
	"verifyproof":            (*Server).verifyProof,
}

var rpcWsHandlers = map[string]func(*Server, request.Params, *subscriber) (interface{}, *response.Error){
	"subscribe":   (*Server).subscribe,
	"unsubscribe": (*Server).unsubscribe,
}

var invalidBlockHeightError = func(index int, height int) *response.Error {
	return response.NewRPCError(fmt.Sprintf("Param at index %d should be greater than or equal to 0 and less then or equal to current block height, got: %d", index, height), "", nil)
}

// upgrader is a no-op websocket.Upgrader that reuses HTTP server buffers and
// doesn't set any Error function.
var upgrader = websocket.Upgrader{}

// New creates a new Server struct.
func New(chain blockchainer.Blockchainer, conf rpc.Config, coreServer *network.Server, log *zap.Logger) Server {
	httpServer := &http.Server{
		Addr: conf.Address + ":" + strconv.FormatUint(uint64(conf.Port), 10),
	}

	var tlsServer *http.Server
	if cfg := conf.TLSConfig; cfg.Enabled {
		tlsServer = &http.Server{
			Addr: net.JoinHostPort(cfg.Address, strconv.FormatUint(uint64(cfg.Port), 10)),
		}
	}

	return Server{
		Server:           httpServer,
		chain:            chain,
		config:           conf,
		network:          chain.GetConfig().Magic,
		stateRootEnabled: chain.GetConfig().StateRootInHeader,
		coreServer:       coreServer,
		log:              log,
		https:            tlsServer,
		shutdown:         make(chan struct{}),

		subscribers: make(map[*subscriber]bool),
		// These are NOT buffered to preserve original order of events.
		blockCh:        make(chan *block.Block),
		executionCh:    make(chan *state.AppExecResult),
		notificationCh: make(chan *state.NotificationEvent),
		transactionCh:  make(chan *transaction.Transaction),
	}
}

// Start creates a new JSON-RPC server listening on the configured port. It's
// supposed to be run as a separate goroutine (like http.Server's Serve) and it
// returns its errors via given errChan.
func (s *Server) Start(errChan chan error) {
	if !s.config.Enabled {
		s.log.Info("RPC server is not enabled")
		return
	}
	s.Handler = http.HandlerFunc(s.handleHTTPRequest)
	s.log.Info("starting rpc-server", zap.String("endpoint", s.Addr))

	go s.handleSubEvents()
	if cfg := s.config.TLSConfig; cfg.Enabled {
		s.https.Handler = http.HandlerFunc(s.handleHTTPRequest)
		s.log.Info("starting rpc-server (https)", zap.String("endpoint", s.https.Addr))
		go func() {
			ln, err := net.Listen("tcp", s.https.Addr)
			if err != nil {
				errChan <- err
				return
			}
			s.https.Addr = ln.Addr().String()
			err = s.https.ServeTLS(ln, cfg.CertFile, cfg.KeyFile)
			if err != http.ErrServerClosed {
				s.log.Error("failed to start TLS RPC server", zap.Error(err))
				errChan <- err
			}
		}()
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		errChan <- err
		return
	}
	s.Addr = ln.Addr().String() // set Addr to the actual address
	go func() {
		err = s.Serve(ln)
		if err != http.ErrServerClosed {
			s.log.Error("failed to start RPC server", zap.Error(err))
			errChan <- err
		}
	}()
}

// Shutdown overrides the http.Server Shutdown
// method.
func (s *Server) Shutdown() error {
	var httpsErr error

	// Signal to websocket writer routines and handleSubEvents.
	close(s.shutdown)

	if s.config.TLSConfig.Enabled {
		s.log.Info("shutting down rpc-server (https)", zap.String("endpoint", s.https.Addr))
		httpsErr = s.https.Shutdown(context.Background())
	}

	s.log.Info("shutting down rpc-server", zap.String("endpoint", s.Addr))
	err := s.Server.Shutdown(context.Background())

	// Wait for handleSubEvents to finish.
	<-s.executionCh

	if err == nil {
		return httpsErr
	}
	return err
}

func (s *Server) handleHTTPRequest(w http.ResponseWriter, httpRequest *http.Request) {
	req := request.NewRequest()

	if httpRequest.URL.Path == "/ws" && httpRequest.Method == "GET" {
		// Technically there is a race between this check and
		// s.subscribers modification 20 lines below, but it's tiny
		// and not really critical to bother with it. Some additional
		// clients may sneak in, no big deal.
		s.subsLock.RLock()
		numOfSubs := len(s.subscribers)
		s.subsLock.RUnlock()
		if numOfSubs >= maxSubscribers {
			s.writeHTTPErrorResponse(
				request.NewIn(),
				w,
				response.NewInternalServerError("websocket users limit reached", nil),
			)
			return
		}
		ws, err := upgrader.Upgrade(w, httpRequest, nil)
		if err != nil {
			s.log.Info("websocket connection upgrade failed", zap.Error(err))
			return
		}
		resChan := make(chan response.AbstractResult) // response.Abstract or response.AbstractBatch
		subChan := make(chan *websocket.PreparedMessage, notificationBufSize)
		subscr := &subscriber{writer: subChan, ws: ws}
		s.subsLock.Lock()
		s.subscribers[subscr] = true
		s.subsLock.Unlock()
		go s.handleWsWrites(ws, resChan, subChan)
		s.handleWsReads(ws, resChan, subscr)
		return
	}

	if httpRequest.Method != "POST" {
		s.writeHTTPErrorResponse(
			request.NewIn(),
			w,
			response.NewInvalidParamsError(
				fmt.Sprintf("Invalid method '%s', please retry with 'POST'", httpRequest.Method), nil,
			),
		)
		return
	}

	err := req.DecodeData(httpRequest.Body)
	if err != nil {
		s.writeHTTPErrorResponse(request.NewIn(), w, response.NewParseError("Problem parsing JSON-RPC request body", err))
		return
	}

	resp := s.handleRequest(req, nil)
	s.writeHTTPServerResponse(req, w, resp)
}

func (s *Server) handleRequest(req *request.Request, sub *subscriber) response.AbstractResult {
	if req.In != nil {
		return s.handleIn(req.In, sub)
	}
	resp := make(response.AbstractBatch, len(req.Batch))
	for i, in := range req.Batch {
		resp[i] = s.handleIn(&in, sub)
	}
	return resp
}

func (s *Server) handleIn(req *request.In, sub *subscriber) response.Abstract {
	var res interface{}
	var resErr *response.Error
	if req.JSONRPC != request.JSONRPCVersion {
		return s.packResponse(req, nil, response.NewInvalidParamsError("Problem parsing JSON", fmt.Errorf("invalid version, expected 2.0 got: '%s'", req.JSONRPC)))
	}

	reqParams, err := req.Params()
	if err != nil {
		return s.packResponse(req, nil, response.NewInvalidParamsError("Problem parsing request parameters", err))
	}

	s.log.Debug("processing rpc request",
		zap.String("method", req.Method),
		zap.Stringer("params", reqParams))

	incCounter(req.Method)

	resErr = response.NewMethodNotFoundError(fmt.Sprintf("Method '%s' not supported", req.Method), nil)
	handler, ok := rpcHandlers[req.Method]
	if ok {
		res, resErr = handler(s, *reqParams)
	} else if sub != nil {
		handler, ok := rpcWsHandlers[req.Method]
		if ok {
			res, resErr = handler(s, *reqParams, sub)
		}
	}
	return s.packResponse(req, res, resErr)
}

func (s *Server) handleWsWrites(ws *websocket.Conn, resChan <-chan response.AbstractResult, subChan <-chan *websocket.PreparedMessage) {
	pingTicker := time.NewTicker(wsPingPeriod)
eventloop:
	for {
		select {
		case <-s.shutdown:
			break eventloop
		case event, ok := <-subChan:
			if !ok {
				break eventloop
			}
			ws.SetWriteDeadline(time.Now().Add(wsWriteLimit))
			if err := ws.WritePreparedMessage(event); err != nil {
				break eventloop
			}
		case res, ok := <-resChan:
			if !ok {
				break eventloop
			}
			ws.SetWriteDeadline(time.Now().Add(wsWriteLimit))
			if err := ws.WriteJSON(res); err != nil {
				break eventloop
			}
		case <-pingTicker.C:
			ws.SetWriteDeadline(time.Now().Add(wsWriteLimit))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				break eventloop
			}
		}
	}
	ws.Close()
	pingTicker.Stop()
	// Drain notification channel as there might be some goroutines blocked
	// on it.
drainloop:
	for {
		select {
		case _, ok := <-subChan:
			if !ok {
				break drainloop
			}
		default:
			break drainloop
		}
	}
}

func (s *Server) handleWsReads(ws *websocket.Conn, resChan chan<- response.AbstractResult, subscr *subscriber) {
	ws.SetReadLimit(wsReadLimit)
	ws.SetReadDeadline(time.Now().Add(wsPongLimit))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(wsPongLimit)); return nil })
requestloop:
	for {
		req := request.NewRequest()
		err := ws.ReadJSON(req)
		if err != nil {
			break
		}
		res := s.handleRequest(req, subscr)
		res.RunForErrors(func(jsonErr *response.Error) {
			s.logRequestError(req, jsonErr)
		})
		select {
		case <-s.shutdown:
			break requestloop
		case resChan <- res:
		}

	}
	s.subsLock.Lock()
	delete(s.subscribers, subscr)
	for _, e := range subscr.feeds {
		if e.event != response.InvalidEventID {
			s.unsubscribeFromChannel(e.event)
		}
	}
	s.subsLock.Unlock()
	close(resChan)
	ws.Close()
}

func (s *Server) getBestBlockHash(_ request.Params) (interface{}, *response.Error) {
	return "0x" + s.chain.CurrentBlockHash().StringLE(), nil
}

func (s *Server) getBlockCount(_ request.Params) (interface{}, *response.Error) {
	return s.chain.BlockHeight() + 1, nil
}

func (s *Server) getConnectionCount(_ request.Params) (interface{}, *response.Error) {
	return s.coreServer.PeerCount(), nil
}

func (s *Server) blockHashFromParam(param *request.Param) (util.Uint256, *response.Error) {
	var hash util.Uint256

	if param == nil {
		return hash, response.ErrInvalidParams
	}

	switch param.Type {
	case request.StringT:
		var err error
		hash, err = param.GetUint256()
		if err != nil {
			return hash, response.ErrInvalidParams
		}
	case request.NumberT:
		num, err := s.blockHeightFromParam(param)
		if err != nil {
			return hash, response.ErrInvalidParams
		}
		hash = s.chain.GetHeaderHash(num)
	default:
		return hash, response.ErrInvalidParams
	}
	return hash, nil
}

func (s *Server) getBlock(reqParams request.Params) (interface{}, *response.Error) {
	param := reqParams.Value(0)
	hash, respErr := s.blockHashFromParam(param)
	if respErr != nil {
		return nil, respErr
	}

	block, err := s.chain.GetBlock(hash)
	if err != nil {
		return nil, response.NewInternalServerError(fmt.Sprintf("Problem locating block with hash: %s", hash), err)
	}

	if reqParams.Value(1).GetBoolean() {
		return result.NewBlock(block, s.chain), nil
	}
	writer := io.NewBufBinWriter()
	block.EncodeBinary(writer.BinWriter)
	return writer.Bytes(), nil
}

func (s *Server) getBlockHash(reqParams request.Params) (interface{}, *response.Error) {
	param := reqParams.ValueWithType(0, request.NumberT)
	if param == nil {
		return nil, response.ErrInvalidParams
	}
	num, err := s.blockHeightFromParam(param)
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	return s.chain.GetHeaderHash(num), nil
}

func (s *Server) getVersion(_ request.Params) (interface{}, *response.Error) {
	port, err := s.coreServer.Port()
	if err != nil {
		return nil, response.NewInternalServerError("Cannot fetch tcp port", err)
	}
	return result.Version{
		Magic:     s.network,
		TCPPort:   port,
		Nonce:     s.coreServer.ID(),
		UserAgent: s.coreServer.UserAgent,
	}, nil
}

func (s *Server) getPeers(_ request.Params) (interface{}, *response.Error) {
	peers := result.NewGetPeers()
	peers.AddUnconnected(s.coreServer.UnconnectedPeers())
	peers.AddConnected(s.coreServer.ConnectedPeers())
	peers.AddBad(s.coreServer.BadPeers())
	return peers, nil
}

func (s *Server) getRawMempool(reqParams request.Params) (interface{}, *response.Error) {
	verbose := reqParams.Value(0).GetBoolean()
	mp := s.chain.GetMemPool()
	hashList := make([]util.Uint256, 0)
	for _, item := range mp.GetVerifiedTransactions() {
		hashList = append(hashList, item.Hash())
	}
	if !verbose {
		return hashList, nil
	}
	return result.RawMempool{
		Height:   s.chain.BlockHeight(),
		Verified: hashList,
	}, nil
}

func (s *Server) validateAddress(reqParams request.Params) (interface{}, *response.Error) {
	param := reqParams.Value(0)
	if param == nil {
		return nil, response.ErrInvalidParams
	}
	return validateAddress(param.Value), nil
}

// getApplicationLog returns the contract log based on the specified txid or blockid.
func (s *Server) getApplicationLog(reqParams request.Params) (interface{}, *response.Error) {
	hash, err := reqParams.Value(0).GetUint256()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	trig := trigger.All
	if len(reqParams) > 1 {
		trigString := reqParams.ValueWithType(1, request.StringT)
		if trigString == nil {
			return nil, response.ErrInvalidParams
		}
		trig, err = trigger.FromString(trigString.String())
		if err != nil {
			return nil, response.ErrInvalidParams
		}
	}

	appExecResults, err := s.chain.GetAppExecResults(hash, trigger.All)
	if err != nil {
		return nil, response.NewRPCError("Unknown transaction or block", "", err)
	}
	return result.NewApplicationLog(hash, appExecResults, trig), nil
}

func (s *Server) getNEP17Balances(ps request.Params) (interface{}, *response.Error) {
	u, err := ps.Value(0).GetUint160FromAddressOrHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	as := s.chain.GetNEP17Balances(u)
	bs := &result.NEP17Balances{
		Address:  address.Uint160ToString(u),
		Balances: []result.NEP17Balance{},
	}
	if as != nil {
		cache := make(map[int32]util.Uint160)
		for id, bal := range as.Trackers {
			h, err := s.getHash(id, cache)
			if err != nil {
				continue
			}
			bs.Balances = append(bs.Balances, result.NEP17Balance{
				Asset:       h,
				Amount:      bal.Balance.String(),
				LastUpdated: bal.LastUpdatedBlock,
			})
		}
	}
	return bs, nil
}

func getTimestampsAndLimit(ps request.Params, index int) (uint64, uint64, int, int, error) {
	var start, end uint64
	var limit, page int

	limit = maxTransfersLimit
	pStart, pEnd, pLimit, pPage := ps.Value(index), ps.Value(index+1), ps.Value(index+2), ps.Value(index+3)
	if pPage != nil {
		p, err := pPage.GetInt()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if p < 0 {
			return 0, 0, 0, 0, errors.New("can't use negative page")
		}
		page = p
	}
	if pLimit != nil {
		l, err := pLimit.GetInt()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if l <= 0 {
			return 0, 0, 0, 0, errors.New("can't use negative or zero limit")
		}
		if l > maxTransfersLimit {
			return 0, 0, 0, 0, errors.New("too big limit requested")
		}
		limit = l
	}
	if pEnd != nil {
		val, err := pEnd.GetInt()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		end = uint64(val)
	} else {
		end = uint64(time.Now().Unix() * 1000)
	}
	if pStart != nil {
		val, err := pStart.GetInt()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		start = uint64(val)
	} else {
		start = uint64(time.Now().Add(-time.Hour*24*7).Unix() * 1000)
	}
	return start, end, limit, page, nil
}

func (s *Server) getNEP17Transfers(ps request.Params) (interface{}, *response.Error) {
	u, err := ps.Value(0).GetUint160FromAddressOrHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	start, end, limit, page, err := getTimestampsAndLimit(ps, 1)
	if err != nil {
		return nil, response.NewInvalidParamsError(err.Error(), err)
	}

	bs := &result.NEP17Transfers{
		Address:  address.Uint160ToString(u),
		Received: []result.NEP17Transfer{},
		Sent:     []result.NEP17Transfer{},
	}
	cache := make(map[int32]util.Uint160)
	var resCount, frameCount int
	err = s.chain.ForEachNEP17Transfer(u, func(tr *state.NEP17Transfer) (bool, error) {
		// Iterating from newest to oldest, not yet reached required
		// time frame, continue looping.
		if tr.Timestamp > end {
			return true, nil
		}
		// Iterating from newest to oldest, moved past required
		// time frame, stop looping.
		if tr.Timestamp < start {
			return false, nil
		}
		frameCount++
		// Using limits, not yet reached required page.
		if limit != 0 && page*limit >= frameCount {
			return true, nil
		}

		h, err := s.getHash(tr.Asset, cache)
		if err != nil {
			return false, err
		}

		transfer := result.NEP17Transfer{
			Timestamp: tr.Timestamp,
			Asset:     h,
			Index:     tr.Block,
			TxHash:    tr.Tx,
		}
		if tr.Amount.Sign() > 0 { // token was received
			transfer.Amount = tr.Amount.String()
			if !tr.From.Equals(util.Uint160{}) {
				transfer.Address = address.Uint160ToString(tr.From)
			}
			bs.Received = append(bs.Received, transfer)
		} else {
			transfer.Amount = new(big.Int).Neg(&tr.Amount).String()
			if !tr.To.Equals(util.Uint160{}) {
				transfer.Address = address.Uint160ToString(tr.To)
			}
			bs.Sent = append(bs.Sent, transfer)
		}

		resCount++
		// Using limits, reached limit.
		if limit != 0 && resCount >= limit {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, response.NewInternalServerError("invalid NEP17 transfer log", err)
	}
	return bs, nil
}

// getHash returns the hash of the contract by its ID using cache.
func (s *Server) getHash(contractID int32, cache map[int32]util.Uint160) (util.Uint160, error) {
	if d, ok := cache[contractID]; ok {
		return d, nil
	}
	h, err := s.chain.GetContractScriptHash(contractID)
	if err != nil {
		return util.Uint160{}, err
	}
	cache[contractID] = h
	return h, nil
}

func (s *Server) contractIDFromParam(param *request.Param) (int32, *response.Error) {
	var result int32
	if param == nil {
		return 0, response.ErrInvalidParams
	}
	switch param.Type {
	case request.StringT:
		var err error
		scriptHash, err := param.GetUint160FromHex()
		if err != nil {
			return 0, response.ErrInvalidParams
		}
		cs := s.chain.GetContractState(scriptHash)
		if cs == nil {
			return 0, response.ErrUnknown
		}
		result = cs.ID
	case request.NumberT:
		id, err := param.GetInt()
		if err != nil {
			return 0, response.ErrInvalidParams
		}
		result = int32(id)
	default:
		return 0, response.ErrInvalidParams
	}
	return result, nil
}

// getContractScriptHashFromParam returns the contract script hash by hex contract hash, address, id or native contract name.
func (s *Server) contractScriptHashFromParam(param *request.Param) (util.Uint160, *response.Error) {
	var result util.Uint160
	if param == nil {
		return result, response.ErrInvalidParams
	}
	switch param.Type {
	case request.StringT:
		var err error
		result, err = param.GetUint160FromAddressOrHex()
		if err == nil {
			return result, nil
		}
		name, err := param.GetString()
		if err != nil {
			return result, response.ErrInvalidParams
		}
		result, err = s.chain.GetNativeContractScriptHash(name)
		if err != nil {
			return result, response.NewRPCError("Unknown contract: querying by name is supported for native contracts only", "", nil)
		}
	case request.NumberT:
		id, err := param.GetInt()
		if err != nil {
			return result, response.ErrInvalidParams
		}
		result, err = s.chain.GetContractScriptHash(int32(id))
		if err != nil {
			return result, response.NewRPCError("Unknown contract", "", err)
		}
	default:
		return result, response.ErrInvalidParams
	}
	return result, nil
}

func makeStorageKey(id int32, key []byte) []byte {
	skey := make([]byte, 4+len(key))
	binary.LittleEndian.PutUint32(skey, uint32(id))
	copy(skey[4:], key)
	return skey
}

var errKeepOnlyLatestState = errors.New("'KeepOnlyLatestState' setting is enabled")

func (s *Server) getProof(ps request.Params) (interface{}, *response.Error) {
	if s.chain.GetConfig().KeepOnlyLatestState {
		return nil, response.NewInvalidRequestError("'getproof' is not supported", errKeepOnlyLatestState)
	}
	root, err := ps.Value(0).GetUint256()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	sc, err := ps.Value(1).GetUint160FromHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	key, err := ps.Value(2).GetBytesHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	cs := s.chain.GetContractState(sc)
	if cs == nil {
		return nil, response.ErrInvalidParams
	}
	skey := makeStorageKey(cs.ID, key)
	proof, err := s.chain.GetStateProof(root, skey)
	return &result.GetProof{
		Result: result.ProofWithKey{
			Key:   skey,
			Proof: proof,
		},
		Success: err == nil,
	}, nil
}

func (s *Server) verifyProof(ps request.Params) (interface{}, *response.Error) {
	if s.chain.GetConfig().KeepOnlyLatestState {
		return nil, response.NewInvalidRequestError("'verifyproof' is not supported", errKeepOnlyLatestState)
	}
	root, err := ps.Value(0).GetUint256()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	proofStr, err := ps.Value(1).GetString()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	var p result.ProofWithKey
	if err := p.FromString(proofStr); err != nil {
		return nil, response.ErrInvalidParams
	}
	vp := new(result.VerifyProof)
	val, ok := mpt.VerifyProof(root, p.Key, p.Proof)
	if ok {
		var si state.StorageItem
		r := io.NewBinReaderFromBuf(val)
		si.DecodeBinary(r)
		if r.Err != nil {
			return nil, response.NewInternalServerError("invalid item in trie", r.Err)
		}
		vp.Value = si.Value
	}
	return vp, nil
}

func (s *Server) getStateHeight(_ request.Params) (interface{}, *response.Error) {
	var height = s.chain.BlockHeight()
	var stateHeight uint32
	if s.chain.GetConfig().StateRootInHeader {
		stateHeight = height - 1
	}
	return &result.StateHeight{
		BlockHeight: height,
		StateHeight: stateHeight,
	}, nil
}

func (s *Server) getStateRoot(ps request.Params) (interface{}, *response.Error) {
	p := ps.Value(0)
	if p == nil {
		return nil, response.NewRPCError("Invalid parameter.", "", nil)
	}
	var rt *state.MPTRootState
	var h util.Uint256
	height, err := p.GetInt()
	if err == nil {
		rt, err = s.chain.GetStateRoot(uint32(height))
	} else if h, err = p.GetUint256(); err == nil {
		hdr, err := s.chain.GetHeader(h)
		if err == nil {
			rt, err = s.chain.GetStateRoot(hdr.Index)
		}
	}
	if err != nil {
		return nil, response.NewRPCError("Unknown state root.", "", err)
	}
	return rt, nil
}

func (s *Server) getStorage(ps request.Params) (interface{}, *response.Error) {
	id, rErr := s.contractIDFromParam(ps.Value(0))
	if rErr == response.ErrUnknown {
		return nil, nil
	}
	if rErr != nil {
		return nil, rErr
	}

	key, err := ps.Value(1).GetBytesHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	item := s.chain.GetStorageItem(id, key)
	if item == nil {
		return "", nil
	}

	return hex.EncodeToString(item.Value), nil
}

func (s *Server) getrawtransaction(reqParams request.Params) (interface{}, *response.Error) {
	txHash, err := reqParams.Value(0).GetUint256()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	tx, height, err := s.chain.GetTransaction(txHash)
	if err != nil {
		err = fmt.Errorf("invalid transaction %s: %w", txHash, err)
		return nil, response.NewRPCError("Unknown transaction", err.Error(), err)
	}
	if reqParams.Value(1).GetBoolean() {
		if height == math.MaxUint32 {
			return result.NewTransactionOutputRaw(tx, nil, nil, s.chain), nil
		}
		_header := s.chain.GetHeaderHash(int(height))
		header, err := s.chain.GetHeader(_header)
		if err != nil {
			return nil, response.NewRPCError("Failed to get header for the transaction", err.Error(), err)
		}
		aers, err := s.chain.GetAppExecResults(txHash, trigger.Application)
		if err != nil {
			return nil, response.NewRPCError("Failed to get application log for the transaction", err.Error(), err)
		}
		if len(aers) == 0 {
			return nil, response.NewRPCError("Application log for the transaction is empty", "", nil)
		}
		return result.NewTransactionOutputRaw(tx, header, &aers[0], s.chain), nil
	}
	return tx.Bytes(), nil
}

func (s *Server) getTransactionHeight(ps request.Params) (interface{}, *response.Error) {
	h, err := ps.Value(0).GetUint256()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	_, height, err := s.chain.GetTransaction(h)
	if err != nil || height == math.MaxUint32 {
		return nil, response.NewRPCError("unknown transaction", "", nil)
	}

	return height, nil
}

// getContractState returns contract state (contract information, according to the contract script hash,
// contract id or native contract name).
func (s *Server) getContractState(reqParams request.Params) (interface{}, *response.Error) {
	scriptHash, err := s.contractScriptHashFromParam(reqParams.Value(0))
	if err != nil {
		return nil, err
	}
	cs := s.chain.GetContractState(scriptHash)
	if cs == nil {
		return nil, response.NewRPCError("Unknown contract", "", nil)
	}
	return cs, nil
}

// getBlockSysFee returns the system fees of the block, based on the specified index.
func (s *Server) getBlockSysFee(reqParams request.Params) (interface{}, *response.Error) {
	param := reqParams.ValueWithType(0, request.NumberT)
	if param == nil {
		return 0, response.ErrInvalidParams
	}

	num, err := s.blockHeightFromParam(param)
	if err != nil {
		return 0, response.NewRPCError("Invalid height", "", nil)
	}

	headerHash := s.chain.GetHeaderHash(num)
	block, errBlock := s.chain.GetBlock(headerHash)
	if errBlock != nil {
		return 0, response.NewRPCError(errBlock.Error(), "", nil)
	}

	var blockSysFee int64
	for _, tx := range block.Transactions {
		blockSysFee += tx.SystemFee
	}

	return blockSysFee, nil
}

// getBlockHeader returns the corresponding block header information according to the specified script hash.
func (s *Server) getBlockHeader(reqParams request.Params) (interface{}, *response.Error) {
	param := reqParams.Value(0)
	hash, respErr := s.blockHashFromParam(param)
	if respErr != nil {
		return nil, respErr
	}

	verbose := reqParams.Value(1).GetBoolean()
	h, err := s.chain.GetHeader(hash)
	if err != nil {
		return nil, response.NewRPCError("unknown block", "", nil)
	}

	if verbose {
		return result.NewHeader(h, s.chain), nil
	}

	buf := io.NewBufBinWriter()
	h.EncodeBinary(buf.BinWriter)
	if buf.Err != nil {
		return nil, response.NewInternalServerError("encoding error", buf.Err)
	}
	return buf.Bytes(), nil
}

// getUnclaimedGas returns unclaimed GAS amount of the specified address.
func (s *Server) getUnclaimedGas(ps request.Params) (interface{}, *response.Error) {
	u, err := ps.ValueWithType(0, request.StringT).GetUint160FromAddressOrHex()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	neo, _ := s.chain.GetGoverningTokenBalance(u)
	if neo.Sign() == 0 {
		return result.UnclaimedGas{
			Address: u,
		}, nil
	}
	gas, err := s.chain.CalculateClaimable(u, s.chain.BlockHeight()+1) // +1 as in C#, for the next block.
	if err != nil {
		return nil, response.NewInternalServerError("can't calculate claimable", err)
	}
	return result.UnclaimedGas{
		Address:   u,
		Unclaimed: *gas,
	}, nil
}

// getNextBlockValidators returns validators for the next block with voting status.
func (s *Server) getNextBlockValidators(_ request.Params) (interface{}, *response.Error) {
	var validators keys.PublicKeys

	validators, err := s.chain.GetNextBlockValidators()
	if err != nil {
		return nil, response.NewRPCError("can't get next block validators", "", err)
	}
	enrollments, err := s.chain.GetEnrollments()
	if err != nil {
		return nil, response.NewRPCError("can't get enrollments", "", err)
	}
	var res = make([]result.Validator, 0)
	for _, v := range enrollments {
		res = append(res, result.Validator{
			PublicKey: *v.Key,
			Votes:     v.Votes.Int64(),
			Active:    validators.Contains(v.Key),
		})
	}
	return res, nil
}

// getCommittee returns the current list of NEO committee members
func (s *Server) getCommittee(_ request.Params) (interface{}, *response.Error) {
	keys, err := s.chain.GetCommittee()
	if err != nil {
		return nil, response.NewInternalServerError("can't get committee members", err)
	}
	return keys, nil
}

// invokeFunction implements the `invokeFunction` RPC call.
func (s *Server) invokeFunction(reqParams request.Params) (interface{}, *response.Error) {
	scriptHash, responseErr := s.contractScriptHashFromParam(reqParams.Value(0))
	if responseErr != nil {
		return nil, responseErr
	}
	tx := &transaction.Transaction{}
	checkWitnessHashesIndex := len(reqParams)
	if checkWitnessHashesIndex > 3 {
		signers, _, err := reqParams[3].GetSignersWithWitnesses()
		if err != nil {
			return nil, response.ErrInvalidParams
		}
		tx.Signers = signers
		checkWitnessHashesIndex--
	}
	if len(tx.Signers) == 0 {
		tx.Signers = []transaction.Signer{{Account: util.Uint160{}, Scopes: transaction.None}}
	}
	script, err := request.CreateFunctionInvocationScript(scriptHash, reqParams[1:checkWitnessHashesIndex])
	if err != nil {
		return nil, response.NewInternalServerError("can't create invocation script", err)
	}
	tx.Script = script
	return s.runScriptInVM(trigger.Application, script, tx)
}

// invokescript implements the `invokescript` RPC call.
func (s *Server) invokescript(reqParams request.Params) (interface{}, *response.Error) {
	if len(reqParams) < 1 {
		return nil, response.ErrInvalidParams
	}

	script, err := reqParams[0].GetBytesBase64()
	if err != nil {
		return nil, response.ErrInvalidParams
	}

	tx := &transaction.Transaction{}
	if len(reqParams) > 1 {
		signers, _, err := reqParams[1].GetSignersWithWitnesses()
		if err != nil {
			return nil, response.ErrInvalidParams
		}
		tx.Signers = signers
	}
	if len(tx.Signers) == 0 {
		tx.Signers = []transaction.Signer{{Account: util.Uint160{}, Scopes: transaction.None}}
	}
	tx.Script = script
	return s.runScriptInVM(trigger.Application, script, tx)
}

// invokeContractVerify implements the `invokecontractverify` RPC call.
func (s *Server) invokeContractVerify(reqParams request.Params) (interface{}, *response.Error) {
	scriptHash, responseErr := s.contractScriptHashFromParam(reqParams.Value(0))
	if responseErr != nil {
		return nil, responseErr
	}

	args := make(request.Params, 1)
	args[0] = request.Param{
		Type:  request.StringT,
		Value: manifest.MethodVerify,
	}
	if len(reqParams) > 1 {
		args = append(args, reqParams[1])
	}
	var tx *transaction.Transaction
	if len(reqParams) > 2 {
		signers, witnesses, err := reqParams[2].GetSignersWithWitnesses()
		if err != nil {
			return nil, response.ErrInvalidParams
		}
		tx = &transaction.Transaction{
			Signers: signers,
			Scripts: witnesses,
		}
	}

	cs := s.chain.GetContractState(scriptHash)
	if cs == nil {
		return nil, response.NewRPCError("unknown contract", scriptHash.StringBE(), nil)
	}
	script, err := request.CreateFunctionInvocationScript(cs.Hash, args)
	if err != nil {
		return nil, response.NewInternalServerError("can't create invocation script", err)
	}
	if tx != nil {
		tx.Script = script
	}
	return s.runScriptInVM(trigger.Verification, script, tx)
}

// runScriptInVM runs given script in a new test VM and returns the invocation
// result.
func (s *Server) runScriptInVM(t trigger.Type, script []byte, tx *transaction.Transaction) (*result.Invoke, *response.Error) {
	// When transfering funds, script execution does no auto GAS claim,
	// because it depends on persisting tx height.
	// This is why we provide block here.
	b := block.New(s.network, s.stateRootEnabled)
	b.Index = s.chain.BlockHeight() + 1
	hdr, err := s.chain.GetHeader(s.chain.GetHeaderHash(int(s.chain.BlockHeight())))
	if err != nil {
		return nil, response.NewInternalServerError("can't get last block", err)
	}
	b.Timestamp = hdr.Timestamp + uint64(s.chain.GetConfig().SecondsPerBlock*int(time.Second/time.Millisecond))

	vm := s.chain.GetTestVM(t, tx, b)
	vm.GasLimit = int64(s.config.MaxGasInvoke)
	vm.LoadScriptWithFlags(script, smartcontract.All)
	err = vm.Run()
	var faultException string
	if err != nil {
		faultException = err.Error()
	}
	result := &result.Invoke{
		State:          vm.State().String(),
		GasConsumed:    vm.GasConsumed(),
		Script:         script,
		Stack:          vm.Estack().ToArray(),
		FaultException: faultException,
	}
	return result, nil
}

// submitBlock broadcasts a raw block over the NEO network.
func (s *Server) submitBlock(reqParams request.Params) (interface{}, *response.Error) {
	blockBytes, err := reqParams.ValueWithType(0, request.StringT).GetBytesBase64()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	b := block.New(s.network, s.stateRootEnabled)
	r := io.NewBinReaderFromBuf(blockBytes)
	b.DecodeBinary(r)
	if r.Err != nil {
		return nil, response.ErrInvalidParams
	}
	err = s.chain.AddBlock(b)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrInvalidBlockIndex) || errors.Is(err, core.ErrAlreadyExists):
			return nil, response.ErrAlreadyExists
		default:
			return nil, response.ErrValidationFailed
		}
	}
	return &result.RelayResult{
		Hash: b.Hash(),
	}, nil
}

func (s *Server) sendrawtransaction(reqParams request.Params) (interface{}, *response.Error) {
	var resultsErr *response.Error
	var results interface{}

	if len(reqParams) < 1 {
		return nil, response.ErrInvalidParams
	} else if byteTx, err := reqParams[0].GetBytesBase64(); err != nil {
		return nil, response.ErrInvalidParams
	} else {
		tx, err := transaction.NewTransactionFromBytes(s.network, byteTx)
		if err != nil {
			return nil, response.ErrInvalidParams
		}
		relayReason := s.coreServer.RelayTxn(tx)
		switch relayReason {
		case network.RelaySucceed:
			results = result.RelayResult{
				Hash: tx.Hash(),
			}
		case network.RelayAlreadyExists:
			resultsErr = response.ErrAlreadyExists
		case network.RelayOutOfMemory:
			resultsErr = response.ErrOutOfMemory
		case network.RelayUnableToVerify:
			resultsErr = response.ErrUnableToVerify
		case network.RelayInvalid:
			resultsErr = response.ErrValidationFailed
		case network.RelayPolicyFail:
			resultsErr = response.ErrPolicyFail
		default:
			resultsErr = response.ErrUnknown
		}
	}

	return results, resultsErr
}

// subscribe handles subscription requests from websocket clients.
func (s *Server) subscribe(reqParams request.Params, sub *subscriber) (interface{}, *response.Error) {
	streamName, err := reqParams.Value(0).GetString()
	if err != nil {
		return nil, response.ErrInvalidParams
	}
	event, err := response.GetEventIDFromString(streamName)
	if err != nil || event == response.MissedEventID {
		return nil, response.ErrInvalidParams
	}
	// Optional filter.
	var filter interface{}
	if p := reqParams.Value(1); p != nil {
		switch event {
		case response.BlockEventID:
			if p.Type != request.BlockFilterT {
				return nil, response.ErrInvalidParams
			}
		case response.TransactionEventID:
			if p.Type != request.TxFilterT {
				return nil, response.ErrInvalidParams
			}
		case response.NotificationEventID:
			if p.Type != request.NotificationFilterT {
				return nil, response.ErrInvalidParams
			}
		case response.ExecutionEventID:
			if p.Type != request.ExecutionFilterT {
				return nil, response.ErrInvalidParams
			}
		}
		filter = p.Value
	}

	s.subsLock.Lock()
	defer s.subsLock.Unlock()
	select {
	case <-s.shutdown:
		return nil, response.NewInternalServerError("server is shutting down", nil)
	default:
	}
	var id int
	for ; id < len(sub.feeds); id++ {
		if sub.feeds[id].event == response.InvalidEventID {
			break
		}
	}
	if id == len(sub.feeds) {
		return nil, response.NewInternalServerError("maximum number of subscriptions is reached", nil)
	}
	sub.feeds[id].event = event
	sub.feeds[id].filter = filter
	s.subscribeToChannel(event)
	return strconv.FormatInt(int64(id), 10), nil
}

// subscribeToChannel subscribes RPC server to appropriate chain events if
// it's not yet subscribed for them. It's supposed to be called with s.subsLock
// taken by the caller.
func (s *Server) subscribeToChannel(event response.EventID) {
	switch event {
	case response.BlockEventID:
		if s.blockSubs == 0 {
			s.chain.SubscribeForBlocks(s.blockCh)
		}
		s.blockSubs++
	case response.TransactionEventID:
		if s.transactionSubs == 0 {
			s.chain.SubscribeForTransactions(s.transactionCh)
		}
		s.transactionSubs++
	case response.NotificationEventID:
		if s.notificationSubs == 0 {
			s.chain.SubscribeForNotifications(s.notificationCh)
		}
		s.notificationSubs++
	case response.ExecutionEventID:
		if s.executionSubs == 0 {
			s.chain.SubscribeForExecutions(s.executionCh)
		}
		s.executionSubs++
	}
}

// unsubscribe handles unsubscription requests from websocket clients.
func (s *Server) unsubscribe(reqParams request.Params, sub *subscriber) (interface{}, *response.Error) {
	id, err := reqParams.Value(0).GetInt()
	if err != nil || id < 0 {
		return nil, response.ErrInvalidParams
	}
	s.subsLock.Lock()
	defer s.subsLock.Unlock()
	if len(sub.feeds) <= id || sub.feeds[id].event == response.InvalidEventID {
		return nil, response.ErrInvalidParams
	}
	event := sub.feeds[id].event
	sub.feeds[id].event = response.InvalidEventID
	sub.feeds[id].filter = nil
	s.unsubscribeFromChannel(event)
	return true, nil
}

// unsubscribeFromChannel unsubscribes RPC server from appropriate chain events
// if there are no other subscribers for it. It's supposed to be called with
// s.subsLock taken by the caller.
func (s *Server) unsubscribeFromChannel(event response.EventID) {
	switch event {
	case response.BlockEventID:
		s.blockSubs--
		if s.blockSubs == 0 {
			s.chain.UnsubscribeFromBlocks(s.blockCh)
		}
	case response.TransactionEventID:
		s.transactionSubs--
		if s.transactionSubs == 0 {
			s.chain.UnsubscribeFromTransactions(s.transactionCh)
		}
	case response.NotificationEventID:
		s.notificationSubs--
		if s.notificationSubs == 0 {
			s.chain.UnsubscribeFromNotifications(s.notificationCh)
		}
	case response.ExecutionEventID:
		s.executionSubs--
		if s.executionSubs == 0 {
			s.chain.UnsubscribeFromExecutions(s.executionCh)
		}
	}
}

func (s *Server) handleSubEvents() {
	b, err := json.Marshal(response.Notification{
		JSONRPC: request.JSONRPCVersion,
		Event:   response.MissedEventID,
		Payload: make([]interface{}, 0),
	})
	if err != nil {
		s.log.Error("fatal: failed to marshal overflow event", zap.Error(err))
		return
	}
	overflowMsg, err := websocket.NewPreparedMessage(websocket.TextMessage, b)
	if err != nil {
		s.log.Error("fatal: failed to prepare overflow message", zap.Error(err))
		return
	}
chloop:
	for {
		var resp = response.Notification{
			JSONRPC: request.JSONRPCVersion,
			Payload: make([]interface{}, 1),
		}
		var msg *websocket.PreparedMessage
		select {
		case <-s.shutdown:
			break chloop
		case b := <-s.blockCh:
			resp.Event = response.BlockEventID
			resp.Payload[0] = b
		case execution := <-s.executionCh:
			resp.Event = response.ExecutionEventID
			resp.Payload[0] = execution
		case notification := <-s.notificationCh:
			resp.Event = response.NotificationEventID
			resp.Payload[0] = notification
		case tx := <-s.transactionCh:
			resp.Event = response.TransactionEventID
			resp.Payload[0] = tx
		}
		s.subsLock.RLock()
	subloop:
		for sub := range s.subscribers {
			if sub.overflown.Load() {
				continue
			}
			for i := range sub.feeds {
				if sub.feeds[i].Matches(&resp) {
					if msg == nil {
						b, err = json.Marshal(resp)
						if err != nil {
							s.log.Error("failed to marshal notification",
								zap.Error(err),
								zap.String("type", resp.Event.String()))
							break subloop
						}
						msg, err = websocket.NewPreparedMessage(websocket.TextMessage, b)
						if err != nil {
							s.log.Error("failed to prepare notification message",
								zap.Error(err),
								zap.String("type", resp.Event.String()))
							break subloop
						}
					}
					select {
					case sub.writer <- msg:
					default:
						sub.overflown.Store(true)
						// MissedEvent is to be delivered eventually.
						go func(sub *subscriber) {
							sub.writer <- overflowMsg
							sub.overflown.Store(false)
						}(sub)
					}
					// The message is sent only once per subscriber.
					break
				}
			}
		}
		s.subsLock.RUnlock()
	}
	// It's important to do it with lock held because no subscription routine
	// should be running concurrently to this one. And even if one is to run
	// after unlock, it'll see closed s.shutdown and won't subscribe.
	s.subsLock.Lock()
	// There might be no subscription in reality, but it's not a problem as
	// core.Blockchain allows unsubscribing non-subscribed channels.
	s.chain.UnsubscribeFromBlocks(s.blockCh)
	s.chain.UnsubscribeFromTransactions(s.transactionCh)
	s.chain.UnsubscribeFromNotifications(s.notificationCh)
	s.chain.UnsubscribeFromExecutions(s.executionCh)
	s.subsLock.Unlock()
drainloop:
	for {
		select {
		case <-s.blockCh:
		case <-s.executionCh:
		case <-s.notificationCh:
		case <-s.transactionCh:
		default:
			break drainloop
		}
	}
	// It's not required closing these, but since they're drained already
	// this is safe and it also allows to give a signal to Shutdown routine.
	close(s.blockCh)
	close(s.transactionCh)
	close(s.notificationCh)
	close(s.executionCh)
}

func (s *Server) blockHeightFromParam(param *request.Param) (int, *response.Error) {
	num, err := param.GetInt()
	if err != nil {
		return 0, nil
	}

	if num < 0 || num > int(s.chain.BlockHeight()) {
		return 0, invalidBlockHeightError(0, num)
	}
	return num, nil
}

func (s *Server) packResponse(r *request.In, result interface{}, respErr *response.Error) response.Abstract {
	resp := response.Abstract{
		HeaderAndError: response.HeaderAndError{
			Header: response.Header{
				JSONRPC: r.JSONRPC,
				ID:      r.RawID,
			},
		},
	}
	if respErr != nil {
		resp.Error = respErr
	} else {
		resp.Result = result
	}
	return resp
}

// logRequestError is a request error logger.
func (s *Server) logRequestError(r *request.Request, jsonErr *response.Error) {
	logFields := []zap.Field{
		zap.Error(jsonErr.Cause),
	}

	if r.In != nil {
		logFields = append(logFields, zap.String("method", r.In.Method))
		params, err := r.In.Params()
		if err == nil {
			logFields = append(logFields, zap.Any("params", params))
		}
	}

	s.log.Error("Error encountered with rpc request", logFields...)
}

// writeHTTPErrorResponse writes an error response to the ResponseWriter.
func (s *Server) writeHTTPErrorResponse(r *request.In, w http.ResponseWriter, jsonErr *response.Error) {
	resp := s.packResponse(r, nil, jsonErr)
	s.writeHTTPServerResponse(&request.Request{In: r}, w, resp)
}

func (s *Server) writeHTTPServerResponse(r *request.Request, w http.ResponseWriter, resp response.AbstractResult) {
	// Errors can happen in many places and we can only catch ALL of them here.
	resp.RunForErrors(func(jsonErr *response.Error) {
		s.logRequestError(r, jsonErr)
	})
	if r.In != nil {
		resp := resp.(response.Abstract)
		if resp.Error != nil {
			w.WriteHeader(resp.Error.HTTPCode)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.config.EnableCORSWorkaround {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")
	}

	encoder := json.NewEncoder(w)
	err := encoder.Encode(resp)

	if err != nil {
		switch {
		case r.In != nil:
			s.log.Error("Error encountered while encoding response",
				zap.String("err", err.Error()),
				zap.String("method", r.In.Method))
		case r.Batch != nil:
			s.log.Error("Error encountered while encoding batch response",
				zap.String("err", err.Error()))
		}
	}
}

// validateAddress verifies that the address is a correct NEO address
// see https://docs.neo.org/en-us/node/cli/2.9.4/api/validateaddress.html
func validateAddress(addr interface{}) result.ValidateAddress {
	resp := result.ValidateAddress{Address: addr}
	if addr, ok := addr.(string); ok {
		_, err := address.StringToUint160(addr)
		resp.IsValid = (err == nil)
	}
	return resp
}
