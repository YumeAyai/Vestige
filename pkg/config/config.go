package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LocalBackend  LocalBackendConfig  `yaml:"local_backend"`
	TrackingCloud TrackingCloudConfig `yaml:"tracking_cloud"`
	Frontend      FrontendConfig      `yaml:"frontend"`
	SCF           SCFConfig           `yaml:"scf"`
}

type LocalBackendConfig struct {
	Addr                string `yaml:"addr"`
	DBPath              string `yaml:"db_path"`
	TrackingBaseURL     string `yaml:"tracking_base_url"`
	TrackingSourceToken string `yaml:"tracking_source_token"`
	QRCodeTargetURL     string `yaml:"qr_code_target_url"`
}

type TrackingCloudConfig struct {
	Addr     string `yaml:"addr"`
	DBPath   string `yaml:"db_path"`
	AssetDir string `yaml:"asset_dir"`
}

type FrontendConfig struct {
	DevPort  int    `yaml:"dev_port"`
	APIProxy string `yaml:"api_proxy"`
}

type SCFConfig struct {
	MongoDB MongoDBConfig `yaml:"mongodb"`
}

type MongoDBConfig struct {
	URI              string `yaml:"uri"`
	Database         string `yaml:"database"`
	EventsCollection string `yaml:"events_collection"`
	AssetsCollection string `yaml:"assets_collection"`
}

func Default() Config {
	return Config{
		LocalBackend: LocalBackendConfig{
			Addr:            ":8080",
			DBPath:          "data/app.db",
			TrackingBaseURL: "http://localhost:8081",
			QRCodeTargetURL: "https://example.com/survey",
		},
		TrackingCloud: TrackingCloudConfig{
			Addr:     ":8081",
			DBPath:   "data/tracking.db",
			AssetDir: "data/tracking-assets",
		},
		Frontend: FrontendConfig{
			DevPort:  5173,
			APIProxy: "http://localhost:8080",
		},
		SCF: SCFConfig{
			MongoDB: MongoDBConfig{
				Database:         "nousmail_tracking",
				EventsCollection: "tracking_events",
				AssetsCollection: "tracking_assets",
			},
		},
	}
}

func LoadDefault() (Config, error) {
	path := strings.TrimSpace(os.Getenv("NOUSMAIL_CONFIG"))
	if path == "" {
		path = "config.yaml"
	}
	return Load(path)
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return cfg, err
			}
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	return cfg, nil
}

func MustLoadDefault() Config {
	cfg, err := LoadDefault()
	if err != nil {
		panic(err)
	}
	return cfg
}

func applyEnv(cfg *Config) {
	setString(&cfg.LocalBackend.Addr, "LOCAL_BACKEND_ADDR")
	setString(&cfg.LocalBackend.DBPath, "LOCAL_BACKEND_DB_PATH")
	setString(&cfg.LocalBackend.TrackingBaseURL, "TRACKING_BASE_URL")
	setString(&cfg.LocalBackend.TrackingSourceToken, "TRACKING_SOURCE_TOKEN")
	setString(&cfg.LocalBackend.QRCodeTargetURL, "QR_CODE_TARGET_URL")
	setString(&cfg.TrackingCloud.Addr, "TRACKING_ADDR")
	setString(&cfg.TrackingCloud.DBPath, "TRACKING_DB_PATH")
	setString(&cfg.TrackingCloud.AssetDir, "TRACKING_ASSET_DIR")
	setInt(&cfg.Frontend.DevPort, "FRONTEND_DEV_PORT")
	setString(&cfg.Frontend.APIProxy, "FRONTEND_API_PROXY")
	setString(&cfg.SCF.MongoDB.URI, "MONGODB_URI")
	setString(&cfg.SCF.MongoDB.Database, "MONGODB_DATABASE")
	setString(&cfg.SCF.MongoDB.EventsCollection, "MONGODB_EVENTS_COLLECTION")
	setString(&cfg.SCF.MongoDB.AssetsCollection, "MONGODB_ASSETS_COLLECTION")
}

func setString(target *string, key string) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		*target = value
	}
}

func setInt(target *int, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil {
		*target = parsed
	}
}
