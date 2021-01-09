package config

import (
	"flag"
	"os"
	"reflect"
	"time"
)

var configReader = ConfigReader{
	ConfigT:   reflect.TypeOf(Config{}),
	EnvPrefix: "",
}

// Config for the application
type Config struct {
	Host        string        `config:"localhost" desc:"The host to listen on"`
	Port        string        `config:"8080" desc:"The port to listen on"`
	LogLevel    string        `config:"error" desc:"The log level"`
	TextLogging bool          `config:"true" desc:"Log in text format instead of json"`
	GracePeriod time.Duration `config:"5s" desc:"Graceful shutdown grace period"`

	APIKey    string `config:"" desc:"The API key"`
	APISecret string `config:"" desc:"The API secret"`
}

func ReadConfig() *Config {
	config, err := readConfig(flag.CommandLine, os.Args[1:])
	if err != nil {
		panic(err)
	}
	return config
}

func readConfig(f *flag.FlagSet, args []string) (*Config, error) {
	config, err := configReader.Read(f, args)
	return config.(*Config), err
}

func DefaultConfig() *Config {
	return configReader.DefaultValue().(*Config)
}

func (c *Config) WithoutSecrets() *Config {
	return configReader.WithoutSecrets(c).(*Config)
}
