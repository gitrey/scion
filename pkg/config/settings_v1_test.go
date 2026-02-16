// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- Struct round-trip tests ---

func TestVersionedSettings_YAMLRoundTrip(t *testing.T) {
	autoHelp := true
	tmux := true

	vs := &VersionedSettings{
		SchemaVersion:   "1",
		ActiveProfile:   "local",
		DefaultTemplate: "gemini",
		Hub: &V1HubClientConfig{
			Enabled:  boolPtr(true),
			Endpoint: "https://hub.example.com",
			GroveID:  "test-grove",
		},
		CLI: &V1CLIConfig{
			AutoHelp:            &autoHelp,
			InteractiveDisabled: boolPtr(false),
		},
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {Type: "docker", Host: ""},
			"container": {Type: "container", Tmux: &tmux},
		},
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {
				Harness: "gemini",
				Image:   "example.com/gemini:latest",
				User:    "scion",
				Model:   "gemini-2.5-pro",
				Args:    []string{"--sandbox=strict"},
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {
				Runtime:              "container",
				DefaultTemplate:      "gemini",
				DefaultHarnessConfig: "gemini",
				Tmux:                 &tmux,
			},
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(vs)
	require.NoError(t, err)

	// Validate against schema
	valErrors, err := ValidateSettings(data, "1")
	require.NoError(t, err)
	assert.Empty(t, valErrors, "round-tripped YAML should validate against schema, got: %v", valErrors)

	// Unmarshal back
	var roundTripped VersionedSettings
	err = yaml.Unmarshal(data, &roundTripped)
	require.NoError(t, err)

	assert.Equal(t, vs.SchemaVersion, roundTripped.SchemaVersion)
	assert.Equal(t, vs.ActiveProfile, roundTripped.ActiveProfile)
	assert.Equal(t, vs.DefaultTemplate, roundTripped.DefaultTemplate)
	assert.Equal(t, vs.Hub.Endpoint, roundTripped.Hub.Endpoint)
	assert.Equal(t, vs.Hub.GroveID, roundTripped.Hub.GroveID)
	assert.Equal(t, vs.HarnessConfigs["gemini"].Model, roundTripped.HarnessConfigs["gemini"].Model)
	assert.Equal(t, vs.HarnessConfigs["gemini"].Args, roundTripped.HarnessConfigs["gemini"].Args)
	assert.Equal(t, vs.Profiles["local"].DefaultHarnessConfig, roundTripped.Profiles["local"].DefaultHarnessConfig)
}

// --- LoadVersionedSettings tests ---

func TestLoadVersionedSettings_DefaultsOnly(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "1", vs.SchemaVersion)
	assert.Equal(t, "local", vs.ActiveProfile)
	assert.Equal(t, "gemini", vs.DefaultTemplate)
	assert.Contains(t, vs.HarnessConfigs, "gemini")
	assert.Equal(t, "gemini", vs.HarnessConfigs["gemini"].Harness)
	assert.Contains(t, vs.Runtimes, "docker")
	assert.Equal(t, "docker", vs.Runtimes["docker"].Type)
}

func TestLoadVersionedSettings_GlobalOverride(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	globalSettings := `
schema_version: "1"
active_profile: prod
default_template: claude
`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(globalSettings), 0644))

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "prod", vs.ActiveProfile)
	assert.Equal(t, "claude", vs.DefaultTemplate)
}

func TestLoadVersionedSettings_GroveOverride(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	globalSettings := `
schema_version: "1"
active_profile: prod
default_template: claude
`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(globalSettings), 0644))

	groveSettings := `
schema_version: "1"
active_profile: staging
`
	require.NoError(t, os.WriteFile(filepath.Join(groveDir, "settings.yaml"), []byte(groveSettings), 0644))

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "staging", vs.ActiveProfile)
	// Template should still be claude from global
	assert.Equal(t, "claude", vs.DefaultTemplate)
}

func TestLoadVersionedSettings_EnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Set environment variable overrides
	os.Setenv("SCION_ACTIVE_PROFILE", "remote")
	defer os.Unsetenv("SCION_ACTIVE_PROFILE")

	os.Setenv("SCION_DEFAULT_TEMPLATE", "opencode")
	defer os.Unsetenv("SCION_DEFAULT_TEMPLATE")

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "remote", vs.ActiveProfile)
	assert.Equal(t, "opencode", vs.DefaultTemplate)
}

