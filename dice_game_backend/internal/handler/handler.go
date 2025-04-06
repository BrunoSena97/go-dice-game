package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/BrunoSena97/dice_game_backend/internal/config"
	"github.com/BrunoSena97/dice_game_backend/internal/constants"
	"github.com/BrunoSena97/dice_game_backend/internal/game"
	"github.com/BrunoSena97/dice_game_backend/internal/wallet"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type WsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ServerMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type GetBalancePayload struct {
	ClientID string `json:"clientId"`
}

type PlayPayload struct {
	ClientID  string `json:"clientId"`
	BetAmount int64  `json:"betAmount"`
	BetType   string `json:"betType"`
}

type EndPlayPayload struct {
	ClientID string `json:"clientId"`
}

type BalanceUpdatePayload struct {
	ClientID string `json:"clientId"`
	Balance  int64  `json:"balance"`
}

type PlayResultPayload struct {
	ClientID  string `json:"clientId"`
	Die1      int    `json:"die1"`
	Die2      int    `json:"die2"`
	Outcome   string `json:"outcome"`
	BetAmount int64  `json:"betAmount"`
	Winnings  int64  `json:"winnings"`
}

type PlayEndedPayload struct {
	ClientID     string `json:"clientId"`
	FinalBalance int64  `json:"finalBalance"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Handler manages incoming requests/connections.
type Handler struct {
	walletSvc   wallet.WalletService
	redisClient *redis.Client
	gameSvc     game.GameService
	appConfig   config.AppConfig
}

// NewHandler creates a new Handler instance.
func NewHandler(walletSvc wallet.WalletService, redisClient *redis.Client, gameSvc game.GameService, appCfg config.AppConfig) *Handler {
	if walletSvc == nil {
		log.Fatal("WalletService is nil in NewHandler")
	}
	if redisClient == nil {
		log.Fatal("RedisClient is nil in NewHandler")
	}
	if gameSvc == nil {
		log.Fatal("GameService is nil in NewHandler")
	}
	return &Handler{
		walletSvc:   walletSvc,
		redisClient: redisClient,
		gameSvc:     gameSvc,
		appConfig:   appCfg,
	}
}

// HandleClient manages a single websocket connection.
func (h *Handler) HandleClient(conn *websocket.Conn) {
	defer conn.Close()
	// TODO: Implement Client ID assignment and association with 'conn'

	var currentClientID string

	log.Printf("Client connected: %s", conn.RemoteAddr())

	for {
		messageType, messageBytes, err := conn.ReadMessage()
		if err != nil {
			h.handleReadError(conn, err)
			break
		}

		if messageType != websocket.TextMessage {
			log.Printf("Received non-text message type: %d from %s. Skipping.", messageType, conn.RemoteAddr())
			continue
		}

		var msg WsMessage
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error unmarshalling base message from %s: %v. Raw: %s", conn.RemoteAddr(), err, string(messageBytes))
			h.sendError(conn, constants.ErrCodeBadRequest, "Invalid message format")
			continue
		}

		clientID, err := extractClientID(msg)
		if err == nil && clientID != "" {
			currentClientID = clientID
		}

		log.Printf("Received message type: %s for client %s from %s", msg.Type, currentClientID, conn.RemoteAddr())

		switch msg.Type {
		case constants.MsgTypePlay:
			h.handlePlay(conn, msg.Payload, currentClientID)
		case constants.MsgTypeGetBalance:
			h.handleGetBalance(conn, msg.Payload, currentClientID)
		case constants.MsgTypeEndPlay:
			h.handleEndPlay(conn, msg.Payload, currentClientID)
			log.Printf("Closing connection after end_play request for client %s", currentClientID)
			return
		default:
			log.Printf("Received unknown message type: %s from client %s", msg.Type, currentClientID)
			h.sendError(conn, constants.ErrCodeUnknownType, "Unknown message type received.")
		}
	}

	log.Printf("Client handler exiting for %s (Client ID: %s)", conn.RemoteAddr(), currentClientID)
}

// Private handlers
func (h *Handler) handlePlay(conn *websocket.Conn, payloadJSON json.RawMessage, clientID string) {
	var payload PlayPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		log.Printf("[Play-%s] Error unmarshalling payload: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeBadRequest, "Invalid play payload format")
		return
	}

	if clientID == "" || payload.ClientID != clientID {
		log.Printf("[Play-%s] Mismatched or missing ClientID in payload (%s)", clientID, payload.ClientID)
		h.sendError(conn, constants.ErrCodeBadRequest, "Client ID mismatch or missing")
		return
	}

	log.Printf("[Play-%s] Processing [Bet: %d, Type: %s]...",
		clientID, payload.BetAmount, payload.BetType)

	opCtx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.HandlerOpTimeout)*time.Second)
	defer cancel()

	if err := h.validatePlayPayload(payload); err != nil {
		log.Printf("[Play-%s] Validation failed: %v", clientID, err)
		// Send specific error based on validation failure
		errCode, errMsg := validationErrorToCode(err)
		h.sendError(conn, errCode, errMsg)
		return
	}

	activePlayKey := constants.RedisKeyPrefixActivePlay + clientID
	lockAcquired, lockErr := h.acquireRedisLock(opCtx, activePlayKey)
	if lockErr != nil {
		log.Printf("[Play-%s] REDIS ERROR checking/setting lock: %v", clientID, lockErr)
		h.sendError(conn, constants.ErrCodeInternalError, "Failed to check play status.")
		return
	}
	if !lockAcquired {
		log.Printf("[Play-%s] Attempted concurrent play.", clientID)
		h.sendError(conn, constants.ErrCodeActivePlayExists, "Previous play still processing.")
		return
	}

	defer func() {
		if lockAcquired {
			released := h.releaseRedisLock(activePlayKey)
			if !released {
				log.Printf("[Play-%s] WARN: Failed to release active_play lock: %s", clientID, activePlayKey)
				h.sendError(conn, constants.ErrCodeFailedLockRelease, "Lock release failed, state may be inconsistent.")
			}
		}
	}()

	ensureCtx, ensureCancel := context.WithTimeout(opCtx, time.Duration(constants.ShortOpTimeout)*time.Second)
	err := h.walletSvc.EnsureWalletExists(ensureCtx, clientID)
	ensureCancel()
	if err != nil {
		log.Printf("[Play-%s] Error ensuring wallet exists: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeInternalError, "Could not prepare wallet.")
		return
	}

	_, debitErr := h.walletSvc.UpdateBalance(opCtx, clientID, -payload.BetAmount)
	if debitErr != nil {
		if errors.Is(debitErr, wallet.ErrInsufficientFunds) {
			h.sendError(conn, constants.ErrCodeInsufficientFunds, "You do not have enough balance for this bet.")
		} else {
			log.Printf("[Play-%s] Wallet debit error: %v", clientID, debitErr)
			h.sendError(conn, constants.ErrCodeInternalError, "Failed to process bet debit.")
		}
		return
	}
	log.Printf("[Play-%s] Debited %d", clientID, payload.BetAmount)

	gameResult, gameErr := h.gameSvc.PlayRound(opCtx, payload.BetType, payload.BetAmount)
	if gameErr != nil {
		log.Printf("[Play-%s] Error during game logic: %v", clientID, gameErr)
		h.sendError(conn, constants.ErrCodeInternalError, "Failed during game logic.")
		refundCtx, refundCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, refundErr := h.walletSvc.UpdateBalance(refundCtx, clientID, payload.BetAmount)
		refundCancel()
		if refundErr != nil {
			log.Printf("[Play-%s] CRITICAL: Failed to refund debit after game error: %v", clientID, refundErr)
		}
		return
	}

	var finalBalance int64 = -1
	var creditErr error

	if gameResult.Winnings > 0 {
		amountToCredit := payload.BetAmount + gameResult.Winnings
		log.Printf("[Play-%s] Crediting %d (bet %d + win %d)", clientID, amountToCredit, payload.BetAmount, gameResult.Winnings)
		creditCtx, creditCancel := context.WithTimeout(context.Background(), 5*time.Second)
		finalBalance, creditErr = h.walletSvc.UpdateBalance(creditCtx, clientID, amountToCredit)
		creditCancel()

		if creditErr != nil {
			log.Printf("[Play-%s] CRITICAL: Failed to credit winnings %d: %v", clientID, amountToCredit, creditErr)
			h.sendError(conn, constants.ErrCodeInternalError, "Failed to credit winnings.")
			balCtx, balCancel := context.WithTimeout(context.Background(), time.Duration(constants.ShortOpTimeout)*time.Second)
			currentBalance, _ := h.walletSvc.GetBalance(balCtx, clientID)
			balCancel()
			finalBalance = currentBalance
		} else {
			log.Printf("[Play-%s] Credited %d, new balance %d", clientID, amountToCredit, finalBalance)
		}
	} else {
		balCtx, balCancel := context.WithTimeout(context.Background(), time.Duration(constants.ShortOpTimeout)*time.Second)
		currentBalance, balanceErr := h.walletSvc.GetBalance(balCtx, clientID)
		balCancel()
		if balanceErr != nil {
			log.Printf("[Play-%s] Error getting balance after loss: %v", clientID, balanceErr)
			h.sendError(conn, constants.ErrCodeInternalError, "Failed to retrieve balance state.")
			finalBalance = -1
		} else {
			finalBalance = currentBalance
		}
	}

	resultPayload := PlayResultPayload{
		ClientID:  clientID,
		Die1:      gameResult.Die1,
		Die2:      gameResult.Die2,
		Outcome:   gameResult.Outcome,
		BetAmount: payload.BetAmount,
		Winnings:  gameResult.Winnings,
	}
	if err := h.sendMessage(conn, constants.MsgTypePlayResult, resultPayload); err != nil {
		log.Printf("[Play-%s] Error sending play result: %v", clientID, err)
	}

	if finalBalance >= 0 {
		balancePayload := BalanceUpdatePayload{ClientID: clientID, Balance: finalBalance}
		if err := h.sendMessage(conn, constants.MsgTypeBalanceUpdate, balancePayload); err != nil {
			log.Printf("[Play-%s] Error sending final balance update: %v", clientID, err)
		}
	} else {
		log.Printf("[Play-%s] Could not determine final balance reliably after play.", clientID)
	}
}

func (h *Handler) handleGetBalance(conn *websocket.Conn, payloadJSON json.RawMessage, clientID string) {
	var payload GetBalancePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		log.Printf("[GetBalance-%s] Error unmarshalling payload: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeBadRequest, "Invalid get_balance payload format")
		return
	}
	if clientID == "" || payload.ClientID != clientID {
		log.Printf("[GetBalance-%s] Mismatched or missing ClientID in payload (%s)", clientID, payload.ClientID)
		h.sendError(conn, constants.ErrCodeBadRequest, "Client ID mismatch or missing")
		return
	}

	log.Printf("[GetBalance-%s] Processing...", clientID)

	opCtx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.ShortOpTimeout)*time.Second)
	defer cancel()

	if err := h.walletSvc.EnsureWalletExists(opCtx, clientID); err != nil {
		log.Printf("[GetBalance-%s] Error ensuring wallet exists: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeInternalError, "Could not prepare wallet.")
		return
	}

	balance, err := h.walletSvc.GetBalance(opCtx, clientID)
	if err != nil {
		log.Printf("[GetBalance-%s] Internal error getting balance: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeInternalError, "Failed to retrieve balance.")
		return
	}

	balancePayload := BalanceUpdatePayload{ClientID: clientID, Balance: balance}
	if err := h.sendMessage(conn, constants.MsgTypeBalanceUpdate, balancePayload); err != nil {
		log.Printf("[GetBalance-%s] Error sending balance update: %v", clientID, err)
	}
}

func (h *Handler) handleEndPlay(conn *websocket.Conn, payloadJSON json.RawMessage, clientID string) {
	var payload EndPlayPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		log.Printf("[EndPlay-%s] Error unmarshalling payload: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeBadRequest, "Invalid end_play payload format")
	} else if clientID == "" || payload.ClientID != clientID {
		log.Printf("[EndPlay-%s] Mismatched or missing ClientID in payload (%s)", clientID, payload.ClientID)
		h.sendError(conn, constants.ErrCodeBadRequest, "Client ID mismatch or missing")
	}

	log.Printf("[EndPlay-%s] Processing leave request...", clientID)

	opCtx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.ShortOpTimeout)*time.Second)
	defer cancel()

	finalBalance, err := h.walletSvc.GetBalance(opCtx, clientID)
	if err != nil {
		log.Printf("[EndPlay-%s] Error getting final balance: %v", clientID, err)
		h.sendError(conn, constants.ErrCodeInternalError, "Failed to retrieve final balance.")
		finalBalance = -1
	}

	endedPayload := PlayEndedPayload{
		ClientID:     clientID,
		FinalBalance: finalBalance,
	}
	if err := h.sendMessage(conn, constants.MsgTypePlayEnded, endedPayload); err != nil {
		log.Printf("[EndPlay-%s] Error sending play_ended response: %v", clientID, err)
	} else if finalBalance != -1 {
		log.Printf("[EndPlay-%s] Sent confirmation with final balance %d", clientID, finalBalance)
	} else {
		log.Printf("[EndPlay-%s] Sent confirmation (balance retrieval failed)", clientID)
	}

}

// Helpers

// acquireRedisLock tries to set a key with NX-Not Exists and an expiry.
func (h *Handler) acquireRedisLock(ctx context.Context, key string) (bool, error) {
	wasSet, err := h.redisClient.SetNX(ctx, key, "locked", time.Duration(constants.RedisLockTimeout)*time.Second).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Redis lock acquisition timed out for key %s", key)
		}
		return false, fmt.Errorf("redis SetNX error for key %s: %w", key, err)
	}
	if wasSet {
		log.Printf("DEBUG: Acquired active_play lock: %s", key)
	}
	return wasSet, nil
}

// releaseRedisLock explicitly deletes the Redis lock key.
// Returns true on success/key-not-found, false on error.
func (h *Handler) releaseRedisLock(key string) bool {
	delCtx, delCancel := context.WithTimeout(context.Background(), time.Duration(constants.RedisDelTimeout)*time.Second)
	defer delCancel()

	deletedCount, delErr := h.redisClient.Del(delCtx, key).Result()
	if delErr != nil {
		if errors.Is(delErr, context.DeadlineExceeded) {
			log.Printf("Redis lock deletion timed out for key %s", key)
		} else {
			log.Printf("REDIS ERROR deleting lock key %s: %v", key, delErr)
		}
		return false
	}

	if deletedCount > 0 {
		log.Printf("DEBUG: Released active_play lock: %s", key)
	} else {
		log.Printf("DEBUG: Attempted to release lock %s, but key did not exist (DEL returned 0 or lock expired).", key)
	}
	return true
}

// sendError sends a structured error message to the client.
func (h *Handler) sendError(conn *websocket.Conn, code string, message string) {
	log.Printf("Sending error to %s: Code=%s, Msg=%s", conn.RemoteAddr(), code, message)
	errPayload := ErrorPayload{Code: code, Message: message}
	if err := h.sendMessage(conn, constants.MsgTypeError, errPayload); err != nil {
		log.Printf("Failed to send error JSON to client %s: %v", conn.RemoteAddr(), err)
	}
}

// sendMessage marshals and sends a structured message to the client.
func (h *Handler) sendMessage(conn *websocket.Conn, msgType string, payload interface{}) error {
	msg := ServerMessage{Type: msgType, Payload: payload}
	err := conn.WriteJSON(msg)
	if err != nil {
		return fmt.Errorf("failed to write JSON message (type: %s): %w", msgType, err)
	}
	log.Printf("DEBUG: Sent message type: %s to %s", msgType, conn.RemoteAddr())
	return nil
}

// handleReadError logs websocket read errors appropriately.
func (h *Handler) handleReadError(conn *websocket.Conn, err error) {
	remoteAddr := conn.RemoteAddr().String()
	if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
		log.Printf("Error reading message from %s (unexpected close): %v", remoteAddr, err)
	} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("Client %s disconnected normally.", remoteAddr)
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		log.Printf("Read operation cancelled or timed out for client %s: %v", remoteAddr, err)
	} else {
		log.Printf("Error reading message from %s: %v", remoteAddr, err)
	}
}

// validatePlayPayload performs validation specific to the PlayPayload.
func (h *Handler) validatePlayPayload(payload PlayPayload) error {
	if payload.BetAmount <= 0 {
		return fmt.Errorf("%w: amount must be positive (%d)", ErrValidationBetAmount, payload.BetAmount)
	}
	if payload.BetAmount > h.appConfig.MaxBetAmount {
		return fmt.Errorf("%w: amount %d exceeds max %d", ErrValidationBetTooHigh, payload.BetAmount, h.appConfig.MaxBetAmount)
	}
	if payload.BetType != constants.BetTypeLt7 && payload.BetType != constants.BetTypeGt7 {
		return fmt.Errorf("%w: invalid type '%s'", ErrValidationBetType, payload.BetType)
	}
	return nil
}

// Define specific validation error types
var (
	ErrValidationBetAmount  = errors.New("invalid bet amount")
	ErrValidationBetTooHigh = errors.New("bet amount too high")
	ErrValidationBetType    = errors.New("invalid bet type")
)

// validationErrorToCode maps specific validation errors to client-facing error codes/messages.
func validationErrorToCode(err error) (code string, message string) {
	switch {
	case errors.Is(err, ErrValidationBetAmount):
		return constants.ErrCodeInvalidBet, "Bet amount must be greater than zero."
	case errors.Is(err, ErrValidationBetTooHigh):
		return constants.ErrCodeBetTooHigh, "Bet amount exceeds maximum limit."
	case errors.Is(err, ErrValidationBetType):
		return constants.ErrCodeInvalidBetType, "Invalid bet type specified (must be 'lt7' or 'gt7')."
	default:
		return constants.ErrCodeBadRequest, "Invalid play request."
	}
}

// extractClientID attempts to get the ClientID from known payload types.
func extractClientID(msg WsMessage) (string, error) {
	switch msg.Type {
	case constants.MsgTypePlay:
		var p PlayPayload
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			return p.ClientID, nil
		}
	case constants.MsgTypeGetBalance:
		var p GetBalancePayload
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			return p.ClientID, nil
		}
	case constants.MsgTypeEndPlay:
		var p EndPlayPayload
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			return p.ClientID, nil
		}
	}
	return "", errors.New("client ID not found in message payload")
}
