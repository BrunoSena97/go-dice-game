package config

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/BrunoSena97/dice_game_backend/internal/constants"
	"github.com/BrunoSena97/dice_game_backend/internal/platform/database"
	redisPlatform "github.com/BrunoSena97/dice_game_backend/internal/platform/redis"
	"github.com/joho/godotenv"
)

// Environment Variable Keys
const (
	envDevMode      = "dev"
	envDBHostDev    = "DB_HOST_DEV"
	envDBPortDev    = "DB_PORT_DEV"
	envDBHost       = "DB_HOST"
	envDBPort       = "DB_PORT"
	envDBUser       = "DB_USER"
	envDBPassword   = "DB_PASSWORD"
	envDBName       = "DB_NAME"
	envDBSSLMode    = "DB_SSLMODE"
	envRedisAddrDev = "REDIS_ADDR_DEV"
	envRedisAddr    = "REDIS_ADDR"
	envRedisPass    = "REDIS_PASSWORD"
	envRedisDB      = "REDIS_DB"
	envListenPort   = "LISTEN_PORT"
	envMaxBet       = "MAX_BET_AMOUNT"
)

type Config struct {
	DB        database.Config
	Redis     redisPlatform.Config
	App       AppConfig
	IsDevMode bool
}

type AppConfig struct {
	ListenPort   string
	MaxBetAmount int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func LoadConfig() (*Config, error) {
	devModePtr := flag.Bool(envDevMode, false, "Enable development mode defaults")
	flag.Parse()
	isDev := *devModePtr

	if isDev {
		log.Println("Development mode enabled (-dev flag). Attempting to load .env file...")
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: Could not load .env file: %v", err)
		}
	}

	// Database configuration
	var dbHost string
	var dbPort int
	if isDev {
		dbHost = getEnv(envDBHostDev, "localhost")
		dbPort = parseEnvInt(envDBPortDev, 5433)
	} else {
		dbHost = getEnv(envDBHost, "db")
		dbPort = parseEnvInt(envDBPort, 5432)
	}

	dbCfg := database.Config{
		Host:     dbHost,
		Port:     dbPort,
		User:     getEnv(envDBUser, "postgres"),
		Password: getEnv(envDBPassword, ""),
		DBName:   getEnv(envDBName, "postgres"),
		SSLMode:  getEnv(envDBSSLMode, "disable"),
	}

	// Redis configuration
	redisCfg := redisPlatform.Config{
		Addr:     getEnv(envRedisAddr, "redis:6379"),
		Password: getEnv(envRedisPass, ""),
		DB:       getEnv(envRedisDB, "0"),
	}
	if isDev {
		redisCfg.Addr = getEnv(envRedisAddrDev, "localhost:6380")
	}

	// Application configuration
	appCfg := AppConfig{
		ListenPort:   getEnv(envListenPort, "8080"),
		MaxBetAmount: int64(parseEnvInt(envMaxBet, 250)),
		ReadTimeout:  time.Duration(constants.DefaultReadTimeout) * time.Second,
		WriteTimeout: time.Duration(constants.DefaultWriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(constants.DefaultIdleTimeout) * time.Second,
	}

	cfg := &Config{
		DB:        dbCfg,
		Redis:     redisCfg,
		App:       appCfg,
		IsDevMode: isDev,
	}

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	if key != "DB_PASSWORD" && key != "REDIS_PASSWORD" {
		log.Printf("Using fallback for environment variable %s: %s", key, fallback)
	}
	return fallback
}

// parseEnvInt parses an environment variable as an integer or returns a fallback value
func parseEnvInt(key string, fallback int) int {
	valueStr := getEnv(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Invalid integer value for %s: %s. Using fallback: %d", key, valueStr, fallback)
		return fallback
	}
	return value
}
