package game

import (
	"context"
	"fmt" // Added
	"log"
	"math/rand"

	"github.com/BrunoSena97/dice_game_backend/internal/constants"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// PlayRound implements the core game logic (<7 / >7 / 7=loss).
func (s *Service) PlayRound(ctx context.Context, betType string, betAmount int64) (GameResult, error) {
	if betType != constants.BetTypeLt7 && betType != constants.BetTypeGt7 {
		log.Printf("GAME SVC ERROR: Invalid bet type received: %s", betType)
		return GameResult{}, fmt.Errorf("%w: %s", ErrInvalidBetType, betType)
	}

	die1 := rand.Intn(6) + 1
	die2 := rand.Intn(6) + 1
	sumResult := die1 + die2

	var outcome string
	var winnings int64 = 0

	switch {
	case sumResult == 7:
		outcome = constants.OutcomeLose
	case sumResult < 7:
		if betType == constants.BetTypeLt7 {
			outcome = constants.OutcomeWin
			winnings = betAmount
		} else {
			outcome = constants.OutcomeLose
		}
	default:
		if betType == constants.BetTypeGt7 {
			outcome = constants.OutcomeWin
			winnings = betAmount
		} else {
			outcome = constants.OutcomeLose
		}
	}

	log.Printf("GAME SVC: Rolled %d + %d = %d. Bet: %s (%d). Outcome: %s, Net Winnings: %d", die1, die2, sumResult, betType, betAmount, outcome, winnings)

	result := GameResult{
		Die1:     die1,
		Die2:     die2,
		Sum:      sumResult,
		Outcome:  outcome,
		Winnings: winnings,
	}

	return result, nil
}
