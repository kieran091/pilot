package pilot

import (
	"net/http"

	"github.com/bytedance/sonic"
)

const (
	noWritten     = -1
	defaultStatus = http.StatusOK
)

type responseWriter struct {
	http.ResponseWriter
	size   int
	status int
}

func withResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		size:           noWritten,
		status:         defaultStatus,
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.size == noWritten {
		rw.size = 0
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func (rw *responseWriter) WriteJSON(status int, res result) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	err := sonic.ConfigDefault.NewEncoder(rw).Encode(res)
	if err != nil {
		defaultLogger.Error().
			Int("status", status).
			Any("result", res).
			Err(err).
			Msg("")
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) Size() int {
	return rw.size
}

func (rw *responseWriter) Written() bool {
	return rw.size != noWritten
}
