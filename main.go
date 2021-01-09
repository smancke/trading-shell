package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/adshao/go-binance/v2"
	"github.com/inancgumus/screen"
	"github.com/smancke/money-machine/config"
	"github.com/smancke/money-machine/logging"
)

const applicationName = "money-machine"

func main() {
	config := config.ReadConfig()
	if err := logging.Set(config.LogLevel, config.TextLogging); err != nil {
		exit(nil, err)
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "list-push-coins" {
			PrintPushCoins(binance.NewClient(config.APIKey, config.APISecret))
			return
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	configToLog := *config
	configToLog.APISecret = "..."
	configToLog.APIKey = "..."
	logging.LifecycleStart(applicationName, configToLog)

	app, err := newApplication(config)
	if err != nil {
		exit(nil, err)
		return
	}

	app.startConsole()

	logging.LifecycleStop(applicationName, <-stop, nil)
	app.stop()
}

func newApplication(config *config.Config) (*application, error) {
	port := config.Port
	if port != "" {
		port = fmt.Sprintf(":%s", port)
	}

	return &application{
		config: config,
		port:   port,
	}, nil
}

type application struct {
	config *config.Config
	port   string

	httpSrv *http.Server
}

func (app *application) startConsole() {
	screen.Clear()
	screen.MoveTopLeft()

	session := StartSession(app.config)
	go func() {
		for {
			fmt.Println(session.Get())
		}
	}()
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			text, _ := reader.ReadString('\n')
			// convert CRLF to LF
			text = strings.Replace(text, "\n", "", -1)

			session.Put(text)
			screen.Clear()
			screen.MoveTopLeft()
		}
	}()
}

func (app *application) startHTTP() {
	handler := app.handlerChain()
	app.httpSrv = &http.Server{Addr: app.port, Handler: handler}

	go func() {
		if err := app.httpSrv.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				logging.ServerClosed(applicationName)
			} else {
				exit(nil, err)
			}
		}
	}()
}

func (app *application) stop() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), app.config.GracePeriod)
	if app.httpSrv != nil {
		app.httpSrv.Shutdown(ctx)
	}
	ctxCancel()

	app.stopService()
}

func (app *application) stopService() {
}

func (app *application) handlerChain() http.Handler {
	client := binance.NewClient(app.config.APIKey, app.config.APISecret)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Ticker \n\n")

		prices, err := client.NewListPricesService().Symbol("BNBETH").Do(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}

		i := 0
		for _, p := range prices {
			fmt.Fprintf(w, "%v\n", p)
			if i >= 20 {
				return
			}
			i++
		}

	})
}

var exit = func(signal os.Signal, err error) {
	logging.LifecycleStop(applicationName, signal, err)
	if err == nil {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
