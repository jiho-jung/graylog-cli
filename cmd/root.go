package cmd

import (
	"fmt"
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
	Short: "graylog cli to browse logs instead of webui",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initConfig()
	},
}

func Execute() {
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
	SearchTUI       SearchTUIConfig          `toml:"SearchTUI"`
}

type GraylogLogin struct {
	Url       string `toml:"url"`
	UserToken string `toml:"user-token"`
}

type SearchTUIConfig struct {
	ExpandedMaxLines                  int                  `toml:"expanded-max-lines"`
	DetailPopupMaxLines               int                  `toml:"detail-popup-max-lines"`
	RowNumberWidth                    int                  `toml:"row-number-width"`
	RowNumberMode                     string               `toml:"row-number-mode"`
	MaxCachedPages                    int                  `toml:"max-cached-pages"`
	PersistExpandedRows               bool                 `toml:"persist-expanded-rows"`
	FollowRefreshPausesOnNonfirstPage bool                 `toml:"follow-refresh-pauses-on-nonfirst-page"`
	MessageWrap                       bool                 `toml:"message-wrap"`
	CellOverflow                      string               `toml:"cell-overflow"`
	MinColumnWidth                    int                  `toml:"min-column-width"`
	ColumnResizeStep                  int                  `toml:"column-resize-step"`
	PageScrollStep                    int                  `toml:"page-scroll-step"`
	DetailPopupWidthRatio             float64              `toml:"detail-popup-width-ratio"`
	DetailPopupPosition               string               `toml:"detail-popup-position"`
	DetailShowEmptyFields             bool                 `toml:"detail-show-empty-fields"`
	DetailFields                      []string             `toml:"detail-fields"`
	FilterCandidateFields             []string             `toml:"filter-candidate-fields"`
	FilterCandidateLimit              int                  `toml:"filter-candidate-limit"`
	MessagePatternCandidateLimit      int                  `toml:"message-pattern-candidate-limit"`
	FilterMatchMode                   string               `toml:"filter-match-mode"`
	FilterCaseSensitive               bool                 `toml:"filter-case-sensitive"`
	HeaderVisible                     bool                 `toml:"header-visible"`
	FooterVisible                     bool                 `toml:"footer-visible"`
	HeaderBoxGap                      int                  `toml:"header-box-gap"`
	FooterBoxGap                      int                  `toml:"footer-box-gap"`
	BoxOverflow                       string               `toml:"box-overflow"`
	Columns                           []SearchTUIColumn    `toml:"columns"`
	HeaderBoxes                       []SearchTUIOutputBox `toml:"header-boxes"`
	FooterBoxes                       []SearchTUIOutputBox `toml:"footer-boxes"`
}

type SearchTUIColumn struct {
	Field string `toml:"field"`
	Width int    `toml:"width"`
}

type SearchTUIOutputBox struct {
	Name  string `toml:"name"`
	Width int    `toml:"width"`
}

func getGraylogConfig() *GraylogLogin {
	cfg, ok := graylogCliConfig.GraylogEndpoint[Tier]
	if !ok {
		return &GraylogLogin{}
	}

	return cfg
}

func saveSearchTUIColumns(columns []SearchTUIColumn) error {
	if strings.HasPrefix(ConfigFile, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			ConfigFile = filepath.Join(home, ConfigFile[2:])
		}
	}
	if graylogCliConfig.GraylogEndpoint == nil {
		graylogCliConfig.GraylogEndpoint = map[string]*GraylogLogin{}
	}
	graylogCliConfig.SearchTUI.Columns = columns
	if err := os.MkdirAll(filepath.Dir(ConfigFile), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.Create(ConfigFile)
	if err != nil {
		return fmt.Errorf("open config for write: %w", err)
	}
	defer file.Close()
	if err := toml.NewEncoder(file).Encode(graylogCliConfig); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().SortFlags = false

	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", Verbose, "verbose to see more logs")

	rootCmd.PersistentFlags().StringVar(&SearchFrom, "from", SearchFrom, "absolute time from in UTC")
	rootCmd.PersistentFlags().StringVar(&SearchTo, "to", SearchTo, "abolute time to in UTC")
	rootCmd.PersistentFlags().StringVar(&SearchRange, "since", SearchRange, "relative time. example. 1M 1w 1d 8h 30m 30s")

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

	graylogCliConfig.SearchTUI = defaultSearchTUIConfig()
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
