package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "graylog-cli",
	Short: "graylog cli to browse logs instead of webui",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initConfig()
	},
}

func flagsContain(flags []string, contains ...string) bool {
	for _, flag := range contains {
		if slices.Contains(flags, flag) {
			return true
		}
	}
	return false
}

func setDefaultCommandIfNonePresent(defaultCommand string) {
	// Taken from cobra source code in command.go::ExecuteC()
	var cmd *cobra.Command
	var err error
	var flags []string
	if rootCmd.TraverseChildren {
		cmd, flags, err = rootCmd.Traverse(os.Args[1:])
	} else {
		cmd, flags, err = rootCmd.Find(os.Args[1:])
	}

	// If no command was on the CLI, then cmd will return with
	// the value of rootCmd.Use (which would run the help output
	// in the full Execute() command)
	if err != nil || cmd.Use == rootCmd.Use {
		if !flagsContain(flags, "-v", "-h", "--version", "--help") {
			rootCmd.SetArgs(append(os.Args[1:], defaultCommand))
		}
	}
}

// let main package set default command ...
func ExecuteWithDefaultCommand(defaultCommand string) {
	if defaultCommand != "" {
		setDefaultCommandIfNonePresent(defaultCommand)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func Execute() {
	setDefaultCommandIfNonePresent("search")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	Query          = "*"
	SearchFrom     = ""
	SearchTo       = ""
	SearchRange    = "8h"
	ServerEndpoint = ""
	Username       = ""
	Password       = ""
	ConfigFile     = "~/.config/graylog.toml"
	Tier           = "dev2" // tier or regions
	Offset         = 0
	Limit          = 100
	Sort           = "timestamp:DESC"
	Pagination     = false
	Verbose        = false

	DecoderConfig = &client.DecoderConfig{
		HostnameKeys: []string{
			"hostname",
			"source",
		},
		TimestampKeys: []string{
			"timestamp",
		},
		LevelKeys: []string{
			"level",
		},
		TextKeys: []string{
			"message",
		},
		SkipFieldKeys: []string{
			"@timestamp",
			"@version",
			"_id",
			"caller",
			"file",
			"function",
			"gl2_accounted_message_size",
			"gl2_message_id",
			"gl2_processing_duration_ms",
			"gl2_processing_timestamp",
			"gl2_receive_timestamp",
			"gl2_remote_ip",
			"gl2_remote_port",
			"gl2_source_input",
			"gl2_source_node",
			"hostname",
			"input",
			"level",
			"line",
			"message",
			"source",
			"streams",
			"timestamp",
		},
		FieldKeys: []string{},
	}
)

var graylogCliConfig GraylogCliConfig

type GraylogCliConfig struct {
	GraylogEndpoint map[string]*GraylogLogin // key: tier(or region), dev2/stg2/ppd2/spc-kr/spc-sg/spc-eu/spc-us
}

type GraylogLogin struct {
	Url       string `toml:"url"`
	UserToken string `toml:"user-token"`
}

func getGraylogConfig() *GraylogLogin {
	cfg, ok := graylogCliConfig.GraylogEndpoint[Tier]
	if !ok {
		return &GraylogLogin{}
	}

	return cfg
}

func init() {
	rootCmd.PersistentFlags().SortFlags = false

	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", Verbose, "verbose to see more logs")

	rootCmd.PersistentFlags().StringVar(&SearchFrom, "from", SearchFrom, "absolute time from in UTC")
	rootCmd.PersistentFlags().StringVar(&SearchTo, "to", SearchTo, "abolute time to in UTC")
	rootCmd.PersistentFlags().StringVar(&SearchRange, "range", SearchRange, "relative time. example. 1M 1w 1d 8h 30m 30s")

	rootCmd.PersistentFlags().StringVar(&ServerEndpoint, "server", ServerEndpoint, "graylog endpoint url")
	rootCmd.PersistentFlags().StringVar(&Username, "username", Username, "")
	rootCmd.PersistentFlags().StringVar(&Password, "password", Password, "")

	rootCmd.PersistentFlags().StringVar(&ConfigFile, "config", ConfigFile, "config file for the endpoint and username/passowrd")
	rootCmd.PersistentFlags().StringVarP(&Tier, "tier", "t", Tier, "tier or region: dev2/stg2/ppd2/spc-kr/spc-sg/spc-eu/spc-us")
}

func initConfig() {
	if strings.HasPrefix(ConfigFile, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			ConfigFile = filepath.Join(home, ConfigFile[2:])
		}
	}

	if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
		// file does not exist
	} else if err != nil {
		// error ?
	} else {
		if _, err := toml.DecodeFile(ConfigFile, &graylogCliConfig); err != nil {
			log.Println(err)
			return
		}
	}
}
