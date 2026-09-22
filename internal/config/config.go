package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultConfigDir   = ".stellar-go-cli"
	LegacyConfigDir    = ".mozartpay" // pre-rename directory; migrated on first run
	DefaultConfigFile  = "config.json"
	Version            = "0.1.0-mvp"
	AppName            = "Stellar Go CLI"
	DefaultHTTPTimeout = 30 * time.Second
)

type Config struct {
	Network       string            `json:"network"`
	WalletType    string            `json:"walletType"`
	DIDMethod     string            `json:"didMethod"`
	ActiveDID     string            `json:"activeDid,omitempty"`
	ActiveAddress string            `json:"activeAddress,omitempty"`
	ContractID    string            `json:"contractID,omitempty"`
	Integrations  IntegrationConfig `json:"integrations"`
	Debug         bool              `json:"debug"`
	LLM           LLMConfig         `json:"llm,omitempty"`
}

type IntegrationConfig struct {
	StellarCarbonEnabled bool   `json:"stellarCarbonEnabled"`
	X402Enabled          bool   `json:"x402Enabled"`
	TempoEnabled         bool   `json:"tempoEnabled"`
	StellarHorizonURL    string `json:"stellarHorizonUrl"`
	AlphaVantageAPIKey   string `json:"alphaVantageApiKey"`
	FinnhubAPIKey        string `json:"finnhubApiKey"`
	TansuEnabled         bool   `json:"tansuEnabled"`
	TansuContractID      string `json:"tansuContractId"`
}

// LLMConfig holds configuration for local LLM inference
type LLMConfig struct {
	Enabled         bool    `json:"enabled"`
	ModelPath       string  `json:"modelPath,omitempty"`
	OllamaURL       string  `json:"ollamaURL,omitempty"`
	OllamaModel     string  `json:"ollamaModel,omitempty"`
	ContextSize     int     `json:"contextSize,omitempty"`
	Threads         int     `json:"threads,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	FallbackToRules bool    `json:"fallbackToRules,omitempty"`

	// Fine-tuning
	FineTunedModel string          `json:"fineTunedModel,omitempty"`
	LoRAPath       string          `json:"loraPath,omitempty"`
	TrainingConfig *TrainingConfig `json:"trainingConfig,omitempty"`
}

// TrainingConfig holds fine-tuning configuration
type TrainingConfig struct {
	Enabled      bool   `json:"enabled"`
	DataPath     string `json:"dataPath"`
	OutputDir    string `json:"outputDir"`
	BaseModel    string `json:"baseModel"`
	Method       string `json:"method"` // "lora" or "qlora"
	WorkDir      string `json:"workDir"`
	LLamaCppPath string `json:"llamaCppPath"`
}

func DefaultConfig() *Config {
	return &Config{
		Network:    "stellar-testnet",
		WalletType: "wwwallet",
		DIDMethod:  "key",
		Integrations: IntegrationConfig{
			StellarCarbonEnabled: true,
			X402Enabled:          true,
			TempoEnabled:         true,
			StellarHorizonURL:    "https://horizon-testnet.stellar.org",
			TansuEnabled:         true,
			TansuContractID:      "CBXKUSLQPVF35FYURR5C42BPYA5UOVDXX2ELKIM2CAJMCI6HXG2BHGZA",
		},
		LLM: LLMConfig{
			Enabled:         false,
			ContextSize:     4096,
			Threads:         4,
			Temperature:     0.1,
			FallbackToRules: true,
		},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, DefaultConfigDir)
	migrateLegacyDir(home, dir)
	return dir, nil
}

// migrateLegacyDir renames the legacy ~/.mozartpay config directory to the
// current location on first run. One-release migration; remove in a later release.
func migrateLegacyDir(home, dir string) {
	legacy := filepath.Join(home, LegacyConfigDir)
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	if _, err := os.Stat(dir); err == nil {
		return
	}
	// Best-effort migration; on failure the user can rename the directory manually.
	if err := os.Rename(legacy, dir); err != nil {
		log.Printf("could not migrate legacy config dir %s: %v", legacy, err)
	}
}

func Load() (*Config, error) {
	dir, err := ConfigDir()
	if err != nil {
		return DefaultConfig(), nil
	}

	path := filepath.Join(dir, DefaultConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), nil
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, DefaultConfigFile)
	return os.WriteFile(path, data, 0600)
}

func StateDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state"), nil
}

func SaveState(name string, v interface{}) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), data, 0600)
}

func LoadState(name string, v interface{}) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// NewHTTPClient returns a preconfigured HTTP client with timeout
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: DefaultHTTPTimeout,
	}
}
