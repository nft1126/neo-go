package interfaces

import (
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
)

// Broadcaster broadcasts oracle responses.
type Broadcaster interface {
	SendResponse(priv *keys.PrivateKey, resp *transaction.OracleResponse, txSig []byte)
}
