package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const supabaseTokenTTL = 1 * time.Hour

// supabaseClaims はSupabase Realtime/PostgRESTが読む"role"クレームを追加したもの。
// subには自前のusers.idをそのまま入れる。auth.usersに実レコードが無くても、
// auth.uid()はJWTクレームを直接読むため機能する想定（実装後にスモークテストで検証する。
// docs/aibo/m3-implementation-plan.md参照）。
type supabaseClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// IssueSupabaseToken はSUPABASE_JWT_SECRET（このアプリ自身のJWT_SECRETとは別）で
// 署名した短命JWTを発行する。Supabase Realtimeのチャンネル認証専用。
func IssueSupabaseToken(secret string, userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := supabaseClaims{
		Role: "authenticated",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"authenticated"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(supabaseTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
