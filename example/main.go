package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/YaleSpinup/flywheel"
	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)

	log.Info("starting main")

	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	pong, err := rdb.Ping(ctx).Result()
	log.Infof(pong, err)

	manager := flywheel.NewManager("testtesttest", 1*time.Minute, rdb)

	task := flywheel.NewTask()
	if err := manager.Start(ctx, task); err != nil {
		log.Errorf("error starting task %s: %s", task.ID, err)
	}

	go taskGetterLoop(ctx, manager, task.ID)

	time.Sleep(1 * time.Second)

	go taskCheckinLoop(ctx, manager, task.ID)

	if err := manager.Log(ctx, task.ID, "some log message"); err != nil {
		log.Errorf("failed to log: %s", err)
	}
	time.Sleep(1 * time.Second)

	if err := manager.Complete(ctx, task.ID); err != nil {
		log.Errorf("failed to log: %s", err)
	}

	time.Sleep(5 * time.Second)

	cancel()

	time.Sleep(2 * time.Second)
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
