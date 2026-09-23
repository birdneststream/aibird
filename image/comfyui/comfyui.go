// Package comfyui integrates with ComfyUI for AI-powered image, video, and audio generation.
package comfyui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/image/comfyui/apiclient"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/text/glm"

	"github.com/schollz/progressbar/v3"
)

const (
	boolType        = "bool"
	paramTypeVoice  = "voice"
	paramTypeString = "string"
	paramTypeInt    = "int"
	paramTypeFloat  = "float"
)

func Process(ctx context.Context, irc *state.State, aiEnhancedPrompt string) (string, error) {
	logger.Debug("Starting comfyui.Process", "action", irc.Action())
	comfyUiConfig := irc.Config.ComfyUi
	model := irc.Action()
	metaData, err := GetAibirdMeta(workflowFile(model))
	if err == nil {
		logger.Info("Using V2 metadata-driven processing", "model", model)
		return processWithMeta(ctx, irc, aiEnhancedPrompt, model, &comfyUiConfig, metaData)
	}
	logger.Error("Failed to load workflow metadata", "error", err)
	return "", fmt.Errorf("failed to process workflow metadata for %s: %w", model, err)
}

// processWithMeta handles the metadata-driven workflow processing via the
// direct ComfyUI HTTP/WebSocket client (apiclient). Workflows must be in
// ComfyUI API JSON format with metadata in a sibling .meta.toml sidecar. The
// context governs cancellation: on ctx cancellation the running ComfyUI prompt
// is interrupted via /interrupt and the call returns promptly.
func processWithMeta(ctx context.Context, irc *state.State, aiEnhancedPrompt, model string, comfyUiConfig *settings.ComfyUiConfig, metaData *AibirdMeta) (string, error) {
	if irc.User.GetAccessLevel() < metaData.AccessLevel {
		logger.Error("Access level too low", "required", metaData.AccessLevel, "user", irc.User.GetAccessLevel())
		return "", fmt.Errorf("⛔️ Sorry, you need access level %d to use this command. Check !support for more info", metaData.AccessLevel)
	}

	client := apiclient.New(comfyUiConfig.Url, comfyUiConfig.Port)
	defer func() {
		freeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Free(freeCtx, true, true); err != nil {
			logger.Error("Error freeing VRAM", "error", err)
		}
	}()

	// Prepare the message/prompt
	message := preparePrompt(irc, aiEnhancedPrompt, model, comfyUiConfig, metaData.RawPrompt)

	// Resolve user args + metadata into named-input updates
	inputUpdates, err := applyParameters(irc, metaData, message)
	if err != nil {
		return "", err
	}

	return executeAndCollect(ctx, irc, client, model, workflowFile(model), inputUpdates, metaData)
}

// workflowFile returns the path for a given workflow model name.
func workflowFile(model string) string {
	return "comfyuijson/" + model + ".json"
}

// preparePrompt builds the final prompt string from user input.
func preparePrompt(irc *state.State, aiEnhancedPrompt, model string, comfyUiConfig *settings.ComfyUiConfig, rawPrompt bool) string {
	var message string
	if !irc.IsAction("tts") {
		message = CleanPrompt(irc.Message())
	} else {
		message = irc.Message()
	}
	if aiEnhancedPrompt != "" {
		if rawPrompt {
			// Preserve structured LLM-curated output (newlines, brackets, timecodes).
			// Only used by workflows that opt in via raw_prompt=true; the bad-words
			// safety check below still applies.
			message = strings.TrimSpace(aiEnhancedPrompt)
		} else {
			message = CleanPrompt(aiEnhancedPrompt)
		}
	}
	if BadWordsCheck(message, comfyUiConfig) {
		message = comfyUiConfig.BadWordsPrompt
	}

	// Special case for ernie: construct the full prompt enhancement template in Go
	// to bypass the erroring StringReplace ComfyUI node in the workflow.
	// No manual quote escaping needed — the workflow JSON is serialized by the
	// direct client; the prompt value is set on the named input as a plain string.
	// TODO: Consider making this generic via a prompt_template field in aibird_meta.
	if model == "ernie" {
		message = strings.ReplaceAll(
			`<s>[SYSTEM_PROMPT]你是一个专业的文生图 Prompt 增强助手。你将收到用户的简短图片描述及目标生成分辨率，请据此扩写为一段内容丰富、细节充分的视觉描述，以帮助文生图模型生成高质量的图片。仅输出增强后的描述，不要包含任何解释或前缀。[/SYSTEM_PROMPT][INST]{"prompt": "{prompt}", "width": 1024, "height": 1024}[/INST]`,
			"{prompt}",
			message,
		)
	}

	return message
}

