package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/BrunoSena97/dice_game_backend/internal/config"
	"github.com/BrunoSena97/dice_game_backend/internal/handler"
	"github.com/BrunoSena97/dice_game_backend/internal/platform/database"
	redisPlatform "github.com/BrunoSena97/dice_game_backend/internal/platform/redis"
	"github.com/BrunoSena97/dice_game_backend/internal/wallet"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		log.Printf("Upgrading connection from origin: %s", r.Header.Get("Origin"))
		return true
	},
}

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	log.Println("Random number generator seeded.")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to Database
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	dbpool, err := database.Connect(dbCtx, cfg.DB)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbpool.Close()
	log.Printf("Connected to database: %s on %s:%d", cfg.DB.DBName, cfg.DB.Host, cfg.DB.Port)

	// Connect to Redis
	redisCtx, redisCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer redisCancel()
	redisClient, err := redisPlatform.ConnectRedis(redisCtx, cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s", cfg.Redis.Addr)

	// Instantiate Services and Handlers
	walletSvc := wallet.NewService(dbpool)
	appHandler := handler.NewHandler(walletSvc, redisClient)

	// WebSocket Handler
	wsHandler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade connection: %v", err)
			return
		}
		go appHandler.HandleClient(conn)
	}
	http.HandleFunc("/ws", wsHandler)

	// Start HTTP Server
	listenAddr := fmt.Sprintf(":%s", cfg.App.ListenPort)
	log.Printf("HTTP server starting on %s", listenAddr)
	err = http.ListenAndServe(listenAddr, nil)
	if err != nil && err != http.ErrServerClosed {
		log.Fatal("ListenAndServe: ", err)
	}
}
