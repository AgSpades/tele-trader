package main

import (
	"fmt"
	"log"
	"os"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file. Please ensure .env exists at project root")
	}
	// api keys and credentials
	telegramAppApi := os.Getenv("TELEGRAM_APP_API")
	telegramAppHash := os.Getenv("TELEGRAM_APP_API_HASH")

	if telegramAppApi != "" && telegramAppHash != "" {
		fmt.Println("Telegram API credentials loaded successfully.")
	} else {
		log.Fatal("Empty telegram credentials")
	}

	fmt.Println("Welcome to Teletrader!")
}
