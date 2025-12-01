package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Clear environment
	defer func() {
		os.Unsetenv("GOBOX_API_KEY")
		os.Unsetenv("GOBOX_BASE_URL")
		os.Unsetenv("GOBOX_POOL_MIN_SIZE")
	}()

	// Test defaults
	config := Load()

	assert.Equal(t, DefaultBaseURL, config.BaseURL)
	assert.Equal(t, DefaultFactoryID, config.FactoryID)
	assert.Equal(t, DefaultDockerImage, config.DockerImage)
	assert.Equal(t, DefaultPoolMinSize, config.PoolMinSize)
	assert.Equal(t, DefaultPoolMaxSize, config.PoolMaxSize)
	assert.Equal(t, DefaultTimeout, config.DefaultTimeout)
	assert.Equal(t, DefaultServerPort, config.ServerPort)
}

func TestLoad_WithEnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("GOBOX_API_KEY", "test-key")
	os.Setenv("GOBOX_BASE_URL", "https://custom.api.com")
	os.Setenv("GOBOX_POOL_MIN_SIZE", "5")
	os.Setenv("GOBOX_DEFAULT_TIMEOUT", "30s")

	defer func() {
		os.Unsetenv("GOBOX_API_KEY")
		os.Unsetenv("GOBOX_BASE_URL")
		os.Unsetenv("GOBOX_POOL_MIN_SIZE")
		os.Unsetenv("GOBOX_DEFAULT_TIMEOUT")
	}()

	config := Load()

	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "https://custom.api.com", config.BaseURL)
	assert.Equal(t, 5, config.PoolMinSize)
	assert.Equal(t, 30*time.Second, config.DefaultTimeout)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  Load(),
			wantErr: false,
		},
		{
			name: "negative pool min size",
			config: &Config{
				PoolMinSize:    -1,
				PoolMaxSize:    10,
				DefaultTimeout: DefaultTimeout,
				MaxFileSize:    DefaultMaxFileSize,
			},
			wantErr: true,
		},
		{
			name: "pool max < min",
			config: &Config{
				PoolMinSize:    10,
				PoolMaxSize:    5,
				DefaultTimeout: DefaultTimeout,
				MaxFileSize:    DefaultMaxFileSize,
			},
			wantErr: true,
		},
		{
			name: "zero timeout",
			config: &Config{
				PoolMinSize:    2,
				PoolMaxSize:    10,
				DefaultTimeout: 0,
				MaxFileSize:    DefaultMaxFileSize,
			},
			wantErr: true,
		},
		{
			name: "zero max file size",
			config: &Config{
				PoolMinSize:    2,
				PoolMaxSize:    10,
				DefaultTimeout: DefaultTimeout,
				MaxFileSize:    0,
			},
			wantErr: true,
		},
		{
			name: "invalid server port",
			config: &Config{
				PoolMinSize:    2,
				PoolMaxSize:    10,
				DefaultTimeout: DefaultTimeout,
				MaxFileSize:    DefaultMaxFileSize,
				ServerPort:     70000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "gobox-config-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create config and save
	original := &Config{
		APIKey:         "saved-key",
		BaseURL:        DefaultBaseURL,
		FactoryID:      DefaultFactoryID,
		DockerImage:    DefaultDockerImage,
		DockerTimeout:  DefaultDockerTimeout,
		PoolMinSize:    5,
		PoolMaxSize:    DefaultPoolMaxSize,
		DefaultTimeout: DefaultTimeout,
		MaxFileSize:    DefaultMaxFileSize,
		ServerPort:     DefaultServerPort,
		LogLevel:       DefaultLogLevel,
		LogFormat:      DefaultLogFormat,
	}

	err = original.SaveToFile(tmpFile.Name())
	require.NoError(t, err)

	// Load back - note that environment variables won't be set,
	// so the loaded config will have values from the file
	loaded, err := LoadFromFile(tmpFile.Name())
	require.NoError(t, err)

	// Check that default values are preserved (since env vars are not set)
	assert.Equal(t, DefaultBaseURL, loaded.BaseURL)
	assert.Equal(t, DefaultFactoryID, loaded.FactoryID)
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test-value")
	defer os.Unsetenv("TEST_VAR")

	assert.Equal(t, "test-value", getEnv("TEST_VAR", "default"))
	assert.Equal(t, "default", getEnv("NONEXISTENT_VAR", "default"))
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	assert.Equal(t, 42, getEnvInt("TEST_INT", 0))
	assert.Equal(t, 100, getEnvInt("NONEXISTENT_INT", 100))

	os.Setenv("TEST_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INVALID")
	assert.Equal(t, 50, getEnvInt("TEST_INVALID", 50))
}

func TestGetEnvInt64(t *testing.T) {
	os.Setenv("TEST_INT64", "1000000000")
	defer os.Unsetenv("TEST_INT64")

	assert.Equal(t, int64(1000000000), getEnvInt64("TEST_INT64", 0))
	assert.Equal(t, int64(999), getEnvInt64("NONEXISTENT_INT64", 999))
}

func TestGetEnvDuration(t *testing.T) {
	// Test duration string
	os.Setenv("TEST_DUR_STR", "5m30s")
	defer os.Unsetenv("TEST_DUR_STR")
	assert.Equal(t, 5*time.Minute+30*time.Second, getEnvDuration("TEST_DUR_STR", time.Second))

	// Test integer as seconds
	os.Setenv("TEST_DUR_INT", "60")
	defer os.Unsetenv("TEST_DUR_INT")
	assert.Equal(t, 60*time.Second, getEnvDuration("TEST_DUR_INT", time.Second))

	// Test default
	assert.Equal(t, 10*time.Second, getEnvDuration("NONEXISTENT_DUR", 10*time.Second))
}

func TestGetEnvBool(t *testing.T) {
	os.Setenv("TEST_BOOL_TRUE", "true")
	os.Setenv("TEST_BOOL_FALSE", "false")
	os.Setenv("TEST_BOOL_1", "1")
	os.Setenv("TEST_BOOL_INVALID", "not-bool")
	defer func() {
		os.Unsetenv("TEST_BOOL_TRUE")
		os.Unsetenv("TEST_BOOL_FALSE")
		os.Unsetenv("TEST_BOOL_1")
		os.Unsetenv("TEST_BOOL_INVALID")
	}()

	assert.True(t, getEnvBool("TEST_BOOL_TRUE", false))
	assert.False(t, getEnvBool("TEST_BOOL_FALSE", true))
	assert.True(t, getEnvBool("TEST_BOOL_1", false))
	assert.False(t, getEnvBool("TEST_BOOL_INVALID", false))
	assert.True(t, getEnvBool("NONEXISTENT_BOOL", true))
}

func TestConfigError(t *testing.T) {
	err := &ConfigError{Field: "pool_size", Message: "must be positive"}
	assert.Contains(t, err.Error(), "pool_size")
	assert.Contains(t, err.Error(), "must be positive")
}
