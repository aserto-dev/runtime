package runtime

import (
	"encoding/json"

	"github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/keys"
	bundleplugin "github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/open-policy-agent/opa/plugins/discovery"
	"github.com/open-policy-agent/opa/plugins/logs"
	"github.com/open-policy-agent/opa/plugins/status"
	"github.com/open-policy-agent/opa/topdown/cache"
)

type Config struct {
	LocalBundles                  LocalBundlesConfig `json:"local_bundles"`
	InstanceID                    string             `json:"instance_id"`
	PluginsErrorLimit             int                `json:"plugins_error_limit"`
	GracefulShutdownPeriodSeconds int                `json:"graceful_shutdown_period_seconds"`
	MaxPluginWaitTimeSeconds      int                `json:"max_plugin_wait_time_seconds"`
	Store                         string             `json:"store"`
	Config                        OPAConfig          `json:"config"`
}

func (c *Config) rawOPAConfig() ([]byte, error) {
	return json.Marshal(c.Config)
}

type LocalBundlesConfig struct {
	Watch              bool                       `json:"watch"`
	Paths              []string                   `json:"paths"`
	Ignore             []string                   `json:"ignore"`
	SkipVerification   bool                       `json:"skip_verification"`
	VerificationConfig *bundle.VerificationConfig `json:"verification_config"`
}

type OPAConfig struct {
	Services                     map[string]interface{}          `json:"services,omitempty"`
	Labels                       map[string]string               `json:"labels,omitempty"`
	Discovery                    *discovery.Config               `json:"discovery,omitempty"`
	Bundles                      map[string]*bundleplugin.Source `json:"bundles,omitempty"`
	DecisionLogs                 *logs.Config                    `json:"decision_logs,omitempty"`
	Status                       *status.Config                  `json:"status,omitempty"`
	Plugins                      map[string]interface{}          `json:"plugins,omitempty"`
	Keys                         map[string]*keys.Config         `json:"keys,omitempty"`
	DefaultDecision              *string                         `json:"default_decision,omitempty"`
	DefaultAuthorizationDecision *string                         `json:"default_authorization_decision,omitempty"`
	Caching                      *cache.Config                   `json:"caching,omitempty"`
	PersistenceDirectory         *string                         `json:"persistence_directory,omitempty"`
}