// applyParameters processes user arguments and metadata to build a named-input
// update map: node (title or class_type) -> input name -> value. This is the
// API-format equivalent of the previous widget-index map; the new execute path
// resolves each node reference to actual node ids via the graph's _meta.title /
// class_type and applies the value to the named input.
func applyParameters(irc *state.State, metaData *AibirdMeta, message string) (map[string]map[string]interface{}, error) { //nolint:gocyclo
	updates := make(map[string]map[string]interface{})

	set := func(node, input string, value interface{}) {
		if _, ok := updates[node]; !ok {
			updates[node] = make(map[string]interface{})
		}
		updates[node][input] = value
	}

	// --- Process Prompt ---
	if metaData.PromptTarget.Node != "" && metaData.PromptTarget.Input != "" {
		set(metaData.PromptTarget.Node, metaData.PromptTarget.Input, message)
	}

	// --- Generic Parameter Processing ---
	for paramName, paramDef := range metaData.Parameters { //nolint:gocritic // rangeValCopy is acceptable for this config struct
		finalValue := processParameter(irc, paramName, &paramDef)
		if finalValue == nil {
			continue
		}
		if errVal, ok := finalValue.(error); ok {
			return nil, errVal
		}
		for _, target := range paramDef.Targets {
			logger.Debug("Setting parameter", "param", paramName, "node", target.Node, "input", target.Input, "value", finalValue)
			set(target.Node, target.Input, finalValue)
		}
	}

	// --- Process Hardcoded Values ---
	for paramName, hardcodedDef := range metaData.Hardcoded {
		finalValue := hardcodedDef.Value
		if finalValue != nil {
			for _, target := range hardcodedDef.Targets {
				logger.Debug("Setting hardcoded parameter", "param", paramName, "node", target.Node, "input", target.Input, "value", finalValue)
				set(target.Node, target.Input, finalValue)
			}
		}
	}

	return updates, nil
}

// processParameter handles a single parameter definition, returning the resolved value.
// Returns nil if the parameter should be skipped.
func processParameter(irc *state.State, paramName string, paramDef *ParameterDef) interface{} { //nolint:gocyclo
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
		return fmt.Errorf("⚠️ Parameter --%s is required: %s", paramName, paramDef.Description)
	}

	// Special pre-flight check for image URLs to give users faster feedback
	if paramName == "img" && userInputProvided {
		if imgURL, ok := rawUserInput.(string); ok {
			// Validate URL to prevent SSRF attacks
			if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
				return fmt.Errorf("⚠️ Invalid URL scheme for --img: %s", imgURL)
			}

			logger.Debug("Performing pre-flight check for image URL", "url", imgURL)
			resp, err := http.Head(imgURL) // #nosec G107 - URL scheme validated above
			if err != nil {
				return fmt.Errorf("⚠️ Failed to reach the image URL for --img: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("⚠️ The image URL for --img appears to be invalid (server response: %s): please check the link", resp.Status)
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
				finalValue = true // Default to halfblock mode
			} else {
				finalValue = false
			}
		} else {
			finalValue = paramDef.Default
		}
	} else {
		finalValue = parseUserInput(irc, paramName, paramDef, rawUserInput)
		if errVal, ok := finalValue.(error); ok {
			return errVal // Propagate parse errors
		}
	}

	// Handle special case for seed randomization
	if paramName == "seed" && !userInputProvided {
		seed, err := rand.Int(rand.Reader, big.NewInt(1<<63-1))
		if err != nil {
			return fmt.Errorf("failed to generate random seed: %w", err)
		}
		finalValue = seed.Int64()
	}

	// Handle special case for voice filename to add .wav suffix
	if paramName == paramTypeVoice {
		if voiceStr, ok := finalValue.(string); ok && !strings.HasSuffix(voiceStr, ".wav") {
			finalValue = voiceStr + ".wav"
		}
	}

	return finalValue
}

