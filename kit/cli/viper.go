package cli

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/influxdata/influxdb/v2"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
)

// Opt is a single command-line option
type Opt struct {
	DestP interface{} // pointer to the destination

	EnvVar     string
	Flag       string
	Hidden     bool
	Persistent bool
	Required   bool
	Short      rune // using rune b/c it guarantees correctness. a short must always be a string of length 1

	Default interface{}
	Desc    string
}

// Program parses CLI options
type Program struct {
	// Run is invoked by cobra on execute.
	Run func() error
	// Name is the name of the program in help usage and the env var prefix.
	Name string
	// Opts are the command line/env var options to the program
	Opts []Opt
}

// NewCommand creates a new cobra command to be executed that respects env vars.
//
// Uses the upper-case version of the program's name as a prefix
// to all environment variables.
//
// This is to simplify the viper/cobra boilerplate.
func NewCommand(v *viper.Viper, p *Program) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:  p.Name,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return p.Run()
		},
	}

	v.SetEnvPrefix(strings.ToUpper(p.Name))
	v.AutomaticEnv()
	// This normalizes "-" to an underscore in env names.
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if configPath := v.GetString("CONFIG_PATH"); configPath != "" {
		switch path.Ext(configPath) {
		case ".json", ".toml", ".yaml", "yml":
			v.SetConfigFile(configPath)
		case "":
			v.AddConfigPath(configPath)
		}
	} else {
		// defaults to looking in same directory as program running for
		// a file with base `config` and extensions .json|.toml|.yaml|.yml
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}

	// done before we bind flags to viper keys.
	// order of precedence (1 highest -> 3 lowest):
	//	1. flags
	//  2. env vars
	//	3. config file
	if err := initializeConfig(v); err != nil {
		return nil, fmt.Errorf("invalid config file caused error: %w", err)
	}
	if err := BindOptions(v, cmd, p.Opts); err != nil {
		return nil, fmt.Errorf("failed to bind CLI options: %w", err)
	}

	return cmd, nil
}

func initializeConfig(v *viper.Viper) error {
	err := v.ReadInConfig()
	if err != nil && !os.IsNotExist(err) {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}
	return nil
}

