package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/gizak/termui"
	"github.com/golang/glog"
	"github.com/johnsiilver/boutique/example/chatterbox/client"
	"github.com/johnsiilver/boutique/example/chatterbox/client/chatterbox/cli/ui"
)

func main() {
	errors := []error{}

	err := termui.Init()
	if err != nil {
		panic(err)
	}
	defer func() {
		for _, err := range errors {
			fmt.Println(err)
		}
		if len(errors) > 0 {
			os.Exit(1)
		}
	}()

	defer termui.Close()
	defer termui.StopLoop()

	if len(os.Args) != 4 {
		fmt.Println("usage error: chatterbox <server:port> <user name> <channel>")
		fmt.Println("got: ", strings.Join(os.Args, " "))
		return
	}

	server := os.Args[1]
	user := os.Args[2]
	comm := os.Args[3]

	// Communications setup.
	usersCh := make(chan []string, 1)
	inputCh := make(chan string, 1)
	msgsCh := make(chan string, 1)
	msgSendCh := make(chan string, 1)

	// Connection to server setup.
	c, err := client.New(server, user)
	if err != nil {
		errors = append(errors, fmt.Errorf("error connecting to server: %s", err))
		return
	}

	if err := subscribe(c, comm, usersCh); err != nil {
		errors = append(errors, fmt.Errorf("error connecting to channel: %s", err))
		return
	}
	msgsCh <- fmt.Sprintf("Subscribed: %s", comm)

	wg := &sync.WaitGroup{}
	stop := make(chan struct{})

	go toServer(c, msgSendCh, stop)
	wg.Add(1)
	go fromServer(c, user, wg, msgsCh, usersCh, stop)

	// UI setup.
	reRender := []chan struct{}{
		make(chan struct{}, 1),
		make(chan struct{}, 1),
		make(chan struct{}, 1),
	}

	ui.UsersDisplay(usersCh, reRender[0])
	ui.InputDisplay(inputCh, reRender[1])
	ui.MessagesDisplay(msgsCh, reRender[2])
	wg.Add(1)
	ui.Input(
		wg,
		ui.InputArgs{
			ReRender:   reRender,
			Stop:       stop,
			InputLine:  inputCh,
			MsgSend:    msgSendCh,
			MsgDisplay: msgsCh,
		},
	)

	// We will wait until the ui.Input() gets a /quit.
	wg.Wait()
}

func subscribe(c *client.ChatterBox, comm string, userDisplay chan []string) error {
	// Must subscribe before doing anything else.
	users, err := c.Subscribe(comm)
	if err != nil {
		return err
	}

	userDisplay <- users
	return nil
}

func toServer(c *client.ChatterBox, msgSendCh chan string, stop chan struct{}) {
	defer close(stop)
	for msg := range msgSendCh {
		if err := c.SendText(msg); err != nil {
			glog.Errorf("Error: %s, lost connection to server. Please quit\n", err)
			return
		}
	}
}

func fromServer(c *client.ChatterBox, user string, wg *sync.WaitGroup, msgDisplay chan string, userDisplay chan []string, stop <-chan struct{}) {
	defer wg.Done()

	for {
		select {
		case <-stop:
			return
		case m := <-c.Messages:
			msgDisplay <- fmt.Sprintf("%s: %s", m.User, strings.TrimSpace(m.Text.Text))
		case e := <-c.ServerErrors:
			msgDisplay <- fmt.Sprintf("Server Error: %s", e)
		case u := <-c.UserUpdates:
			userDisplay <- u
		}
	}
}

/*
ERROR: logging before flag.Parse: E0704 12:37:12.450426   25685 client.go:154] problem reading message from server, killing the client connection: websocket: close 1006 (abnormal closure): unexpected EOF                                                                   ││                       │
ERROR: logging before flag.Parse: E0704 12:37:12.450481   25685 client.go:121] client error receiving from server, client is dead: websocket: close 1006 (abnormal closure): unexpected EOF
*/
