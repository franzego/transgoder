package observability

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderRequestID = "X-Request-ID"

func Middleware(metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("request_id", requestID)
		c.Writer.Header().Set(HeaderRequestID, requestID)

		if metrics != nil {
			metrics.RequestStarted()
		}
		start := time.Now()
		c.Next()
		_ = start
		if metrics != nil {
			metrics.RequestFinished(c.Writer.Status())
		}
	}
}