func TestLoadVersionedSettings_HubEnvVars(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Test SCION_HUB_GROVE_ID maps correctly (regression test)
	os.Setenv("SCION_HUB_GROVE_ID", "my-grove-id")
	defer os.Unsetenv("SCION_HUB_GROVE_ID")

	os.Setenv("SCION_HUB_LOCAL_ONLY", "true")
	defer os.Unsetenv("SCION_HUB_LOCAL_ONLY")

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	require.NotNil(t, vs.Hub)
	assert.Equal(t, "my-grove-id", vs.Hub.GroveID)
}

func TestLoadVersionedSettings_CLIEnvVars(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	os.Setenv("SCION_CLI_AUTOHELP", "false")
	defer os.Unsetenv("SCION_CLI_AUTOHELP")

	os.Setenv("SCION_CLI_INTERACTIVE_DISABLED", "true")
	defer os.Unsetenv("SCION_CLI_INTERACTIVE_DISABLED")

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	require.NotNil(t, vs.CLI)
}

func TestLoadVersionedSettings_JSONFallback(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	// Write JSON settings (should load via JSON fallback)
	globalJSON := `{
		"schema_version": "1",
		"active_profile": "json-profile",
		"default_template": "json-template"
	}`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.json"), []byte(globalJSON), 0644))

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "json-profile", vs.ActiveProfile)
	assert.Equal(t, "json-template", vs.DefaultTemplate)
}

func TestLoadVersionedSettings_NewFields(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	groveSettings := `
schema_version: "1"
harness_configs:
  gemini-custom:
    harness: gemini
    image: example.com/gemini:v2
    user: scion
    model: gemini-2.5-pro
    args: ["--sandbox=strict", "--verbose"]
runtimes:
  my-docker:
    type: docker
    host: tcp://remote:2376
profiles:
  custom:
    runtime: my-docker
    default_template: gemini
    default_harness_config: gemini-custom
`
	require.NoError(t, os.WriteFile(filepath.Join(groveDir, "settings.yaml"), []byte(groveSettings), 0644))

	vs, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	// Check new harness config fields
	hc, ok := vs.HarnessConfigs["gemini-custom"]
	require.True(t, ok)
	assert.Equal(t, "gemini", hc.Harness)
	assert.Equal(t, "gemini-2.5-pro", hc.Model)
	assert.Equal(t, []string{"--sandbox=strict", "--verbose"}, hc.Args)

	// Check runtime type field
	rt, ok := vs.Runtimes["my-docker"]
	require.True(t, ok)
	assert.Equal(t, "docker", rt.Type)
	assert.Equal(t, "tcp://remote:2376", rt.Host)

	// Check new profile fields
	profile, ok := vs.Profiles["custom"]
	require.True(t, ok)
	assert.Equal(t, "gemini", profile.DefaultTemplate)
	assert.Equal(t, "gemini-custom", profile.DefaultHarnessConfig)
}

// --- AdaptLegacySettings tests ---

