package comfyui

import (
	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/text/gemini"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"

	"aibird/shared/meta"

	"github.com/richinsley/comfy2go/client"
	"github.com/schollz/progressbar/v3"
)

// Define a local struct for status.AibirdMeta
// Only the fields needed for access/routing

type StatusAibirdMeta struct {
	AccessLevel int
	BigModel    bool
}

// getPortByName searches for a port by name in the ComfyUiConfig.
// It returns the port string and a boolean indicating if it was found.
func getPortByName(config settings.ComfyUiConfig, name string) (int, bool) {
	for _, p := range config.Ports {
		if p.Name == name {
			return p.Port, true
		}
	}
	return 0, false
}

func freeVram(clientAddr string, clientPort int) error {
	url := fmt.Sprintf("http://%s:%d/free", clientAddr, clientPort)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("could not create free request: %w", err)
	}

	// It seems we need to include the client_id, but the value doesn't matter
	req.Header.Set("Content-Type", "application/json")
	body := `{"unload_models": true, "free_memory": true}`
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not send free request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("free request failed with status: %s", resp.Status)
	}

	logger.Info("Successfully sent free VRAM request to ComfyUI")
	return nil
}

func Process(irc state.State, aiEnhancedPrompt string, gpu meta.GPUType) (string, error) {
	logger.Debug("Starting comfyui.Process", "gpu", gpu, "action", irc.Action())
	comfyUiConfig := irc.Config.ComfyUi
	model := irc.Action()
	workflowFile := "comfyuijson/" + model + ".json"
	metaData, err := GetAibirdMeta(workflowFile)
	if err == nil {
		logger.Info("Using V2 metadata-driven processing", "model", model)
		if irc.User.GetAccessLevel() < metaData.AccessLevel {
			logger.Error("Access level too low", "required", metaData.AccessLevel, "user", irc.User.GetAccessLevel())
			return "", fmt.Errorf("⛔️ Sorry, you need access level %d to use this command. Check !support for more info", metaData.AccessLevel)
		}
		var clientPort int
		var portFound bool
		if gpu == meta.GPU4090 {
			clientPort, portFound = getPortByName(comfyUiConfig, "4090")
		} else {
			clientPort, portFound = getPortByName(comfyUiConfig, "2070")
		}
		if !portFound {
			if len(comfyUiConfig.Ports) > 0 {
				clientPort = comfyUiConfig.Ports[0].Port
				portFound = true
			}
		}
		if !portFound {
			logger.Error("No ComfyUI ports configured")
			return "", errors.New("no ComfyUI ports configured")
		}
		clientAddr := comfyUiConfig.Url
		defer func() {
			if err := freeVram(clientAddr, clientPort); err != nil {
				logger.Error("Error freeing VRAM", "error", err)
			}
		}()
		var message string
		if !irc.IsAction("tts") {
			message = CleanPrompt(irc.Message())
		} else {
			message = irc.Message()
		}
		if BadWordsCheck(message, comfyUiConfig) {
			message = comfyUiConfig.BadWordsPrompt
		}
		if aiEnhancedPrompt != "" {
			message = aiEnhancedPrompt
		}

		// Create a map to hold the widget values that need to be updated
		widgetUpdates := make(map[string]map[int]interface{})

		// --- Process Prompt ---
		if metaData.PromptTarget.Node != "" {
			if _, ok := widgetUpdates[metaData.PromptTarget.Node]; !ok {
				widgetUpdates[metaData.PromptTarget.Node] = make(map[int]interface{})
			}
			widgetUpdates[metaData.PromptTarget.Node][metaData.PromptTarget.WidgetIndex] = message
		}

		// --- Generic Parameter Processing ---
		for paramName, paramDef := range metaData.Parameters {
			rawUserInput := irc.FindArgument(paramName, "").(string)
			userInputProvided := rawUserInput != ""

			// Special pre-flight check for image URLs to give users faster feedback
			if paramName == "img" && userInputProvided {
				// Validate URL to prevent SSRF attacks
				if !strings.HasPrefix(rawUserInput, "http://") && !strings.HasPrefix(rawUserInput, "https://") {
					errMsg := fmt.Sprintf("⚠️ Invalid URL scheme for --img: %s", rawUserInput)
					return "", errors.New(errMsg)
				}

				logger.Debug("Performing pre-flight check for image URL", "url", rawUserInput)
				resp, err := http.Head(rawUserInput)
				if err != nil {
					errMsg := fmt.Sprintf("⚠️ Failed to reach the image URL for --img: %v", err)
					return "", errors.New(errMsg)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errMsg := fmt.Sprintf("⚠️ The image URL for --img appears to be invalid (server response: %s). Please check the link.", resp.Status)
					return "", errors.New(errMsg)
				}
				logger.Debug("Image URL check passed", "status", resp.Status)
			}

			var finalValue interface{}

			if !userInputProvided {
				finalValue = paramDef.Default
			} else {
				var parseErr error
				switch paramDef.Type {
				case "string":
					finalValue = rawUserInput
				case "int":
					val, parseErr := strconv.ParseInt(rawUserInput, 10, 64)
					if parseErr == nil {
						finalValue = val
						// Perform validation
						if paramDef.Min != nil && float64(val) < *paramDef.Min {
							errMsg := fmt.Sprintf("⚠️ Value for --%s is too low. Minimum is %g, but you gave %d.", paramName, *paramDef.Min, val)
							return "", errors.New(errMsg)
						}
						if paramDef.Max != nil && float64(val) > *paramDef.Max {
							errMsg := fmt.Sprintf("⚠️ Value for --%s is too high. Maximum is %g, but you gave %d.", paramName, *paramDef.Max, val)
							return "", errors.New(errMsg)
						}
					}
				case "float":
					val, parseErr := strconv.ParseFloat(rawUserInput, 64)
					if parseErr == nil {
						finalValue = val
						// Perform validation
						if paramDef.Min != nil && val < *paramDef.Min {
							errMsg := fmt.Sprintf("⚠️ Value for --%s is too low. Minimum is %g, but you gave %g.", paramName, *paramDef.Min, val)
							return "", errors.New(errMsg)
						}
						if paramDef.Max != nil && val > *paramDef.Max {
							errMsg := fmt.Sprintf("⚠️ Value for --%s is too high. Maximum is %g, but you gave %g.", paramName, *paramDef.Max, val)
							return "", errors.New(errMsg)
						}
					}
				case "lyrics":
					lyricsPrompt := rawUserInput
					var lyrics string
					var lyErr error
					if lyricsPrompt == "" {
						if paramDef.Default != nil {
							lyrics = paramDef.Default.(string)
						} else {
							lyrics = ""
						}
					} else {
						if strings.HasPrefix(lyricsPrompt, "http") && strings.HasSuffix(lyricsPrompt, ".txt") {
							irc.Send("📜 Downloading lyrics from URL! ✨")
							resp, httpErr := http.Get(lyricsPrompt)
							if httpErr != nil {
								return "", fmt.Errorf("failed to download lyrics from URL: %w", httpErr)
							}
							defer resp.Body.Close()

							if resp.StatusCode != http.StatusOK {
								return "", fmt.Errorf("failed to download lyrics from URL: status code %d", resp.StatusCode)
							}

							bodyBytes, ioErr := io.ReadAll(resp.Body)
							if ioErr != nil {
								return "", fmt.Errorf("failed to read lyrics from response body: %w", ioErr)
							}
							lyrics = string(bodyBytes)
						} else {
							irc.Send("✍️ Generating lyrics with ai! ✨")
							lyrics, lyErr = gemini.GenerateLyrics(lyricsPrompt, irc.Config.Gemini)
							if lyErr != nil {
								return "", fmt.Errorf("failed to generate lyrics: %w", lyErr)
							}
						}
					}
					finalValue = lyrics
					parseErr = nil
				default:
					return "", fmt.Errorf("unsupported parameter type '%s' in metadata for '%s'", paramDef.Type, paramName)
				}
				if parseErr != nil {
					errMsg := fmt.Sprintf("⚠️ Invalid value for --%s. Expected a %s, but got '%s'.", paramName, paramDef.Type, rawUserInput)
					return "", errors.New(errMsg) // also return error to stop processing
				}
			}

			// Handle special case for seed randomization
			if paramName == "seed" && !userInputProvided {
				// Use crypto/rand for secure random number generation
				seed, err := rand.Int(rand.Reader, big.NewInt(1<<63-1))
				if err != nil {
					return "", fmt.Errorf("failed to generate random seed: %w", err)
				}
				finalValue = seed.Int64()
			}

			// Handle special case for voice filename to add .wav suffix
			if paramName == "voice" {
				if voiceStr, ok := finalValue.(string); ok && !strings.HasSuffix(voiceStr, ".wav") {
					finalValue = voiceStr + ".wav"
				}
			}

			// Apply value to all targets, if a value was determined
			if finalValue != nil {
				for _, target := range paramDef.Targets {
					if _, ok := widgetUpdates[target.Node]; !ok {
						widgetUpdates[target.Node] = make(map[int]interface{})
					}
					logger.Debug("Setting parameter", "param", paramName, "node", target.Node, "widget", target.WidgetIndex, "value", finalValue)
					widgetUpdates[target.Node][target.WidgetIndex] = finalValue
				}
			}
		}

		// --- Process Hardcoded Values ---
		for paramName, hardcodedDef := range metaData.Hardcoded {
			finalValue := hardcodedDef.Value
			if finalValue != nil {
				for _, target := range hardcodedDef.Targets {
					if _, ok := widgetUpdates[target.Node]; !ok {
						widgetUpdates[target.Node] = make(map[int]interface{})
					}
					logger.Debug("Setting hardcoded parameter", "param", paramName, "node", target.Node, "widget", target.WidgetIndex, "value", finalValue)
					widgetUpdates[target.Node][target.WidgetIndex] = finalValue
				}
			}
		}

		// Create ComfyUI client
		c := client.NewComfyClient(clientAddr, clientPort, nil)
		if !c.IsInitialized() {
			if err := c.Init(); err != nil {
				return "", fmt.Errorf("error initializing client: %w", err)
			}
		}

		// Load the workflow graph
		graph, _, err := c.NewGraphFromJsonFile(workflowFile)
		if err != nil {
			return "", fmt.Errorf("error loading graph JSON: %w", err)
		}

		// Get only the nodes in the "API" group
		apiNodes := graph.GetNodesInGroup(graph.GetGroupWithTitle("API"))

		// Apply the updates to the graph nodes
		for _, node := range apiNodes {
			updates, typeExists := widgetUpdates[node.Type]
			if !typeExists {
				updates = widgetUpdates[node.Title]
			}

			if typeExists || (updates != nil) {
				if values, ok := node.WidgetValues.([]interface{}); ok {
					for widgetIndex, value := range updates {
						if widgetIndex < len(values) {
							// Special handling for the original prompt which might be a concatenation
							if (node.Title == metaData.PromptTarget.Node || node.Type == metaData.PromptTarget.Node) && widgetIndex == metaData.PromptTarget.WidgetIndex {
								if originalPrompt, ok := values[widgetIndex].(string); ok && originalPrompt != "" {
									values[widgetIndex] = originalPrompt + " " + value.(string)
								} else {
									values[widgetIndex] = value
								}
							} else {
								values[widgetIndex] = value
							}
							logger.Debug("Set widget value", "widget", widgetIndex, "node", node.Title, "type", node.Type, "value", value)
						}
					}
				}
			}
		}

		// Queue the prompt
		item, err := c.QueuePrompt(graph)
		if err != nil {
			return "", fmt.Errorf("failed to queue prompt: %w", err)
		}

		// --- Handle Queue and Get Result ---
		var bar *progressbar.ProgressBar = nil
		var currentNodeTitle string
		for continueLoop := true; continueLoop; {
			msg := <-item.Messages
			switch msg.Type {
			case "started":
				qm := msg.ToPromptMessageStarted()
				logger.Info("Start executing prompt", "prompt_id", qm.PromptID)
			case "executing":
				bar = nil
				qm := msg.ToPromptMessageExecuting()
				currentNodeTitle = qm.Title
				logger.Debug("Executing node", "node_id", qm.NodeID)
			case "progress":
				qm := msg.ToPromptMessageProgress()
				if bar == nil {
					bar = progressbar.Default(int64(qm.Max), currentNodeTitle)
				}
				bar.Set(qm.Value)
			case "stopped":
				qm := msg.ToPromptMessageStopped()
				if qm.Exception != nil {
					return "", fmt.Errorf("execution stopped with exception: %s: %s", qm.Exception.ExceptionType, qm.Exception.ExceptionMessage)
				}
				continueLoop = false
			case "data":
				qm := msg.ToPromptMessageData()
				for k, v := range qm.Data {
					if k == "images" || k == "gifs" || k == "audio" {
						for _, output := range v {
							img_data, err := c.GetImage(output)
							if err != nil {
								return "", fmt.Errorf("failed to get image: %w", err)
							}
							f, err := os.Create(output.Filename)
							if err != nil {
								return "", fmt.Errorf("failed to write image: %w", err)
							}
							f.Write(*img_data)
							f.Close()

							// Example of a post-generation action, can be made generic later
							if strings.Contains(model, "wan") && irc.User.GetAccessLevel() <= 2 {
								cacheKey := fmt.Sprintf("img2wan_%s", irc.User.NickName)
								err := birdbase.PutStringExpireSeconds(cacheKey, "1", 60*60*3)
								if err != nil {
									logger.Error("Failed to set cache key", "error", err)
								}
							}

							return output.Filename, nil
						}
					}
				}
			}
		}

		logger.Debug("Finishing comfyui.Process", "gpu", gpu, "action", irc.Action())
		return "", errors.New("error processing comfyui: no output file received")
	}
	if err != nil {
		logger.Error("Failed to load workflow metadata", "error", err)
	}
	return "", fmt.Errorf("failed to process workflow metadata for %s: %w", model, err)
}
