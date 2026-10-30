package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"sis2/internal/common"
	"sis2/internal/common/utils"

	"github.com/creasty/defaults"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var configs sync.Map

// GetConfig loads a config struct once per type and caches it.
func GetConfig[T any]() *T {
	configType := reflect.TypeFor[T]()
	if val, ok := configs.Load(configType); ok {
		return val.(*T)
	}
	var cfg T
	if err := LoadConfig(&cfg); err != nil {
		log.Fatalf("Failed to load configuration for type %v: %v", configType, err)
	}
	if c, ok := any(&cfg).(*BlogPlatformConfig); ok {
		c.JWT.Secret = utils.GetEnv("JWT_SECRET", c.JWT.Secret)
		c.S3.SecretKey = utils.GetEnv("S3_SECRET_KEY", c.S3.SecretKey)
	}
	actual, _ := configs.LoadOrStore(configType, &cfg)
	log.Infof("Configuration for type %v loaded successfully", configType)
	return actual.(*T)
}

// LoadConfigFromFile reads a YAML file over the struct's `default` tags.
func LoadConfigFromFile(filename string, cfg any) error {
	path := filename
	if !utils.IsFullPath(filename) {
		path = filepath.Join(utils.GetProjectRoot(), common.ConfigDirPrefix, filename)
	}
	log.Infof("Loading configuration from %s", path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		if err = file.Close(); err != nil {
			log.Errorf("Failed to close file: %v", err)
		}
	}(file)

	if err = defaults.Set(cfg); err != nil {
		return err
	}
	return yaml.NewDecoder(file).Decode(cfg)
}

// LoadConfig loads the file named by CONFIG_FILE from the `configs` folder
// (test.yaml under `go test`, default.yaml otherwise).
func LoadConfig(cfg any) error {
	defaultConfigFile := "default.yaml"
	if utils.IsTestMode() {
		defaultConfigFile = "test.yaml"
	}
	return LoadConfigFromFile(utils.GetEnv("CONFIG_FILE", defaultConfigFile), cfg)
}
