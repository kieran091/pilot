package pilot

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

func Recovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				stackTrace := stack(3)

				event := defaultLogger.Error()

				if traceID, exists := c.Get("trace_id"); exists {
					event.Any("trace_id", traceID)
				}

				event.
					Str("method", c.Request.Method).
					Str("path", c.Request.URL.Path).
					Str("panic", fmt.Sprintf("%v", err)).
					Str("stack", stackTrace).
					Time("recovered_at", time.Now()).
					Msg("Request panic recovered")

				c.AddError(fmt.Errorf("panic: %v", err))

				if !c.Writer.Written() {
					c.Writer.WriteHeader(http.StatusInternalServerError)
					_, _ = c.Writer.Write([]byte("Internal Server Error"))
				}

				c.Abort()
			}
		}()

		c.Next()
	}
}

func stack(skip int) string {
	var buf [2 << 10]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])

	lines := strings.Split(s, "\n")
	if len(lines) > skip*2 {
		lines = lines[skip*2:]
	}

	return strings.Join(lines, "\n")
}