func TestAdaptLegacySettings_FullMapping(t *testing.T) {
	autoHelp := true
	enabled := true
	tmux := true

	legacy := &Settings{
		ActiveProfile:   "local",
		DefaultTemplate: "gemini",
		Hub: &HubClientConfig{
			Enabled:  &enabled,
			Endpoint: "https://hub.example.com",
			GroveID:  "test-grove",
		},
		CLI: &CLIConfig{
			AutoHelp: &autoHelp,
		},
		Runtimes: map[string]RuntimeConfig{
			"docker":    {Host: "tcp://localhost:2375"},
			"container": {Tmux: &tmux},
		},
		Harnesses: map[string]HarnessConfig{
			"gemini": {Image: "example.com/gemini:latest", User: "scion"},
			"claude": {Image: "example.com/claude:latest", User: "scion"},
		},
		Profiles: map[string]ProfileConfig{
			"local": {Runtime: "container", Tmux: &tmux},
		},
	}

	vs, warnings := AdaptLegacySettings(legacy)

	assert.Equal(t, "1", vs.SchemaVersion)
	assert.Equal(t, "local", vs.ActiveProfile)
	assert.Equal(t, "gemini", vs.DefaultTemplate)

	// Hub mapping
	require.NotNil(t, vs.Hub)
	assert.Equal(t, "https://hub.example.com", vs.Hub.Endpoint)
	assert.Equal(t, "test-grove", vs.Hub.GroveID)
	assert.True(t, *vs.Hub.Enabled)

	// CLI mapping
	require.NotNil(t, vs.CLI)
	assert.True(t, *vs.CLI.AutoHelp)
	assert.Nil(t, vs.CLI.InteractiveDisabled) // New field, should be nil

	// Runtime type inference
	assert.Equal(t, "docker", vs.Runtimes["docker"].Type)
	assert.Equal(t, "container", vs.Runtimes["container"].Type)
	assert.Equal(t, "tcp://localhost:2375", vs.Runtimes["docker"].Host)

	// Harness → HarnessConfig mapping
	assert.Equal(t, "gemini", vs.HarnessConfigs["gemini"].Harness)
	assert.Equal(t, "example.com/gemini:latest", vs.HarnessConfigs["gemini"].Image)
	assert.Equal(t, "claude", vs.HarnessConfigs["claude"].Harness)

	// Profile mapping — new fields should be zero
	assert.Equal(t, "container", vs.Profiles["local"].Runtime)
	assert.Equal(t, "", vs.Profiles["local"].DefaultTemplate)
	assert.Equal(t, "", vs.Profiles["local"].DefaultHarnessConfig)

	// Should have warning about harnesses rename
	assert.NotEmpty(t, warnings)
	hasHarnessWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "harnesses is deprecated") {
			hasHarnessWarning = true
			break
		}
	}
	assert.True(t, hasHarnessWarning, "should warn about harnesses deprecation")
}

func TestAdaptLegacySettings_HubFieldWarnings(t *testing.T) {
	legacy := &Settings{
		Hub: &HubClientConfig{
			Token:          "secret-token",
			APIKey:         "api-key",
			BrokerID:       "broker-123",
			BrokerNickname: "my-broker",
			BrokerToken:    "broker-token",
			LastSyncedAt:   "2024-01-01T00:00:00Z",
		},
	}

	vs, warnings := AdaptLegacySettings(legacy)

	// These fields should NOT be in the versioned settings
	assert.NotNil(t, vs.Hub)

	// Should have warnings for all deprecated fields
	warningTexts := map[string]bool{
		"hub.token":          false,
		"hub.apiKey":         false,
		"hub.brokerId":       false,
		"hub.brokerNickname": false,
		"hub.brokerToken":    false,
		"hub.lastSyncedAt":   false,
	}
	for _, w := range warnings {
		for key := range warningTexts {
			if strings.Contains(w, key) {
				warningTexts[key] = true
			}
		}
	}
	for key, found := range warningTexts {
		assert.True(t, found, "expected warning about %s", key)
	}
}

func TestAdaptLegacySettings_BucketWarning(t *testing.T) {
	legacy := &Settings{
		Bucket: &BucketConfig{
			Provider: "GCS",
			Name:     "my-bucket",
			Prefix:   "agents",
		},
	}

	_, warnings := AdaptLegacySettings(legacy)

	hasBucketWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "bucket") {
			hasBucketWarning = true
			break
		}
	}
	assert.True(t, hasBucketWarning, "should warn about bucket config deprecation")
}

func TestAdaptLegacySettings_NilInput(t *testing.T) {
	vs, warnings := AdaptLegacySettings(nil)

	assert.Equal(t, "1", vs.SchemaVersion)
	assert.Empty(t, warnings)
}

func TestAdaptLegacySettings_EmptyFields(t *testing.T) {
	legacy := &Settings{}

	vs, warnings := AdaptLegacySettings(legacy)

	assert.Equal(t, "1", vs.SchemaVersion)
	assert.Nil(t, vs.Hub)
	assert.Nil(t, vs.CLI)
	assert.Nil(t, vs.Runtimes)
	assert.Nil(t, vs.HarnessConfigs)
	assert.Nil(t, vs.Profiles)
	assert.Empty(t, warnings)
}

// --- convertVersionedToLegacy tests ---

