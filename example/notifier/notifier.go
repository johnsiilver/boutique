package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xAX/notificator"
	"github.com/doneland/yquotes"
	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state"
	"github.com/johnsiilver/boutique/example/notifier/state/actions"
	"github.com/johnsiilver/boutique/example/notifier/state/data"
	"github.com/olekukonko/tablewriter"
)

var (
	reader = bufio.NewReader(os.Stdin)
	store  *boutique.Store
	notify *notificator.Notificator
)

func init() {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		fmt.Printf("Error: your OS %s is not supported\n", runtime.GOOS)
		os.Exit(1)
	}
}

// clear clears the terminal screen.
func clear() {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd := exec.Command("clear") //Linux example, its tested
		cmd.Stdout = os.Stdout
		cmd.Run()
	case "windows":
		cmd := exec.Command("cls") //Windows example it is untested, but I think its working
		cmd.Stdout = os.Stdout
		cmd.Run()

	default:
		panic("clear is trying to clear on an unsupported OS, bad developer, bad!!!!")
	}
}

// readInt reads a single character from the command line, looking for an int.
func readInt() (int, error) {
	text, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	text = strings.Replace(text, "\n", "", -1)

	i, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid integer: %s", text, err)
	}

	return i, nil
}

// readLine reads the next line of input and returns it.
func readLine() (string, error) {
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	text = strings.Replace(text, "\n", "", -1)
	return text, nil
}

// topMenu displays the top level menu.
func topMenu() error {
	clear()

	fmt.Println("Options")
	fmt.Println("----------------------")
	fmt.Println("(1)add stock symbol")
	fmt.Println("(2)remove stock symbol")
	fmt.Println("(3)change buy point")
	fmt.Println("(4)change sell point")
	fmt.Println("(5)list positions")
	fmt.Println("(6)quit")
	fmt.Println("----------------------")
	fmt.Print("> ")

	i, err := readInt()
	if err != nil {
		return err
	}

	switch i {
	case 1:
		return addStockMenu()
	case 2:
		return removeStockMenu()
	case 3:
		return changeBuyMenu()
	case 4:
		return changeSellMenu()
	case 5:
		return listPositions()
	case 6:
		os.Exit(0)
	default:
		return fmt.Errorf("%d was not a valid option", i)
	}
	return nil
}

// addStockMenu shows the add stock menu.
func addStockMenu() error {
	clear()
	fmt.Println("Provide the symbol, buy point, and sell point. Comma separated.")
	fmt.Println("Exmple: googl;940.25;1050.00")
	fmt.Println("----------------------")
	fmt.Print("> ")
	input, err := readLine()
	if err != nil {
		return err
	}

	sl, err := parse(input, 3)
	if err != nil {
		return err
	}

	buy, err := strconv.ParseFloat(sl[1], 64)
	if err != nil {
		return fmt.Errorf("%s is incorrect format, 2nd value should be a float64 representing the buy price: %s", input, err)
	}

	sell, err := strconv.ParseFloat(sl[2], 64)
	if err != nil {
		return fmt.Errorf("%s is incorrect format, 3rd value should be a float64 representing the sell price: %s", input, err)
	}
	sl[0] = strings.ToUpper(sl[0])
	d := data.Stock{Symbol: sl[0], Buy: buy, Sell: sell}

	glog.Infof("adding stock %q", d.Symbol)
	s, err := yquotes.NewStock(d.Symbol, false)
	if err != nil || s.Price.Last == 0 {
		return fmt.Errorf("problem retreiving stock information for %s: %s", d.Symbol, err)
	}

	go checkBuySell(d.Symbol)

	if err := store.Perform(actions.Track(d.Symbol, d.Buy, d.Sell)); err != nil {
		return fmt.Errorf("problem adding stock information for %s: %s", d.Symbol, err)
	}
	fmt.Println("Stock added successfully, Hit return to continue")
	readLine()

	return nil
}

// removeStockMenu shows the remove stock menu.
func removeStockMenu() error {
	clear()
	fmt.Println("Provide the symbol to remove")
	fmt.Println("----------------------")
	fmt.Print("> ")
	input, err := readLine()
	if err != nil {
		return err
	}
	input = strings.ToUpper(input)

	if err := store.Perform(actions.Untrack(input)); err != nil {
		return fmt.Errorf("problem removing stock %s: %s", input, err)
	}

	return nil
}

// changeBuyMenu shows the change buy point menu.
func changeBuyMenu() error {
	clear()
	fmt.Println("Provide the symbol;new buy point")
	fmt.Println("Example: googl;850")
	fmt.Println("----------------------")
	fmt.Print("> ")
	input, err := readLine()
	if err != nil {
		return err
	}

	sl, err := parse(input, 2)
	if err != nil {
		return err
	}
	sl[0] = strings.ToUpper(sl[0])

	buy, err := strconv.ParseFloat(sl[1], 64)
	if err != nil || buy <= 0 {
		return fmt.Errorf("the second value you entered was not a valid price")
	}

	if _, ok := store.State().Data.(data.State).Tracking[sl[0]]; !ok {
		return fmt.Errorf("error: you are not currently tracking stock %s", sl[0])
	}

	glog.Infof("changing stock %q buy to: %v", sl[0], buy)

	if err := store.Perform(actions.ChangeBuy(sl[0], buy), boutique.NoUpdate()); err != nil {
		return fmt.Errorf("problem changing stock information for %s: %s", sl[0], err)
	}
	fmt.Println("Buy changed successfully, Hit return to continue")
	readLine()

	return nil
}

