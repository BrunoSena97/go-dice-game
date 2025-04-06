package constants

// Message Types Client -> Server & Server -> Client
const (
	MsgTypePlay          = "play"
	MsgTypeEndPlay       = "end_play"
	MsgTypeGetBalance    = "get_balance"
	MsgTypePlayResult    = "play_result"
	MsgTypeBalanceUpdate = "balance_update"
	MsgTypePlayEnded     = "play_ended"
	MsgTypeError         = "error"
)

// Error Codes Server -> Client
const (
	ErrCodeBadRequest        = "BAD_REQUEST"
	ErrCodeInternalError     = "INTERNAL_ERROR"
	ErrCodeActivePlayExists  = "ACTIVE_PLAY_EXISTS"
	ErrCodeInvalidBet        = "INVALID_BET"
	ErrCodeBetTooHigh        = "BET_TOO_HIGH"
	ErrCodeInvalidBetType    = "INVALID_BET_TYPE"
	ErrCodeInsufficientFunds = "INSUFFICIENT_FUNDS"
	ErrCodeWalletNotFound    = "WALLET_NOT_FOUND"
	ErrCodeUnknownType       = "UNKNOWN_TYPE"
	ErrCodeFailedLockRelease = "FAILED_LOCK_RELEASE"
)

// Game Related
const (
	BetTypeLt7  = "lt7"
	BetTypeGt7  = "gt7"
	OutcomeWin  = "win"
	OutcomeLose = "lose"
)

// Redis Keys
const (
	RedisKeyPrefixActivePlay = "active_play:"
)

// Wallet Defaults
const (
	DefaultCurrency       = "PTS"
	DefaultInitialBalance = 500
)

// Timeouts
const (
	DefaultReadTimeout  = 5
	DefaultWriteTimeout = 10
	DefaultIdleTimeout  = 120
	ShutdownTimeout     = 15
	DBConnectTimeout    = 10
	RedisConnectTimeout = 10
	HandlerOpTimeout    = 10
	ShortOpTimeout      = 3
	RedisLockTimeout    = 15
	RedisDelTimeout     = 2
)
