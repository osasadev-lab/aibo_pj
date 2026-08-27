package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannel"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannelmember"
)

const ctxDMChannelKey = "dm_channel"
const ctxDMChannelMembershipKey = "dm_channel_membership"

// RequireDMChannelAccess はパスの :channel_id を解決し、呼び出しユーザーが
// そのチャンネルのメンバーであるか検証する（RequireTaskAccessと同型）。
func RequireDMChannelAccess(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		channelID, err := uuid.Parse(c.Param("channel_id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "channel not found"})
			return
		}

		ctx := c.Request.Context()

		ch, err := client.DMChannel.Query().Where(dmchannel.IDEQ(channelID)).Only(ctx)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "channel not found"})
			return
		}

		membership, err := client.DMChannelMember.Query().
			Where(dmchannelmember.ChannelIDEQ(channelID), dmchannelmember.UserIDEQ(u.ID)).
			Only(ctx)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a channel member"})
			return
		}

		c.Set(ctxDMChannelKey, ch)
		c.Set(ctxDMChannelMembershipKey, membership)
		c.Next()
	}
}

// CurrentDMChannel はRequireDMChannelAccessが格納した*ent.DMChannelを取り出す。
func CurrentDMChannel(c *gin.Context) *ent.DMChannel {
	v, ok := c.Get(ctxDMChannelKey)
	if !ok {
		return nil
	}
	ch, _ := v.(*ent.DMChannel)
	return ch
}

// CurrentDMChannelMembership はRequireDMChannelAccessが格納した*ent.DMChannelMemberを取り出す。
func CurrentDMChannelMembership(c *gin.Context) *ent.DMChannelMember {
	v, ok := c.Get(ctxDMChannelMembershipKey)
	if !ok {
		return nil
	}
	m, _ := v.(*ent.DMChannelMember)
	return m
}
