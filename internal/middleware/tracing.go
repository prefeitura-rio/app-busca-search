package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// RequestTiming adds OpenTelemetry tracing to HTTP requests
func RequestTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Create a span for the entire request
		ctx, span := otel.Tracer("http").Start(c.Request.Context(), "http.request")
		defer span.End()

		// Set span attributes
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.url", c.Request.URL.String()),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.user_agent", c.Request.UserAgent()),
			attribute.String("http.client_ip", c.ClientIP()),
			attribute.String("http.host", c.Request.Host),
		)

		// Update the request context with the span
		c.Request = c.Request.WithContext(ctx)

		// Process the request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		status := c.Writer.Status()

		// Add response attributes
		span.SetAttributes(
			attribute.Int("http.status_code", status),
			attribute.Int64("http.duration_ms", duration.Milliseconds()),
			attribute.String("http.duration", duration.String()),
			attribute.Int("http.response_size", c.Writer.Size()),
		)

		// Mark span as error if status code indicates failure
		if status >= 400 {
			span.SetStatus(codes.Error, "HTTP request failed")
			span.SetAttributes(attribute.Bool("http.error", true))

			// Add error message if available
			if len(c.Errors) > 0 {
				span.SetAttributes(attribute.String("http.error_message", c.Errors.String()))
			}
		} else {
			span.SetStatus(codes.Ok, "HTTP request succeeded")
		}
	}
}
