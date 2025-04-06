package config

import (
	"flag"
	"log"
	"os"
	"strconv"

	"github.com/BrunoSena97/dice_game_backend/internal/platform/database"
	redisPlatform "github.com/BrunoSena97/dice_game_backend/internal/platform/redis"
	"github.com/joho/godotenv"
)

type Config struct {
	DB        database.Config      // Database configuration
	Redis     redisPlatform.Config // Redis configuration
	App       AppConfig            // Application-specific configuration
	IsDevMode bool                 // Development mode flag
}

type AppConfig struct {
	ListenPort string
}

// LoadConfig loads configuration from environment variables and flags
func LoadConfig() (*Config, error) {
	devModePtr := flag.Bool("dev", false, "Enable development mode defaults")
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
		dbHost = getEnv("DB_HOST_DEV", "localhost")
		dbPort = parseEnvInt("DB_PORT_DEV", 5433)
	} else {
		dbHost = getEnv("DB_HOST", "db")
		dbPort = parseEnvInt("DB_PORT", 5432)
	}

	dbCfg := database.Config{
		Host:     dbHost,
		Port:     dbPort,
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", ""),
		DBName:   getEnv("DB_NAME", "postgres"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	// Redis configuration
	redisCfg := redisPlatform.Config{
		Addr:     getEnv("REDIS_ADDR", "redis:6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       getEnv("REDIS_DB", "0"),
	}
	if isDev {
		redisCfg.Addr = getEnv("REDIS_ADDR_DEV", "localhost:6380")
	}

	// Application configuration
	appCfg := AppConfig{
		ListenPort: getEnv("LISTEN_PORT", "8080"),
	}

	// Final configuration struct
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
