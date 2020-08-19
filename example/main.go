package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/YaleSpinup/flywheel"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)

	log.Info("starting flywheel")

	// // create a default flywheel manager
	// manager, err := flywheel.NewManager("testtesttest")
	//
	// // ... or create a custom redis connection to pass in with the configuration
	// rdb := redis.NewClient(&redis.Options{
	// 	Addr: "127.0.0.1:6379",
	// 	DB:   0,
	// })
	// manager, err := flywheel.NewManager("testtesttest", flywheel.WithRedis(rdb))
	//
	// ... or pass in some options
	manager, err := flywheel.NewManager("testtesttest", flywheel.WithRedisAddress("127.0.0.1:6379"), flywheel.WithRedisDatabase(0), flywheel.WithTTL(3*time.Minute))
	if err != nil {
		log.Errorf("failed to create new flywheel manager")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// generate a new task to track and start it
	task := flywheel.NewTask()
	if err := manager.Start(ctx, task); err != nil {
		log.Errorf("error starting task %s: %s", task.ID, err)
	}

	// start a loop to repeatedly get the task and print it
	go taskGetterLoop(ctx, manager, task.ID)

	time.Sleep(1 * time.Second)

	// start a loop to repeatedly checkin.
	// note:  this is not how we expect this to be used, but was a
	// convenient way to get a bunch of checkins
	go taskCheckinLoop(ctx, manager, task.ID)

	// log a message
	if err := manager.Log(ctx, task.ID, "some log message"); err != nil {
		log.Errorf("failed to log: %s", err)
	}

	// do some work
	time.Sleep(1 * time.Second)

	// mark the task complete
	if err := manager.Complete(ctx, task.ID); err != nil {
		log.Errorf("failed to log: %s", err)
	}

	// // mark the task failed
	// if err := manager.Fail(ctx, task.ID, "things blew up"); err != nil {
	// 	log.Errorf("failed to log: %s", err)
	// }

	time.Sleep(5 * time.Second)

	// shut 'er down
	cancel()

	time.Sleep(1 * time.Second)
}

func taskGetterLoop(ctx context.Context, manager *flywheel.Manager, id string) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			out, err := manager.GetTask(ctx, id)
			if err != nil {
				log.Errorf("failed to get task %s", err)
			}

			j, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				log.Errorf("failed to marshall json task %s", err)
			}

			log.Infof("%s", string(j))
		case <-ctx.Done():
			log.Debug("shutting down taskgetter timer")
			ticker.Stop()
			return
		}
	}
}

func taskCheckinLoop(ctx context.Context, manager *flywheel.Manager, id string) {
	ticker := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-ticker.C:
			if err := manager.CheckIn(ctx, id); err != nil {
				log.Errorf("failed to checkin to task %s", err)
			}

		case <-ctx.Done():
			log.Debug("shutting down task checkin timer")
			ticker.Stop()
			return
		}
	}
}
