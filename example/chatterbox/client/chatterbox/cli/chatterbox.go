package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/johnsiilver/boutique/example/chatterbox/client"
	"github.com/johnsiilver/boutique/example/chatterbox/client/chatterbox/cli/ui"
)

var (
	comm atomic.Value // string
)

func main() {
	flag.Parse()
	errors := []error{}

	comm.Store("")

	defer func() {
		for _, err := range errors {
			fmt.Println(err)
		}
		if len(errors) > 0 {
			os.Exit(1)
		}
	}()

	if len(os.Args) != 2 {
		fmt.Println("usage error: chatterbox <server:port> <user name> <channel>")
		fmt.Println("got: ", strings.Join(os.Args, " "))
		return
	}

	server := os.Args[1]

	// Connection to server setup.
	c, err := client.New(server)
	if err != nil {
		errors = append(errors, fmt.Errorf("error connecting to server: %s", err))
		return
	}

	wg := &sync.WaitGroup{}
	stop := make(chan struct{})

	// Create our UI.
	term, display, input := ui.New(stop)
	if err := term.Start(); err != nil {
		errors = append(errors, fmt.Errorf("Error: problem initing the terminal UI: %s", err))
		return
	}
	defer term.Stop()

	wg.Add(1)
	go inputRouter(c, display, input, stop)
	go fromServer(c, wg, display, stop)

	// We will wait until the ui.Input() gets a /quit.
	wg.Wait()
}

func subscribe(c *client.ChatterBox, channel string, user string, displays ui.Displays) error {
	if comm.Load().(string) != "" {
		if err := c.Drop(); err != nil {
			return fmt.Errorf("could not drop from existing channel %s", comm.Load().(string))
		}
	}
	// Must subscribe before doing anything else.
	users, err := c.Subscribe(channel, user)
	if err != nil {
		return fmt.Errorf("could not subscribe to channel %s: %s", channel, err)
	}
	comm.Store(channel)
	comm.Store(user)

	displays.Users <- users
	return nil
}

func inputRouter(c *client.ChatterBox, displays ui.Displays, inputs ui.Inputs, stop chan struct{}) {
	defer close(stop)
	for {
		select {
		case msg := <-inputs.Msgs:
			if comm.Load().(string) == "" {
				displays.Msgs <- fmt.Sprintf("not subscribed to channel")
			} else if err := c.SendText(msg); err != nil {
				// TODO(johnsiilver): This isn't going to work, because the UI is going to die.
				// Add a channel for after UI messages.  Or maybe don't kill the UI?
				displays.Msgs <- fmt.Sprintf("Error: %s, lost connection to server. Please quit", err)
				return
			}
		case cmd := <-inputs.Cmds:
			switch cmd.Name {
			case "quit":
				displays.Msgs <- fmt.Sprintf("quiting...")
				return
			case "subscribe":
				displays.Msgs <- fmt.Sprintf("setting comm to %s, user to %s", cmd.Args[0], cmd.Args[1]) // debug, remove
				if err := subscribe(c, cmd.Args[0], cmd.Args[1], displays); err != nil {
					displays.Msgs <- fmt.Sprintf("Subscribe error: %s", err)
					return
				}
				cmd.Resp <- fmt.Sprintf("Subscribed to channel: %s as user %s", cmd.Args[0], cmd.Args[1])
			}
		}
	}
}

func fromServer(c *client.ChatterBox, wg *sync.WaitGroup, displays ui.Displays, stop <-chan struct{}) {
	defer wg.Done()

	for {
		select {
		case <-stop:
			return
		case m := <-c.Messages:
			displays.Msgs <- fmt.Sprintf("%s: %s", m.User, strings.TrimSpace(m.Text.Text))
		case e := <-c.ServerErrors:
			displays.Msgs <- fmt.Sprintf("Server Error: %s", e)
		case u := <-c.UserUpdates:
			displays.Users <- u
		}
	}
}
