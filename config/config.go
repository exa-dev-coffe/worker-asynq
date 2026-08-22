package config

import (
	"log"

	"github.com/spf13/viper"
)

type ConfigSchema struct {
	RedisUrl              string `mapstructure:"REDIS_URL"`
	RedisUsername         string `mapstructure:"REDIS_USERNAME"`
	RedisPassword         string `mapstructure:"REDIS_PASSWORD"`
	InternalToken         string `mapstructure:"INTERNAL_TOKEN"`
	Port                  string `mapstructure:"PORT"`
}

var Config ConfigSchema

func LoadConfig() {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("PORT", "8085")

	if err := viper.ReadInConfig(); err != nil {
		log.Println("Warning: .env file not found, reading from environment variables")
	}

	if err := viper.Unmarshal(&Config); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}
}
