package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/irc.v3"
)

func sdRequest(prompt string, c *irc.Client, m *irc.Message) {
	posturl := config.StableDiffusion.Host + "/sdapi/v1/txt2img"

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing Stable Diffusion: " + prompt + "...",
		},
	})

	// new variable that converts prompt to slug
	slug := cleanFileName(prompt)

	// Create a new struct
	sd := StableDiffusionRequest{
		// EnableHr:          false,
		// DenoisingStrength: 0,
		// FirstphaseWidth:   0,
		// FirstphaseHeight:  0,
		Prompt: prompt,
		Seed:   -1,
		// Subseed:           -1,
		// SubseedStrength:   0,
		// SeedResizeFromH:   -1,
		// SeedResizeFromW:   -1,
		BatchSize:    1,
		NIter:        1,
		Steps:        config.StableDiffusion.Steps,
		CfgScale:     config.StableDiffusion.CfgScale,
		Width:        config.StableDiffusion.Width,
		Height:       config.StableDiffusion.Height,
		RestoreFaces: config.StableDiffusion.RestoreFace,
		// Tiling:            false,
		NegativePrompt: config.StableDiffusion.NegativePrompt,
		// Eta:               0,
		// SChurn:            0,
		// STmax:             0,
		// STmin:             0,
		// SNoise:            1,
		SamplerIndex: config.StableDiffusion.Sampler,
	}

	// Prepare sd for http NewRequest
	reqStr, err := json.Marshal(sd)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", posturl, strings.NewReader(string(reqStr)))

	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}

	post := &StableDiffusionResponse{}
	err = json.Unmarshal(body, post)

	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}

	if res.StatusCode != http.StatusOK {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				fmt.Sprint(res.StatusCode),
			},
		})
		return
	}

	// generate random string
	randValue := rand.Int63n(10000)

	// generate a random value with length of 4
	randValue = randValue % 10000
	fileName := slug + "_" + strconv.FormatInt(randValue, 4) + ".png"

	// decode base64 image and save to fileName
	decoded, err := base64.StdEncoding.DecodeString(post.Images[0])
	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}

	err = ioutil.WriteFile(fileName, decoded, 0644)
	if err != nil {
		errString := fmt.Sprintf("%v", err)
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				errString,
			},
		})
		return
	}

	// append the current pwd to fileName
	fileName = filepath.Base(fileName)

	// download image
	content := fileHole("https://filehole.org/", fileName)

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			m.Prefix.Name + ": " + content,
		},
	})

}
