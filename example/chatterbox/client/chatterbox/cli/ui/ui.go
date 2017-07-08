package ui

import (
	"fmt"
	"regexp"
	"strings"

	ui "github.com/gizak/termui"
)

// Command represents a command sent by the user, which is proceeded with "/".
type Command struct {
	// Name is the name of the command. If the user typed /quit, it would be quit.
	Name string
	// Args is arguments to the command.
	Args []string
	// Resp is the response from trying the command.
	Resp chan string
}

// NewCommand is the constuctor for Command.
func NewCommand(name string, args []string) Command {
	return Command{
		Name: name,
		Args: args,
		Resp: make(chan string, 1),
	}
}

// Displays contains channels that update the display.
type Displays struct {
	Users chan []string
	Input chan string
	Msgs  chan string
}

// Inputs holds channels that may communicate with various subsystems.
type Inputs struct {
	// Msgs are used to send messages to the server.
	Msgs chan string
	// Cmds are used to tell other client methods to do something, like quit
	// or subscribe to a channel.
	Cmds chan Command
}

// Terminal represents the terminal UI.
type Terminal struct {
	display   Displays
	input     Inputs
	stop      chan struct{}
	reRenders map[int]chan struct{}
}

const (
	usersRender = iota
	msgsRender
	inputRender
)

// New returns the Terminal and sets of channels for sending or receiving
// to the display or subysystems.
func New(stop chan struct{}) (*Terminal, Displays, Inputs) {
	t := &Terminal{
		stop: stop,
		reRenders: map[int]chan struct{}{
			usersRender: make(chan struct{}, 1),
			msgsRender:  make(chan struct{}, 1),
			inputRender: make(chan struct{}, 1),
		},
		display: Displays{
			Users: make(chan []string, 1),
			Input: make(chan string, 1),
			Msgs:  make(chan string, 1),
		},
		input: Inputs{
			Msgs: make(chan string, 1),
			Cmds: make(chan Command, 1),
		},
	}
	return t, t.display, t.input
}

// Start is a non-blocking call that starts all the displays.
func (t *Terminal) Start() error {
	err := ui.Init()
	if err != nil {
		return err
	}

	go t.UsersDisplay()
	go t.InputDisplay()
	go t.MessagesDisplay()
	go t.Input()
	return nil
}

// Stop stops our terminal UI.
func (t *Terminal) Stop() {
	ui.StopLoop()
	ui.Close()
}

// UsersDisplay provides the UI part for the usering listing in the comm channel.
func (t *Terminal) UsersDisplay() {
	ls := ui.NewList()

	ls.ItemFgColor = ui.ColorYellow
	ls.BorderLabel = "Users"
	ls.Height = ui.TermHeight()
	ls.Width = 25
	ls.Y = 0
	ls.X = ui.TermWidth() - 25

	go func() {
		ui.Render(ls)
		for {
			select {
			case <-t.stop:
				return
			case <-t.reRenders[usersRender]:
				ls.X = ui.TermWidth() - 25
				ls.Height = ui.TermHeight()
				ls.Width = 25
			case u := <-t.display.Users:
				ls.Items = u
			}
			ui.Render(ls)
		}
	}()
}

// InputDisplay provides the UI part for the text Input from each user.
func (t *Terminal) InputDisplay() {
	par := ui.NewPar("")
	par.BorderLabel = "Input"
	par.Height = 10
	par.Width = ui.TermWidth() - 25
	par.WrapLength = ui.TermWidth() - 25
	par.Y = ui.TermHeight() - 10
	par.X = 0
	par.Text = ">"

	go func() {
		for {
			ui.Render(par)
			select {
			case <-t.stop:
				return
			case <-t.reRenders[inputRender]:
				par.Y = ui.TermHeight() - 10
				par.Height = 10
				par.Width = ui.TermWidth() - 25
				par.WrapLength = ui.TermWidth() - 25
			case l := <-t.display.Input:
				par.Text = ">" + l
			}
		}
	}()
}

