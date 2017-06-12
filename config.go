package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const CONFIG_DIR_ENV_VAR = "FIFTYMM_CONFIG_DIR"
const DEFAULT_CONFIG_DIR = "/etc/fiftymm/"

const PORT_ENV_VAR = "FIFTYMM_PORT"
const DEFAULT_PORT = "8080"

type App struct {
	port string

	configDir string
	sites     map[string]*Site
}

func NewApp() *App {
	port := os.Getenv(PORT_ENV_VAR)
	if port == "" {
		port = DEFAULT_PORT
	}

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
			return nil
		}

		configFilesMap[siteConfig.Domain] = siteConfig
		return nil
	})

	return &App{
		port:      port,
		configDir: configDir,
		sites:     configFilesMap,
	}
}

func (a *App) SiteForDomain(domain string) (*Site, error) {
	if cs, ok := a.sites[domain]; !ok {
		return nil, fmt.Errorf("No site configured for domain %s", domain)
	} else {
		return cs, nil
	}
}