// parseUserInput converts raw user input to the appropriate type based on parameter definition.
func parseUserInput(irc *state.State, paramName string, paramDef *ParameterDef, rawUserInput interface{}) interface{} { //nolint:gocyclo
	switch paramDef.Type {
	case paramTypeString:
		if strVal, ok := rawUserInput.(string); ok {
			return strVal
		}
	case boolType:
		if paramName == "fullblocks" {
			if boolVal, ok := rawUserInput.(bool); ok {
				return !boolVal // Invert: --fullblocks means disable halfblock mode
			}
		} else {
			if boolVal, ok := rawUserInput.(bool); ok {
				return boolVal
			}
		}
	case paramTypeInt:
		if strVal, ok := rawUserInput.(string); ok {
			val, err := strconv.ParseInt(strVal, 10, 64)
			if err != nil {
				return fmt.Errorf("⚠️ Invalid value for --%s: expected an int, but got '%s'", paramName, strVal)
			}
			if paramDef.Min != nil && float64(val) < *paramDef.Min {
				return fmt.Errorf("⚠️ Value for --%s is too low: minimum is %g, but you gave %d", paramName, *paramDef.Min, val)
			}
			if paramDef.Max != nil && float64(val) > *paramDef.Max {
				return fmt.Errorf("⚠️ Value for --%s is too high: maximum is %g, but you gave %d", paramName, *paramDef.Max, val)
			}
			return val
		}
	case paramTypeFloat:
		if strVal, ok := rawUserInput.(string); ok {
			val, err := strconv.ParseFloat(strVal, 64)
			if err != nil {
				return fmt.Errorf("⚠️ Invalid value for --%s: expected a float, but got '%s'", paramName, strVal)
			}
			if paramDef.Min != nil && val < *paramDef.Min {
				return fmt.Errorf("⚠️ Value for --%s is too low: minimum is %g, but you gave %g", paramName, *paramDef.Min, val)
			}
			if paramDef.Max != nil && val > *paramDef.Max {
				return fmt.Errorf("⚠️ Value for --%s is too high: maximum is %g, but you gave %g", paramName, *paramDef.Max, val)
			}
			return val
		}
	case "lyrics":
		return parseLyrics(irc, paramDef, rawUserInput)
	default:
		return fmt.Errorf("unsupported parameter type '%s' in metadata for '%s'", paramDef.Type, paramName)
	}
	return nil
}

// parseLyrics handles the special "lyrics" parameter type (URL download or AI generation).
func parseLyrics(irc *state.State, paramDef *ParameterDef, rawUserInput interface{}) interface{} {
	lyricsPrompt, ok := rawUserInput.(string)
	if !ok {
		return nil
	}

	var lyrics string
	if lyricsPrompt == "" {
		if paramDef.Default != nil {
			if defaultStr, ok := paramDef.Default.(string); ok {
				lyrics = defaultStr
			}
		}
	} else {
		if strings.HasPrefix(lyricsPrompt, "http") && strings.HasSuffix(lyricsPrompt, ".txt") {
			irc.Send("📜 Downloading lyrics from URL! ✨")
			resp, httpErr := http.Get(lyricsPrompt) // #nosec G107 - URL validated for http prefix and .txt suffix
			if httpErr != nil {
				return fmt.Errorf("failed to download lyrics from URL: %w", httpErr)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to download lyrics from URL: status code %d", resp.StatusCode)
			}

			bodyBytes, ioErr := io.ReadAll(resp.Body)
			if ioErr != nil {
				return fmt.Errorf("failed to read lyrics from response body: %w", ioErr)
			}
			lyrics = string(bodyBytes)
		} else {
			irc.Send("✍️ Generating lyrics with ai! ✨")
			var lyErr error
			lyrics, lyErr = glm.GenerateLyrics(lyricsPrompt, &irc.Config.Glm)
			if lyErr != nil {
				return fmt.Errorf("failed to generate lyrics: %w", lyErr)
			}
		}
	}
	return lyrics
}

