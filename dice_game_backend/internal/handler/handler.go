package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"strconv"
	"time"

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

type Handler struct {
	walletSvc   wallet.WalletService
	redisClient *redis.Client
}

func NewHandler(walletSvc wallet.WalletService, redisClient *redis.Client) *Handler {
	if walletSvc == nil {
		log.Fatal("WalletService is nil in NewHandler")
	}
	if redisClient == nil {
		log.Fatal("RedisClient is nil in NewHandler")
	}
	return &Handler{
		walletSvc:   walletSvc,
		redisClient: redisClient,
	}
}

func (h *Handler) HandleClient(conn *websocket.Conn) {
	defer conn.Close()
	var currentClientID string
	log.Printf("Client connected: %s", conn.RemoteAddr())

	baseCtx := context.Background()

outerLoop:
	for {
		messageType, messageBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error reading message (unexpected close): %v", err)
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Client disconnected normally: %s", conn.RemoteAddr())
			} else {
				log.Printf("Error reading message: %v", err)
			}
			break
		}

		if messageType != websocket.TextMessage {
			log.Printf("Received non-text message type: %d. Skipping.", messageType)
			continue
		}

		var msg WsMessage
		err = json.Unmarshal(messageBytes, &msg)
		if err != nil {
			log.Printf("Error unmarshalling base message: %v. Raw message: %s", err, string(messageBytes))
			h.sendError(conn, "BAD_REQUEST", "Invalid message format")
			continue
		}

		log.Printf("Received message type: %s from potential client", msg.Type)

		switch msg.Type {
		case "play":
			var payload PlayPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error unmarshalling play payload: %v", err)
				h.sendError(conn, "BAD_REQUEST", "Invalid play payload format")
				continue
			}

			currentClientID = payload.ClientID
			log.Printf("Processing 'play' for client %s [Bet: %d, Type: %s]", currentClientID, payload.BetAmount, payload.BetType)

			ensureCtx, ensureCancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := h.walletSvc.EnsureWalletExists(ensureCtx, currentClientID, "PTS")
			ensureCancel()
			if err != nil {
				log.Printf("Error ensuring wallet exists for client %s: %v", currentClientID, err)
				h.sendError(conn, "INTERNAL_ERROR", "Could not prepare wallet.")
				continue
			}

			opCtxPlay, cancelPlay := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelPlay()

			activePlayKey := "active_play:" + currentClientID
			_, err = h.redisClient.Get(opCtxPlay, activePlayKey).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				log.Printf("REDIS ERROR checking active play for %s: %v", currentClientID, err)
				h.sendError(conn, "INTERNAL_ERROR", "Failed to check play status.")
				continue
			}
			if err == nil {
				log.Printf("Client %s tried to 'play' while previous play is active.", currentClientID)
				h.sendError(conn, "ACTIVE_PLAY_EXISTS", "You must end your previous play before starting a new one.")
				continue
			}

			allowedBets := map[int64]bool{1: true, 5: true, 10: true, 25: true, 50: true, 100: true}
			if !allowedBets[payload.BetAmount] {
				log.Printf("Invalid bet amount %d for client %s", payload.BetAmount, currentClientID)
				h.sendError(conn, "INVALID_BET", "Invalid bet amount specified")
				continue
			}
			maxBet := int64(250)
			if payload.BetAmount > maxBet {
				log.Printf("Bet amount %d exceeds max %d for client %s", payload.BetAmount, maxBet, currentClientID)
				h.sendError(conn, "BET_TOO_HIGH", "Bet amount exceeds maximum limit")
				continue
			}
			if payload.BetType != "lt7" && payload.BetType != "gt7" {
				log.Printf("Invalid bet type '%s' for client %s", payload.BetType, currentClientID)
				h.sendError(conn, "INVALID_BET_TYPE", "Invalid bet type specified (must be 'lt7' or 'gt7')")
				continue
			}

			_, err = h.walletSvc.UpdateBalance(opCtxPlay, currentClientID, -payload.BetAmount)
			if err != nil {
				if errors.Is(err, wallet.ErrInsufficientFunds) {
					h.sendError(conn, "INSUFFICIENT_FUNDS", "You do not have enough balance for this bet.")
				} else if errors.Is(err, wallet.ErrWalletNotFound) {
					h.sendError(conn, "INTERNAL_ERROR", "Wallet state error.")
				} else {
					h.sendError(conn, "INTERNAL_ERROR", "Failed to process bet due to an internal error.")
				}
				continue
			}

			die1 := rand.Intn(6) + 1
			die2 := rand.Intn(6) + 1
			sumResult := die1 + die2
			var outcome string
			var winnings int64 = 0
			if sumResult == 7 {
				outcome = "lose"
			} else if sumResult < 7 {
				if payload.BetType == "lt7" {
					outcome = "win"
					winnings = payload.BetAmount
				} else {
					outcome = "lose"
				}
			} else {
				if payload.BetType == "gt7" {
					outcome = "win"
					winnings = payload.BetAmount
				} else {
					outcome = "lose"
				}
			}

			expiryDuration := 5 * time.Minute
			pendingWinningsKey := "pending_winnings:" + currentClientID
			pipe := h.redisClient.Pipeline()
			pipe.Set(opCtxPlay, pendingWinningsKey, winnings, expiryDuration)
			pipe.Set(opCtxPlay, activePlayKey, "true", expiryDuration)
			_, err = pipe.Exec(opCtxPlay)
			if err != nil {
				h.sendError(conn, "INTERNAL_ERROR", "Failed to save play result state.")
				continue
			}

			resultPayload := PlayResultPayload{
				ClientID:  currentClientID,
				Die1:      die1,
				Die2:      die2,
				Outcome:   outcome,
				BetAmount: payload.BetAmount,
				Winnings:  winnings,
			}
			err = conn.WriteJSON(ServerMessage{Type: "play_result", Payload: resultPayload})
			if err != nil {
				break outerLoop
			}

			balanceCtx, balanceCancel := context.WithTimeout(context.Background(), 3*time.Second)
			balanceAfterDebit, balanceErr := h.walletSvc.GetBalance(balanceCtx, currentClientID)
			balanceCancel()
			if balanceErr == nil {
				balancePayload := BalanceUpdatePayload{ClientID: currentClientID, Balance: balanceAfterDebit}
				err = conn.WriteJSON(ServerMessage{Type: "balance_update", Payload: balancePayload})
				if err != nil {
					break outerLoop
				}
			}
		case "end_play":
			var payload EndPlayPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				h.sendError(conn, "BAD_REQUEST", "Invalid end_play payload format")
				continue
			}

			currentClientID = payload.ClientID
			pendingWinningsKey := "pending_winnings:" + currentClientID
			activePlayKey := "active_play:" + currentClientID

			winningsStr, err := h.redisClient.Get(baseCtx, pendingWinningsKey).Result()
			isActive, activeErr := h.redisClient.Del(baseCtx, activePlayKey).Result()

			if activeErr != nil && !errors.Is(activeErr, redis.Nil) {
				continue
			}
			if isActive == 0 {
				h.sendError(conn, "NO_ACTIVE_PLAY", "No active play found to end.")
				h.redisClient.Del(baseCtx, pendingWinningsKey)
				continue
			}

			if errors.Is(err, redis.Nil) {
				winningsStr = "0"
			} else if err != nil {
				h.sendError(conn, "INTERNAL_ERROR", "Failed to retrieve play state (winnings).")
				continue
			}

			if !errors.Is(err, redis.Nil) {
				_, delWinningsErr := h.redisClient.Del(baseCtx, pendingWinningsKey).Result()
				if delWinningsErr != nil {
					continue
				}
			}

			pendingWinnings, convErr := strconv.ParseInt(winningsStr, 10, 64)
			if convErr != nil {
				h.sendError(conn, "INTERNAL_ERROR", "Failed to process stored winnings.")
				continue
			}

			if pendingWinnings > 0 {
				_, err = h.walletSvc.UpdateBalance(baseCtx, currentClientID, pendingWinnings)
				if err != nil {
					h.sendError(conn, "INTERNAL_ERROR", "Failed to credit winnings.")
				}
			}

			finalBalance, err := h.walletSvc.GetBalance(baseCtx, currentClientID)
			if err != nil {
				h.sendError(conn, "INTERNAL_ERROR", "Failed to retrieve final balance.")
				finalBalance = -1
			}

			if finalBalance >= 0 {
				endedPayload := PlayEndedPayload{ClientID: currentClientID, FinalBalance: finalBalance}
				err = conn.WriteJSON(ServerMessage{Type: "play_ended", Payload: endedPayload})
				if err != nil {
					break outerLoop
				}
			}

			break outerLoop
		}
	}

	log.Printf("Client handler exiting: %s", conn.RemoteAddr())
}

func (h *Handler) sendError(conn *websocket.Conn, code string, message string) {
	errPayload := ErrorPayload{
		Code:    code,
		Message: message,
	}
	errMsg := ServerMessage{
		Type:    "error",
		Payload: errPayload,
	}
	conn.WriteJSON(errMsg)
}
