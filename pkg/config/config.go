package config

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var BuildVersion = "app-dev"

type Config struct {
	Client ClientConfig `yaml:"client"`
	SCF    SCFConfig    `yaml:"scf"`
}

type ClientConfig struct {
	Addr                string `yaml:"addr"`
	DBPath              string `yaml:"db_path"`
	TrackingBaseURL     string `yaml:"tracking_base_url"`
	TrackingSourceToken string `yaml:"tracking_source_token"`
	QRCodeTargetURL     string `yaml:"qr_code_target_url"`
}

type SCFConfig struct {
	TCB TCBConfig `yaml:"tcb"`
}

type TCBConfig struct {
	EnvID            string `yaml:"env_id"`
	Region           string `yaml:"region"`
	EventsCollection string `yaml:"events_collection"`
	AssetsCollection string `yaml:"assets_collection"`
}

func Default() Config {
	return Config{
		Client: ClientConfig{
			Addr:            ":8080",
			DBPath:          "data/app.db",
			TrackingBaseURL: "https://xray-7g6vc4y2d2fc01be-1309857796.ap-shanghai.app.tcloudbase.com/jianji",
			QRCodeTargetURL: "https://example.com/survey",
		},
		SCF: SCFConfig{
			TCB: TCBConfig{
				EnvID:            "xray-7g6vc4y2d2fc01be",
				Region:           "ap-shanghai",
				EventsCollection: "tracking_events",
				AssetsCollection: "tracking_assets",
			},
		},
	}
}

func LoadDefault() (Config, error) {
	path := strings.TrimSpace(os.Getenv("VESTIGE_CONFIG"))
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
	setString(&cfg.Client.Addr, "CLIENT_ADDR")
	setString(&cfg.Client.DBPath, "CLIENT_DB_PATH")
	setString(&cfg.Client.TrackingBaseURL, "TRACKING_BASE_URL")
	setString(&cfg.Client.TrackingSourceToken, "TRACKING_SOURCE_TOKEN")
	setString(&cfg.Client.QRCodeTargetURL, "QR_CODE_TARGET_URL")
	setStringAny(&cfg.SCF.TCB.EnvID, "TCB_ENV_ID", "TCB_ENV", "SCF_NAMESPACE")
	setString(&cfg.SCF.TCB.Region, "TCB_REGION")
	setString(&cfg.SCF.TCB.EventsCollection, "TCB_EVENTS_COLLECTION")
	setString(&cfg.SCF.TCB.AssetsCollection, "TCB_ASSETS_COLLECTION")
}

func setString(target *string, key string) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		*target = value
	}
}

func setStringAny(target *string, keys ...string) {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			*target = value
			return
		}
	}
}
