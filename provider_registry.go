package main

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed data/providers.json
var providerRegistryFS embed.FS

type ProviderRegistryModel struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	IsDefault        bool     `json:"isDefault"`
	OwnedBy          string   `json:"ownedBy,omitempty"`
	ContextWindow    int      `json:"contextWindow,omitempty"`
	Pricing          *Pricing `json:"pricing,omitempty"`
	ToolCapabilities []string `json:"toolCapabilities,omitempty"`
}

type Pricing struct {
	InputPerMTok  float64 `json:"inputPerMTok,omitempty"`
	OutputPerMTok float64 `json:"outputPerMTok,omitempty"`
	Currency      string  `json:"currency,omitempty"`
}

type ProviderPreset struct {
	ID                 string                  `json:"id"`
	Name               string                  `json:"name"`
	BaseURL            string                  `json:"baseUrl"`
	APIFormat          string                  `json:"apiFormat"`
	Color              string                  `json:"color"`
	APIKeyURL          string                  `json:"apiKeyUrl,omitempty"`
	EndpointCandidates []string                `json:"endpointCandidates,omitempty"`
	AuthHeaders        []string                `json:"authHeaders,omitempty"`
	Models             []ProviderRegistryModel `json:"models"`
}

func LoadProviderRegistry() ([]ProviderPreset, error) {
	b, err := providerRegistryFS.ReadFile("data/providers.json")
	if err != nil {
		return nil, err
	}
	var presets []ProviderPreset
	if err := json.Unmarshal(b, &presets); err != nil {
		return nil, err
	}
	for i := range presets {
		presets[i].APIFormat = NormalizeAPIFormat(presets[i].APIFormat)
		if presets[i].ID == "" || presets[i].Name == "" || presets[i].BaseURL == "" {
			return nil, fmt.Errorf("provider preset %d missing required fields", i)
		}
	}
	return presets, nil
}

func (a *App) ListProviderPresets() ([]ProviderPreset, error) {
	return LoadProviderRegistry()
}

func presetToProvider(p ProviderPreset, apiKey string) Provider {
	models := make([]ProviderModel, 0, len(p.Models))
	for _, m := range p.Models {
		models = append(models, ProviderModel{
			ID:        m.ID,
			Name:      m.Name,
			Enabled:   m.Enabled,
			IsDefault: m.IsDefault,
			OwnedBy:   m.OwnedBy,
		})
	}
	return Provider{
		ID:        p.ID,
		Name:      p.Name,
		BaseURL:   p.BaseURL,
		APIKey:    apiKey,
		Color:     p.Color,
		APIFormat: NormalizeAPIFormat(p.APIFormat),
		Models:    models,
	}
}
