package pilot

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func getClientIP(r *http.Request) string {
	// X-Forwarded-For 可能包含多个IP，取第一个
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	// X-Client-IP
	if xci := r.Header.Get("X-Client-IP"); xci != "" {
		if net.ParseIP(xci) != nil {
			return xci
		}
	}

	// CF-Connecting-IP (Cloudflare)
	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		if net.ParseIP(cfip) != nil {
			return cfip
		}
	}

	// True-Client-IP
	if tci := r.Header.Get("True-Client-IP"); tci != "" {
		if net.ParseIP(tci) != nil {
			return tci
		}
	}

	// RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	return r.RemoteAddr
}

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
	clientIP := getClientIP(c.Request)

	traceID, _ := c.Get("trace_id")
	service := c.GetService()
	methodName := c.GetMethodName()

	event := defaultLogger.Info()
	if isError {
		event = defaultLogger.Error()
	}

	event = event.
		Time("timestamp", c.startTime).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Str("ip", clientIP).
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
