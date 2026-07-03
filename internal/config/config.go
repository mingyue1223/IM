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
	Moment   MomentConfig   `yaml:"moment"`
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

// MomentConfig 控制朋友圈 Feed 的推拉结合策略。
type MomentConfig struct {
	// BigUserFriendThreshold 是"大V"判定的好友数阈值。
	// 好友数 > 阈值的作者发布动态时不再写扩散到好友收件箱，
	// 仅存入作者自己的寄件箱，由好友读取时拉取合并。默认 500。
	BigUserFriendThreshold int `yaml:"big_user_friend_threshold"`
	// TimelineMaxLen 是每个用户收件箱/寄件箱 ZSet 的最大保留条数，
	// 扇出后按此长度裁剪最旧的条目，防止无限膨胀。默认 1000。
	TimelineMaxLen int `yaml:"timeline_max_len"`
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

	// 朋友圈 Feed 默认值（未配置时生效）
	if cfg.Moment.BigUserFriendThreshold <= 0 {
		cfg.Moment.BigUserFriendThreshold = 500
	}
	if cfg.Moment.TimelineMaxLen <= 0 {
		cfg.Moment.TimelineMaxLen = 1000
	}

	return &cfg, nil
}
