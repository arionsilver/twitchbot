package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/gempir/go-twitch-irc"
)

type authInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type configInfo struct {
	Channels []string  `json:"channels"`
	Commands []command `json:"commands"`
}

type command struct {
	Permissions []string `json:"permissions"`
	Command     string   `json:"command"`
	Output      bool     `json:"output"`
	Executable  string   `json:"executable"`
	Arguments   []string `json:"args"`
}

func main() {
	var authFile string
	var configFile string

	flag.StringVar(&authFile, "auth", "", "authentication json file")
	flag.StringVar(&configFile, "config", "", "config file")

	flag.Parse()

	if len(authFile) == 0 {
		fmt.Println("Authentication file is necessary")
		os.Exit(-1)
	}

	if len(configFile) == 0 {
		fmt.Println("Config file is necessary")
		os.Exit(-1)
	}

	var auth authInfo
	var config configInfo

	authData, err := ioutil.ReadFile(authFile)
	if err != nil {
		fmt.Println("Error reading authentication file.")
		fmt.Println(err)
		os.Exit(-2)
	}

	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Println("Error reading config file.")
		fmt.Println(err)
		os.Exit(-2)
	}

	if err = json.Unmarshal(authData, &auth); err != nil {
		fmt.Println("Error parsing authentication file")
		fmt.Println(err)
		os.Exit(-3)
	}

	if err = json.Unmarshal(configData, &config); err != nil {
		fmt.Println("Error parsing config file")
		fmt.Println(err)
		os.Exit(-3)
	}

	startBot(auth, config)
}

func startBot(auth authInfo, config configInfo) {
	client := twitch.NewClient(auth.Username, auth.Password)

	for _, channel := range config.Channels {
		client.Join(channel)
	}

	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		for _, command := range config.Commands {
			if command.Command != message.Message {
				continue
			}

			username := strings.ToLower(message.User.Name)
			found := false
			for _, permitted := range command.Permissions {
				if strings.ToLower(permitted) == username {
					found = true
					break
				}
			}

			if !found {
				break
			}

			cmd := exec.Command(command.Executable, command.Arguments...)
			var buffer bytes.Buffer
			cmd.Stdout = &buffer

			if err := cmd.Run(); err != nil {
				log.Printf("Error while running command(%s): %s", command.Command, err)
				break
			}

			if !command.Output {
				break
			}

			output := buffer.String()
			client.Say(message.Channel, fmt.Sprintf("%s", output))
		}
	})

	if err := client.Connect(); err != nil {
		fmt.Println("Error connecting to Twitch")
		fmt.Println(err)
		os.Exit(-4)
	}
}
