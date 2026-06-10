package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Database  DatabaseConfig  `yaml:"database"`
	Security  SecurityConfig  `yaml:"security"`
	Thumbnails ThumbnailsConfig `yaml:"thumbnails"`
	CORS      CORSConfig      `yaml:"cors"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
	MaxUploadSize int64         `yaml:"max_upload_size"`
}

type StorageConfig struct {
	DataDir       string `yaml:"data_dir"`
	TmpDir        string `yaml:"tmp_dir"`
	ThumbnailDir  string `yaml:"thumbnail_dir"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type SecurityConfig struct {
	APIKeys      []APIKeyConfig `yaml:"api_keys"`
	AllowedTypes []string       `yaml:"allowed_types"`
	BlockedTypes []string       `yaml:"blocked_types"`
}

type APIKeyConfig struct {
	Name        string   `yaml:"name"`
	Key         string   `yaml:"key"`
	Permissions []string `yaml:"permissions"`
}

type ThumbnailsConfig struct {
	Enabled     bool `yaml:"enabled"`
	DefaultSize int  `yaml:"default_size"`
	MaxSize     int  `yaml:"max_size"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	MaxAge         int      `yaml:"max_age"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:          "0.0.0.0",
			Port:          8080,
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  300 * time.Second,
			MaxUploadSize: 5 << 30, // 5GB
		},
		Storage: StorageConfig{
			DataDir:      "./data",
			TmpDir:       "./data/files/tmp",
			ThumbnailDir: "./data/thumbnails",
		},
		Database: DatabaseConfig{
			Path: "./data/metadata.db",
		},
		Security: SecurityConfig{
			APIKeys: []APIKeyConfig{
				{
					Name:        "admin",
					Key:         "bucketd_admin_key_change_me",
					Permissions: []string{"read", "write", "delete", "admin"},
				},
			},
		},
		Thumbnails: ThumbnailsConfig{
			Enabled:     true,
			DefaultSize: 200,
			MaxSize:     800,
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{"*"},
			MaxAge:         86400,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BUCKETD_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("BUCKETD_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
	}
	if v := os.Getenv("BUCKETD_STORAGE_DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv("BUCKETD_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("BUCKETD_SERVER_MAX_UPLOAD_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Server.MaxUploadSize = n
		}
	}
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) FilesDir() string {
	return c.Storage.DataDir + "/files"
}

func (c *Config) IsTypeAllowed(mimeType string) bool {
	if len(c.Security.BlockedTypes) > 0 {
		for _, blocked := range c.Security.BlockedTypes {
			if matchMimeType(blocked, mimeType) {
				return false
			}
		}
	}
	if len(c.Security.AllowedTypes) == 0 {
		return true
	}
	for _, allowed := range c.Security.AllowedTypes {
		if matchMimeType(allowed, mimeType) {
			return true
		}
	}
	return false
}

func matchMimeType(pattern, mimeType string) bool {
	if pattern == mimeType {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(mimeType, prefix)
	}
	return false
}
