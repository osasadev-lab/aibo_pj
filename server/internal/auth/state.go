package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
)

const stateCookieName = "oauth_state"
const stateCookieMaxAge = 10 * 60 // 10分。ログイン→コールバックの往復のみで使う短命Cookie。

// GenerateState はOAuthのCSRF対策用stateパラメータを生成する。
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetStateCookie はコールバック検証用に、発行したstateを短命Cookieへ保存する。
func SetStateCookie(c *gin.Context, state string, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(stateCookieName, state, stateCookieMaxAge, "/", "", secure, true)
}

// VerifyStateCookie はコールバックで受け取ったstateがCookieの値と一致するか確認する。
func VerifyStateCookie(c *gin.Context, gotState string) bool {
	cookieState, err := c.Cookie(stateCookieName)
	if err != nil || cookieState == "" {
		return false
	}
	// 使い切り。検証後は破棄する。
	c.SetCookie(stateCookieName, "", -1, "/", "", false, true)
	return cookieState == gotState && gotState != ""
}
