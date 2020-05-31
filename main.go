package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hekmon/malwatcher/mal"

	"github.com/hekmon/hllogger"
	"github.com/hekmon/pushover/v2"
	systemd "github.com/iguanesolutions/go-systemd"
)

var (
	logger         *hllogger.HlLogger
	pushoverClient *pushover.Controller
	watcher        *mal.Controller
	mainLock       chan struct{}
	mainCtx        context.Context
	mainCtxCancel  func()
)

func main() {
	// Parse flags
	logLevelFlag := flag.String("loglevel", "info", "Set loglevel: debug, info, warning, error, fatal. Default info.")
	// confFile := flag.String("conf", "config.json", "Relative or absolute path to the json configuration file")
	flag.Parse()

	// Init logger
	var logLevel hllogger.LogLevel
	switch strings.ToLower(*logLevelFlag) {
	case "debug":
		logLevel = hllogger.Debug
	case "info":
		logLevel = hllogger.Info
	case "warning":
		logLevel = hllogger.Warning
	case "error":
		logLevel = hllogger.Error
	case "fatal":
		logLevel = hllogger.Fatal
	default:
		logLevel = hllogger.Info
	}
	var flags int
	if !systemd.IsNotifyEnabled() {
		flags = hllogger.Ldate | hllogger.Ltime
	}
	logger = hllogger.New(os.Stdout, &hllogger.Config{
		LogLevel:              logLevel,
		LoggerFlags:           flags,
		SystemdJournaldCompat: systemd.IsNotifyEnabled(),
	})
	logger.Output(" ")
	logger.Output(" • MyAnimeList Watcher •")
	logger.Output("      (づ ◕‿◕ )づ")
	logger.Output(" ")

	// Init the pushover client
	pushoverClient = pushover.New(&pushoverApp, &pushoverUser)

	// Init the mal watcher core
	mainCtx, mainCtxCancel = context.WithCancel(context.Background())
	defer mainCtxCancel()
	watcher = mal.New(mainCtx, mal.Config{
		NbSeasons: 1,
		Pushover:  pushoverClient,
		Logger:    logger,
	})
	if watcher == nil {
		logger.Fatal(1, "[Main] Failted to instanciate the watcher")
	}

	// Prepare to handle signals and go to sleep
	mainLock = make(chan struct{})
	go handleSignals()
	<-mainLock
}

func handleSignals() {
	// If we exit, allow main goroutine to do so
	defer close(mainLock)
	// Register signals
	var sig os.Signal
	signalChannel := make(chan os.Signal)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
	// Waiting for signals to catch
	for {
		sig = <-signalChannel
		switch sig {
		case syscall.SIGTERM:
			fallthrough
		case syscall.SIGINT:
			// Notify everything
			logger.Infof("[Main] Signal '%v' caught: cleaning up before exiting", sig)
			if err := systemd.NotifyStopping(); err != nil {
				logger.Errorf("[Main] can't send systemd stopping notification: %v", err)
			} else {
				logger.Debug("[Main] systemd stopping notification sent")
			}
			pushoverClient.SendHighPriorityMsg("Application is stopping...", "")
			// Cancel main ctx & wait for watcher
			mainCtxCancel()
			watcher.WaitStopped()
			logger.Debugf("[Main] Signal '%v' caught: watcher stopped: unlocking main goroutine to exit", sig)
			return
		default:
			logger.Warningf("[Main] Signal '%v' caught but no process set to handle it: skipping", sig)
		}
	}
}
