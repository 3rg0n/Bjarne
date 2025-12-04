package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings represents user-configurable settings stored in ~/.bjarne/settings.json
type Settings struct {
	Models     ModelSettings      `json:"models"`
	Validation ValidationSettings `json:"validation"`
	Tokens     TokenSettings      `json:"tokens"`
	Container  ContainerSettings  `json:"container"`
	Theme      ThemeSettings      `json:"theme"`
}

// ModelSettings configures which models to use for different tasks
type ModelSettings struct {
	// Chat is used for conversational responses (no code generation)
	Chat string `json:"chat"`
	// Reflection is used for initial prompt analysis (classifies EASY/MEDIUM/COMPLEX)
	Reflection string `json:"reflection"`
	// Generate is used for initial code generation
	Generate string `json:"generate"`
	// Oracle is used for deep architectural analysis (COMPLEX tasks)
	Oracle string `json:"oracle"`
	// Escalation is a list of models to try when validation fails (in order)
	Escalation []string `json:"escalation"`
}

// ValidationSettings configures the validation behavior
type ValidationSettings struct {
	// MaxIterations is the maximum number of fix attempts before giving up
	MaxIterations int `json:"maxIterations"`
	// EscalateOnFailure enables model escalation when validation fails
	EscalateOnFailure bool `json:"escalateOnFailure"`
}

// TokenSettings configures token budgets
type TokenSettings struct {
	// MaxPerResponse is the maximum tokens per API response
	MaxPerResponse int `json:"maxPerResponse"`
	// MaxPerSession is the maximum total tokens per session (0 = unlimited)
	MaxPerSession int `json:"maxPerSession"`
}

// ContainerSettings configures the validation container
type ContainerSettings struct {
	// Image is the container image to use for validation
	Image string `json:"image"`
}

// ThemeSettings configures the UI appearance
type ThemeSettings struct {
	// Name is the theme preset name
	Name string `json:"name"`
}

// ThemePreset defines colors for a complete theme
type ThemePreset struct {
	Prompt  string
	Success string
	Error   string
	Warning string
	Info    string
	Accent  string
}

// DefaultSettings returns the default settings
func DefaultSettings() *Settings {
	return &Settings{
		Models: ModelSettings{
			Chat:       "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			Reflection: "global.anthropic.claude-haiku-4-5-20251001-v1:0", // Haiku for quick classification
			Generate:   "global.anthropic.claude-haiku-4-5-20251001-v1:0", // Default gen (overridden by complexity)
			Oracle:     "global.anthropic.claude-opus-4-5-20251101-v1:0",  // Opus for COMPLEX
			Escalation: []string{
				"global.anthropic.claude-sonnet-4-5-20250929-v1:0", // Haiku → Sonnet
				"global.anthropic.claude-opus-4-5-20251101-v1:0",   // Sonnet → Opus
			},
		},
		Validation: ValidationSettings{
			MaxIterations:     3,
			EscalateOnFailure: true,
		},
		Tokens: TokenSettings{
			MaxPerResponse: 8192,
			MaxPerSession:  150000,
		},
		Container: ContainerSettings{
			Image: "bjarne-validator:local",
		},
		Theme: ThemeSettings{
			Name: "default",
		},
	}
}

// SettingsPath returns the path to the settings file
func SettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bjarne", "settings.json"), nil
}

// LoadSettings loads settings from ~/.bjarne/settings.json
// Returns default settings if the file doesn't exist or can't be read
func LoadSettings() (*Settings, error) {
	settings := DefaultSettings()

	path, err := SettingsPath()
	if err != nil {
		// Can't determine home directory - return defaults (not an error for the user)
		return settings, nil //nolint:nilerr // intentional: return defaults when path unavailable
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil // Return defaults if file doesn't exist
		}
		return settings, err
	}

	// Parse JSON, keeping defaults for missing fields
	if err := json.Unmarshal(data, settings); err != nil {
		return settings, err
	}

	return settings, nil
}

