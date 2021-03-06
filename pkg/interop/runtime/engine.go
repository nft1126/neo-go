package runtime

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/blockchain"
)

// GetScriptContainer returns the transaction that initially triggered current
// execution context. It never changes in a single execution, no matter how deep
// this execution goes. This function uses
// `System.Runtime.GetScriptContainer` syscall.
func GetScriptContainer() *blockchain.Transaction {
	return &blockchain.Transaction{}
}

// GetExecutingScriptHash returns script hash (160 bit in BE form represented
// as 20-byte slice) of the contract that is currently being executed. Any
// AppCall can change the value returned by this function if it calls a
// different contract. This function uses
// `System.Runtime.GetExecutingScriptHash` syscall.
func GetExecutingScriptHash() interop.Hash160 {
	return nil
}

// GetCallingScriptHash returns script hash (160 bit in BE form represented
// as 20-byte slice) of the contract that started the execution of the currently
// running context (caller of current contract or function), so it's one level
// above the GetExecutingScriptHash in the call stack. It uses
// `System.Runtime.GetCallingScriptHash` syscall.
func GetCallingScriptHash() interop.Hash160 {
	return nil
}

// GetEntryScriptHash returns script hash (160 bit in BE form represented
// as 20-byte slice) of the contract that initially started current execution
// (this is a script that is contained in a transaction returned by
// GetScriptContainer) execution from the start. This function uses
// `System.Runtime.GetEntryScriptHash` syscall.
func GetEntryScriptHash() interop.Hash160 {
	return nil
}
