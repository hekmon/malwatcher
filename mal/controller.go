package mal

import (
	"context"
	"sync"
	"time"

	"github.com/hekmon/hllogger"
)

const (
	nbSeasonsMin = 1
	nbSeaonsMax  = 40
)

// Config allow to pass configuration when instanciating a new Controller
type Config struct {
	NbSeasons int
	Logger    *hllogger.HlLogger
}

// New returns an initialized & ready to use controller
func New(ctx context.Context, conf Config) (c *Controller) {
	// config checks
	if conf.Logger == nil {
		panic("can't init mal controller with a nil logger")
	}
	if conf.NbSeasons < nbSeasonsMin {
		conf.Logger.Warningf("[MAL] nbSeasons for initial list building can't be lower than %d (currently: %d): defaulting to %d",
			nbSeasonsMin, conf.NbSeasons, nbSeasonsMin)
		conf.NbSeasons = nbSeasonsMin
	} else if conf.NbSeasons > nbSeaonsMax {
		conf.Logger.Warningf("[MAL] nbSeasons for initial list building can't be more than %d (currently: %d): defaulting to %d",
			nbSeaonsMax, conf.NbSeasons, nbSeaonsMax)
		conf.NbSeasons = nbSeaonsMax
	}
	// create the controller
	c = &Controller{
		nbSeasons: conf.NbSeasons,
		ctx:       ctx,
		stopped:   make(chan struct{}),
		log:       conf.Logger,
	}
	// recover previous state if any
	if !c.load() {
		c = nil
		return
	}
	// start the worker
	c.workers.Add(1)
	go func() {
		go c.fetcher()
		c.workers.Done()
	}()
	// Create the auto-stopper (must be launch after the worker(s) in case ctx is cancelled while launching workers)
	go c.autostop()
	// ready
	return
}

// Controller abstract all the logic of the MAL watcher
type Controller struct {
	// config
	nbSeasons int
	// state
	ctx       context.Context
	watchList map[int]string
	// worker(s)
	workers     sync.WaitGroup
	stopped     chan struct{}
	lastRequest time.Time
	// sub controllers
	log *hllogger.HlLogger
}

func (c *Controller) autostop() {
	// Wait for signal
	<-c.ctx.Done()
	// Begin the stopping proceedure
	c.workers.Wait()
	// save state
	c.save()
	// Close the stopped chan to indicate we are fully stopped
	close(c.stopped)
}

// WaitStopped will block until c is fully stopped.
// To be stopped, c needs to have its context cancelled.
// WaitStopped is safe to be called from multiples goroutines.
func (c *Controller) WaitStopped() {
	<-c.stopped
}
