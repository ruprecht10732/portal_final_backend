package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

func RequestTimer() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		_ = time.Since(start)
	}
}
