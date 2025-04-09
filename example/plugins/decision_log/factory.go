package decision_log

import (
	"bytes"

	"github.com/go-viper/mapstructure/v2"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/util"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type PluginFactory struct{}

func NewPluginFactory() PluginFactory {
	return PluginFactory{}
}

func (PluginFactory) New(m *plugins.Manager, config any) plugins.Plugin { //nolint:ireturn
	cfg, _ := config.(*Config)
	return newDecisionLogger(cfg, m)
}

func (PluginFactory) Validate(m *plugins.Manager, config []byte) (any, error) { //nolint:ireturn
	parsedConfig := Config{}
	v := viper.New()
	v.SetConfigType("json")

	if err := v.ReadConfig(bytes.NewReader(config)); err != nil {
		return nil, errors.Wrap(err, "error parsing decision logs config")
	}

	if err := v.UnmarshalExact(
		&parsedConfig,
		func(dc *mapstructure.DecoderConfig) {
			dc.TagName = "json"
		},
	); err != nil {
		return nil, errors.Wrap(err, "error parsing decision logs config")
	}

	return &parsedConfig, util.Unmarshal(config, &parsedConfig)
}