// executeAndCollect creates a ComfyUI client, runs the workflow, and collects output files.
// executeAndCollect runs the workflow against ComfyUI via the direct
// HTTP/WebSocket client, applies the resolved named-input updates, streams
// progress, and writes the first output artifact to disk.
//
// Context-aware cancellation: when ctx is canceled (queue shutdown or admin
// cancel), the WebSocket stream is torn down (the apiclient watcher closes the
// connection on ctx.Done), and /interrupt is sent so the server-side prompt
// actually stops and frees GPU. The previous comfy2go channel loop could do
// neither. WebSocket messages are filtered to the prompt id returned by
// QueuePrompt so a shared ComfyUI server does not cross-talk between concurrent
// prompts.
func executeAndCollect(ctx context.Context, irc *state.State, c *apiclient.Client, model, workflowFile string, inputUpdates map[string]map[string]interface{}, metaData *AibirdMeta) (string, error) { //nolint:gocyclo
	// Load the API-format workflow graph and apply each resolved input. The
	// prompt target uses append semantics so the workflow's default positive
	// prompt text is preserved and extended with the user's message.
	graph, err := LoadAPIGraph(workflowFile)
	if err != nil {
		return "", fmt.Errorf("error loading graph JSON: %w", err)
	}
	graph = graph.Clone()
	for nodeRef, inputs := range inputUpdates {
		for inputName, value := range inputs {
			appendPrompt := nodeRef == metaData.PromptTarget.Node && inputName == metaData.PromptTarget.Input
			if applyErr := graph.ApplyInput(nodeRef, inputName, value, appendPrompt); applyErr != nil {
				return "", fmt.Errorf("error applying input %s.%s: %w", nodeRef, inputName, applyErr)
			}
		}
	}

	// Connect the WebSocket BEFORE queueing the prompt. This eliminates the
	// race where a fast prompt completes between QueuePrompt and Connect,
	// causing us to miss the terminal execution_success frame and hang until
	// the context deadline. ComfyUI broadcasts messages for all prompts on the
	// connection, so messages for other clients (prompt_id != ours) are
	// filtered out below. The /history fallback recovers output if any frame
	// is still missed.
	ch, stop, err := c.Connect(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to open ws stream: %w", err)
	}
	defer stop()

	// Queue the prompt.
	promptID, err := c.QueuePrompt(ctx, graph.PromptJSON())
	if err != nil {
		return "", fmt.Errorf("failed to queue prompt: %w", err)
	}
	logger.Info("Queued ComfyUI prompt", "prompt_id", promptID, "model", model)

	// Publish live progress so !status can report the current node / step /
	// elapsed. Cleared on every exit path below (success, error, stall, cancel).
	act := Activity{Action: model, PromptID: promptID, StartedAt: time.Now()}
	SetActivity(act)
	defer ClearActivity()

	var bar *progressbar.ProgressBar
	var currentNode, capturedFile string

	// Stall watchdog: ComfyUI streams a message on essentially every step
	// (executing/progress/progress_state/executed), so a gap of stallTimeout
	// with no frames for our prompt means the job is wedged — e.g. a silent
	// WebSocket drop, a server-side hang, or an OOM stall. We interrupt and
	// bail rather than blocking the queue forever. This replaces the previous
	// fixed-duration queue timeout: a job that keeps making progress (even an
	// 8-minute H3 render) runs to completion; only genuinely-stalled jobs die.
	// 5 minutes because long silent phases are legitimate: llama.cpp inference
	// inside H3PromptComposer, model loads, and final video encode can each go
	// well past 60s with no frames for our prompt.
	const stallTimeout = 5 * time.Minute
	stallTimer := time.NewTimer(stallTimeout)
	defer stallTimer.Stop()

	resetStall := func() {
		if !stallTimer.Stop() {
			// Drain the timer channel if it already fired, to avoid the
			// reset-on-fired-timer no-op. Safe because we only get here from
			// the msg branch (timer.C is not being concurrently selected).
			select {
			case <-stallTimer.C:
			default:
			}
		}
		stallTimer.Reset(stallTimeout)
	}

loop:
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				break loop // channel closed (WS teardown)
			}
			// Filter to our prompt (ComfyUI broadcasts every prompt on the conn).
			if msg.PromptID != "" && msg.PromptID != promptID {
				continue
			}
			// Any frame for our prompt means progress — keep the watchdog alive.
			resetStall()
			switch msg.Type {
			case apiclient.TypeExecuting:
				bar = nil
				currentNode = msg.Node
				// New node started: reset step counters and surface its title.
				// Skip the title update on the terminal executing frame, where
				// ComfyUI sends an empty node to signal "execution done" — we
				// don't want to wipe the last node's title before success.
				if msg.Node != "" {
					act.NodeTitle = graph.nodeTitle(msg.Node)
					act.Step, act.Max = 0, 0
					SetActivity(act)
					logger.Debug("Executing node", "node_id", msg.Node, "title", act.NodeTitle)
				}
			case apiclient.TypeProgress:
				if bar == nil {
					bar = progressbar.Default(int64(msg.Max), currentNode)
				}
				_ = bar.Set(msg.Value)
				act.Step, act.Max = msg.Value, msg.Max
				SetActivity(act)
			case apiclient.TypeProgressState:
				// Whole-graph snapshot: derive node completion counts and the
				// running node's display title. progress_state is the most
				// reliable liveness + completion signal ComfyUI sends.
				done, total, runningTitle := summarizeProgressState(msg.ProgressState)
				act.NodesDone, act.NodesTotal = done, total
				if runningTitle != "" {
					act.NodeTitle = runningTitle
				}
				SetActivity(act)
			case apiclient.TypeExecutionError:
				return "", fmt.Errorf("comfyui execution error: %s: %s", msg.ExceptionType, msg.ExceptionMessage)
			case apiclient.TypeExecutionInterrupted:
				break loop
			case apiclient.TypeExecuted:
				if capturedFile == "" {
					f, ferr := collectOutput(ctx, c, &msg)
					if ferr != nil {
						return "", ferr
					}
					capturedFile = f
				}
			case apiclient.TypeExecutionSuccess:
				break loop // terminal; stop reading (do NOT rely on the WS closing)
			}
		case <-stallTimer.C:
			// No progress for stallTimeout — interrupt and report a stall.
			logger.Warn("ComfyUI job stalled — no progress, interrupting", "prompt_id", promptID, "stall", stallTimeout)
			interruptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if iErr := c.Interrupt(interruptCtx); iErr != nil {
				logger.Warn("Failed to interrupt stalled ComfyUI prompt", "prompt_id", promptID, "error", iErr)
			}
			cancel()
			return "", fmt.Errorf("comfyui job stalled: no progress for %v", stallTimeout)
		case <-ctx.Done():
			break loop // queue shutdown / admin cancel
		}
	}

	// If the context was canceled mid-run, stop the server-side job and bail.
	// This is what actually delivers queue/admin cancellation to ComfyUI.
	if ctxErr := ctx.Err(); ctxErr != nil {
		interruptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if iErr := c.Interrupt(interruptCtx); iErr != nil {
			logger.Warn("Failed to interrupt ComfyUI prompt after cancellation", "prompt_id", promptID, "error", iErr)
		}
		cancel()
		return "", fmt.Errorf("comfyui job canceled: %w", ctxErr)
	}

	// Success path: prefer the captured artifact, then fall back to /history
	// (in case the executed frame was missed on a brief reconnect).
	if capturedFile == "" {
		capturedFile = outputFromHistory(ctx, c, promptID)
	}
	if capturedFile == "" {
		logger.Debug("Finishing comfyui.Process with no output", "action", irc.Action(), "prompt_id", promptID)
		return "", errors.New("error processing comfyui: no output file received")
	}

	// Post-generation rate limit for wan models.
	if strings.Contains(model, "wan") && irc.User.GetAccessLevel() <= 2 {
		cacheKey := fmt.Sprintf("img2wan_%s", irc.User.NickName)
		birdbase.RateLimiter.SetRateLimit(cacheKey, 3*time.Hour)
	}
	return capturedFile, nil
}

