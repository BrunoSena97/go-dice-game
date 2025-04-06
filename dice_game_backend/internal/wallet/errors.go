package wallet

import "errors"

// Define specific error types.
var (
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrUpdateFailed      = errors.New("wallet balance update failed unexpectedly")
)