// BindOptions adds opts to the specified command and automatically
// registers those options with viper.
func BindOptions(v *viper.Viper, cmd *cobra.Command, opts []Opt) error {
	for _, o := range opts {
		flagset := cmd.Flags()
		if o.Persistent {
			flagset = cmd.PersistentFlags()
		}
		envVal := lookupEnv(v, &o)
		hasShort := o.Short != 0

		switch destP := o.DestP.(type) {
		case *string:
			var d string
			if o.Default != nil {
				d = o.Default.(string)
			}
			if hasShort {
				flagset.StringVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.StringVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				s, err := cast.ToStringE(envVal)
				if err != nil {
					return fmt.Errorf("invalid string value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = s
			}

		case *int:
			var d int
			if o.Default != nil {
				d = o.Default.(int)
			}
			if hasShort {
				flagset.IntVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.IntVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				i, err := cast.ToIntE(envVal)
				if err != nil {
					return fmt.Errorf("invalid int value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = i
			}

		case *int32:
			var d int32
			if o.Default != nil {
				// N.B. since our CLI kit types default values as interface{} and
				// literal numbers get typed as int by default, it's very easy to
				// create an int32 CLI flag with an int default value.
				//
				// The compiler doesn't know to complain in that case, so you end up
				// with a runtime panic when trying to bind the CLI options.
				//
				// To avoid that headache, we support both int32 and int defaults
				// for int32 fields. This introduces a new runtime bomb if somebody
				// specifies an int default > math.MaxInt32, but that's hopefully
				// less likely.
				var ok bool
				d, ok = o.Default.(int32)
				if !ok {
					d = int32(o.Default.(int))
				}
			}
			if hasShort {
				flagset.Int32VarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.Int32Var(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				i, err := cast.ToInt32E(envVal)
				if err != nil {
					return fmt.Errorf("invalid int value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = i
			}

		case *int64:
			var d int64
			if o.Default != nil {
				// N.B. since our CLI kit types default values as interface{} and
				// literal numbers get typed as int by default, it's very easy to
				// create an int64 CLI flag with an int default value.
				//
				// The compiler doesn't know to complain in that case, so you end up
				// with a runtime panic when trying to bind the CLI options.
				//
				// To avoid that headache, we support both int64 and int defaults
				// for int64 fields.
				var ok bool
				d, ok = o.Default.(int64)
				if !ok {
					d = int64(o.Default.(int))
				}
			}
			if hasShort {
				flagset.Int64VarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.Int64Var(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				i, err := cast.ToInt64E(envVal)
				if err != nil {
					return fmt.Errorf("invalid int value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = i
			}

		case *bool:
			var d bool
			if o.Default != nil {
				d = o.Default.(bool)
			}
			if hasShort {
				flagset.BoolVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.BoolVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				b, err := cast.ToBoolE(envVal)
				if err != nil {
					return fmt.Errorf("invalid bool value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = b
			}

		case *time.Duration:
			var d time.Duration
			if o.Default != nil {
				d = o.Default.(time.Duration)
			}
			if hasShort {
				flagset.DurationVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.DurationVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				d, err := cast.ToDurationE(envVal)
				if err != nil {
					return fmt.Errorf("invalid duration value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = d
			}

		case *[]string:
			var d []string
			if o.Default != nil {
				d = o.Default.([]string)
			}
			if hasShort {
				flagset.StringSliceVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.StringSliceVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				ss, err := cast.ToStringSliceE(envVal)
				if err != nil {
					return fmt.Errorf("invalid string-slice value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = ss
			}

		case *map[string]string:
			var d map[string]string
			if o.Default != nil {
				d = o.Default.(map[string]string)
			}
			if hasShort {
				flagset.StringToStringVarP(destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				flagset.StringToStringVar(destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				sms, err := cast.ToStringMapStringE(envVal)
				if err != nil {
					return fmt.Errorf("invalid string-map-string value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
				*destP = sms
			}

		case pflag.Value:
			if hasShort {
				flagset.VarP(destP, o.Flag, string(o.Short), o.Desc)
			} else {
				flagset.Var(destP, o.Flag, o.Desc)
			}
			if o.Default != nil {
				_ = destP.Set(o.Default.(string))
			}
			if envVal != nil {
				s, err := cast.ToStringE(envVal)
				if err == nil {
					err = destP.Set(s)
				}
				if err != nil {
					return fmt.Errorf("invalid value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
			}

		case *influxdb.ID:
			var d influxdb.ID
			if o.Default != nil {
				d = o.Default.(influxdb.ID)
			}
			if hasShort {
				IDVarP(flagset, destP, o.Flag, string(o.Short), d, o.Desc)
			} else {
				IDVar(flagset, destP, o.Flag, d, o.Desc)
			}
			if envVal != nil {
				s, err := cast.ToStringE(envVal)
				if err == nil {
					err = (*destP).DecodeFromString(s)
				}
				if err != nil {
					return fmt.Errorf("invalid ID value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
			}

		case *zapcore.Level:
			var l zapcore.Level
			if o.Default != nil {
				l = o.Default.(zapcore.Level)
			}
			if hasShort {
				LevelVarP(flagset, destP, o.Flag, string(o.Short), l, o.Desc)
			} else {
				LevelVar(flagset, destP, o.Flag, l, o.Desc)
			}
			if envVal != nil {
				s, err := cast.ToStringE(envVal)
				if err == nil {
					err = (*destP).Set(s)
				}
				if err != nil {
					return fmt.Errorf("invalid log-level value %q found in config or env for option %q: %w", envVal, o.Flag, err)
				}
			}

		default:
			// if you get a panic here, sorry about that!
			// anyway, go ahead and make a PR and add another type.
			return fmt.Errorf("unknown destination type %t", o.DestP)
		}

		if o.Persistent {
			if err := v.BindPFlag(o.Flag, flagset.Lookup(o.Flag)); err != nil {
				return err
			}
		}

		// N.B. these "Mark" calls must run after the block above,
		// otherwise cobra will return a "no such flag" error.

		// Cobra will complain if a flag marked as required isn't present
		// on the CLI or in config. To support setting required args via env
		// variables, we only enforce the required check if we didn't find
		// a value in the env.
		if o.Required && envVal == nil {
			if err := cmd.MarkFlagRequired(o.Flag); err != nil {
				return err
			}
		}
		if o.Hidden {
			if err := flagset.MarkHidden(o.Flag); err != nil {
				return err
			}
		}
	}

	return nil
}

// lookupEnv returns the value for a CLI option found in the environment, if any.
func lookupEnv(v *viper.Viper, o *Opt) interface{} {
	envVar := o.Flag
	if o.EnvVar != "" {
		envVar = o.EnvVar
	}
	return v.Get(envVar)
}
