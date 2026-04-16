package comfyui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/shared/meta"
	"aibird/text/glm"

	"github.com/richinsley/comfy2go/client"
	"github.com/schollz/progressbar/v3"
)

const boolType = "bool"


func freeVram(clientAddr string, clientPort int) error {
	url := fmt.Sprintf("http://%s:%d/free", clientAddr, clientPort)
	req, err := http.NewRequest("POST", url, http.NoBody)
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

func Process(irc state.State, aiEnhancedPrompt string, gpu meta.GPUType) (string, error) { //nolint:gocyclo
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

		// Single ComfyUI instance - determine CUDA device based on GPU type
		// GPU5090 -> cuda:1 (RTX 5090)
		// GPU4090 -> cuda:0 (RTX 4090)
		cudaDevice := "cuda:0" // Default to 4090
		if gpu == meta.GPU5090 {
			cudaDevice = "cuda:1" // 5090
		}
		logger.Debug("ComfyUI CUDA device selection", "gpu", gpu, "cudaDevice", cudaDevice)

		clientPort := comfyUiConfig.Port
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
			var rawUserInput interface{}
			var userInputProvided bool

			// Handle different parameter types for input retrieval
			if paramDef.Type == boolType {
				rawUserInput = irc.GetBoolArg(paramName)
				if boolVal, ok := rawUserInput.(bool); ok {
					userInputProvided = boolVal
				}
			} else {
				if strVal, ok := irc.FindArgument(paramName, "").(string); ok {
					rawUserInput = strVal
					userInputProvided = strVal != ""
				}
			}

			// Check if required parameter is provided
			if paramDef.Required && !userInputProvided {
				return "", fmt.Errorf("⚠️ Parameter --%s is required. %s", paramName, paramDef.Description)
			}

			// Special pre-flight check for image URLs to give users faster feedback
			if paramName == "img" && userInputProvided {
				if imgURL, ok := rawUserInput.(string); ok {
					// Validate URL to prevent SSRF attacks
					if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
						errMsg := fmt.Sprintf("⚠️ Invalid URL scheme for --img: %s", imgURL)
						return "", errors.New(errMsg)
					}

					logger.Debug("Performing pre-flight check for image URL", "url", imgURL)
					resp, err := http.Head(imgURL) // #nosec G107 - URL scheme validated above
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
			}

			var finalValue interface{}

			if !userInputProvided {
				// Set default values based on parameter type
				if paramDef.Type == boolType {
					// Special handling for fullblocks parameter - invert the logic
					if paramName == "fullblocks" {
						finalValue = true // Default to halfblock mode (true for ComfyUI half_block_mode)
					} else {
						finalValue = false // Other boolean parameters default to false
					}
				} else {
					finalValue = paramDef.Default
				}
			} else {
				var parseErr error
				switch paramDef.Type {
				case "string":
					if strVal, ok := rawUserInput.(string); ok {
						finalValue = strVal
					}
				case "bool":
					// Special handling for fullblocks parameter - invert the boolean
					if paramName == "fullblocks" {
						if boolVal, ok := rawUserInput.(bool); ok {
							finalValue = !boolVal // Invert: --fullblocks means disable halfblock mode
						}
					} else {
						if boolVal, ok := rawUserInput.(bool); ok {
							finalValue = boolVal
						}
					}
				case "int":
					if strVal, ok := rawUserInput.(string); ok {
						val, parseErr := strconv.ParseInt(strVal, 10, 64)
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
					}
				case "float":
					if strVal, ok := rawUserInput.(string); ok {
						val, parseErr := strconv.ParseFloat(strVal, 64)
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
					}
				case "lyrics":
					if lyricsPrompt, ok := rawUserInput.(string); ok {
						var lyrics string
						var lyErr error
						if lyricsPrompt == "" {
							if paramDef.Default != nil {
								if defaultStr, ok := paramDef.Default.(string); ok {
									lyrics = defaultStr
								}
							} else {
								lyrics = ""
							}
						} else {
							if strings.HasPrefix(lyricsPrompt, "http") && strings.HasSuffix(lyricsPrompt, ".txt") {
								irc.Send("📜 Downloading lyrics from URL! ✨")
								resp, httpErr := http.Get(lyricsPrompt) // #nosec G107 - URL validated for http prefix and .txt suffix
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
								lyrics, lyErr = glm.GenerateLyrics(lyricsPrompt, irc.Config.Glm)
								if lyErr != nil {
									return "", fmt.Errorf("failed to generate lyrics: %w", lyErr)
								}
							}
						}
						finalValue = lyrics
						parseErr = nil
					}
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

		// --- Inject GPU device for GPU parameters ---
		// Parameters like gpu_unet, gpu_clip, gpu_vae get the cuda device based on user's assigned GPU
		for paramName, paramDef := range metaData.Parameters {
			if strings.HasPrefix(paramName, "gpu_") {
				for _, target := range paramDef.Targets {
					if _, ok := widgetUpdates[target.Node]; !ok {
						widgetUpdates[target.Node] = make(map[int]interface{})
					}
					logger.Debug("Injecting GPU device", "param", paramName, "node", target.Node, "widget", target.WidgetIndex, "cudaDevice", cudaDevice)
					widgetUpdates[target.Node][target.WidgetIndex] = cudaDevice
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
									if strVal, ok := value.(string); ok {
										values[widgetIndex] = originalPrompt + " " + strVal
									}
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

							// Example of a post-generation rate limit, can be made generic later
							if strings.Contains(model, "wan") && irc.User.GetAccessLevel() <= 2 {
								cacheKey := fmt.Sprintf("img2wan_%s", irc.User.NickName)
								// Set rate limit using in-memory rate limiter (3 hours)
								birdbase.RateLimiter.SetRateLimit(cacheKey, 3*time.Hour)
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
	logger.Error("Failed to load workflow metadata", "error", err)
	return "", fmt.Errorf("failed to process workflow metadata for %s: %w", model, err)
}
