package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const CONFIG_DIR_ENV_VAR = "FIFTYMM_CONFIG_DIR"
const DEFAULT_CONFIG_DIR = "/etc/fiftymm/"

type AppConfig struct {
	configDir string
	sites     map[string]*Site
}

func NewApp() *AppConfig {
	configDir := os.Getenv(CONFIG_DIR_ENV_VAR)
	if configDir == "" {
		configDir = DEFAULT_CONFIG_DIR
	}

	configFilesMap := make(map[string]*Site)
	filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		// We only look at the top level files in the config dir
		if info.Mode().IsDir() && path != configDir {
			return filepath.SkipDir
		}

		if !(info.Mode().IsRegular() && filepath.Ext(path) == ".ini") {
			return nil
		}

		siteConfig, loadErr := LoadSiteFromFile(path)
		if loadErr != nil {
			fmt.Printf("Unable to load config from file %s. Error: %s\n", path, loadErr.Error())
		}

		configFilesMap[siteConfig.Domain] = siteConfig
		return nil
	})

	return &AppConfig{
		configDir: configDir,
		sites:     configFilesMap,
	}
}
