package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireInternalCronSecret はCloud Schedulerからの内部cronエンドポイント呼び出しを、
// X-Internal-Cron-Secret ヘッダーの一致で認証する（M7）。Cloud SchedulerはユーザーJWTを
// 持たないためRequireAuthは使えない、共有シークレット方式（docs/aibo/m7-implementation-plan.md
// 設計判断2）。CurrentUser()は使わない独立した経路。
func RequireInternalCronSecret(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("X-Internal-Cron-Secret")
		if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(secret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid cron secret"})
			return
		}
		c.Next()
	}
}
