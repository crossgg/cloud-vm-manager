package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Azure   AzureConfig   `yaml:"azure"`
	Default DefaultConfig `yaml:"default"`
}

type AzureConfig struct {
	TenantId     string `yaml:"tenant_id"`
	ClientId     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	SubId        string `yaml:"subscription_id"`
}

type DefaultConfig struct {
	ResourceGroup string `yaml:"resource_group"`
	Location      string `yaml:"location"`
}

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
