package wallet

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletService interface {
	GetBalance(ctx context.Context, userID string) (int64, error)
	UpdateBalance(ctx context.Context, userID string, amountChange int64) (int64, error)
	EnsureWalletExists(ctx context.Context, userID string, defaultCurrency string) error
}

type Service struct {
	dbpool *pgxpool.Pool
}

func NewService(dbpool *pgxpool.Pool) *Service {
	return &Service{
		dbpool: dbpool,
	}
}

func (s *Service) EnsureWalletExists(ctx context.Context, userID string, defaultCurrency string) error {
	if defaultCurrency == "" {
		defaultCurrency = "PTS"
	}
	query := `
        INSERT INTO wallets (user_id, balance, currency, created_at, updated_at)
        VALUES ($1, 500, $2, NOW(), NOW())
        ON CONFLICT (user_id) DO NOTHING;
    `
	_, err := s.dbpool.Exec(ctx, query, userID, defaultCurrency)
	if err != nil {
		log.Printf("Error ensuring wallet for user %s: %v", userID, err)
		return err
	}
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
		return 0, err
	}

	return balance, nil
}

func (s *Service) UpdateBalance(ctx context.Context, userID string, amountChange int64) (int64, error) {
	var newBalance int64

	tx, err := s.dbpool.Begin(ctx)
	if err != nil {
		log.Printf("Error starting transaction for UpdateBalance (user: %s): %v", userID, err)
		return 0, err
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
		return 0, err
	}

	potentialNewBalance := currentBalance + amountChange
	if potentialNewBalance < 0 {
		log.Printf("Insufficient funds for user %s (current: %d, change: %d)", userID, currentBalance, amountChange)
		return currentBalance, ErrInsufficientFunds
	}

	queryUpdate := `
        UPDATE wallets
        SET balance = $1, updated_at = NOW()
        WHERE user_id = $2;
    `
	cmdTag, err := tx.Exec(ctx, queryUpdate, potentialNewBalance, userID)
	if err != nil {
		log.Printf("Error updating balance in transaction (user: %s): %v", userID, err)
		return currentBalance, err
	}

	if cmdTag.RowsAffected() != 1 {
		log.Printf("Unexpected number of rows affected (%d) during balance update for user %s", cmdTag.RowsAffected(), userID)
		return currentBalance, errors.New("wallet balance update failed unexpectedly")
	}

	err = tx.Commit(ctx)
	if err != nil {
		log.Printf("Error committing transaction for UpdateBalance (user: %s): %v", userID, err)
		return currentBalance, err
	}

	newBalance = potentialNewBalance
	return newBalance, nil
}
