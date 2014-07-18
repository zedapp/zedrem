package main

import (
	"code.google.com/p/gcfg"
	"os"
	"fmt"
)

type Config struct {
    Client struct {
        Url string
        UserKey string
    }

    Server struct {
        Ip string
        Port int
        Sslcert string
        Sslkey string
    }
}

func ParseConfig() Config {
    var config Config
    config.Client.Url = "wss://server.zedapp.org:443"
    config.Server.Ip = "0.0.0.0"
    config.Server.Port = 7337

    configFile := os.ExpandEnv("$HOME/.zedremrc")
    if _, err := os.Stat(configFile); err == nil {
        err = gcfg.ReadFileInto(&config, configFile)
        if err != nil {
            fmt.Println("Could not read config file ~/.zedremrc", err);
            os.Exit(4)
        }
    }

    return config
}
