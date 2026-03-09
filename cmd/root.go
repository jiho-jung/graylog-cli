package cmd

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "graylog-cli",
	Short: "A brief description of your application",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initConfig()
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
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

type GraylogCliConfig struct {
	GraylogEndpoint map[string]*GraylogLogin // key: tier(or region), dev2/stg2/ppd2/spc-kr/spc-sg/spc-eu/spc-us
}

type GraylogLogin struct {
	Url       string `toml:"url"`
	UserToken string `toml:"user-token"`
}

var graylogCliConfig GraylogCliConfig

func init() {
	rootCmd.PersistentFlags().SortFlags = false

	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", Verbose, "")

	rootCmd.PersistentFlags().StringVar(&SearchFrom, "from", SearchFrom, "")
	rootCmd.PersistentFlags().StringVar(&SearchTo, "to", SearchTo, "")
	rootCmd.PersistentFlags().StringVar(&SearchRange, "range", SearchRange, "example. 1M 1w 1d 8h 30m 30s")

	rootCmd.PersistentFlags().StringVar(&ServerEndpoint, "server", ServerEndpoint, "")
	rootCmd.PersistentFlags().StringVar(&Username, "username", Username, "")
	rootCmd.PersistentFlags().StringVar(&Password, "password", Password, "")

	rootCmd.PersistentFlags().StringVar(&ConfigFile, "config", ConfigFile, "config file")
	rootCmd.PersistentFlags().StringVar(&Tier, "tier", Tier, "Tier or region: dev2/stg2/ppd2/spc-kr/spc-sg/spc-eu/spc-us")
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
