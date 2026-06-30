package main

import (
	"flag"
	"fmt"
	"os"

	"gatewaykit/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to gateway YAML config")
	flag.Parse()

	if *configPath == "" {
		*configPath = os.Getenv("GATEWAY_CONFIG")
	}

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing config path: pass --config or set GATEWAY_CONFIG")
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "GatewayKit config loaded: port=%d routes=%d\n", cfg.Gateway.Port, len(cfg.Gateway.Routes))
}
