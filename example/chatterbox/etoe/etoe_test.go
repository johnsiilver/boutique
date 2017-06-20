package etoe

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique/example/chatterbox/client"
	"github.com/johnsiilver/boutique/example/chatterbox/server"
)

func TestClientServer(t *testing.T) {
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

	wg := &sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		i := i
		var cli *client.ChatterBox
		if i%2 == 0 {
			cli = cli1
		} else {
			cli = cli2
		}

		text := fmt.Sprintf("%d", i)
		go func() {
			glog.Infof("sending %s", text)
			if err := cli.SendText(text); err != nil {
				t.Fatal(err)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := waitFor(cli1, text); err != nil {
				time.Sleep(3 * time.Second)
				t.Fatal(err)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := waitFor(cli2, text); err != nil {
				time.Sleep(3 * time.Second)
				t.Fatal(err)
			}
		}()
	}

	time.Sleep(1 * time.Second)
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

		cli, err = client.New("localhost:6024", user)
		if err != nil {
			glog.Error(err)
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
	if err = cli.Subscribe("empty"); err != nil {
		return nil, err
	}
	return cli, nil
}

func waitFor(cli *client.ChatterBox, text string) error {
	select {
	case m := <-cli.Messages:
		if strings.TrimSpace(m.Text.Text) != text {
			return fmt.Errorf("got %s, want %s", m.Text, text)
		}
		glog.Infof("waited for: %s", text)
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for %s", text)
	}
}