// MessagesDisplay provides the UI part for the Messages from each user.
func (t *Terminal) MessagesDisplay() {
	data := make([]string, 0, 300)

	par := ui.NewPar("Welcome to ChatterBox")
	par.BorderLabel = "Messages"
	par.Height = ui.TermHeight() - 10
	par.Width = ui.TermWidth() - 25
	par.WrapLength = ui.TermWidth() - 25
	par.Y = 0
	par.X = 0

	go func() {
		for {
			ui.Render(par)
			select {
			case <-t.stop:
				return
			case <-t.reRenders[msgsRender]:
				par.Height = ui.TermHeight() - 10
				par.Width = ui.TermWidth() - 25
				par.WrapLength = ui.TermWidth() - 25
				if len(data) > 0 {
					start := 0
					if par.Height-3 < len(data) { // I hate magic, but 3 seems to be a magic number for the term.
						start = len(data) - par.Height + 3
					}
					par.Text = strings.Join(data[start:], "\n")
				}
			case l := <-t.display.Msgs:
				data = append(data, l)
				// Trim down if we have more than 300 lines.
				if len(data) >= 300 {
					newData := make([]string, 150, 300)
					copy(newData, data[150:])
					data = newData
				}
				if len(data) > 0 {
					start := 0
					if par.Height-3 < len(data) { // I hate magic, but 3 seems to be a magic number for the term.
						start = len(data) - par.Height + 3
					}
					par.Text = strings.Join(data[start:], "\n")
				}
			}
		}
	}()

}

var subscribeRE = regexp.MustCompile(`^subscribe\s+(.+)\s(.*)`)

// Input handles all input from the mouse and keyboard.
func (t *Terminal) Input() {
	content := make([]string, 0, 500)

	//////////////////////////////////
	// Register input handlers
	//////////////////////////////////

	// handle key backspace
	ui.Handle("/sys/kbd/C-8", func(e ui.Event) {
		if len(content) == 0 {
			return
		}
		content = content[0 : len(content)-1]
		t.display.Input <- strings.Join(content, "")
	})

	ui.Handle("/sys/kbd/C-x", func(ui.Event) {
		// handle Ctrl + x combination
		ui.StopLoop()
		return
	})

	ui.Handle("/sys/kbd/C-c", func(ui.Event) {
		ui.StopLoop()
		return
	})

	ui.Handle("/sys/kbd/<enter>", func(e ui.Event) {
		switch {
		case len(content) == 0:
			// Do nothing
		case content[0] == "/":
			if len(content) == 1 {
				t.display.Msgs <- "'/' is not a valid command"
				return
			}

			v := strings.ToLower(strings.Join(content[1:], ""))
			switch {
			case v == "quit":
				cmd := NewCommand("quit", nil)
				t.input.Cmds <- cmd
				<-cmd.Resp
				ui.StopLoop()
			case strings.HasPrefix(v, `subscribe`):
				m := subscribeRE.FindStringSubmatch(v)
				if len(m) != 3 {
					t.display.Msgs <- fmt.Sprintf("/subscribe command incorrect syntax. Expected '/subscribe <channel> <user>'")
					return
				}
				cmd := NewCommand("subscribe", []string{m[1], m[2]})
				t.input.Cmds <- cmd
				t.display.Msgs <- <-cmd.Resp
				content = content[0:0]
				t.display.Input <- ""
			default:
				t.display.Msgs <- fmt.Sprintf("unsupported command: /%s", v)
			}
		default:
			t.input.Msgs <- strings.Join(content, "")
			content = content[0:0]
			t.display.Input <- ""
		}
	})

	// handle a space
	ui.Handle("/sys/kbd/<space>", func(e ui.Event) {
		if len(content)+1 > 500 {
			t.display.Msgs <- fmt.Sprintf("cannot send more than 500 characters")
			return
		}
		content = append(content, " ")
		t.display.Input <- strings.Join(content, "")
	})

	// handle all other key pressing
	ui.Handle("/sys/kbd", func(e ui.Event) {
		s := e.Data.(ui.EvtKbd).KeyStr
		if len(content)+1 > 500 {
			t.display.Msgs <- fmt.Sprintf("cannot send more than 500 characters")
			return
		}
		content = append(content, s)
		t.display.Input <- strings.Join(content, "")
	})

	// tell windows to redraw if the size has changed.
	ui.Handle("/sys/wnd/resize", func(ui.Event) {
		for _, ch := range t.reRenders {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	})

	//////////////////////////////////
	// Loop our ui until ui.StopLoop() to <-t.stop
	//////////////////////////////////
	go func() {
		defer ui.Close()
		ui.Loop()
	}()
}
