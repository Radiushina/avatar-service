package config

import (
	"fmt"
	"os"
	"reflect"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

const (
	defaultConfigPath = "config/content.yml"
	structTagDelim    = "."
)

type fieldMeta struct {
	path string
	env  string
	flag string
}

type Loader struct {
	k          *koanf.Koanf
	configPath string
	flags      *pflag.FlagSet
	fields     []fieldMeta
}

func NewLoader(configPath string) *Loader {
	if configPath == "" {
		configPath = defaultConfigPath
	}
	return &Loader{
		k:          koanf.New(structTagDelim),
		configPath: configPath,
		flags:      pflag.NewFlagSet("config", pflag.ContinueOnError),
		fields:     collectFields(reflect.TypeOf(Config{}), ""),
	}
}

func (l *Loader) IgnoreUnknownFlags() {
	l.flags.ParseErrorsAllowlist.UnknownFlags = true
}

func (l *Loader) Load() (*Config, error) {
	if err := l.loadDefaults(); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	l.defineFlags()
	if err := l.flags.Parse(os.Args[1:]); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %w", err)
	}

	if err := l.loadYAML(); err != nil {
		return nil, fmt.Errorf("failed to load YAML config: %w", err)
	}

	if err := l.loadEnv(); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	if err := l.loadFlags(); err != nil {
		return nil, fmt.Errorf("failed to apply CLI flags: %w", err)
	}

	var cfg Config
	if err := l.k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &cfg, nil
}

func (l *Loader) loadDefaults() error {
	defaults := DefaultConfig()
	if err := l.k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return fmt.Errorf("loadDefaults: %w", err)
	}
	return nil
}

func (l *Loader) loadYAML() error {
	if _, err := os.Stat(l.configPath); os.IsNotExist(err) {
		return nil
	}
	if err := l.k.Load(file.Provider(l.configPath), yaml.Parser()); err != nil {
		return fmt.Errorf("loadYAML: %w", err)
	}
	return nil
}

func (l *Loader) loadEnv() error {
	for _, f := range l.fields {
		if f.env == "" {
			continue
		}
		raw, ok := os.LookupEnv(f.env)
		if !ok {
			continue
		}
		if err := l.k.Set(f.path, raw); err != nil {
			return fmt.Errorf("loadEnv: set %s: %w", f.path, err)
		}
	}
	return nil
}

func (l *Loader) loadFlags() error {
	var visitErr error
	l.flags.Visit(func(flag *pflag.Flag) {
		if visitErr != nil || flag.Name == "config" {
			return
		}
		for _, f := range l.fields {
			if f.flag != flag.Name {
				continue
			}
			if err := l.k.Set(f.path, flag.Value.String()); err != nil {
				visitErr = fmt.Errorf("loadFlags: set %s: %w", f.path, err)
			}
			return
		}
	})
	return visitErr
}

func (l *Loader) defineFlags() {
	l.flags.StringVar(&l.configPath, "config", l.configPath, "path to YAML config")
	for _, f := range l.fields {
		if f.flag == "" {
			continue
		}
		l.flags.String(f.flag, "", f.path)
	}
}

func collectFields(t reflect.Type, prefix string) []fieldMeta {
	var out []fieldMeta
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		koanfTag := sf.Tag.Get("koanf")
		if koanfTag == "" || koanfTag == "-" {
			continue
		}
		path := koanfTag
		if prefix != "" {
			path = prefix + structTagDelim + koanfTag
		}
		if sf.Type.Kind() == reflect.Struct {
			out = append(out, collectFields(sf.Type, path)...)
			continue
		}
		out = append(out, fieldMeta{
			path: path,
			env:  sf.Tag.Get("env"),
			flag: sf.Tag.Get("flag"),
		})
	}
	return out
}
