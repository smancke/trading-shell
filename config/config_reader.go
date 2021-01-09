package config

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ConfigReader struct {
	ConfigT   reflect.Type
	EnvPrefix string
}

func (reader *ConfigReader) DefaultValue() interface{} {
	return reader.defaultValue().Interface()
}

func (reader *ConfigReader) Read(f *flag.FlagSet, args []string) (interface{}, error) {
	c, err := reader.readConfig(f, args)
	return c.Interface(), err
}

func (reader *ConfigReader) WithoutSecrets(config interface{}) interface{} {
	copy := reader.newConfig()

	src := reflect.ValueOf(config).Elem()
	dst := copy.Elem()
	iterateFields(copy, func(field reflect.StructField, _ string, _ string, secret bool) {
		if secret {
			return
		}

		fSrc := src.FieldByName(field.Name)
		fDst := dst.FieldByName(field.Name)

		fDst.Set(fSrc)
	})

	return copy.Interface()
}

func (reader *ConfigReader) defaultValue() reflect.Value {
	config := reader.newConfig()

	value := config.Elem()
	iterateFields(config, func(field reflect.StructField, defaultValue string, _ string, _ bool) {
		if defaultValue == "" {
			return
		}

		f := value.FieldByName(field.Name)

		switch f.Type().Kind() {
		case reflect.String:
			f.SetString(defaultValue)
		case reflect.Bool:
			f.SetBool(defaultValue == "true")
		case reflect.Int64:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				d, err := time.ParseDuration(defaultValue)
				if err != nil {
					panic(err)
				}
				f.SetInt(int64(d))
				return
			}
			fallthrough

		case reflect.Int:
			i, err := strconv.ParseInt(defaultValue, 10, 64)
			if err != nil {
				panic(err)
			}
			f.SetInt(i)

		case reflect.Uint64:
			i, err := strconv.ParseUint(defaultValue, 10, 64)
			if err != nil {
				panic(err)
			}
			f.SetUint(i)

		default:
			panic(fmt.Errorf("unsupported kind '%v'", field.Type.Kind()))
		}
	})

	return config
}

func (reader *ConfigReader) newConfig() reflect.Value {
	return reflect.New(reader.ConfigT)
}

func (reader *ConfigReader) readConfig(f *flag.FlagSet, args []string) (reflect.Value, error) {
	config := reader.defaultValue()
	configureFlagSet(config, f)

	// prefer environment settings
	f.VisitAll(func(f *flag.Flag) {
		if val, isPresent := os.LookupEnv(reader.envNameWithPrefix(f.Name)); isPresent {
			f.Value.Set(val)
		} else if val, isPresent := os.LookupEnv(envName(f.Name)); isPresent {
			f.Value.Set(val)
		}
	})

	// prefer flags over environment settings
	err := f.Parse(args)
	if err != nil {
		return reflect.Value{}, err
	}

	return config, err
}

func configureFlagSet(config reflect.Value, flags *flag.FlagSet) {
	value := config.Elem()
	iterateFields(config, func(field reflect.StructField, _ string, desc string, _ bool) {
		argName := toArgName(field.Name)

		f := value.FieldByName(field.Name)

		switch f.Type().Kind() {
		case reflect.String:
			flags.StringVar(f.Addr().Interface().(*string), argName, f.String(), desc)

		case reflect.Bool:
			flags.BoolVar(f.Addr().Interface().(*bool), argName, f.Bool(), desc)

		case reflect.Int64:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				flags.DurationVar(f.Addr().Interface().(*time.Duration), argName, time.Duration(f.Int()), desc)
				return
			}
			flags.Int64Var(f.Addr().Interface().(*int64), argName, f.Int(), desc)

		case reflect.Int:
			flags.IntVar(f.Addr().Interface().(*int), argName, int(f.Int()), desc)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			flags.Uint64Var(f.Addr().Interface().(*uint64), argName, f.Uint(), desc)

		default:
			panic(fmt.Errorf("unsupported kind '%v'", field.Type.Kind()))
		}

	})
}

func (reader *ConfigReader) envNameWithPrefix(flagName string) string {
	return reader.EnvPrefix + envName(flagName)
}

func envName(flagName string) string {
	return strings.Replace(strings.ToUpper(flagName), "-", "_", -1)
}

func iterateFields(config reflect.Value, body func(reflect.StructField, string, string, bool)) {
	value := config.Elem()
	valueT := value.Type()

	for i := 0; i < valueT.NumField(); i++ {
		field := valueT.Field(i)

		defaultValue := ""
		description := ""
		secret := false

		if tag, ok := field.Tag.Lookup("config"); ok {
			values := strings.Split(tag, ",")
			if len(values) > 0 {
				defaultValue = values[0]
			}
			if len(values) > 1 {
				secret = values[1] == "secret"
			}
		}

		if tag, ok := field.Tag.Lookup("desc"); ok {
			description = tag
		}

		body(field, defaultValue, description, secret)
	}
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z]+)")

func toArgName(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}-${2}")
	return strings.ToLower(snake)
}
