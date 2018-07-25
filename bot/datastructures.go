package bot

import (
	"hash/fnv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type messageMap struct {
	// userMap[userID] -> *userMapValue
	userMap sync.Map
}

type userMapValue struct {
	// channelMap[channelID] -> *channelMapValue
	channelMap sync.Map
	bufferSize int
}

type channelMapValue struct {
	sync.Mutex
	messages   map[uint32]*discordgo.Message
	keyhistory []uint32
}

func (m *messageMap) Put(message *discordgo.Message) bool {
	value := &userMapValue{
		bufferSize: 5,
	}
	v, _ := m.userMap.LoadOrStore(message.Author.ID, value)
	value = v.(*userMapValue)

	channelValue := &channelMapValue{}
	v, _ = value.channelMap.LoadOrStore(message.ChannelID, channelValue)
	channelValue = v.(*channelMapValue)

	if channelValue.DuplicatePresent(message, 30*time.Second) {
		return false
	}
	channelValue.Add(message, value.bufferSize)
	return true
}

func (c *channelMapValue) Add(message *discordgo.Message, size int) {
	c.Lock()
	defer c.Unlock()

	if c.messages == nil {
		c.messages = make(map[uint32]*discordgo.Message)
	}

	hash := hashMessage(message)
	deleted := 0
	for len(c.keyhistory) >= size {
		delete(c.messages, c.keyhistory[deleted])
		deleted++
	}
	c.messages[hash] = message
	c.keyhistory = append(c.keyhistory[deleted:], hash)
}

func (c *channelMapValue) DuplicatePresent(message *discordgo.Message, maxDuplicateAge time.Duration) bool {
	c.Lock()
	defer c.Unlock()

	hash := hashMessage(message)
	oldmsg, present := c.messages[hash]
	if !present {
		return false
	}
	timestamp, err := oldmsg.Timestamp.Parse()
	return err == nil && timestamp.After(time.Now().Add(-maxDuplicateAge))
}

func hashMessage(m *discordgo.Message) uint32 {
	h := fnv.New32a()
	h.Write([]byte(m.Content))
	return h.Sum32()
}
