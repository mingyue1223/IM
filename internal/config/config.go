package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	Redis    RedisConfig    `yaml:"redis"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	JWT      JWTConfig      `yaml:"jwt"`
	LLM      LLMConfig      `yaml:"llm"`
	File     FileConfig     `yaml:"file"`
}

type ServerConfig struct {
	Port      int    `yaml:"port"`
	WsPath    string `yaml:"ws_path"`
	UploadDir string `yaml:"upload_dir"`
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"db_name"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type RabbitMQConfig struct {
	URL string `yaml:"url"`
}

type JWTConfig struct {
	Secret         string `yaml:"secret"`
	AccessExpHours int    `yaml:"access_exp_hours"`
	RefreshExpDays int    `yaml:"refresh_exp_days"`
}

type LLMConfig struct {
	Provider  string `yaml:"provider"`   // "openai" 或 "domestic"
	APIKey    string `yaml:"api_key"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"` // LLM响应的最大token数（默认2048）
}

type FileConfig struct {
	MaxSizeMB   int      `yaml:"max_size_mb"`
	AllowedExts []string `yaml:"allowed_exts"`
	UploadDir   string   `yaml:"upload_dir"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
