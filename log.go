package pilot

import (
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

func formatDuration(d time.Duration) string {
	us := d.Microseconds()

	if us < 1000 {
		return strconv.FormatInt(us, 10) + "us"
	}

	ms := us / 1000
	if ms < 1000 {
		return strconv.FormatInt(ms, 10) + "ms"
	}

	s := float64(us) / 1000000.0
	return strconv.FormatFloat(s, 'f', 2, 64) + "s"
}

func buildLogEvent(c *Context, isError bool) *zerolog.Event {
	duration := c.Duration()
	status := c.Writer.Status()

	traceID, _ := c.Get("trace_id")
	service := c.GetService()
	methodName := c.GetMethodName()

	event := defaultLogger.Info()
	if isError {
		event = defaultLogger.Warn()
	}

	event = event.
		Time("timestamp", c.startTime).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Str("ip", c.ClientIP()).
		Int("status", status).
		Str("duration", formatDuration(duration))

	if service != "" {
		event.Str("service", service)
	}

	if methodName != "" {
		event.Str("method_name", methodName)
	}

	if traceID != nil {
		event.Any("trace_id", traceID)
	}

	return event
}

func Log() HandlerFunc {
	return func(c *Context) {
		c.Next()

		hasErrors := len(c.errors) > 0
		event := buildLogEvent(c, hasErrors)

		if hasErrors {
			event.Errs("errors", c.errors).Send()
			return
		}

		event.Send()
	}
}
