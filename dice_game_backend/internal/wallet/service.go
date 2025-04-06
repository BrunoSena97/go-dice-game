package wallet

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/BrunoSena97/dice_game_backend/internal/constants"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletService interface {
	GetBalance(ctx context.Context, userID string) (int64, error)
	UpdateBalance(ctx context.Context, userID string, amountChange int64) (int64, error)
	EnsureWalletExists(ctx context.Context, userID string) error
}

type Service struct {
	dbpool *pgxpool.Pool
}

func NewService(dbpool *pgxpool.Pool) *Service {
	if dbpool == nil {
		log.Fatal("WalletService requires a non-nil dbpool")
	}
	return &Service{dbpool: dbpool}
}

// EnsureWalletExists creates a wallet if it doesn't exist, using default constants.
func (s *Service) EnsureWalletExists(ctx context.Context, userID string) error {
	query := `
		INSERT INTO wallets (user_id, balance, currency, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id) DO NOTHING;
	`
	_, err := s.dbpool.Exec(ctx, query, userID, constants.DefaultInitialBalance, constants.DefaultCurrency)
	if err != nil {
		log.Printf("Error ensuring wallet for user %s: %v", userID, err)
		return fmt.Errorf("failed to ensure wallet for user %s: %w", userID, err)
	}
	log.Printf("Wallet ensured for user %s (created if didn't exist)", userID)
	return nil
}

func (s *Service) GetBalance(ctx context.Context, userID string) (int64, error) {
	query := `SELECT balance FROM wallets WHERE user_id = $1;`
	var balance int64

	err := s.dbpool.QueryRow(ctx, query, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Wallet not found for user %s during GetBalance", userID)
			return 0, ErrWalletNotFound
		}
		log.Printf("Error getting balance for user %s: %v", userID, err)
		return 0, fmt.Errorf("database error getting balance for user %s: %w", userID, err)
	}

	return balance, nil
}

// UpdateBalance updates the user's balance within a transaction.
// It returns balance on success.
func (s *Service) UpdateBalance(ctx context.Context, userID string, amountChange int64) (int64, error) {
	var newBalance int64

	tx, err := s.dbpool.Begin(ctx)
	if err != nil {
		log.Printf("Error starting transaction for UpdateBalance (user: %s): %v", userID, err)
		return 0, fmt.Errorf("failed to start db transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	querySelect := `SELECT balance FROM wallets WHERE user_id = $1 FOR UPDATE;`
	var currentBalance int64
	err = tx.QueryRow(ctx, querySelect, userID).Scan(&currentBalance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Wallet not found for user %s during UpdateBalance transaction", userID)
			return 0, ErrWalletNotFound
		}
		log.Printf("Error selecting balance in transaction (user: %s): %v", userID, err)
		return 0, fmt.Errorf("db error selecting balance for update: %w", err)
	}

	potentialNewBalance := currentBalance + amountChange
	if potentialNewBalance < 0 {
		log.Printf("Insufficient funds for user %s (current: %d, change: %d)", userID, currentBalance, amountChange)
		return 0, ErrInsufficientFunds
	}

	queryUpdate := `
		UPDATE wallets
		SET balance = $1, updated_at = NOW()
		WHERE user_id = $2;
	`
	cmdTag, err := tx.Exec(ctx, queryUpdate, potentialNewBalance, userID)
	if err != nil {
		log.Printf("Error updating balance in transaction (user: %s): %v", userID, err)
		return 0, fmt.Errorf("db error updating balance: %w", err)
	}

	if cmdTag.RowsAffected() != 1 {
		log.Printf("Unexpected number of rows affected (%d) during balance update for user %s", cmdTag.RowsAffected(), userID)
		return 0, ErrUpdateFailed
	}

	err = tx.Commit(ctx)
	if err != nil {
		log.Printf("Error committing transaction for UpdateBalance (user: %s): %v", userID, err)
		return 0, fmt.Errorf("failed to commit db transaction: %w", err)
	}

	newBalance = potentialNewBalance
	log.Printf("User %s balance updated by %d to %d", userID, amountChange, newBalance)
	return newBalance, nil
}
