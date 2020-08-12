package flywheel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// NewID returns a new random ID.  Currently this is just a UUID string
func NewID() string {
	id := uuid.New().String()
	log.Debugf("generated random id %s", id)
	return id
}

// Manager controls the communication with redis
type Manager struct {
	id        string
	namespace string
	redis     *redis.Client
	ttl       time.Duration

	redisAddress  string
	redisDatabase int
	redisUsername string
	redisPassword string
	redisPing     bool
}

type ManagerOption func(*Manager)

// NewManager creates a new manager instance used to communicate with the redis storage engine
func NewManager(namespace string, opts ...ManagerOption) (*Manager, error) {
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}

	log.Infof("creating new flywheel manager with namespace %s", namespace)

	// setup the manager with some default options
	m := Manager{
		id:        NewID(),
		namespace: namespace,
		ttl:       60 * time.Minute,

		redisAddress:  "127.0.0.1:6379",
		redisDatabase: 0,
		redisUsername: "",
		redisPassword: "",
		redisPing:     true,
	}

	for _, opt := range opts {
		opt(&m)
	}

	if m.redis == nil {
		rdb := redis.NewClient(&redis.Options{
			Addr:     m.redisAddress,
			Username: m.redisUsername,
			Password: m.redisPassword,
			DB:       m.redisDatabase,
		})

		m.redis = rdb
	}

	if m.redisPing {
		if err := m.redis.Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("failed to ping redis for new flywheel manager: %s", err)
		}
	}

	return &m, nil
}

// WithTTL sets the amount of time a job will linger in the persistence layer (redis) after the last check-in, log message,
// completion or failure.  This sets a TTL on the redis keys used by the library.
func WithTTL(ttl time.Duration) ManagerOption {
	return func(m *Manager) {
		log.Debugf("setting ttl to %s", ttl.String())
		m.ttl = ttl
	}
}

// WithRedis sets the redis client to support advanced options
func WithRedis(client *redis.Client) ManagerOption {
	return func(m *Manager) {
		log.Debug("setting redis client")
		m.redis = client
	}
}

// WithRedisAddress sets the address for redis in the form <ipaddress|hostname>:<port>.  This will be ignored if WithRedis() is passed.
func WithRedisAddress(address string) ManagerOption {
	return func(m *Manager) {
		log.Debugf("setting redis address to %s", address)
		m.redisAddress = address
	}
}

// WithRedisDatabase sets the redis database to be used.  This will be ignored if WithRedis() is passed.
func WithRedisDatabase(db int) ManagerOption {
	return func(m *Manager) {
		log.Debugf("setting redis database to %d", db)
		m.redisDatabase = db
	}
}

// WithRedisUsername sets the redis username to be used.  This will be ignored if WithRedis() is passed.
func WithRedisUsername(username string) ManagerOption {
	return func(m *Manager) {
		log.Debugf("setting redis username to %s", username)
		m.redisUsername = username
	}
}

// WithRedisPassword sets the redis password to be used.  This will be ignored if WithRedis() is passed.
func WithRedisPassword(password string) ManagerOption {
	return func(m *Manager) {
		log.Debug("setting redis password")
		m.redisPassword = password
	}
}

// WithoutRedisPing disables the initial redis ping check
func WithoutRedisPing() ManagerOption {
	return func(m *Manager) {
		log.Debug("disabling the redis ping check")
		m.redisPing = false
	}
}

