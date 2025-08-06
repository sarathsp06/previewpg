package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	ProductionDB DBConfig `mapstructure:"production_db"`
	FreshDB      DBConfig `mapstructure:"fresh_db"`
	Server       ServerConfig `mapstructure:"server"`
}

type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type ServerConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	LogLevel       string `mapstructure:"log_level"`
	MaxConnections int    `mapstructure:"max_connections"`
}

func LoadConfig() (config Config, err error) {
	viper.AddConfigPath("./config")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}