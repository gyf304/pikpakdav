package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gyf304/pikpakdav/client"
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
	client := &client.Client{
		ConfigFile: os.Getenv("CONFIG_FILE"),
		StateFile:  os.Getenv("STATE_FILE"),
	}
	client.Config.User.Username = os.Getenv("USERNAME")
	client.Config.User.Password = os.Getenv("PASSWORD")

	err := client.LoadConfig()
	if err != nil {
		log.Println("Failed to load config, recreating it")
		client.SaveConfig()
	}
	err = client.LoadState()
	if err != nil {
		log.Println("Failed to load state, recreating it")
		client.SaveState()
	}

	if client.Config.User.Username == "" || client.Config.User.Password == "" {
		client.SaveConfig()
		log.Panicln("Please set username and password in config.json")
	}
	user, err := client.User()
	if err != nil {
		log.Panicln("Failed to get user client", err)
	}
	err = user.SignIn()
	if err != nil {
		log.Panicln("Failed to authenticate", err)
	}
	log.Println("Authenticated as", client.Config.User.Username)

	drive, err := client.Drive()
	if err != nil {
		log.Panicln("Failed to get drive client", err)
	}

	// fs := webdav.NewMemFS()
	davHandler, err := drive.NewWebDAVHandler()
	if err != nil {
		log.Panicln("Failed to create WebDAV handler", err)
	}
	log.Printf("Listening on port %d\n", port)
	http.Handle("/", davHandler)
	panic(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
