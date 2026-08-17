package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/internal/auth"
)

const (
	ctxUserKey = "user"
)

// RequireAuth はAuthorization: Bearer <JWT> を検証し、ent.Userをcontextへ格納する。
func RequireAuth(client *ent.Client, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		userID, err := auth.ParseToken(jwtSecret, strings.TrimPrefix(header, prefix))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		u, err := client.User.Get(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		c.Set(ctxUserKey, u)
		c.Next()
	}
}

// CurrentUser はRequireAuthが格納したent.Userを取り出す。
func CurrentUser(c *gin.Context) *ent.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*ent.User)
	return u
}
