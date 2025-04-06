package game

import (
	"context"
	"errors"
)

// GameResult holds the outcome of a single dice game round.
type GameResult struct {
	Die1     int
	Die2     int
	Sum      int
	Outcome  string
	Winnings int64
}

// GameService defines the contract for the core game logic.
type GameService interface {
	PlayRound(ctx context.Context, betType string, betAmount int64) (GameResult, error)
}

var ErrInvalidBetType = errors.New("invalid bet type provided")