// summarizeProgressState reduces a progress_state snapshot (node id -> state)
// into whole-graph completion counts plus the display title of the first
// running node. Returns (finishedCount, totalCount, runningTitle); runningTitle
// is "" when no node is currently running. Used to drive the !status detail.
func summarizeProgressState(states map[string]apiclient.NodeState) (done, total int, runningTitle string) {
	total = len(states)
	for _, ns := range states {
		if ns.State == nodeStateFinished {
			done++
		}
		if ns.IsRunning() && runningTitle == "" && ns.DisplayNodeID != "" {
			runningTitle = ns.DisplayNodeID
		}
	}
	return
}

// outputFromHistory polls /history/{promptID} as a fallback for output
// discovery when no "executed" WebSocket frame was captured. Returns the local
// filename of the first artifact written, or "" if none was found. Failures are
// logged at debug; this is best-effort.
func outputFromHistory(ctx context.Context, c *apiclient.Client, promptID string) string {
	hist, err := c.History(ctx, promptID)
	if err != nil {
		logger.Debug("history fallback failed", "prompt_id", promptID, "error", err)
		return ""
	}
	if hist == nil {
		return ""
	}
	entry, ok := hist[promptID].(map[string]interface{})
	if !ok {
		return ""
	}
	outputs, ok := entry["outputs"].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, v := range outputs {
		nodeOut, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if f := firstHistoryArtifact(ctx, c, nodeOut); f != "" {
			return f
		}
	}
	return ""
}

