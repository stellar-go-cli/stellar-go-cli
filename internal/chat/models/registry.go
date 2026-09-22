// Package models provides model registry and version management for fine-tuned models
package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Model represents a fine-tuned model in the registry
type Model struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	BaseModel     string            `json:"baseModel"`
	Description   string            `json:"description"`
	Type          ModelType         `json:"type"`
	Format        ModelFormat       `json:"format"`
	Path          string            `json:"path"`
	Size          int64             `json:"size"`
	Quantization  string            `json:"quantization,omitempty"`
	Checksum      string            `json:"checksum,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	TrainingJobID string            `json:"trainingJobId,omitempty"`
	Metrics       ModelMetrics      `json:"metrics,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Active        bool              `json:"active"`
}

// ModelType represents the type of model
type ModelType string

const (
	TypeBaseModel   ModelType = "base"
	TypeLoRAAdapter ModelType = "lora_adapter"
	TypeMergedModel ModelType = "merged"
	TypeGGUFModel   ModelType = "gguf"
)

// ModelFormat represents the storage format
type ModelFormat string

const (
	FormatSafetensors ModelFormat = "safetensors"
	FormatGGUF        ModelFormat = "gguf"
	FormatBin         ModelFormat = "bin"
	FormatOllama      ModelFormat = "ollama"
)

// ModelMetrics contains performance metrics for the model
type ModelMetrics struct {
	TrainingLoss    float64 `json:"trainingLoss,omitempty"`
	ValidationLoss  float64 `json:"validationLoss,omitempty"`
	Perplexity      float64 `json:"perplexity,omitempty"`
	Accuracy        float64 `json:"accuracy,omitempty"`
	InferenceTimeMs int64   `json:"inferenceTimeMs,omitempty"`
	MemoryUsageMB   int64   `json:"memoryUsageMB,omitempty"`
}

// Registry manages the model repository
type Registry struct {
	rootDir     string
	indexPath   string
	models      map[string]*Model
	activeModel string
}

// NewRegistry creates a new model registry
func NewRegistry(rootDir string) (*Registry, error) {
	registry := &Registry{
		rootDir:   rootDir,
		indexPath: filepath.Join(rootDir, "registry.json"),
		models:    make(map[string]*Model),
	}

	// Ensure directory structure exists
	dirs := []string{
		rootDir,
		filepath.Join(rootDir, "base"),
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "gguf"),
		filepath.Join(rootDir, "ollama"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Load existing registry
	if err := registry.load(); err != nil {
		// New registry, that's fine
		registry.models = make(map[string]*Model)
	}

	return registry, nil
}

// Register adds a model to the registry
func (r *Registry) Register(model *Model) error {
	if model.ID == "" {
		model.ID = r.generateID(model.Name, model.Version)
	}

	model.UpdatedAt = time.Now()
	if model.CreatedAt.IsZero() {
		model.CreatedAt = model.UpdatedAt
	}

	r.models[model.ID] = model

	// Save to disk if it's a GGUF model
	if model.Format == FormatGGUF && model.Path != "" {
		targetPath := filepath.Join(r.rootDir, "gguf", filepath.Base(model.Path))
		if model.Path != targetPath {
			if err := r.copyFile(model.Path, targetPath); err != nil {
				return fmt.Errorf("copy model file: %w", err)
			}
			model.Path = targetPath
		}
	}

	return r.save()
}

// Get retrieves a model by ID
func (r *Registry) Get(id string) (*Model, error) {
	model, ok := r.models[id]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", id)
	}
	return model, nil
}

// GetByName retrieves models by name
func (r *Registry) GetByName(name string) []*Model {
	var result []*Model
	for _, model := range r.models {
		if model.Name == name {
			result = append(result, model)
		}
	}
	return result
}

// List returns all registered models
func (r *Registry) List() []*Model {
	models := make([]*Model, 0, len(r.models))
	for _, model := range r.models {
		models = append(models, model)
	}

	// Sort by created date, newest first
	sort.Slice(models, func(i, j int) bool {
		return models[i].CreatedAt.After(models[j].CreatedAt)
	})

	return models
}

// ListByType returns models of a specific type
func (r *Registry) ListByType(modelType ModelType) []*Model {
	var result []*Model
	for _, model := range r.models {
		if model.Type == modelType {
			result = append(result, model)
		}
	}
	return result
}

// SetActive sets the active model for inference
func (r *Registry) SetActive(id string) error {
	model, err := r.Get(id)
	if err != nil {
		return err
	}

	// Deactivate previous active model
	if r.activeModel != "" {
		if prev, ok := r.models[r.activeModel]; ok {
			prev.Active = false
		}
	}

	model.Active = true
	r.activeModel = id

	return r.save()
}

// GetActive returns the currently active model
func (r *Registry) GetActive() (*Model, error) {
	if r.activeModel == "" {
		return nil, fmt.Errorf("no active model set")
	}
	return r.Get(r.activeModel)
}

// Delete removes a model from the registry
func (r *Registry) Delete(id string) error {
	model, err := r.Get(id)
	if err != nil {
		return err
	}

	// Remove file if it exists and is in our directory
	if model.Path != "" && strings.HasPrefix(model.Path, r.rootDir) {
		os.Remove(model.Path) //nolint:errcheck // best-effort cleanup
	}

	// Remove from index
	delete(r.models, id)

	return r.save()
}

// UpdateMetrics updates the metrics for a model
func (r *Registry) UpdateMetrics(id string, metrics ModelMetrics) error {
	model, err := r.Get(id)
	if err != nil {
		return err
	}

	model.Metrics = metrics
	model.UpdatedAt = time.Now()

	return r.save()
}

