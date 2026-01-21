package api

import (
	"context"
)

type AgentK8sMetadata struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	SyncedAt  string `json:"syncedAt,omitempty"`
}

type VolumeMount struct {
	Source   string `json:"source" yaml:"source"`
	Target   string `json:"target" yaml:"target"`
	ReadOnly bool   `json:"read_only,omitempty" yaml:"read_only,omitempty"`
	Type     string `json:"type,omitempty" yaml:"type,omitempty"`     // "local" (default) or "gcs"
	Bucket   string `json:"bucket,omitempty" yaml:"bucket,omitempty"` // For GCS
	Prefix   string `json:"prefix,omitempty" yaml:"prefix,omitempty"` // For GCS
	Mode     string `json:"mode,omitempty" yaml:"mode,omitempty"`     // Mount options
}

type KubernetesConfig struct {
	Context            string        `json:"context,omitempty" yaml:"context,omitempty"`
	Namespace          string        `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	RuntimeClassName   string        `json:"runtimeClassName,omitempty" yaml:"runtimeClassName,omitempty"`
	ServiceAccountName string        `json:"serviceAccountName,omitempty" yaml:"serviceAccountName,omitempty"` // For Workload Identity
	Resources          *K8sResources `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type K8sResources struct {
	Requests map[string]string `json:"requests,omitempty" yaml:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty" yaml:"limits,omitempty"`
}

type GeminiConfig struct {
	AuthSelectedType string `json:"auth_selectedType,omitempty" yaml:"auth_selectedType,omitempty"`
}

type ScionConfig struct {
	Harness     string            `json:"harness,omitempty" yaml:"harness,omitempty"`
	ConfigDir   string            `json:"config_dir,omitempty" yaml:"config_dir,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Volumes     []VolumeMount     `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	Detached    *bool             `json:"detached" yaml:"detached"`
	CommandArgs []string          `json:"command_args,omitempty" yaml:"command_args,omitempty"`
	Model       string            `json:"model,omitempty" yaml:"model,omitempty"`
	Kubernetes  *KubernetesConfig `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
	Gemini      *GeminiConfig     `json:"gemini,omitempty" yaml:"gemini,omitempty"`
	Image       string            `json:"image,omitempty" yaml:"image,omitempty"`

	// Info contains persisted metadata about the agent
	Info *AgentInfo `json:"-" yaml:"-"`
}

func (c *ScionConfig) IsDetached() bool {
	if c.Detached == nil {
		return true
	}
	return *c.Detached
}

type AuthConfig struct {
	GeminiAPIKey         string
	GoogleAPIKey         string
	VertexAPIKey         string
	GoogleAppCredentials string
	GoogleCloudProject   string
	OAuthCreds           string
	AnthropicAPIKey      string
	OpenCodeAuthFile     string
	CodexAuthFile        string
	SelectedType         string
}

type AuthProvider interface {
	GetAuthConfig(context.Context) (AuthConfig, error)
}

type AgentInfo struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	Template    string            `json:"template"`
	Grove       string            `json:"grove"`
	GrovePath   string            `json:"grovePath,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	ContainerStatus string        `json:"containerStatus,omitempty"` // Container status (e.g., Up 2 hours)
	Status          string            `json:"status,omitempty"`          // Scion agent high-level status (e.g., running, stopped)
	SessionStatus   string            `json:"sessionStatus,omitempty"`   // Agent session status (e.g., started, waiting, completed)
	Image       string            `json:"image,omitempty"`
	Detached    bool              `json:"detached,omitempty"`
	Runtime     string            `json:"runtime,omitempty"`
	Profile     string            `json:"profile,omitempty"`
	Kubernetes  *AgentK8sMetadata `json:"kubernetes,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
}

type StartOptions struct {
	Name      string
	Task      string
	Template  string
	Profile   string
	Image     string
	GrovePath string
	Env       map[string]string
	Detached  *bool
	Resume    bool
	Auth      AuthProvider
	NoAuth    bool
	Branch    string
	Workspace string
}

type StatusEvent struct {
	AgentID   string `json:"agent_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}