func TestConvertVersionedToLegacy(t *testing.T) {
	tmux := true
	vs := &VersionedSettings{
		SchemaVersion:   "1",
		ActiveProfile:   "local",
		DefaultTemplate: "gemini",
		Hub: &V1HubClientConfig{
			Enabled:  boolPtr(true),
			Endpoint: "https://hub.example.com",
			GroveID:  "test-grove",
		},
		CLI: &V1CLIConfig{
			AutoHelp:            boolPtr(true),
			InteractiveDisabled: boolPtr(false),
		},
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {Type: "docker", Host: "tcp://localhost:2375"},
		},
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {
				Harness: "gemini",
				Image:   "example.com/gemini:latest",
				User:    "scion",
				Model:   "gemini-2.5-pro",
				Args:    []string{"--sandbox"},
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {
				Runtime:              "docker",
				DefaultTemplate:      "gemini",
				DefaultHarnessConfig: "gemini",
				Tmux:                 &tmux,
			},
		},
	}

	legacy := convertVersionedToLegacy(vs)

	assert.Equal(t, "local", legacy.ActiveProfile)
	assert.Equal(t, "gemini", legacy.DefaultTemplate)

	// Hub — only v1 fields should be mapped
	require.NotNil(t, legacy.Hub)
	assert.Equal(t, "https://hub.example.com", legacy.Hub.Endpoint)
	assert.Equal(t, "test-grove", legacy.Hub.GroveID)
	assert.True(t, *legacy.Hub.Enabled)
	assert.Empty(t, legacy.Hub.Token) // Not in v1

	// CLI — InteractiveDisabled should not be in legacy
	require.NotNil(t, legacy.CLI)
	assert.True(t, *legacy.CLI.AutoHelp)

	// Runtimes — Type should be dropped
	assert.Equal(t, "tcp://localhost:2375", legacy.Runtimes["docker"].Host)

	// Harnesses — Model and Args should be dropped
	assert.Equal(t, "example.com/gemini:latest", legacy.Harnesses["gemini"].Image)

	// Profiles — new fields should be dropped
	assert.Equal(t, "docker", legacy.Profiles["local"].Runtime)
}

func TestConvertVersionedToLegacy_Nil(t *testing.T) {
	legacy := convertVersionedToLegacy(nil)
	assert.NotNil(t, legacy)
	assert.Empty(t, legacy.ActiveProfile)
}

// --- LoadEffectiveSettings tests ---

func TestLoadEffectiveSettings_VersionedFileRouting(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Write versioned grove settings
	groveSettings := `
schema_version: "1"
active_profile: versioned-profile
harness_configs:
  gemini:
    harness: gemini
    image: example.com/gemini:latest
    user: scion
`
	require.NoError(t, os.WriteFile(filepath.Join(groveDir, "settings.yaml"), []byte(groveSettings), 0644))

	vs, warnings, err := LoadEffectiveSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "versioned-profile", vs.ActiveProfile)
	assert.Empty(t, warnings, "versioned path should produce no deprecation warnings")
}

func TestLoadEffectiveSettings_LegacyFileRouting(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Write legacy grove settings (has harnesses, no schema_version)
	groveSettings := `
active_profile: legacy-profile
harnesses:
  gemini:
    image: example.com/gemini:latest
    user: scion
profiles:
  legacy-profile:
    runtime: docker
`
	require.NoError(t, os.WriteFile(filepath.Join(groveDir, "settings.yaml"), []byte(groveSettings), 0644))

	vs, warnings, err := LoadEffectiveSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "legacy-profile", vs.ActiveProfile)
	assert.Equal(t, "1", vs.SchemaVersion) // Should be set by adapter
	assert.NotEmpty(t, warnings, "legacy path should produce deprecation warnings")
}

func TestLoadEffectiveSettings_NoUserFiles(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// No settings files — should use defaults via legacy path
	vs, warnings, err := LoadEffectiveSettings(groveDir)
	require.NoError(t, err)

	assert.Equal(t, "local", vs.ActiveProfile)
	assert.Equal(t, "gemini", vs.DefaultTemplate)
	// Defaults flow through legacy path since no user files, so we get harness warnings
	// from the adaptation of embedded defaults
	_ = warnings
}

// --- Default settings compatibility tests ---

