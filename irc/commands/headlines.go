package commands

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"aibird/irc/state"
	"aibird/queue"
	"aibird/settings"
	"aibird/shared/meta"
)

var (
	headlinesMu        sync.RWMutex
	headlinesCache     []string
	headlinesCacheTime time.Time
	processedHeadlines = make(map[string]bool)
)

type RedditResponse struct {
	Data struct {
		Children []struct {
			Data struct {
				Title string `json:"title"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func fetchRedditHeadlines(proxy settings.Proxy) ([]string, error) {
	headlinesMu.RLock()
	if time.Since(headlinesCacheTime) < time.Hour {
		cached := make([]string, len(headlinesCache))
		copy(cached, headlinesCache)
		headlinesMu.RUnlock()
		return cached, nil
	}
	headlinesMu.RUnlock()

	req, err := http.NewRequest("GET", "https://old.reddit.com/r/worldnews/new.json", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request for headlines: %w", err)
	}
	// Reddit requires a custom User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0")

	var resp *http.Response
	if proxy.Host != "" && proxy.Port != "" {
		proxyStr := fmt.Sprintf("http://%s:%s@%s:%s", proxy.User, proxy.Pass, proxy.Host, proxy.Port)
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing proxy URL: %w", err)
		}

		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			Timeout: 30 * time.Second,
		}
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching headlines from Reddit via proxy: %w", err)
		}
	} else {
		client := &http.Client{Timeout: 10 * time.Second}
		var err error
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching headlines from Reddit: %w", err)
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var redditResponse RedditResponse
	if err := json.Unmarshal(body, &redditResponse); err != nil {
		return nil, fmt.Errorf("error parsing headlines from Reddit: %w", err)
	}

	if len(redditResponse.Data.Children) == 0 {
		return nil, fmt.Errorf("no headlines found")
	}

	var titles []string
	for _, child := range redditResponse.Data.Children {
		titles = append(titles, child.Data.Title)
	}

	headlinesMu.Lock()
	headlinesCache = titles
	headlinesCacheTime = time.Now()
	processedHeadlines = make(map[string]bool)
	headlinesMu.Unlock()

	return titles, nil
}

// callAIAndSend sends a "please wait" message and calls the AI with the given prompt.
// Used by commands that were queued (the AI call runs inside the queue processor).
func callAIAndSend(irc state.State, prompt string, message string) {
	irc.Send(fmt.Sprintf("%s, %s", irc.User.NickName, message))

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	answer, err := singleRequestWithFallback("", prompt, irc.Config)
	if err != nil {
		irc.Send("Error getting AI response: " + err.Error())
		return
	}

	irc.Send(answer)
}

// queueAIAndSend enqueues an AI text generation request through the GPU queue.
// The Reddit fetch or other prep work should be done before calling this.
func queueAIAndSend(irc state.State, q *queue.ProcessingQueue, prompt string, message string, model string) {
	if q == nil {
		callAIAndSend(irc, prompt, message)
		return
	}

	queueItem := queue.QueueItem{
		Item: queue.Item{
			State: irc,
			Function: func(ctx context.Context, s state.State, gpu meta.GPUType) {
				callAIAndSend(s, prompt, message)
			},
		},
		Model: model,
		User:  irc.User,
		GPU:   meta.GPU4090,
	}

	msg, err := q.Enqueue(queueItem)
	if err != nil {
		irc.SendError(err.Error())
	} else if msg != "" {
		irc.Send(msg)
	}
}

func ParseHeadlines(irc state.State, q *queue.ProcessingQueue) {
	go func() {
		headlines, err := fetchRedditHeadlines(irc.Config.AiBird.Proxy)
		if err != nil {
			irc.Send(err.Error())
			return
		}

		var titles []string
		for i, headline := range headlines {
			if i >= 25 {
				break
			}
			titles = append(titles, headline)
		}

		if len(titles) == 0 {
			irc.Send("No headlines found.")
			return
		}

		allTitles := strings.Join(titles, "\n")
		prompt := fmt.Sprintf("As a man who is skeptical and think the satanists control everything, summarize the following headlines into a single, concise paragraph blaming the satanists and the illuminati:\n\n%s", allTitles)
		message := "fetching a summary of the latest headlines..."

		queueAIAndSend(irc, q, prompt, message, "headlies")
	}()
}

func ParseIrcNews(irc state.State, q *queue.ProcessingQueue) {
	go func() {
		headlines, err := fetchRedditHeadlines(irc.Config.AiBird.Proxy)
		if err != nil {
			irc.Send(err.Error())
			return
		}

		var resetMsg string
		headlinesMu.Lock()
		var availableHeadlines []string
		for _, h := range headlines {
			if _, exists := processedHeadlines[h]; !exists {
				availableHeadlines = append(availableHeadlines, h)
			}
		}

		if len(availableHeadlines) == 0 && len(headlines) > 0 {
			// All headlines have been processed, reset the map
			processedHeadlines = make(map[string]bool)
			availableHeadlines = headlines
			resetMsg = "All headlines have been used, starting over."
		}

		if len(availableHeadlines) == 0 {
			headlinesMu.Unlock()
			irc.Send("No headlines available to process.")
			return
		}

		// Use crypto/rand for secure random number generation
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(availableHeadlines))))
		if err != nil {
			headlinesMu.Unlock()
			irc.Send("Error generating random headline")
			return
		}
		randomHeadline := availableHeadlines[randomIndex.Int64()]
		processedHeadlines[randomHeadline] = true
		headlinesMu.Unlock()

		if resetMsg != "" {
			irc.Send(resetMsg)
		}

		prompt := fmt.Sprintf(`Rewrite the following real-world news headline into a single, creative, and humorous IRC-themed headline. The theme must be based on the culture and lore of the EFNet IRC network.

Here are the rules for the rewrite:
1.  **One Headline Only:** Your entire response must be ONLY the single rewritten headline. Do not provide options, explanations, or any text other than the final headline.
2.  **Replace Countries with Channels:** Map country names to famous EFNet channel names from this list: #lrh, #birdnest, #evildojo, #efnetnews, #h4x, #warez, #chat, #help, #hrl, #wyzrds-tower, #dragonflybsd, #bex, #mircart.
3.  **Replace People with Nicks:** Map names of leaders, groups, or individuals to well-known EFNet user nickname from this list: darkmage, l0de, bex, ralph, jrra, kuntz, moony, sniff, astro, anji, b-rex, canada420, clamkin, skg, gary, beenz, deakin, interdome, syn, darkness, vae, gowce, moneytree, Retarded, spoon, sylar, stovepipe, morthrane, chrono, acidvegas, again, hgc, durendal, knio, mavericks, pyrex, sh, irie, seirdy, sq, stratum, WeEatnKid, dieforirc, tater, buttvomit, luldangs, MichealK, AnalMan, poccri, vap0r, kakama, fregyXin, kayos, stovepipe, Audasity, PsyMaster, perplexa, alyosha, Darn, efsenable, EchoShun, dumbguy, phobos, COMPUTERS, dave, nance, sthors, X-Bot, lamer, ChanServ.
4.  **Translate Actions to IRC Events:** Convert real-world actions into IRC equivalents. For example:
    *   **Military Conflict:** A "channel takeover," "flame war," "mass-kick script," "DDoS attack," or a "netsplit" for a major war.
    *   **Military Action (Missile, Bomb, Strike):** A "malicious script," "flood bot," "CTCP flood," or a user being "/killed" by an op.
    *   **Defense/Interception:** A "kick/ban" (+b), an op using "/kill," a server-wide "K-line" or "G-line," or a "clone block."
    *   **Diplomacy/Negotiations:** A "private message (/query)," an "op meeting," or someone getting "opped" (+o).
    *   **Sanctions/Penalties:** A channel "ban" (+b), a "server-wide K-line/G-line," being "shunned," or added to a "shitlist."
    *   **Protests/Uprisings:** A "mass-join," "spamming slogans," users "mass-parting," or a "revolt against the channel founder."
    *   **Espionage/Spying:** "Lurking," using "/whois," "social engineering an op," or sniffing DCC traffic.
    *   **Alliances/Treaties:** Linking two servers, sharing a "ban list," adding friendly bots, or forming a "council of ops."
    *   **Economic/Financial Events:**
        *   **Economy/Trade:** "DCC file trading," "XDCC pack serving," or "bot currency transfers."
        *   **Economic Crisis:** "Channel is dead," "everyone is /away," or a "netsplit wiped out the user list."
    *   **Legal/Political Events:**
        *   **Elections:** "Ops holding a vote for founder," or a "poll in the topic."
        *   **Legislation:** "New channel rule (+R) set," or "topic updated with new policies."
        *   **Scandal/Corruption:** "Op caught sharing chan keys," or a "DCC transfer was intercepted."
    *   **Technology/Cybersecurity Events:**
        *   **New Invention:** "A new TCL script was released," or "a new mIRC version is out."
        *   **Data Breach:** "User list was leaked," or "server passwords compromised."
    *   **Disasters/Infrastructure Failures:**
        *   **Natural Disaster:** A "server crash," "massive lag," or the "main server going down."

Headline to rewrite: %s`, randomHeadline)
		message := "Getting the latest IRC news..."

		queueAIAndSend(irc, q, prompt, message, "ircnews")
	}()
}
