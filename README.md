# flywheel

Flywheel is a task observability library for Spinup Go APIs.  Many of our APIs must handle asynchronous tasks
and this is done in a variety of ways that are less than ideal.

Sometimes...

* custom logic is encoded into the calling application (usually the UI) to check back on the resource and decide if it's been created

or...

* it's fire and forget, with the assumption that the service will get created/delete/updated eventually

or...

* we hold the connection open until the service is actually created/deleted/updated and risk a timeout

or...

* we do something I haven't thought of

Instead, using `flywheel` we can observe the state of a task.  Tasks in `flywheel` are usually high level orchestration
processes... usually assembling a few components into a service.  Note that `flywheel` is currently *not* a job queuing or management
platform.  The scope is simply to track the state of an ephemeral task throughout its lifecycle in a lightweight way accessible from
multiple instances of an API.

## Usage

### Install the library

```bash
go get github.com/YaleSpinup/flywheel
go get github.com/go-redis/redis/v8
```

**Note:** We may abstract the redis library in future releases.

### Create a redis client and flywheel manager that can be shared by multiple tasks

```golang
    rdb := redis.NewClient(&redis.Options{
        Addr:     "127.0.0.1:6379",
        Password: "",
        DB:       0,
    })

    pong, err := rdb.Ping(ctx).Result()
    log.Infof(pong, err)

    // create a new manager with the namespace, the time to keep tasks in redis and the redis client
    manager := flywheel.NewManager("testtesttest", 60*time.Minute, rdb)
```

### Create a new task and start it

```golang
    ctx := context.Background()
    task := flywheel.NewTask()
    if err := manager.Start(ctx, task); err != nil {
        log.Errorf("error starting task %s: %s", task.ID, err)
    }
```

### Checkin periodically with the job id to extend the TTL and notify the job is active

```golang
    if err := manager.CheckIn(ctx, id); err != nil {
        log.Errorf("failed to checkin to task %s", err)
    }
```

### Write some log messages to the event log

```golang
    if err := manager.Log(ctx, id, "some log message"); err != nil {
        log.Errorf("failed to log: %s", err)
    }
```

### Mark the task as completed

```golang
    if err := manager.Complete(ctx, id); err != nil {
        log.Errorf("failed to mark task %s completed: %s", id, err)
    }
```

### Mark the task as failed

```golang
    if err := manager.Fail(ctx, id, "something went horribly wrong"); err != nil {
        log.Errorf("failed to mark task %s failed: %s", id, err)
    }
```

### Get the task details

Although the manager should be safe to use concurrently, this doesn't need to use the same instance of the manager as long as the namespace is the same.

```golang
    ctx := context.Background()

    out, err := manager.GetTask(ctx, id)
    if err != nil {
        log.Errorf("failed to get task %s: %s", id, err)
    }

    j, err := json.MarshalIndent(out, "", "  ")
    if err != nil {
        log.Errorf("failed to marshall task %s as json %s", id, err)
    }

    log.Infof("%s", string(j))
```

## Todo

* A task reaper running on the manager to check for tasks that have been running too long and mark them as failed so upstream systems can get
  see the failure before the task disappears.

## License

GNU Affero General Public License v3.0 (GNU AGPLv3)  
Copyright © 2020 Yale University