func TestGetDefaultSettingsData_ProducesSameEffectiveDefaults(t *testing.T) {
	// GetDefaultSettingsData should produce the same effective config regardless
	// of whether the embedded file is versioned or legacy
	data, err := GetDefaultSettingsData()
	require.NoError(t, err)

	var settings Settings
	require.NoError(t, json.Unmarshal(data, &settings))

	// Should have all expected harnesses
	assert.Contains(t, settings.Harnesses, "gemini")
	assert.Contains(t, settings.Harnesses, "claude")
	assert.Contains(t, settings.Harnesses, "opencode")
	assert.Contains(t, settings.Harnesses, "codex")

	// Should have expected images
	assert.Contains(t, settings.Harnesses["gemini"].Image, "scion-gemini")
	assert.Contains(t, settings.Harnesses["claude"].Image, "scion-claude")

	// Should have expected runtimes
	assert.Contains(t, settings.Runtimes, "docker")
	assert.Contains(t, settings.Runtimes, "container")
	assert.Contains(t, settings.Runtimes, "kubernetes")

	// Should have expected profiles
	assert.Contains(t, settings.Profiles, "local")
	assert.Contains(t, settings.Profiles, "remote")

	// OS-specific runtime check
	expectedRuntime := "docker"
	if runtime.GOOS == "darwin" {
		expectedRuntime = "container"
	}
	assert.Equal(t, expectedRuntime, settings.Profiles["local"].Runtime)
}

func TestDefaultSettingsValidateAgainstSchema(t *testing.T) {
	// The embedded default_settings.yaml should validate against the v1 schema
	data, err := EmbedsFS.ReadFile("embeds/default_settings.yaml")
	require.NoError(t, err)

	valErrors, err := ValidateSettings(data, "1")
	require.NoError(t, err)
	assert.Empty(t, valErrors, "default settings should validate against v1 schema, got: %v", valErrors)
}

func TestDefaultSettingsDataYAML_OSAdjustment(t *testing.T) {
	data, err := GetDefaultSettingsDataYAML()
	require.NoError(t, err)

	// Parse as versioned settings to check OS adjustment
	var vs VersionedSettings
	require.NoError(t, yaml.Unmarshal(data, &vs))

	expectedRuntime := "docker"
	if runtime.GOOS == "darwin" {
		expectedRuntime = "container"
	}

	localProfile, ok := vs.Profiles["local"]
	require.True(t, ok, "local profile should exist")
	assert.Equal(t, expectedRuntime, localProfile.Runtime)
}

// --- Adapter round-trip consistency ---

func TestAdapterRoundTripConsistency(t *testing.T) {
	// Load defaults via legacy path + adapt, vs load directly via versioned
	// The results should be equivalent in the shared fields
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Load via legacy path
	legacySettings, err := LoadSettingsKoanf(groveDir)
	require.NoError(t, err)
	adapted, _ := AdaptLegacySettings(legacySettings)

	// Load via versioned path
	versioned, err := LoadVersionedSettings(groveDir)
	require.NoError(t, err)

	// Compare shared fields
	assert.Equal(t, adapted.ActiveProfile, versioned.ActiveProfile)
	assert.Equal(t, adapted.DefaultTemplate, versioned.DefaultTemplate)

	// Compare harness config images (adapted from legacy harnesses)
	for name, hc := range adapted.HarnessConfigs {
		vhc, ok := versioned.HarnessConfigs[name]
		if assert.True(t, ok, "versioned should have harness config %q", name) {
			assert.Equal(t, hc.Image, vhc.Image, "image mismatch for %q", name)
			assert.Equal(t, hc.User, vhc.User, "user mismatch for %q", name)
		}
	}

	// Compare profiles
	for name, profile := range adapted.Profiles {
		vProfile, ok := versioned.Profiles[name]
		if assert.True(t, ok, "versioned should have profile %q", name) {
			assert.Equal(t, profile.Runtime, vProfile.Runtime, "runtime mismatch for profile %q", name)
		}
	}
}

// --- resolveEffectiveGrovePath tests ---

func TestResolveEffectiveGrovePath_Global(t *testing.T) {
	result := resolveEffectiveGrovePath("global")
	assert.Equal(t, "", result, "global should resolve to empty (already loaded)")

	result = resolveEffectiveGrovePath("home")
	assert.Equal(t, "", result, "home should resolve to empty (already loaded)")
}

func TestResolveEffectiveGrovePath_Explicit(t *testing.T) {
	result := resolveEffectiveGrovePath("/some/path/.scion")
	assert.Equal(t, "/some/path/.scion", result)
}

// --- versionedEnvKeyMapper tests ---