// SaveSettings saves settings to ~/.bjarne/settings.json
func SaveSettings(settings *Settings) error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ANSI color codes (256-color mode for richer themes)
var colorCodes = map[string]string{
	// Basic colors
	"black":   "\033[30m",
	"red":     "\033[91m",
	"green":   "\033[92m",
	"yellow":  "\033[93m",
	"blue":    "\033[94m",
	"magenta": "\033[95m",
	"cyan":    "\033[96m",
	"white":   "\033[97m",
	"reset":   "\033[0m",

	// Extended colors for themes (256-color)
	"matrix_green":     "\033[38;5;46m",  // Bright green
	"matrix_dim":       "\033[38;5;22m",  // Dark green
	"solarized_blue":   "\033[38;5;33m",  // Solarized blue
	"solarized_cyan":   "\033[38;5;37m",  // Solarized cyan
	"solarized_green":  "\033[38;5;64m",  // Solarized green
	"solarized_red":    "\033[38;5;160m", // Solarized red
	"solarized_yellow": "\033[38;5;136m", // Solarized yellow
	"gruvbox_orange":   "\033[38;5;208m", // Gruvbox orange
	"gruvbox_green":    "\033[38;5;142m", // Gruvbox green
	"gruvbox_red":      "\033[38;5;167m", // Gruvbox red
	"gruvbox_yellow":   "\033[38;5;214m", // Gruvbox yellow
	"gruvbox_aqua":     "\033[38;5;108m", // Gruvbox aqua
	"dracula_purple":   "\033[38;5;141m", // Dracula purple
	"dracula_pink":     "\033[38;5;212m", // Dracula pink
	"dracula_green":    "\033[38;5;84m",  // Dracula green
	"dracula_red":      "\033[38;5;210m", // Dracula red
	"dracula_cyan":     "\033[38;5;117m", // Dracula cyan
	"nord_blue":        "\033[38;5;67m",  // Nord frost blue
	"nord_cyan":        "\033[38;5;110m", // Nord frost cyan
	"nord_green":       "\033[38;5;108m", // Nord aurora green
	"nord_red":         "\033[38;5;174m", // Nord aurora red
	"nord_yellow":      "\033[38;5;222m", // Nord aurora yellow
}

// ThemePresets contains all available theme presets
var ThemePresets = map[string]ThemePreset{
	"default": {
		Prompt:  "blue",
		Success: "green",
		Error:   "red",
		Warning: "yellow",
		Info:    "cyan",
		Accent:  "magenta",
	},
	"matrix": {
		Prompt:  "matrix_green",
		Success: "matrix_green",
		Error:   "matrix_dim",
		Warning: "matrix_green",
		Info:    "matrix_green",
		Accent:  "matrix_green",
	},
	"solarized": {
		Prompt:  "solarized_blue",
		Success: "solarized_green",
		Error:   "solarized_red",
		Warning: "solarized_yellow",
		Info:    "solarized_cyan",
		Accent:  "solarized_blue",
	},
	"gruvbox": {
		Prompt:  "gruvbox_orange",
		Success: "gruvbox_green",
		Error:   "gruvbox_red",
		Warning: "gruvbox_yellow",
		Info:    "gruvbox_aqua",
		Accent:  "gruvbox_orange",
	},
	"dracula": {
		Prompt:  "dracula_purple",
		Success: "dracula_green",
		Error:   "dracula_red",
		Warning: "dracula_pink",
		Info:    "dracula_cyan",
		Accent:  "dracula_purple",
	},
	"nord": {
		Prompt:  "nord_blue",
		Success: "nord_green",
		Error:   "nord_red",
		Warning: "nord_yellow",
		Info:    "nord_cyan",
		Accent:  "nord_blue",
	},
}

// Theme provides color formatting based on settings
type Theme struct {
	preset ThemePreset
}

// NewTheme creates a theme from settings
func NewTheme(settings *ThemeSettings) *Theme {
	preset, ok := ThemePresets[settings.Name]
	if !ok {
		preset = ThemePresets["default"]
	}
	return &Theme{preset: preset}
}

// Prompt formats text with the prompt color
func (t *Theme) Prompt(text string) string {
	return t.colorize(t.preset.Prompt, text)
}

// Success formats text with the success color
func (t *Theme) Success(text string) string {
	return t.colorize(t.preset.Success, text)
}

// Error formats text with the error color
func (t *Theme) Error(text string) string {
	return t.colorize(t.preset.Error, text)
}

// Warning formats text with the warning color
func (t *Theme) Warning(text string) string {
	return t.colorize(t.preset.Warning, text)
}

// Info formats text with the info color
func (t *Theme) Info(text string) string {
	return t.colorize(t.preset.Info, text)
}

// Accent formats text with the accent color
func (t *Theme) Accent(text string) string {
	return t.colorize(t.preset.Accent, text)
}

// PromptCode returns just the ANSI code for the prompt color
func (t *Theme) PromptCode() string {
	return getColorCode(t.preset.Prompt)
}

// Reset returns the reset code
func (t *Theme) Reset() string {
	return colorCodes["reset"]
}

// Dim formats text with dim/faint styling
func (t *Theme) Dim(text string) string {
	return "\033[2m" + text + colorCodes["reset"]
}

func (t *Theme) colorize(color, text string) string {
	code := getColorCode(color)
	return code + text + colorCodes["reset"]
}

func getColorCode(color string) string {
	if code, ok := colorCodes[color]; ok {
		return code
	}
	return colorCodes["white"]
}

// AvailableThemes returns the list of available theme names
func AvailableThemes() []string {
	return []string{"default", "matrix", "solarized", "gruvbox", "dracula", "nord"}
}
