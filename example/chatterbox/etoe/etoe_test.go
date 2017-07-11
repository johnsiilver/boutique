package etoe

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique/example/chatterbox/client"
	"github.com/johnsiilver/boutique/example/chatterbox/server"
	"github.com/johnsiilver/boutique/example/chatterbox/server/state/middleware"
)

func TestClientServer(t *testing.T) {
	// Set our timer to happen faster than normal.
	middleware.CleanTimer = 5 * time.Second

	cb := server.New()

	http.HandleFunc("/", cb.Handler)

	go func() {
		err := http.ListenAndServe("localhost:6024", nil)
		if err != nil {
			glog.Fatal("ListenAndServe: ", err)
		}
	}()

	cli1, err := startClient("john")
	if err != nil {
		t.Fatal(err)
	}

	cli2, err := startClient("beck")
	if err != nil {
		t.Fatal(err)
	}

	waitingFor := []string{}
	for i := 0; i < 1000; i++ {
		i := i
		var cli *client.ChatterBox
		if i%2 == 0 {
			cli = cli1
		} else {
			cli = cli2
		}

		text := fmt.Sprintf("%d", i)
		waitingFor = append(waitingFor, text)
		go func() {
			glog.Infof("sending %s", text)
			if err := cli.SendText(text); err != nil {
				t.Fatal(err)
			}
		}()
	}

	waitFor(cli1, waitingFor)
	waitFor(cli2, waitingFor)
}

func startClient(user string) (*client.ChatterBox, error) {
	var (
		cli *client.ChatterBox
		err error
	)

	start := time.Now()
	for {
		if time.Now().Sub(start) > 10*time.Second {
			return nil, fmt.Errorf("could not reach server in < 10 seconds")
		}

		cli, err = client.New("localhost:6024")
		if err != nil {
			glog.Error(err)
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
	if _, err = cli.Subscribe("empty", user); err != nil {
		return nil, err
	}
	return cli, nil
}

func waitFor(cli *client.ChatterBox, text []string) error {
	waiting := make(map[string]bool, len(text))
	for _, t := range text {
		waiting[t] = true
	}

	for {
		if len(waiting) == 0 {
			return nil
		}
		select {
		case m := <-cli.Messages:
			delete(waiting, strings.TrimSpace(m.Text.Text))
			continue
		case <-time.After(2 * time.Second):
			return fmt.Errorf("timeout waiting for: %+v", waiting)
		}
	}
}