func TestVersionedEnvKeyMapper(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SCION_ACTIVE_PROFILE", "active_profile"},
		{"SCION_DEFAULT_TEMPLATE", "default_template"},
		{"SCION_HUB_ENDPOINT", "hub.endpoint"},
		{"SCION_HUB_GROVE_ID", "hub.grove_id"},
		{"SCION_HUB_LOCAL_ONLY", "hub.local_only"},
		{"SCION_HUB_ENABLED", "hub.enabled"},
		{"SCION_CLI_AUTOHELP", "cli.autohelp"},
		{"SCION_CLI_INTERACTIVE_DISABLED", "cli.interactive_disabled"},
		{"SCION_SERVER_ENV", "server.env"},
		{"SCION_SERVER_LOG_LEVEL", "server.log_level"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := versionedEnvKeyMapper(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- detectHierarchyFormat tests ---

func TestDetectHierarchyFormat_Versioned(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	versionedSettings := `schema_version: "1"
active_profile: local
`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(versionedSettings), 0644))

	assert.True(t, detectHierarchyFormat(""))
}

func TestDetectHierarchyFormat_Legacy(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	legacySettings := `active_profile: local
harnesses:
  gemini:
    image: test
`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(legacySettings), 0644))

	assert.False(t, detectHierarchyFormat(""))
}

func TestDetectHierarchyFormat_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	assert.False(t, detectHierarchyFormat(""))
}

func TestDetectHierarchyFormat_GroveVersioned(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	groveDir := filepath.Join(tmpDir, "my-grove", ".scion")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	// Global is legacy, grove is versioned
	globalScionDir := filepath.Join(tmpDir, ".scion")
	require.NoError(t, os.MkdirAll(globalScionDir, 0755))

	legacySettings := `active_profile: local
harnesses:
  gemini:
    image: test
`
	require.NoError(t, os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(legacySettings), 0644))

	versionedSettings := `schema_version: "1"
active_profile: custom
`
	require.NoError(t, os.WriteFile(filepath.Join(groveDir, "settings.yaml"), []byte(versionedSettings), 0644))

	assert.True(t, detectHierarchyFormat(groveDir))
}

// --- ResolveHarnessConfig tests ---

func TestResolveHarnessConfig_Default(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {
				Harness: "gemini",
				Image:   "example.com/gemini:latest",
				User:    "scion",
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "docker"},
		},
	}

	hc, err := vs.ResolveHarnessConfig("", "gemini")
	require.NoError(t, err)
	assert.Equal(t, "example.com/gemini:latest", hc.Image)
	assert.Equal(t, "scion", hc.User)
	assert.Equal(t, "gemini", hc.Harness)
}

func TestResolveHarnessConfig_Named(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {
				Harness: "gemini",
				Image:   "example.com/gemini:latest",
				User:    "scion",
			},
			"gemini-high-security": {
				Harness: "gemini",
				Image:   "example.com/gemini:hardened",
				User:    "restricted",
				Model:   "gemini-2.5-pro",
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "docker"},
		},
	}

	hc, err := vs.ResolveHarnessConfig("local", "gemini-high-security")
	require.NoError(t, err)
	assert.Equal(t, "example.com/gemini:hardened", hc.Image)
	assert.Equal(t, "restricted", hc.User)
	assert.Equal(t, "gemini", hc.Harness)
	assert.Equal(t, "gemini-2.5-pro", hc.Model)
}

func TestResolveHarnessConfig_WithProfileOverrides(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "staging",
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {
				Harness: "gemini",
				Image:   "example.com/gemini:latest",
				User:    "scion",
				Env:     map[string]string{"BASE_KEY": "base_value"},
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"staging": {
				Runtime: "docker",
				Env:     map[string]string{"PROFILE_KEY": "profile_value"},
				Volumes: []api.VolumeMount{{Source: "/profile/vol", Target: "/mnt/vol"}},
				HarnessOverrides: map[string]HarnessOverride{
					"gemini": {
						Image: "example.com/gemini:staging",
						Env:   map[string]string{"OVERRIDE_KEY": "override_value"},
					},
				},
			},
		},
	}

	hc, err := vs.ResolveHarnessConfig("", "gemini")
	require.NoError(t, err)
	assert.Equal(t, "example.com/gemini:staging", hc.Image, "image should be overridden by profile")
	assert.Equal(t, "scion", hc.User, "user should remain from base config")
	assert.Equal(t, "base_value", hc.Env["BASE_KEY"], "base env should be preserved")
	assert.Equal(t, "profile_value", hc.Env["PROFILE_KEY"], "profile env should be merged")
	assert.Equal(t, "override_value", hc.Env["OVERRIDE_KEY"], "override env should be merged")
	assert.Len(t, hc.Volumes, 1, "profile volume should be appended")
	assert.Equal(t, "/mnt/vol", hc.Volumes[0].Target)
}