// GetTask pulls the details of a task out of redis, the related events and
func (m *Manager) GetTask(ctx context.Context, id string) (*Task, error) {
	log.Infof("getting task with id %s", id)

	hkey := m.namespace + ":tasks:" + id
	hout, err := m.redis.HGetAll(ctx, hkey).Result()
	if err != nil {
		return nil, err
	}

	task := &Task{}
	if err := task.mapToTask(hout, []string{}); err != nil {
		return nil, err
	}

	ekey := m.namespace + ":events:" + id
	eout, err := m.redis.LRange(ctx, ekey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	task.Events = eout

	log.Debugf("returning task %+v", task)

	return task, nil
}

func (m *Manager) Start(ctx context.Context, t *Task) error {
	log.Infof("starting task %s", t.ID)

	start := time.Now().UTC().Format(time.RFC3339Nano)

	var err error
	var rollBackTasks []rollbackFunc
	defer func() {
		if err != nil {
			log.Errorf("recovering from error: %s, executing %d rollback tasks", err, len(rollBackTasks))
			rollBack(&rollBackTasks)
		}
	}()

	t.Status = STATUS_RUNNING
	key := m.namespace + ":tasks:" + t.ID

	if err = m.redis.HSet(ctx, key, t.taskToMapString()).Err(); err != nil {
		log.Errorf("failed to set hash key %s: %s", key, err)
		return err
	}

	rollBackTasks = append(rollBackTasks, func(ctx context.Context) error {
		return func() error {
			if err := m.redis.HDel(ctx, key).Err(); err != nil {
				return fmt.Errorf("failed to delete redis key %s after error: %s", key, err)
			}

			log.Infof("successfully removed hash %s in rollback", key)

			return nil
		}()
	})

	if err = m.redis.Expire(ctx, key, m.ttl).Err(); err != nil {
		return err
	}

	ekey := m.namespace + ":events:" + t.ID
	if err = m.redis.RPush(ctx, ekey, start+" starting task "+t.ID).Err(); err != nil {
		return err
	}

	rollBackTasks = append(rollBackTasks, func(ctx context.Context) error {
		return func() error {
			if err := m.redis.Del(ctx, ekey).Err(); err != nil {
				return fmt.Errorf("failed to delete redis key %s after error: %s", ekey, err)
			}

			log.Infof("successfully removed hash %s in rollback", key)

			return nil
		}()
	})

	if err = m.redis.Expire(ctx, ekey, m.ttl).Err(); err != nil {
		return err
	}

	return nil
}

func (m *Manager) CheckIn(ctx context.Context, id string) error {
	log.Infof("checking in for task %s", id)

	checkin := time.Now().UTC().Format(time.RFC3339Nano)

	key := m.namespace + ":tasks:" + id
	if num, err := m.redis.Exists(ctx, key).Result(); num == 0 || err != nil {
		return fmt.Errorf("failed to check for task hash key %s (%d)! %s", key, num, err)
	}

	if err := m.redis.HSet(ctx, key, "checkin_at", checkin).Err(); err != nil {
		log.Errorf("failed to set hash values %s: %s", key, err)
		return err
	}

	if err := m.redis.Expire(ctx, key, m.ttl).Err(); err != nil {
		return err
	}

	if err := m.Log(ctx, id, fmt.Sprintf("%s checkin task %s", checkin, id)); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Log(ctx context.Context, id, message string) error {
	logt := time.Now().UTC().Format(time.RFC3339Nano)

	ekey := m.namespace + ":events:" + id
	if err := m.redis.RPush(ctx, ekey, logt+" "+message).Err(); err != nil {
		return err
	}

	if err := m.redis.Expire(ctx, ekey, m.ttl).Err(); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Complete(ctx context.Context, id string) error {
	log.Infof("completing task %s", id)

	complete := time.Now().UTC().Format(time.RFC3339Nano)

	key := m.namespace + ":tasks:" + id
	if num, err := m.redis.Exists(ctx, key).Result(); err != nil {
		return fmt.Errorf("error checking hash key %s! %s", key, err)
	} else if num == 0 {
		return fmt.Errorf("task hash key %s doesn't exist!", key)
	}

	if err := m.redis.HSet(ctx, key, "completed_at", complete, "status", STATUS_COMPLETED).Err(); err != nil {
		log.Errorf("failed to set hash values %s: %s", key, err)
		return err
	}

	if err := m.redis.Expire(ctx, key, m.ttl).Err(); err != nil {
		return err
	}

	if err := m.Log(ctx, id, fmt.Sprintf("%s complete task %s", complete, id)); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Fail(ctx context.Context, id, message string) error {
	log.Infof("failing task %s", id)

	failed := time.Now().UTC().Format(time.RFC3339Nano)

	key := m.namespace + ":tasks:" + id
	if num, err := m.redis.Exists(ctx, key).Result(); err != nil {
		return fmt.Errorf("error checking hash key %s! %s", key, err)
	} else if num == 0 {
		return fmt.Errorf("task hash key %s doesn't exist!", key)
	}

	if err := m.redis.HSet(ctx, key, "failed_at", failed, "status", STATUS_FAILED, "failure", message).Err(); err != nil {
		log.Errorf("failed to set hash values %s: %s", key, err)
		return err
	}

	if err := m.redis.Expire(ctx, key, m.ttl).Err(); err != nil {
		return err
	}

	if err := m.Log(ctx, id, fmt.Sprintf("%s failed task %s", failed, id)); err != nil {
		return err
	}

	return nil
}

type rollbackFunc func(ctx context.Context) error

// rollBack executes functions from a stack of rollback functions
func rollBack(t *[]rollbackFunc) {
	if t == nil {
		return
	}

	timeout, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	done := make(chan string, 1)
	go func() {
		tasks := *t
		log.Errorf("executing rollback of %d tasks", len(tasks))
		for i := len(tasks) - 1; i >= 0; i-- {
			f := tasks[i]
			if funcerr := f(timeout); funcerr != nil {
				log.Errorf("rollback task error: %s, continuing rollback", funcerr)
			}
			log.Infof("executed rollback task %d of %d", len(tasks)-i, len(tasks))
		}
		done <- "success"
	}()

	// wait for a done context
	select {
	case <-timeout.Done():
		log.Error("timeout waiting for successful rollback")
	case <-done:
		log.Info("successfully rolled back")
	}
}
