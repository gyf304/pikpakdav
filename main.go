package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gyf304/pikpakdav/client"
	"golang.org/x/net/webdav"
)

var (
	port = 8080
)

func init() {
	portEnv := os.Getenv("PORT")
	if portEnv != "" {
		newPort, _ := strconv.Atoi(portEnv)
		if newPort > 0 && newPort < 65536 {
			port = newPort
		}
	}
}

func main() {
	var err error
	client := client.Client{
		StateFile:  "state.json",
		ConfigFile: "config.json",
	}
	err = client.LoadConfig()
	if err != nil {
		log.Println("Failed to load config, recreating it")
		client.SaveConfig()
	}
	err = client.LoadState()
	if err != nil {
		log.Println("Failed to load state, recreating it")
		client.SaveState()
	}
	if client.Config.Username == "" || client.Config.Password == "" {
		log.Panicln("Please set username and password in config.json")
	}
	client.Initialize()
	err = client.SignIn()
	if err != nil {
		log.Panicln("Failed to authenticate", err)
	}
	log.Println("Authenticated as", client.Config.Username)
	fs, err := client.FileSystem()
	// fs := webdav.NewMemFS()
	davHandler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
	log.Printf("Listening on port %d\n", port)
	http.Handle("/", davHandler)
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
