package bot

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Disduper is a Discord bot that deduplicates messages
type Disduper struct {
	log               *log.Logger
	session           *discordgo.Session
	self              *discordgo.Application
	msgMap            messageMap
	currentGuilds     sync.Map
	operatingChannels sync.Map

	maxDuplicateAge time.Duration

	handledCount   int
	actedUponCount int
}

// Start starts the Discord bot
func (dd *Disduper) Start(token string, log *log.Logger) (err error) {
	dd.log = log
	dd.maxDuplicateAge = 30 * time.Second

	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	dd.session, err = discordgo.New(token)
	if err != nil {
		return err
	}

	dd.self, err = dd.session.Application("@me")
	if err != nil {
		return err
	}

	dd.session.AddHandler(dd.messageCreate)
	dd.session.AddHandler(dd.messageUpdate)
	dd.session.AddHandler(dd.guildCreate)
	dd.session.AddHandler(dd.guildDelete)
	dd.session.AddHandler(dd.channelCreate)
	dd.session.AddHandler(dd.channelUpdate)
	dd.session.AddHandler(dd.channelDelete)
	dd.session.AddHandler(dd.userUpdate)
	dd.session.AddHandler(dd.guildMemberUpdate)
	dd.session.AddHandler(dd.roleUpdate)

	err = dd.session.Open()
	if err != nil {
		return err
	}
	dd.log.Println("Bot is now running.")
	return nil
}

// InitIntegrated initializes Disduper to work as part of a larger bot
func (dd *Disduper) InitIntegrated(log *log.Logger, session *discordgo.Session) (err error) {
	dd.log = log
	dd.maxDuplicateAge = 30 * time.Second

	dd.session = session

	dd.self, err = dd.session.Application("@me")
	if err != nil {
		return err
	}

	dd.session.AddHandler(dd.messageUpdate)
	dd.session.AddHandler(dd.guildCreate)
	dd.session.AddHandler(dd.guildDelete)
	dd.session.AddHandler(dd.channelCreate)
	dd.session.AddHandler(dd.channelUpdate)
	dd.session.AddHandler(dd.channelDelete)
	dd.session.AddHandler(dd.userUpdate)
	dd.session.AddHandler(dd.guildMemberUpdate)
	dd.session.AddHandler(dd.roleUpdate)

	return nil
}

// Stop stops the Discord bot
func (dd *Disduper) Stop() {
	// Cleanly close down the Discord session.
	if dd.session != nil {
		dd.session.Close()
	}
	dd.log.Println("Bot is now stopped.")
}

// Handle attempts to handle the provided message; if it fails, it returns false
func (dd *Disduper) Handle(s *discordgo.Session, m *discordgo.MessageCreate, muted bool) bool {
	if !dd.operatesIn(m.ChannelID) {
		return false
	}
	dd.handledCount++

	if !dd.msgMap.Put(m.Message, dd.maxDuplicateAge) {
		if err := s.ChannelMessageDelete(m.ChannelID, m.ID); err != nil {
			dd.log.Println("Error deleting message: " + err.Error())
			return false
		}
		dd.actedUponCount++
		// if we deleted this message, then it shouldn't be processed any further
		return true
	}

	return false
}

// MessagesHandled returns the number of messages handled by this CommandLibrary
func (dd *Disduper) MessagesHandled() int {
	return dd.handledCount
}

// MessagesActedUpon returns the number of messages acted upon by this CommandLibrary
func (dd *Disduper) MessagesActedUpon() int {
	return dd.actedUponCount
}

// Name returns the name of this message handler
func (dd *Disduper) Name() string {
	return "Disduper"
}

func (dd *Disduper) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	dd.Handle(s, m, false)
}

func (dd *Disduper) messageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
	timestamp, err := m.Timestamp.Parse()
	if err == nil && timestamp.After(time.Now().Add(-dd.maxDuplicateAge)) {
		dd.msgMap.Delete(m.Message)
	}
}

func (dd *Disduper) guildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	dd.currentGuilds.Store(g.ID, true)
	for _, channel := range g.Channels {
		dd.refreshPermissions(g.ID, channel.ID)
	}
}

func (dd *Disduper) guildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
	dd.currentGuilds.Delete(g.ID)
	// TODO
}

func (dd *Disduper) channelCreate(s *discordgo.Session, c *discordgo.ChannelCreate) {
	dd.refreshPermissions(c.GuildID, c.ID)
}

func (dd *Disduper) channelDelete(s *discordgo.Session, c *discordgo.ChannelDelete) {
	dd.setOperatesIn(c.ID, false)
}

func (dd *Disduper) userUpdate(s *discordgo.Session, c *discordgo.UserUpdate) {
	if c.ID == dd.self.ID {
		dd.refreshPermissionsForAllGuilds()
	}
}

func (dd *Disduper) guildMemberUpdate(s *discordgo.Session, g *discordgo.GuildMemberUpdate) {
	if g.Member.User.ID == dd.self.ID {
		dd.refreshPermissionsForGuild(g.GuildID)
	}
}

func (dd *Disduper) channelUpdate(s *discordgo.Session, c *discordgo.ChannelUpdate) {
	dd.refreshPermissions(c.GuildID, c.ID)
}

func (dd *Disduper) roleUpdate(s *discordgo.Session, r *discordgo.GuildRoleUpdate) {
	dd.refreshPermissionsForGuild(r.GuildID)
}

func (dd *Disduper) refreshPermissionsForAllGuilds() {
	// refresh permissions for all the guilds/channels we're in
	dd.currentGuilds.Range(func(key, value interface{}) bool {
		dd.refreshPermissionsForGuild(key.(string))
		return true
	})
}

func (dd *Disduper) refreshPermissionsForGuild(guildID string) {
	channels, err := dd.session.GuildChannels(guildID)
	if err != nil {
		dd.log.Println("Error getting guild channels", err.Error())
		return
	}
	for _, channel := range channels {
		dd.refreshPermissions(channel.GuildID, channel.ID)
	}
}

func (dd *Disduper) refreshPermissions(guildID string, channelID string) {
	dd.log.Println("Refreshing permissions for guild", guildID, "channel", channelID)
	// get member into discordgo cache
	_, err := dd.session.State.Member(guildID, dd.self.ID)
	if err != nil {
		if _, err = dd.session.GuildMember(guildID, dd.self.ID); err != nil {
			dd.log.Println("Error getting self guild member: " + err.Error())
			return
		}
	}

	permissions, err := dd.session.State.UserChannelPermissions(dd.self.ID, channelID)
	if err != nil {
		dd.log.Println("Error getting permissions: " + err.Error())
		return
	}
	if permissions&discordgo.PermissionManageMessages != 0 {
		// permission to delete messages in this channel
		dd.setOperatesIn(channelID, true)
		dd.log.Println("Will operate in guild", guildID, "channel", channelID)
	} else {
		dd.setOperatesIn(channelID, false)
	}
}

func (dd *Disduper) operatesIn(channelID string) bool {
	_, ok := dd.operatingChannels.Load(channelID)
	return ok
}

func (dd *Disduper) setOperatesIn(channelID string, operates bool) {
	if !operates {
		dd.operatingChannels.Delete(channelID)
	} else {
		dd.operatingChannels.Store(channelID, true)
	}
}

type discordChannel struct {
	ID      string
	GuildID string
}
