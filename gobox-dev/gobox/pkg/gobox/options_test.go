package gobox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExecConfig(t *testing.T) {
	config := DefaultExecConfig()

	assert.Equal(t, "python", config.Kernel)
	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, "", config.Cwd)
	assert.NotNil(t, config.Env)
}

func TestWithKernel(t *testing.T) {
	config := ApplyExecOptions(WithKernel("bash"))
	assert.Equal(t, "bash", config.Kernel)
}

func TestWithTimeout(t *testing.T) {
	config := ApplyExecOptions(WithTimeout(30 * time.Second))
	assert.Equal(t, 30*time.Second, config.Timeout)
}

func TestWithCwd(t *testing.T) {
	config := ApplyExecOptions(WithCwd("/tmp"))
	assert.Equal(t, "/tmp", config.Cwd)
}

func TestWithEnv(t *testing.T) {
	env := map[string]string{"FOO": "bar"}
	config := ApplyExecOptions(WithEnv(env))
	assert.Equal(t, "bar", config.Env["FOO"])
}

func TestWithEnvVar(t *testing.T) {
	config := ApplyExecOptions(WithEnvVar("KEY", "value"))
	assert.Equal(t, "value", config.Env["KEY"])
}

func TestApplyExecOptions_Multiple(t *testing.T) {
	config := ApplyExecOptions(
		WithKernel("bash"),
		WithTimeout(120*time.Second),
		WithCwd("/app"),
		WithEnvVar("VAR1", "val1"),
		WithEnvVar("VAR2", "val2"),
	)

	assert.Equal(t, "bash", config.Kernel)
	assert.Equal(t, 120*time.Second, config.Timeout)
	assert.Equal(t, "/app", config.Cwd)
	assert.Equal(t, "val1", config.Env["VAR1"])
	assert.Equal(t, "val2", config.Env["VAR2"])
}

func TestDefaultSandboxConfig(t *testing.T) {
	config := DefaultSandboxConfig()

	assert.Equal(t, "docker", config.Driver)
	assert.Equal(t, "shroominic/codebox:latest", config.Image)
	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, int64(512*1024*1024), config.Memory)
	assert.Equal(t, int64(100000), config.CPUQuota)
	assert.Equal(t, "bridge", config.NetworkMode)
	assert.NotNil(t, config.PoolConfig)
}

func TestDefaultPoolConfig(t *testing.T) {
	config := DefaultPoolConfig()

	assert.Equal(t, 2, config.MinSize)
	assert.Equal(t, 10, config.MaxSize)
	assert.Equal(t, 5*time.Minute, config.IdleTimeout)
	assert.Equal(t, 2, config.WarmupCount)
}

func TestSandboxOptions(t *testing.T) {
	config := ApplySandboxOptions(
		WithDriver("remote"),
		WithImage("custom:latest"),
		WithDefaultTimeout(30*time.Second),
		WithMemory(1024*1024*1024),
		WithCPUQuota(200000),
		WithNetworkMode("host"),
		WithAPIKey("test-key"),
		WithBaseURL("https://example.com"),
		WithFactoryID("factory-1"),
		WithSessionID("session-1"),
		WithPoolSize(5, 20),
	)

	assert.Equal(t, "remote", config.Driver)
	assert.Equal(t, "custom:latest", config.Image)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, int64(1024*1024*1024), config.Memory)
	assert.Equal(t, int64(200000), config.CPUQuota)
	assert.Equal(t, "host", config.NetworkMode)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "https://example.com", config.BaseURL)
	assert.Equal(t, "factory-1", config.FactoryID)
	assert.Equal(t, "session-1", config.SessionID)
	assert.Equal(t, 5, config.PoolConfig.MinSize)
	assert.Equal(t, 20, config.PoolConfig.MaxSize)
}

func TestWithPoolConfig(t *testing.T) {
	poolConfig := &PoolConfig{
		MinSize:     3,
		MaxSize:     15,
		IdleTimeout: 10 * time.Minute,
		WarmupCount: 5,
	}

	config := ApplySandboxOptions(WithPoolConfig(poolConfig))

	assert.Equal(t, poolConfig, config.PoolConfig)
	assert.Equal(t, 3, config.PoolConfig.MinSize)
	assert.Equal(t, 15, config.PoolConfig.MaxSize)
}

func TestSandboxConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *SandboxConfig
		wantErr bool
	}{
		{
			name:    "valid docker config",
			config:  DefaultSandboxConfig(),
			wantErr: false,
		},
		{
			name: "valid remote config",
			config: &SandboxConfig{
				Driver:  "remote",
				APIKey:  "test-key",
				BaseURL: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "invalid driver",
			config: &SandboxConfig{
				Driver: "invalid",
			},
			wantErr: true,
		},
		{
			name: "docker without image",
			config: &SandboxConfig{
				Driver: "docker",
				Image:  "",
			},
			wantErr: true,
		},
		{
			name: "remote without api key",
			config: &SandboxConfig{
				Driver:  "remote",
				BaseURL: "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "remote without base url",
			config: &SandboxConfig{
				Driver: "remote",
				APIKey: "test-key",
			},
			wantErr: true,
		},
		{
			name: "invalid pool config - negative min",
			config: &SandboxConfig{
				Driver: "docker",
				Image:  "test:latest",
				PoolConfig: &PoolConfig{
					MinSize: -1,
					MaxSize: 10,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid pool config - max < min",
			config: &SandboxConfig{
				Driver: "docker",
				Image:  "test:latest",
				PoolConfig: &PoolConfig{
					MinSize: 10,
					MaxSize: 5,
				},
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
