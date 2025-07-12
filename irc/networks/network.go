package networks

import (
	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/channels"
	"aibird/irc/servers"
	"aibird/irc/users"
	"aibird/logger"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"
)

func (n *Network) String() string {
	return fmt.Sprintf("{b}Enabled{b}%s, {b}NetworkName{b}: %s, {b}Nick{b}: %s, {b}User{b}: %s, {b}Name{b}: %s, {b}ModesAtOnce{b}: %d, {b}PingDelay{b}: %d, {b}Version{b}: %s, {b}Throttle{b}: %d, {b}Burst{b}: %d, {b}ActionTrigger{b}: %s, {b}Users{b}: %d, {b}Channels{b}: %d, {b}Servers{b}: %d, {b}AdminHosts{b}: %d",
		helpers.StringToStatusIndicator(strconv.FormatBool(n.Enabled)),
		n.NetworkName,
		n.Nick,
		n.User,
		n.Name,
		n.GetModesAtOnce(),
		n.PingDelay,
		n.Version,
		n.Throttle,
		n.Burst,
		n.ActionTrigger,
		len(n.Users),
		len(n.Channels),
		len(n.Servers),
		len(n.AdminHosts))
}

func (n *Network) GetRandomServer() *servers.Server {
	if len(n.Servers) == 0 {
		return nil // or handle the error as appropriate
	}
	// Use crypto/rand for secure random number generation
	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(n.Servers))))
	if err != nil {
		return nil // fallback to first server if random generation fails
	}
	return &n.Servers[randomIndex.Int64()]
}

func (n *Network) ProvideStateInit(channelName string, ident string, host string) (*channels.Channel, *users.User) {
	return n.GetNetworkChannel(channelName), n.GetUserWithIdentAndHost(ident, host)
}

func (n *Network) GetNetworkChannel(channelName string) *channels.Channel {
	for i, channel := range n.Channels {
		if channel.Name == channelName {
			return &n.Channels[i]
		}
	}
	return nil
}

func (n *Network) GetUserWithIdentAndHost(ident string, host string) *users.User {
	var foundUsers []*users.User
	for i := range n.Users {
		if n.Users[i].Ident == ident && n.Users[i].Host == host {
			foundUsers = append(foundUsers, &n.Users[i])
		}
	}

	if len(foundUsers) == 0 {
		return nil
	}

	if len(foundUsers) == 1 {
		return foundUsers[0]
	}

	latestUser := foundUsers[0]
	for _, user := range foundUsers {
		if user.LatestActivity > latestUser.LatestActivity {
			latestUser = user
		}
	}

	return latestUser
}

func (n *Network) GetUserWithNick(nick string) *users.User {
	var foundUsers []*users.User
	for i := range n.Users {
		if n.Users[i].NickName == nick {
			foundUsers = append(foundUsers, &n.Users[i])
		}
	}

	if len(foundUsers) == 0 {
		return nil
	}

	if len(foundUsers) == 1 {
		return foundUsers[0]
	}

	latestUser := foundUsers[0]
	for _, user := range foundUsers {
		if user.LatestActivity > latestUser.LatestActivity {
			latestUser = user
		}
	}

	return latestUser
}

func (n *Network) GetModesAtOnce() int {
	if n.ModesAtOnce == 0 {
		return 4
	}
	return n.ModesAtOnce
}

func (n *Network) IsNickIgnored(nick string) bool {
	for _, ignoredNick := range n.IgnoredNicks {
		if ignoredNick == nick {
			return true
		}
	}
	return false
}

func (n *Network) IsIdentHostAdmin(ident string, host string) bool {
	for _, admin := range n.AdminHosts {
		if admin.Host == host && admin.Ident == ident {
			return true
		}
	}
	return false
}

func (n *Network) IsIdentHostOwner(ident string, host string) bool {
	for _, admin := range n.AdminHosts {
		if admin.Host == host && admin.Ident == ident && admin.Owner {
			return true
		}
	}
	return false
}

func (n *Network) Save() {
	if n.SaveTimer == nil {
		n.SaveTimer = time.NewTimer(0)
		// Drain initial timer
		if !n.SaveTimer.Stop() {
			<-n.SaveTimer.C
		}
	} else {
		if !n.SaveTimer.Stop() {
			select {
			case <-n.SaveTimer.C:
			default:
			}
		}
	}
	n.SaveTimer.Reset(3 * time.Second)

	go func() {
		<-n.SaveTimer.C
		userKey := n.NetworkName + "_users"
		usersJson, _ := json.Marshal(n.Users)
		if err := birdbase.PutBytes(userKey, usersJson); err != nil {
			logger.Error("Error saving network", "error", err)
		}
	}()
}

func (n *Network) Load() {
	userKey := n.NetworkName + "_users"

	if birdbase.Has(userKey) {
		usersJson, err := birdbase.Get(userKey)
		if err != nil {
			logger.Error("Error loading network from birdbase", "key", userKey, "error", err)
		}
		err = json.Unmarshal(usersJson, &n.Users)
		if err != nil {
			logger.Error("Error unmarshalling network users", "key", userKey, "error", err)
		}
	}
}
