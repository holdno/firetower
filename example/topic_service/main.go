package main

import (
	"fmt"

	"github.com/holdno/firetower/service/manager"

	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ConfigTree 配置信息
var ConfigTree *toml.Tree

type Options struct {
	ConfigPath string
}

func (o *Options) AddFlags(flagSet *pflag.FlagSet) {
	flagSet.StringVarP(&o.ConfigPath, "config", "c", "./topicmanage.toml", "config path")
}

func main() {
	var opts Options
	cmd := &cobra.Command{
		Use:   "topic",
		Short: "订阅管理服务",
		RunE: func(c *cobra.Command, args []string) error {
			var (
				err error
			)
			if ConfigTree, err = toml.LoadFile(opts.ConfigPath); err != nil {
				fmt.Println("config load failed:", err)
			}

			m := &manager.Manager{}
			go m.StartGrpcService(fmt.Sprintf(":%d", ConfigTree.Get("grpc.port").(int64)))
			m.StartSocketService(fmt.Sprintf("0.0.0.0:%d", ConfigTree.Get("socket.port").(int64)))
			return nil
		},
	}
	opts.AddFlags(cmd.Flags())
	if err := cmd.Execute(); err != nil {
		fmt.Printf("failed to run command, %s\n", err.Error())
	}
}
