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
	"time"

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
	Permissions     []string `json:"permissions"`
	Command         string   `json:"command"`
	Output          bool     `json:"output"`
	Executable      string   `json:"executable"`
	Arguments       []string `json:"args"`
	ReloadConfig    bool     `json:"reloadConfig"`
	CaseInsensitive bool     `json:"case-insensitive"`
	Timeout         int      `json:"timeout"`
}

type client struct {
	internal *twitch.Client
	timeouts map[string]time.Time
}

func newClient(internal *twitch.Client) *client {
	result := &client{}
	result.internal = internal
	result.timeouts = map[string]time.Time{}

	return result
}

func loadConfig(configFile string) (result configInfo, err error) {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Print("Error reading config file.")
		return
	}

	if err = json.Unmarshal(configData, &result); err != nil {
		log.Print("Error parsing config file.")
	}

	return
}

func main() {
	var authFile string
	var configFile string

	flag.StringVar(&authFile, "auth", "", "authentication json file")
	flag.StringVar(&configFile, "config", "", "config file")

	flag.Parse()

	if len(authFile) == 0 {
		log.Println("Authentication file is necessary")
		os.Exit(-1)
	}

	if len(configFile) == 0 {
		log.Println("Config file is necessary")
		os.Exit(-1)
	}

	var auth authInfo

	authData, err := ioutil.ReadFile(authFile)
	if err != nil {
		log.Println("Error reading authentication file.")
		log.Println(err)
		os.Exit(-2)
	}

	if err = json.Unmarshal(authData, &auth); err != nil {
		log.Println("Error parsing authentication file")
		log.Println(err)
		os.Exit(-3)
	}

	startBot(auth, configFile)
}

func findCommand(commands []command, message string) (*command, []string) {
	parts := strings.Split(message, " ")

	if len(parts) == 0 {
		return nil, []string{}
	}

	for _, current := range commands {
		if current.Command == parts[0] ||
			(current.CaseInsensitive && strings.ToLower(current.Command) == strings.ToLower(parts[0])) {
			return &current, parts[1:]
		}
	}

	return nil, []string{}
}

func hasPermission(comm command, name string) bool {
	if len(comm.Permissions) == 0 {
		return true
	}

	lowercase := strings.ToLower(name)

	for _, current := range comm.Permissions {
		if current == lowercase {
			return true
		}
	}

	return false
}

func listenCommand(client *client, config configInfo, reload chan bool, exit chan bool) func(twitch.PrivateMessage) {
	return func(message twitch.PrivateMessage) {
		command, rest := findCommand(config.Commands, message.Message)
		if command == nil {
			return
		}

		if !hasPermission(*command, message.User.Name) {
			return
		}

		if command.ReloadConfig {
			reload <- true
			return
		}

		if client.IsInTimeout(*command) {
			return
		}

		client.UpdateTimeout(*command)

		args := []string{}
		for _, current := range command.Arguments {
			if current == "$name" {
				args = append(args, message.User.Name)
			} else if current == "$message" {
				args = append(args, strings.Join(rest, " "))
			} else {
				args = append(args, current)
			}
		}

		cmd := exec.Command(command.Executable, args...)
		var buffer bytes.Buffer
		cmd.Env = os.Environ()
		cmd.Stdout = &buffer

		if err := cmd.Run(); err != nil {
			log.Printf("Error while running command(%s): %s", command.Command, err)
			return
		}

		if !command.Output {
			return
		}

		output := buffer.String()
		client.Say(message.Channel, fmt.Sprintf("%s", output))
	}
}

func startBot(auth authInfo, configFile string) {
	for {
		internal := twitch.NewClient(auth.Username, auth.Password)
		client := newClient(internal)
		reloadChan := make(chan bool)
		exitChan := make(chan bool)

		config, err := loadConfig(configFile)
		if err != nil {
			log.Print(err)
			os.Exit(-3)
		}

		for _, channel := range config.Channels {
			internal.Join(channel)
		}

		internal.OnPrivateMessage(listenCommand(client, config, reloadChan, exitChan))

		go func() {
			if err := internal.Connect(); err != nil {
				log.Printf("Error on connect: %s", err)
			}
		}()

		select {
		case <-reloadChan:
			if err := internal.Disconnect(); err != nil {
				log.Printf("Error while disconnecting. %s", err)
				os.Exit(-5)
			}
			continue
		case <-exitChan:
			internal.Disconnect()
			return
		}
	}
}

func (client *client) Say(channel string, message string) {
	client.internal.Say(channel, message)
}

func (client *client) IsInTimeout(command command) bool {
	if command.Timeout <= 0 {
		return false
	}

	timeout := client.timeouts[command.Command]
	timeout = timeout.Add(time.Duration(command.Timeout) * time.Second)
	return timeout.After(time.Now())
}

func (client *client) UpdateTimeout(command command) {
	client.timeouts[command.Command] = time.Now()
}
