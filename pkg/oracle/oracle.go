package oracle

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/config"
	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core/blockchainer"
	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/oracle/interfaces"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"go.uber.org/zap"
)

type (
	// Oracle represents oracle module capable of talking
	// with the external world.
	Oracle struct {
		Config

		// mtx protects setting callbacks.
		mtx sync.RWMutex

		// accMtx protects account and oracle nodes.
		accMtx             sync.RWMutex
		currAccount        *wallet.Account
		oracleNodes        keys.PublicKeys
		oracleSignContract []byte

		oracleScript []byte
		oracleHash   util.Uint160

		close      chan struct{}
		requestCh  chan request
		requestMap chan map[uint64]*state.OracleRequest

		// respMtx protects responses map.
		respMtx   sync.RWMutex
		responses map[uint64]*incompleteTx

		wallet *wallet.Wallet
	}

	// Config contains oracle module parameters.
	Config struct {
		Log             *zap.Logger
		Network         netmode.Magic
		MainCfg         config.OracleConfiguration
		Client          HTTPClient
		Chain           blockchainer.Blockchainer
		ResponseHandler interfaces.Broadcaster
		OnTransaction   TxCallback
		URIValidator    URIValidator
	}

	// HTTPClient is an interface capable of doing oracle requests.
	HTTPClient interface {
		Get(string) (*http.Response, error)
	}

	defaultResponseHandler struct{}

	// TxCallback executes on new transactions when they are ready to be pooled.
	TxCallback = func(tx *transaction.Transaction)
	// URIValidator is used to check if provided URL is valid.
	URIValidator = func(*url.URL) error
)

const (
	// MaxAllowedResponse is a maximum size of response in bytes.
	MaxAllowedResponse = math.MaxUint16

	// defaultRequestTimeout is default request timeout.
	defaultRequestTimeout = time.Second * 5
)

// NewOracle returns new oracle instance.
func NewOracle(cfg Config) (*Oracle, error) {
	o := &Oracle{
		Config: cfg,

		close:      make(chan struct{}),
		requestMap: make(chan map[uint64]*state.OracleRequest, 1),
		responses:  make(map[uint64]*incompleteTx),
	}
	o.oracleScript, o.oracleHash = state.CreateNativeContractHash(nativenames.Oracle)
	if o.MainCfg.RequestTimeout == 0 {
		o.MainCfg.RequestTimeout = defaultRequestTimeout
	}
	if o.MainCfg.MaxConcurrentRequests == 0 {
		o.MainCfg.MaxConcurrentRequests = defaultMaxConcurrentRequests
	}
	o.requestCh = make(chan request, o.MainCfg.MaxConcurrentRequests)

	var err error
	w := cfg.MainCfg.UnlockWallet
	if o.wallet, err = wallet.NewWalletFromFile(w.Path); err != nil {
		return nil, err
	}

	haveAccount := false
	for _, acc := range o.wallet.Accounts {
		if err := acc.Decrypt(w.Password); err == nil {
			haveAccount = true
			break
		}
	}
	if !haveAccount {
		return nil, errors.New("no wallet account could be unlocked")
	}

	if o.Client == nil {
		var client http.Client
		client.Transport = &http.Transport{DisableKeepAlives: true}
		client.Timeout = o.MainCfg.RequestTimeout
		o.Client = &client
	}
	if o.ResponseHandler == nil {
		o.ResponseHandler = defaultResponseHandler{}
	}
	if o.OnTransaction == nil {
		o.OnTransaction = func(*transaction.Transaction) {}
	}
	if o.URIValidator == nil {
		o.URIValidator = defaultURIValidator
	}
	return o, nil
}

// Shutdown shutdowns Oracle.
func (o *Oracle) Shutdown() {
	close(o.close)
	o.getBroadcaster().Shutdown()
}

// Run runs must be executed in a separate goroutine.
func (o *Oracle) Run() {
	for i := 0; i < o.MainCfg.MaxConcurrentRequests; i++ {
		go o.runRequestWorker()
	}
	for {
		select {
		case <-o.close:
			return
		case reqs := <-o.requestMap:
			for id, req := range reqs {
				o.requestCh <- request{
					ID:  id,
					Req: req,
				}
			}
		}
	}
}

func (o *Oracle) getOnTransaction() TxCallback {
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	return o.OnTransaction
}

// SetOnTransaction sets callback to pool and broadcast tx.
func (o *Oracle) SetOnTransaction(cb TxCallback) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	o.OnTransaction = cb
}

func (o *Oracle) getBroadcaster() interfaces.Broadcaster {
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	return o.ResponseHandler
}

// SetBroadcaster sets callback to broadcast response.
func (o *Oracle) SetBroadcaster(b interfaces.Broadcaster) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	o.ResponseHandler.Shutdown()
	o.ResponseHandler = b
	go b.Run()
}

// SendResponse implements Broadcaster interface.
func (defaultResponseHandler) SendResponse(*keys.PrivateKey, *transaction.OracleResponse, []byte) {
}

// Run implements Broadcaster interface.
func (defaultResponseHandler) Run() {}

// Shutdown implements Broadcaster interface.
func (defaultResponseHandler) Shutdown() {}