// AddTag adds a tag to a model
func (r *Registry) AddTag(id string, tag string) error {
	model, err := r.Get(id)
	if err != nil {
		return err
	}

	// Check if tag already exists
	for _, t := range model.Tags {
		if t == tag {
			return nil
		}
	}

	model.Tags = append(model.Tags, tag)
	model.UpdatedAt = time.Now()

	return r.save()
}

// RemoveTag removes a tag from a model
func (r *Registry) RemoveTag(id string, tag string) error {
	model, err := r.Get(id)
	if err != nil {
		return err
	}

	var newTags []string
	for _, t := range model.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}

	model.Tags = newTags
	model.UpdatedAt = time.Now()

	return r.save()
}

// FindByTag returns models matching a tag
func (r *Registry) FindByTag(tag string) []*Model {
	var result []*Model
	for _, model := range r.models {
		for _, t := range model.Tags {
			if t == tag {
				result = append(result, model)
				break
			}
		}
	}
	return result
}

// GetLatestVersion returns the latest version of a model by name
func (r *Registry) GetLatestVersion(name string) (*Model, error) {
	models := r.GetByName(name)
	if len(models) == 0 {
		return nil, fmt.Errorf("no models found with name: %s", name)
	}

	// Sort by version (assuming semantic versioning)
	sort.Slice(models, func(i, j int) bool {
		return compareVersions(models[i].Version, models[j].Version) > 0
	})

	return models[0], nil
}

// generateID creates a unique model ID
func (r *Registry) generateID(name, version string) string {
	safeName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	safeVersion := strings.ReplaceAll(version, ".", "-")
	return fmt.Sprintf("%s-%s-%d", safeName, safeVersion, time.Now().Unix())
}

// save persists the registry to disk
func (r *Registry) save() error {
	data, err := json.MarshalIndent(r.models, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	return os.WriteFile(r.indexPath, data, 0644)
}

// load reads the registry from disk
func (r *Registry) load() error {
	data, err := os.ReadFile(r.indexPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &r.models)
}

// copyFile copies a file from source to destination
func (r *Registry) copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// compareVersions compares two semantic version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int

		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &num1) //nolint:errcheck // non-numeric parts parse as 0
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &num2) //nolint:errcheck // non-numeric parts parse as 0
		}

		if num1 > num2 {
			return 1
		}
		if num1 < num2 {
			return -1
		}
	}

	return 0
}

// ModelStats provides statistics about the registry
type ModelStats struct {
	TotalModels   int            `json:"totalModels"`
	ByType        map[string]int `json:"byType"`
	ByFormat      map[string]int `json:"byFormat"`
	TotalSize     int64          `json:"totalSize"`
	ActiveModelID string         `json:"activeModelId,omitempty"`
}

// GetStats returns statistics about the registry
func (r *Registry) GetStats() ModelStats {
	stats := ModelStats{
		ByType:   make(map[string]int),
		ByFormat: make(map[string]int),
	}

	for _, model := range r.models {
		stats.TotalModels++
		stats.ByType[string(model.Type)]++
		stats.ByFormat[string(model.Format)]++
		stats.TotalSize += model.Size
	}

	stats.ActiveModelID = r.activeModel

	return stats
}

// ImportFromPath imports a model file into the registry
func (r *Registry) ImportFromPath(path string, model *Model) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("model file not found: %w", err)
	}

	model.Size = info.Size()

	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gguf":
		model.Format = FormatGGUF
		model.Type = TypeGGUFModel
	case ".bin", ".pt", ".pth":
		model.Format = FormatBin
		model.Type = TypeMergedModel
	default:
		model.Format = FormatSafetensors
		model.Type = TypeLoRAAdapter
	}

	model.Path = path

	return r.Register(model)
}

// ExportToOllama creates an Ollama-compatible model from a GGUF model
func (r *Registry) ExportToOllama(modelID string, ollamaName string) error {
	model, err := r.Get(modelID)
	if err != nil {
		return err
	}

	if model.Format != FormatGGUF {
		return fmt.Errorf("only GGUF models can be exported to Ollama")
	}

	// Create Ollama model directory structure
	ollamaDir := filepath.Join(r.rootDir, "ollama", ollamaName)
	if err := os.MkdirAll(ollamaDir, 0755); err != nil {
		return fmt.Errorf("create ollama directory: %w", err)
	}

	// Link or copy the GGUF file
	ggufName := fmt.Sprintf("model-%s.gguf", model.Quantization)
	ollamaModelPath := filepath.Join(ollamaDir, ggufName)

	// Create symlink instead of copy to save space
	if err := os.Symlink(model.Path, ollamaModelPath); err != nil {
		// Fallback to copy if symlink fails
		if err := r.copyFile(model.Path, ollamaModelPath); err != nil {
			return fmt.Errorf("copy model to ollama dir: %w", err)
		}
	}

	// Create Modelfile
	modelfileContent := fmt.Sprintf(`FROM ./%s

SYSTEM """You are Stellar Go CLI, a cryptocurrency payment CLI assistant."""

PARAMETER temperature 0.1
PARAMETER top_p 0.9
PARAMETER top_k 40
`, ggufName)

	modelfilePath := filepath.Join(ollamaDir, "Modelfile")
	if err := os.WriteFile(modelfilePath, []byte(modelfileContent), 0644); err != nil {
		return fmt.Errorf("create Modelfile: %w", err)
	}

	// Register the Ollama model
	ollamaModel := &Model{
		Name:         ollamaName,
		Version:      model.Version,
		BaseModel:    model.BaseModel,
		Type:         TypeGGUFModel,
		Format:       FormatOllama,
		Path:         ollamaDir,
		Size:         model.Size,
		Quantization: model.Quantization,
		Tags:         append(model.Tags, "ollama"),
	}

	return r.Register(ollamaModel)
}