func TestResolveHarnessConfig_NotFound(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {Harness: "gemini", Image: "test"},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "docker"},
		},
	}

	_, err := vs.ResolveHarnessConfig("", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestResolveHarnessConfig_ProfileNotFound(t *testing.T) {
	// When the profile is not found, we should still return the base config without error.
	vs := &VersionedSettings{
		ActiveProfile: "missing-profile",
		HarnessConfigs: map[string]HarnessConfigEntry{
			"gemini": {Harness: "gemini", Image: "test", User: "scion"},
		},
		Profiles: map[string]V1ProfileConfig{},
	}

	hc, err := vs.ResolveHarnessConfig("", "gemini")
	require.NoError(t, err)
	assert.Equal(t, "test", hc.Image)
	assert.Equal(t, "scion", hc.User)
}

// --- ResolveRuntime tests ---

func TestResolveRuntime_Basic(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {Type: "docker", Host: "tcp://localhost:2375"},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "docker"},
		},
	}

	rtConfig, runtimeType, err := vs.ResolveRuntime("")
	require.NoError(t, err)
	assert.Equal(t, "docker", runtimeType)
	assert.Equal(t, "tcp://localhost:2375", rtConfig.Host)
}

func TestResolveRuntime_WithType(t *testing.T) {
	// Runtime with explicit Type field different from map key
	vs := &VersionedSettings{
		ActiveProfile: "remote",
		Runtimes: map[string]V1RuntimeConfig{
			"my-remote-cluster": {
				Type:      "kubernetes",
				Namespace: "scion",
				Context:   "prod-cluster",
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"remote": {Runtime: "my-remote-cluster"},
		},
	}

	rtConfig, runtimeType, err := vs.ResolveRuntime("")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes", runtimeType, "should use explicit Type field")
	assert.Equal(t, "scion", rtConfig.Namespace)
	assert.Equal(t, "prod-cluster", rtConfig.Context)
}

func TestResolveRuntime_TypeFromKey(t *testing.T) {
	// Type field absent — should fall back to map key name
	vs := &VersionedSettings{
		ActiveProfile: "local",
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {Host: "unix:///var/run/docker.sock"},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "docker"},
		},
	}

	_, runtimeType, err := vs.ResolveRuntime("")
	require.NoError(t, err)
	assert.Equal(t, "docker", runtimeType, "should fall back to map key name when Type is empty")
}

func TestResolveRuntime_ProfileEnvMerge(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {
				Type: "docker",
				Env:  map[string]string{"RUNTIME_KEY": "runtime_value"},
			},
		},
		Profiles: map[string]V1ProfileConfig{
			"local": {
				Runtime: "docker",
				Env:     map[string]string{"PROFILE_KEY": "profile_value"},
			},
		},
	}

	rtConfig, _, err := vs.ResolveRuntime("")
	require.NoError(t, err)
	assert.Equal(t, "runtime_value", rtConfig.Env["RUNTIME_KEY"], "runtime env should be preserved")
	assert.Equal(t, "profile_value", rtConfig.Env["PROFILE_KEY"], "profile env should be merged")
}

func TestResolveRuntime_ProfileNotFound(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "nonexistent",
		Runtimes: map[string]V1RuntimeConfig{
			"docker": {Type: "docker"},
		},
		Profiles: map[string]V1ProfileConfig{},
	}

	_, _, err := vs.ResolveRuntime("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestResolveRuntime_RuntimeNotFound(t *testing.T) {
	vs := &VersionedSettings{
		ActiveProfile: "local",
		Runtimes:      map[string]V1RuntimeConfig{},
		Profiles: map[string]V1ProfileConfig{
			"local": {Runtime: "missing-runtime"},
		},
	}

	_, _, err := vs.ResolveRuntime("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-runtime")
}

