package config

import (
	"log"

	"github.com/spf13/viper"
)

type ConfigSchema struct {
	RedisUrl      string
	RedisUsername string
	RedisPassword string
	InternalToken string
	Port          string
}

var Config ConfigSchema

func LoadConfig() {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Println("Warning: .env file not found, reading from environment variables")
	}

	redisUrl := viper.GetString("REDIS_URL")
	if redisUrl == "" {
		redisUrl = "localhost:6379"
	}

	port := viper.GetString("PORT")
	if port == "" {
		port = "8085"
	}

	Config = ConfigSchema{
		RedisUrl:      redisUrl,
		RedisUsername: viper.GetString("REDIS_USERNAME"),
		RedisPassword: viper.GetString("REDIS_PASSWORD"),
		InternalToken: viper.GetString("INTERNAL_TOKEN"),
		Port:          port,
	}
}
