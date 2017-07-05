package ui

import (
	"fmt"
	"strings"
	"sync"

	ui "github.com/gizak/termui"
)

// UsersDisplay provides the UI part for the usering listing in the comm channel.
// "users" is where we receive new user list and "reRender" signals us to
// redraw the screen.
func UsersDisplay(users chan []string, reRender chan struct{}) {
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
			case <-reRender:
				ls.X = ui.TermWidth() - 25
				ls.Height = ui.TermHeight()
				ls.Width = 25
			case u := <-users:
				ls.Items = u
			}
			ui.Render(ls)
		}
	}()
}

// InputDisplay provides the UI part for the text Input from each user.
// "lines" is where we receive new input and "reRender" signals us to
// redraw the screen.
func InputDisplay(line chan string, reRender chan struct{}) {
	par := ui.NewPar("")
	par.BorderLabel = "Input"
	par.Height = 10
	par.Width = ui.TermWidth() - 25
	par.WrapLength = ui.TermWidth() - 25
	par.Y = ui.TermHeight() - 10
	par.X = 0
	par.Text = ">"

	go func() {
		ui.Render(par)
		for {
			select {
			case <-reRender:
				par.Y = ui.TermHeight() - 10
				par.Height = 10
				par.Width = ui.TermWidth() - 25
				par.WrapLength = ui.TermWidth() - 25
			case l := <-line:
				par.Text = ">" + l
			}
			ui.Render(par)
		}
	}()
}

// MessagesDisplay provides the UI part for the Messages from each user.
// "lines" is where we receive new input and "reRender" signals us to
// redraw the screen.
func MessagesDisplay(lines chan string, reRender chan struct{}) {
	data := make([]string, 0, 300)

	par := ui.NewPar("")
	par.BorderLabel = "Messages"
	par.Height = ui.TermHeight() - 10
	par.Width = ui.TermWidth() - 25
	par.WrapLength = ui.TermWidth() - 25
	par.Y = 0
	par.X = 0

	go func() {
		ui.Render(par)
		for {
			select {
			case <-reRender:
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
			case l := <-lines:
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
			ui.Render(par)
		}
	}()

}

// InputArgs are arguements to Input().
type InputArgs struct {
	// ReRender is a list of channels that are signalled whenver the screen
	// size is updated.
	ReRender []chan struct{}

	// Stop is a channel that is closed when the user wants to quit.
	Stop chan struct{}

	// InputLine is a chan we use to update what is displayed in InputDisplay.
	InputLine chan<- string

	// MsgSend is a message to send to the server.
	MsgSend chan<- string

	// MsgDisplay is used to send messages to the display.
	MsgDisplay chan<- string
}

// Input handles all input from the mouse and keyboard.
func Input(wg *sync.WaitGroup, args InputArgs) {
	content := make([]string, 0, 500)

	go func() {
		defer wg.Done()

		// handle key backspace
		ui.Handle("/sys/kbd/C-8", func(e ui.Event) {
			if len(content) == 0 {
				return
			}
			content = content[0 : len(content)-1]
			args.InputLine <- strings.Join(content, "")
		})

		ui.Handle("/sys/kbd/C-x", func(ui.Event) {
			// handle Ctrl + x combination
			close(args.Stop)
			ui.StopLoop()
			ui.Close()
			return
		})

		ui.Handle("/sys/kbd/C-c", func(ui.Event) {
			close(args.Stop)
			ui.StopLoop()
			ui.Close()
			return
		})

		ui.Handle("/sys/kbd/<enter>", func(e ui.Event) {
			switch {
			case len(content) == 0:
				// Do nothing
			case content[0] == "/":
				if len(content) == 1 {
					args.MsgDisplay <- "'/' is not a valid command"
					return
				}
				switch v := strings.ToLower(strings.Join(content[1:], "")); v {
				case "quit":
					close(args.Stop)
					ui.StopLoop()
				default:
					args.MsgDisplay <- fmt.Sprintf("unsupported command: /%s", v)
				}
			default:
				send := strings.Join(content, "")
				args.MsgSend <- send
				content = content[0:0]
				args.InputLine <- ""
			}
		})

		// handle a space
		ui.Handle("/sys/kbd/<space>", func(e ui.Event) {
			if len(content)+1 > 500 {
				args.MsgDisplay <- fmt.Sprintf("cannot send more than 500 characters")
				return
			}
			content = append(content, " ")
			args.InputLine <- strings.Join(content, "")
		})

		// handle all other key pressing
		ui.Handle("/sys/kbd", func(e ui.Event) {
			s := e.Data.(ui.EvtKbd).KeyStr

			if len(content)+1 > 500 {
				args.MsgDisplay <- fmt.Sprintf("cannot send more than 500 characters")
				return
			}
			content = append(content, s)
			args.InputLine <- strings.Join(content, "")
		})

		// tell windows to redraw if the size has changed.
		ui.Handle("/sys/wnd/resize", func(ui.Event) {
			for _, ch := range args.ReRender {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		})

		ui.Loop()
	}()
}