// changeSellMenu shows the change sell point menu.
func changeSellMenu() error {
	clear()
	fmt.Println("Provide the symbol;new sell point")
	fmt.Println("Example: googl;850")
	fmt.Println("----------------------")
	fmt.Print("> ")
	input, err := readLine()
	if err != nil {
		return err
	}

	sl, err := parse(input, 2)
	if err != nil {
		return err
	}
	sl[0] = strings.ToUpper(sl[0])

	sell, err := strconv.ParseFloat(sl[1], 64)
	if err != nil || sell <= 0 {
		return fmt.Errorf("the second value you entered was not a valid price")
	}

	if _, ok := store.State().Data.(data.State).Tracking[sl[0]]; !ok {
		return fmt.Errorf("error: you are not currently tracking stock %s", sl[0])
	}

	glog.Infof("changing stock %q sell to: %v", sl[0], sell)

	if err := store.Perform(actions.ChangeSell(sl[0], sell), boutique.NoUpdate()); err != nil {
		return fmt.Errorf("problem changing stock information for %s: %s", sl[0], err)
	}
	fmt.Println("Sell changed successfully, Hit return to continue")
	readLine()

	return nil
}

// bySymbol is a sorter to sort printed output by stock symbol.
type bySymbol [][]string

func (a bySymbol) Len() int           { return len(a) }
func (a bySymbol) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySymbol) Less(i, j int) bool { return a[i][0] < a[j][0] }

func listPositions() error {
	clear()
	fmt.Println("Current positions being watched:")
	fmt.Println("----------------------")
	out := [][]string{}
	for _, v := range store.State().Data.(data.State).Tracking {
		out = append(out, []string{v.Symbol, fmt.Sprintf("%v", v.Buy), fmt.Sprintf("%v", v.Sell), fmt.Sprintf("%v", v.Current)})
	}

	sort.Sort(bySymbol(out))

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Symbol", "Buy", "Sell", "Current"})

	for _, v := range out {
		table.Append(v)
	}
	table.Render() // Send output

	fmt.Println("Hit return to continue")
	readLine()
	return nil
}

// parse parses the input strings.
func parse(input string, expecting int) ([]string, error) {
	sl := strings.Split(input, ";")
	switch len(sl) {
	case expecting + 1:
		if sl[len(sl)-1] != "" {
			return nil, fmt.Errorf("%s is incorrect format, expecting %d values got %d", input, expecting, len(sl))
		}
		sl = sl[:len(sl)-1]
	case expecting:
	default:
		return nil, fmt.Errorf("%s is incorrect format, expecting %d values got %d", input, expecting, len(sl))
	}
	return sl, nil
}

// watcher loops forever and updates all prices of stocks we are tracking.
func watcher() {
	wg := sync.WaitGroup{}
	for {
		for k, v := range store.State().Data.(data.State).Tracking {
			k := k
			v := v

			go func() {
				stock, err := yquotes.NewStock(k, false)
				if err != nil {
					glog.Errorf("problem retreiving stock information for %s: %s", k, err)
					return
				}

				if v.Current != stock.Price.Last {
					if err := store.Perform(actions.ChangeCurrent(k, stock.Price.Last)); err != nil {
						glog.Errorf("problem updating store for current price of stock %s: %s", k, err)
					}
				}
			}()
		}

		wg.Wait()
		time.Sleep(10 * time.Second)
	}
}

// checkBuySell watches stock updates and notifies us if the stock is in a buy
// or sell position.
func checkBuySell(symbol string) {
	// Use these to tell if we have already sent a notification for buying or selling.
	shouldBuyLast := false
	shouldSellLast := false

	ch, cancel, err := store.Subscribe("Tracking")
	if err != nil {
		panic(fmt.Sprintf("software is broken, bad developer: %s", err))
	}

	defer cancel()
	defer glog.Infof("unsubscribing for %s", symbol)

	for up := range ch {
		m := up.State.Data.(data.State).Tracking

		// If the map doesn't contain our stock symbol, it has been untracked.
		stock, ok := m[symbol]
		if !ok {
			return
		}

		// We haven't updated the stock data yet.
		if stock.Current == 0 {
			continue
		}

		if stock.Current <= stock.Buy {
			if !shouldBuyLast {
				notify.Push(
					fmt.Sprintf("Buy %s", symbol),
					fmt.Sprintf("Stock %s reached Buy limit of %v, current: %v", symbol, stock.Buy, stock.Current),
					"/home/user/icon.png",
					notificator.UR_CRITICAL,
				)
				glog.Infof("stock %s is at Buy. Buy was %v, current is %v", symbol, stock.Buy, stock.Current)
				shouldBuyLast = true
			}
		} else {
			shouldBuyLast = false
		}

		if stock.Current >= stock.Sell {
			if !shouldSellLast {
				notify.Push(
					fmt.Sprintf("Sell %s", symbol),
					fmt.Sprintf("Stock %s reached Sell limit of %v, current: %v", symbol, stock.Sell, stock.Current),
					"/home/user/icon.png",
					notificator.UR_CRITICAL,
				)
				shouldSellLast = true
			}
		} else {
			shouldSellLast = false
		}
	}
}

func main() {
	var err error
	store, err = state.New()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	notify = notificator.New(notificator.Options{
		DefaultIcon: "icon/default.png",
		AppName:     "Stock Notifier",
	})

	go watcher()

	for {
		if err := topMenu(); err != nil {
			fmt.Println(err)
			fmt.Println("Hit return to continue")
			readLine()
		}
	}
}
