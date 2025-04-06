package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/BrunoSena97/dice_game_backend/internal/config"
	"github.com/BrunoSena97/dice_game_backend/internal/constants"
	"github.com/BrunoSena97/dice_game_backend/internal/game"
	"github.com/BrunoSena97/dice_game_backend/internal/handler"
	"github.com/BrunoSena97/dice_game_backend/internal/platform/database"
	redisPlatform "github.com/BrunoSena97/dice_game_backend/internal/platform/redis"
	"github.com/BrunoSena97/dice_game_backend/internal/wallet"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking based on config for production
		// allowedOrigins := cfg.AllowedOrigins
		// origin := r.Header.Get("Origin")
		// for _, allowed := range allowedOrigins {
		//     if origin == allowed {
		// 		log.Printf("Upgrading WebSocket connection from allowed origin: %s", origin)
		// 		return true
		// 	}
		// }
		// log.Printf("WebSocket connection blocked from origin: %s", origin)
		// return false
		log.Printf("WARN: Allowing WebSocket upgrade from any origin: %s (Dev only!)", r.Header.Get("Origin"))
		return true
	},
}

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	log.Println("Global random number generator seeded.")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}
	log.Println("Configuration loaded.")
	if cfg.IsDevMode {
		log.Println("WARN: Running in Development Mode.")
	}

	mainCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbpool := connectDB(mainCtx, cfg.DB)
	defer func() {
		log.Println("Closing database connection pool...")
		dbpool.Close()
		log.Println("Database connection pool closed.")
	}()

	redisClient := connectRedis(mainCtx, cfg.Redis)

	var walletSvc wallet.WalletService = wallet.NewService(dbpool)
	var gameSvc game.GameService = game.NewService()

	appHandler := handler.NewHandler(walletSvc, redisClient, gameSvc, cfg.App)

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler(appHandler))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	listenAddr := fmt.Sprintf(":%s", cfg.App.ListenPort)
	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.App.ReadTimeout,
		WriteTimeout: cfg.App.WriteTimeout,
		IdleTimeout:  cfg.App.IdleTimeout,
	}

	go func() {
		log.Printf("HTTP server starting on %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {

			log.Fatalf("FATAL: ListenAndServe error: %v", err)
		}
		log.Println("HTTP server ListenAndServe routine finished.")
	}()

	<-mainCtx.Done()

	log.Println("Shutdown signal received. Initiating graceful shutdown...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Duration(constants.ShutdownTimeout)*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("ERROR: HTTP server graceful shutdown failed: %v", err)
	} else {
		log.Println("HTTP server gracefully stopped.")
	}

	log.Println("Shutdown complete.")
}

// wsHandler creates the HTTP handler function for WebSocket upgrades.
func wsHandler(appHandler *handler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade connection from %s: %v", r.RemoteAddr, err)
			return
		}
		go appHandler.HandleClient(conn)
	}
}

// connectDB helper function with context for cancellation.
func connectDB(ctx context.Context, cfg database.Config) *pgxpool.Pool {
	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(constants.DBConnectTimeout)*time.Second)
	defer cancel()

	dbpool, err := database.Connect(connectCtx, cfg)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to database: %v", err)
	}
	log.Printf("Connected to database: %s on %s:%d", cfg.DBName, cfg.Host, cfg.Port)
	return dbpool
}

// connectRedis helper function with context for cancellation.
func connectRedis(ctx context.Context, cfg redisPlatform.Config) *redis.Client {
	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(constants.RedisConnectTimeout)*time.Second)
	defer cancel()

	redisClient, err := redisPlatform.ConnectRedis(connectCtx, cfg)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s (DB %s)", cfg.Addr, cfg.DB)
	return redisClient
}
