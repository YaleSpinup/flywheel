package flywheel

import (
	"encoding/json"
	"net/http"

	"github.com/YaleSpinup/apierror"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func (m *Manager) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("getting flywheel tasks")

		q := r.URL.Query()
		taskids, ok := q["task"]
		if !ok || len(taskids) < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Debugf("getting taskids %+v", taskids)

		tasks := make(map[string]*Task)
		for _, id := range taskids {
			task, err := m.GetTask(r.Context(), id)
			if err != nil {
				handleError(w, err)
			}

			if task != nil {
				tasks[id] = task
			}
		}

		j, err := json.Marshal(tasks)
		if err != nil {
			log.Errorf("cannot marshal response (%v) into JSON: %s", tasks, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(j)
	})
}

// handleError handles standard apierror return codes
func handleError(w http.ResponseWriter, err error) {
	log.Error(err.Error())
	if aerr, ok := errors.Cause(err).(apierror.Error); ok {
		switch aerr.Code {
		case apierror.ErrForbidden:
			w.WriteHeader(http.StatusForbidden)
		case apierror.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
		case apierror.ErrConflict:
			w.WriteHeader(http.StatusConflict)
		case apierror.ErrBadRequest:
			w.WriteHeader(http.StatusBadRequest)
		case apierror.ErrLimitExceeded:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Write([]byte(aerr.Message))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
}
