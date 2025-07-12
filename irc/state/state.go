package state

import (
	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/irc/users"
	"aibird/settings"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lrstanley/girc"
	"golang.org/x/crypto/sha3"
)

func (s *State) String() string {
	return girc.Fmt(fmt.Sprintf("{b}Channel{b}: %s, {b}User{b}: %s, {b}Command{b}: %s, {b}Arguments{b}: %s",
		s.Channel,
		s.User,
		s.Command,
		s.Arguments))

}

func (s *State) IsSelf() bool {
	return s.Event.Source.Name == s.Network.Nick
}

func (s *State) SendError(response string) {
	s.Client.Cmd.Reply(s.Event, girc.Fmt("{b}{red}[ERROR] {reset}"+response))
}

func (s *State) SendSuccess(response string) {
	s.Client.Cmd.Reply(s.Event, girc.Fmt("{b}{green}[SUCCESS] {reset}"+response))
}

func (s *State) SendInfo(response string) {
	s.Client.Cmd.Reply(s.Event, girc.Fmt("{b}{blue}[INFO] {reset}"+response))
}

func (s *State) SendWarning(response string) {
	s.Client.Cmd.Reply(s.Event, girc.Fmt("{b}{yellow}[WARNING] {reset}"+response))
}

func (s *State) Action() string {
	return s.Command.Action
}

func (s *State) IsAction(action string) bool {
	return s.Command.Action == action
}

func (s *State) Message() string {
	return s.Command.Message
}

func (s *State) SetMessage(message string) {
	s.Command.Message = message
}

func (s *State) IsMessage(message string) bool {
	return s.Command.Message == message
}

func (s *State) IsEmptyMessage() bool {
	return s.Command.Message == ""
}

func (s *State) IsEmptyArguments() bool {
	return len(s.Arguments) == 0
}

func (s *State) UserCacheKey(extra string) string {
	return s.Event.Source.Ident + s.Event.Source.Host + s.Network.NetworkName + extra
}

func (s *State) UserAiChatCacheKey() string {
	basePromptHash := sha3.Sum224([]byte(s.User.GetBasePrompt()))
	hashValue := hex.EncodeToString(basePromptHash[:])
	return s.UserCacheKey(s.User.AiService + hashValue)
}

func (s *State) ShouldTrimOutput(message string) bool {
	return (s.Channel.TrimOutput && len(message) > 350) || strings.Contains(message, "<think>")
}

func (s *State) FindArgument(name string, def interface{}) interface{} {
	for _, arg := range s.Arguments {
		if arg.Key == name {
			return arg.Value
		}
	}
	return def
}

func (s *State) GetStringArg(name, def string) (string, bool) {
	val := s.FindArgument(name, def)
	str, ok := val.(string)
	if !ok {
		return def, false
	}
	return str, true
}

func (s *State) GetIntArg(name string, def int) (int, bool) {
	val := s.FindArgument(name, def)
	// Also handle string-to-int conversion for robustness
	if strVal, ok := val.(string); ok {
		intVal, err := strconv.Atoi(strVal)
		if err == nil {
			return intVal, true
		}
	}

	intVal, ok := val.(int)
	if !ok {
		return def, false
	}
	return intVal, true
}

func (s *State) GetBoolArg(name string) bool {
	val := s.FindArgument(name, false)
	boolVal, _ := val.(bool)
	return boolVal
}

func (s *State) GetArguments() []Argument {
	return s.Arguments
}

// ParseArguments extracts key-value and boolean arguments from the message.
// Arguments are expected in the format: --key=value, --key="a value with spaces", or --verbose.
func (s *State) ParseArguments() {
	if s.Arguments == nil {
		s.Arguments = make([]Argument, 0)
	}

	words := strings.Fields(s.Message())
	var newMessageWords []string
	i := 0

	for i < len(words) {
		word := words[i]

		if !strings.HasPrefix(word, "--") {
			newMessageWords = append(newMessageWords, word)
			i++
			continue
		}

		// It's an argument
		parts := strings.SplitN(word[2:], "=", 2)
		key := parts[0]

		if len(parts) == 1 {
			// Boolean flag like --verbose
			s.Arguments = append(s.Arguments, Argument{Key: key, Value: true})
			i++
			continue
		}

		// Key-value argument like --key=value
		valueStr := parts[1]
		if (strings.HasPrefix(valueStr, "'") && strings.HasSuffix(valueStr, "'")) ||
			(strings.HasPrefix(valueStr, "\"") && strings.HasSuffix(valueStr, "\"")) {
			// Quoted value on the same word, like --key="value"
			s.Arguments = append(s.Arguments, Argument{Key: key, Value: valueStr[1 : len(valueStr)-1]})
			i++
			continue
		}

		if strings.HasPrefix(valueStr, "'") || strings.HasPrefix(valueStr, "\"") {
			// Quoted value that might span multiple words, like --key="a b c"
			quoteChar := string(valueStr[0])
			valueParts := []string{valueStr[1:]}
			i++
			for i < len(words) {
				part := words[i]
				if strings.HasSuffix(part, quoteChar) {
					valueParts = append(valueParts, part[:len(part)-1])
					break
				}
				valueParts = append(valueParts, part)
				i++
			}
			s.Arguments = append(s.Arguments, Argument{Key: key, Value: strings.Join(valueParts, " ")})
			i++
			continue
		}

		// Unquoted value, like --key=value
		s.Arguments = append(s.Arguments, Argument{Key: key, Value: valueStr})
		i++
	}

	// Reconstruct the message without the arguments
	s.SetMessage(strings.Join(newMessageWords, " "))
}

