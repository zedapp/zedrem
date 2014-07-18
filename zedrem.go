package main

import (
	"os"
	"code.google.com/p/go-uuid/uuid"
	"strings"
	"fmt"
)

func main() {
	mode := "client"

	if len(os.Args) > 1 && os.Args[1] == "--server" {
		mode = "server"
	} else if len(os.Args) > 1 && os.Args[1] == "--help" {
		mode = "help"
	}

	switch mode {
	case "server":
		ip, port, sslCrt, sslKey := ParseServerFlags(os.Args[2:])
		RunServer(ip, port, sslCrt, sslKey, false)
	case "client":
		url, userKey := ParseClientFlags(os.Args[1:])
		id := strings.Replace(uuid.New(), "-", "", -1)
		RunClient(url, id, userKey)
	case "help":
		fmt.Println(`zedrem runs in one of two possible modes: client or server:

Usage: zedrem [-u url] <dir>
       Launches a Zed client and attaches to a Zed server exposing
       directory <dir> (or current directory if omitted). Default URL is
       ws://server.zedapp.org

Usage: zedrem --server [-h ip] [-p port] [--sslcrt file.crt] [--sslkey file.key]
       Launches a Zed server, binding to IP <ip> on port <port>.
       If --sslcrt and --sslkey are provided, will run in TLS mode for more security.
`)
	}
}