// --- Hub helper method tests ---

func TestVersionedSettings_GetHubEndpoint(t *testing.T) {
	vs := &VersionedSettings{}
	assert.Equal(t, "", vs.GetHubEndpoint())

	vs.Hub = &V1HubClientConfig{Endpoint: "https://hub.example.com"}
	assert.Equal(t, "https://hub.example.com", vs.GetHubEndpoint())
}

func TestVersionedSettings_IsHubConfigured(t *testing.T) {
	vs := &VersionedSettings{}
	assert.False(t, vs.IsHubConfigured())

	vs.Hub = &V1HubClientConfig{}
	assert.False(t, vs.IsHubConfigured())

	vs.Hub.Endpoint = "https://hub.example.com"
	assert.True(t, vs.IsHubConfigured())
}

func TestVersionedSettings_IsHubEnabled(t *testing.T) {
	vs := &VersionedSettings{}
	assert.False(t, vs.IsHubEnabled())

	vs.Hub = &V1HubClientConfig{}
	assert.False(t, vs.IsHubEnabled())

	vs.Hub.Enabled = boolPtr(false)
	assert.False(t, vs.IsHubEnabled())

	vs.Hub.Enabled = boolPtr(true)
	assert.True(t, vs.IsHubEnabled())
}

func TestVersionedSettings_IsHubExplicitlyDisabled(t *testing.T) {
	vs := &VersionedSettings{}
	assert.False(t, vs.IsHubExplicitlyDisabled())

	vs.Hub = &V1HubClientConfig{Enabled: boolPtr(true)}
	assert.False(t, vs.IsHubExplicitlyDisabled())

	vs.Hub.Enabled = boolPtr(false)
	assert.True(t, vs.IsHubExplicitlyDisabled())
}

func TestVersionedSettings_IsHubLocalOnly(t *testing.T) {
	vs := &VersionedSettings{}
	assert.False(t, vs.IsHubLocalOnly())

	vs.Hub = &V1HubClientConfig{}
	assert.False(t, vs.IsHubLocalOnly())

	vs.Hub.LocalOnly = boolPtr(true)
	assert.True(t, vs.IsHubLocalOnly())
}

// --- Compatibility test ---

func TestLegacyAndVersionedResolution_SameResult(t *testing.T) {
	// Build legacy settings
	tmux := true
	legacy := &Settings{
		ActiveProfile: "local",
		Runtimes: map[string]RuntimeConfig{
			"docker": {Host: "tcp://localhost:2375"},
		},
		Harnesses: map[string]HarnessConfig{
			"gemini": {
				Image: "example.com/gemini:latest",
				User:  "scion",
				Env:   map[string]string{"KEY1": "val1"},
				Volumes: []api.VolumeMount{
					{Source: "/host/path", Target: "/container/path"},
				},
			},
		},
		Profiles: map[string]ProfileConfig{
			"local": {
				Runtime: "docker",
				Tmux:    &tmux,
				Env:     map[string]string{"PROFILE_KEY": "profile_val"},
				HarnessOverrides: map[string]HarnessOverride{
					"gemini": {
						Env: map[string]string{"OVERRIDE_KEY": "override_val"},
					},
				},
			},
		},
	}

	// Resolve via legacy path
	legacyHC, err := legacy.ResolveHarness("local", "gemini")
	require.NoError(t, err)

	// Adapt to versioned and resolve
	vs, _ := AdaptLegacySettings(legacy)
	versionedHC, err := vs.ResolveHarnessConfig("local", "gemini")
	require.NoError(t, err)

	// Compare results
	assert.Equal(t, legacyHC.Image, versionedHC.Image, "image should match")
	assert.Equal(t, legacyHC.User, versionedHC.User, "user should match")
	assert.Equal(t, legacyHC.Env["KEY1"], versionedHC.Env["KEY1"], "base env should match")
	assert.Equal(t, legacyHC.Env["PROFILE_KEY"], versionedHC.Env["PROFILE_KEY"], "profile env should match")
	assert.Equal(t, legacyHC.Env["OVERRIDE_KEY"], versionedHC.Env["OVERRIDE_KEY"], "override env should match")
	assert.Equal(t, len(legacyHC.Volumes), len(versionedHC.Volumes), "volume count should match")
}

// --- Helper ---

func boolPtr(b bool) *bool {
	return &b
}