// firstHistoryArtifact scans one node's /history output map for the first
// images/gifs/audio artifact, downloads it, and returns the local filename.
func firstHistoryArtifact(ctx context.Context, c *apiclient.Client, nodeOut map[string]interface{}) string {
	for _, kind := range []string{"images", "gifs", "audio"} {
		items, ok := nodeOut[kind].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range items {
			item, okItem := raw.(map[string]interface{})
			if !okItem {
				continue
			}
			filename, okFn := item["filename"].(string)
			if !okFn || filename == "" {
				continue
			}
			subfolder, okSub := item["subfolder"].(string)
			fileType, okType := item["type"].(string)
			if !okSub || !okType {
				continue
			}
			if f, ferr := downloadArtifact(ctx, c, filename, subfolder, fileType); ferr == nil {
				return f
			}
		}
	}
	return ""
}

// collectOutput downloads the first images/gifs/audio artifact from an Executed
// message via /view and writes it to disk under a sanitized basename, returning
// the local filename. It returns "" if the message had no fetchable artifact.
func collectOutput(ctx context.Context, c *apiclient.Client, msg *apiclient.Message) (string, error) { //nolint:gocritic // hugeParam: pass by ptr; Message is a snapshot we read once.
	for _, kind := range []string{"images", "gifs", "audio"} {
		items, ok := msg.Output[kind]
		if !ok {
			continue
		}
		for _, item := range items {
			return downloadArtifact(ctx, c, item.Filename, item.Subfolder, item.Type)
		}
	}
	return "", nil
}

// downloadArtifact fetches one output via /view and writes it to the current
// directory under filepath.Base(filename) (sanitized — the filename comes from
// the ComfyUI server response and must not be trusted as a path).
func downloadArtifact(ctx context.Context, c *apiclient.Client, filename, subfolder, fileType string) (string, error) {
	rc, err := c.View(ctx, filename, subfolder, fileType)
	if err != nil {
		return "", fmt.Errorf("failed to fetch output %s: %w", filename, err)
	}
	defer rc.Close()
	safeName := filepath.Base(filename) // #nosec G304 - sanitized to a bare basename
	if safeName == "." || safeName == string(filepath.Separator) || safeName == "" {
		return "", fmt.Errorf("comfyui returned an invalid output filename: %q", filename)
	}
	f, err := os.Create(safeName) // #nosec G304 - safeName is filepath.Base(filename), cannot escape CWD
	if err != nil {
		return "", fmt.Errorf("failed to create output file %s: %w", safeName, err)
	}
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		return "", fmt.Errorf("failed to write output %s: %w", safeName, err)
	}
	f.Close()
	return safeName, nil
}
