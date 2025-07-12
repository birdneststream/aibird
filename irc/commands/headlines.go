package commands

import (
	"aibird/irc/state"
	"aibird/settings"
	"aibird/text/gemini"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
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
	if time.Since(headlinesCacheTime) < time.Hour {
		return headlinesCache, nil
	}

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

	headlinesCache = titles
	headlinesCacheTime = time.Now()
	processedHeadlines = make(map[string]bool) // Reset processed headlines when we fetch new ones

	return titles, nil
}

func callGeminiAndSend(irc state.State, prompt string, message string) {
	irc.Send(fmt.Sprintf("%s, %s", irc.User.NickName, message))

	if irc.Config.Gemini.ApiKey == "" {
		irc.Send("Error: Gemini API key is not configured.")
		return
	}

	answer, err := gemini.SingleRequest(prompt, irc.Config.Gemini)
	if err != nil {
		irc.Send("Error getting summary from AI.")
		return
	}

	irc.Send(answer)
}

func ParseHeadlines(irc state.State) {
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

		callGeminiAndSend(irc, prompt, message)
	}()
}

func ParseIrcNews(irc state.State) {
	go func() {
		headlines, err := fetchRedditHeadlines(irc.Config.AiBird.Proxy)
		if err != nil {
			irc.Send(err.Error())
			return
		}

		var availableHeadlines []string
		for _, h := range headlines {
			if _, exists := processedHeadlines[h]; !exists {
				availableHeadlines = append(availableHeadlines, h)
			}
		}

		if len(availableHeadlines) == 0 && len(headlines) > 0 {
			// All headlines have been processed, reset the map
			processedHeadlines = make(map[string]bool)
			// and refill availableHeadlines
			availableHeadlines = headlines
			irc.Send("All headlines have been used, starting over.")
		}

		if len(availableHeadlines) == 0 {
			irc.Send("No headlines available to process.")
			return
		}

		// Use crypto/rand for secure random number generation
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(availableHeadlines))))
		if err != nil {
			irc.Send("Error generating random headline")
			return
		}
		randomHeadline := availableHeadlines[randomIndex.Int64()]
		processedHeadlines[randomHeadline] = true

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

		callGeminiAndSend(irc, prompt, message)
	}()
}