func (s *State) ReplyTo(message string) {
	s.Client.Cmd.ReplyTo(s.Event, message)
}

func (s *State) Send(message string) {
	message = helpers.MarkdownToIrc(message)

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(message, "\n") {
		sendString := ""
		// Remove blank or one/two char lines
		if len(line) <= 2 {
			continue
		}

		// split line into chunks slice with space
		chunks := strings.Split(line, " ")

		// for each chunk
		for _, chunk := range chunks {

			// append chunk to sendString
			sendString += chunk + " "

			// Trim by words for a cleaner output
			if len(sendString) > 450 {

				s.Client.Cmd.Reply(s.Event, sendString)
				sendString = ""
			}
		}

		s.Client.Cmd.Reply(s.Event, sendString)
	}
}

func (s *State) Verify() error {
	// User already verified to use the bot in PM
	if s.Event.IsFromUser() {
		channelUser := s.Network.GetUserWithIdentAndHost(s.Event.Source.Ident, s.Event.Source.Host)

		newChannel := &channels.Channel{
			Name:  s.Event.Source.Name,
			Users: []*users.User{channelUser},
			Ai:    true,
			Sd:    true,
		}

		s.Channel = newChannel
		s.User = channelUser
	}

	if s.Network.NetworkName == "soyjak" && !s.User.HasAnyMode() {
		return errors.New("no modes is not allowed on soyjak")
	}

	if s.Channel == nil || s.User == nil || s.IsSelf() {
		return errors.New("not enough parameters in s.event.Params to proceed")
	}

	if s.User.Ignored {
		return errors.New("user is ignored")
	}

	s.User.Touch(s.Event.Last())
	action := s.Event.Last()

	if !strings.HasPrefix(action, s.GetActionTrigger()) {
		return errors.New("no action trigger")
	}

	// Extract command name for validation
	tmpAction := strings.TrimPrefix(action, s.GetActionTrigger())
	tmpParts := strings.SplitN(tmpAction, " ", 2)
	cmdName := strings.TrimSpace(tmpParts[0])

	// Check if this is a valid command
	isValidCommand := s.ValidateCommand(cmdName)

	if !isValidCommand {
		return errors.New("invalid command")
	}

	if !strings.HasPrefix(action, s.GetActionTrigger()) {
		s.Command = Command{Action: "", Message: ""}
		return nil
	}

	// Only check for flood if the command is valid
	if s.MessageFloodCheck() {
		return errors.New("flood check")
	}

	if s.User.GetAccessLevel() < 2 {
		nagUserToGiveMoney(*s)
	}

	action = strings.TrimPrefix(action, s.GetActionTrigger())
	parts := strings.SplitN(action, " ", 2)

	message := ""
	if len(parts) > 1 {
		// If there's a prompt, update message
		message = strings.TrimSpace(parts[1])
	}

	s.Command = Command{Action: strings.TrimSpace(parts[0]), Message: strings.TrimSpace(message)}

	s.ParseArguments()

	return nil
}

func (s *State) ShouldPreserveModes() bool {
	if s.Channel.PreserveModes {
		return s.Channel.PreserveModes
	}

	return s.Network.PreserveModes
}

func Init(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) State {
	s := State{
		Client:  c,
		Event:   e,
		Network: network,
		Config:  config,
	}

	s.Channel, s.User = network.ProvideStateInit(helpers.FindChannelNameInEventParams(e), e.Source.Ident, e.Source.Host)

	return s
}

func nagUserToGiveMoney(s State) {
	if s.User.GetAccessLevel() > 0 {
		return
	}

	key := s.User.Ident + s.User.Host + "nag"

	if birdbase.Has(key) {
		// get value from key as int
		counter, _ := birdbase.Get(key)
		noOfUses, _ := strconv.Atoi(string(counter))

		noOfUses = noOfUses + 1

		if noOfUses > 50 {
			s.Send("Hey there chat pal " + s.User.NickName + " thanks for using aibird! Please support if you can https://www.patreon.com/birdnestlive or !support for more.")
			_ = birdbase.Delete(key)
			return
		}

		_ = birdbase.PutInt(key, noOfUses)
	} else {
		_ = birdbase.PutInt(key, 0)
	}
}
