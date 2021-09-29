module github.com/aserto-dev/verdict

go 1.16

replace github.com/aserto-dev/runtime => ../

require (
	github.com/alecthomas/kong v0.2.17
	github.com/aserto-dev/runtime v0.0.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/open-policy-agent/opa v0.32.1
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.25.0
	github.com/spf13/viper v1.8.1
	sigs.k8s.io/controller-runtime v0.9.6
)
