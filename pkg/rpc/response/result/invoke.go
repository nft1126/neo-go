package result

import (
	"encoding/json"
	"errors"

	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/encoding/fixedn"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

// Invoke represents code invocation result and is used by several RPC calls
// that invoke functions, scripts and generic bytecode. Transaction is
// represented in raw serialized format, use transaction.NewTransactionFromBytes
// or GetTransaction method to deserialize it.
type Invoke struct {
	State          string
	GasConsumed    int64
	Script         []byte
	Stack          []stackitem.Item
	FaultException string
	// Transaction represents transaction bytes. Use GetTransaction method to decode it.
	Transaction []byte
}

type invokeAux struct {
	State          string          `json:"state"`
	GasConsumed    fixedn.Fixed8   `json:"gasconsumed"`
	Script         []byte          `json:"script"`
	Stack          json.RawMessage `json:"stack"`
	FaultException string          `json:"exception,omitempty"`
	Transaction    []byte          `json:"tx,omitempty"`
}

// MarshalJSON implements json.Marshaler.
func (r Invoke) MarshalJSON() ([]byte, error) {
	var st json.RawMessage
	arr := make([]json.RawMessage, len(r.Stack))
	for i := range arr {
		data, err := stackitem.ToJSONWithTypes(r.Stack[i])
		if err != nil {
			st = []byte(`"error: recursive reference"`)
			break
		}
		arr[i] = data
	}

	var err error
	if st == nil {
		st, err = json.Marshal(arr)
		if err != nil {
			return nil, err
		}
	}

	return json.Marshal(&invokeAux{
		GasConsumed:    fixedn.Fixed8(r.GasConsumed),
		Script:         r.Script,
		State:          r.State,
		Stack:          st,
		FaultException: r.FaultException,
		Transaction:    r.Transaction,
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *Invoke) UnmarshalJSON(data []byte) error {
	aux := new(invokeAux)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(aux.Stack, &arr); err == nil {
		st := make([]stackitem.Item, len(arr))
		for i := range arr {
			st[i], err = stackitem.FromJSONWithTypes(arr[i])
			if err != nil {
				break
			}
		}
		if err == nil {
			r.Stack = st
		}
	}
	r.GasConsumed = int64(aux.GasConsumed)
	r.Script = aux.Script
	r.State = aux.State
	r.FaultException = aux.FaultException
	r.Transaction = aux.Transaction
	return nil
}

// GetTransaction returns decoded transaction from Invoke.Transaction bytes.
func (r *Invoke) GetTransaction(magic netmode.Magic) (*transaction.Transaction, error) {
	if r.Transaction == nil {
		return nil, errors.New("empty transaction")
	}
	return transaction.NewTransactionFromBytes(magic, r.Transaction)
}
