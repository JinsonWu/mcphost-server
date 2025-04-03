package main

import (
	"mcphost-server/cmd"
	"mcphost-server/server"
)

var server_mode bool = true

func main() {
	server.LoadServerEnv()
	if server_mode {
		server.NewServer().Start()
	} else {
		cmd.Execute()
	}
}
