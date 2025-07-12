package request

import (
	"aibird/logger"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (r *Request) GetUrl() string {
	return r.Url
}

func (r *Request) GetMethod() string {
	return r.Method
}

func (r *Request) IsPost() bool {
	return r.Method == "POST"
}

func (r *Request) IsGet() bool {
	return r.Method == "GET"
}

func (r *Request) GetHeaders() []Headers {
	return r.Headers
}

func (r *Request) GetPayload() interface{} {
	return r.Payload
}

func (r *Request) AddHeader(key string, value string) {
	r.Headers = append(r.Headers, Headers{Key: key, Value: value})
}

// Add this helper function
func getFileContentType(file *os.File) (string, error) {
	// Only the first 512 bytes are used to detect content type
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", err
	}

	// Reset the file pointer to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", err
	}

	// Detect content type
	contentType := http.DetectContentType(buffer)

	if contentType == "application/octet-stream" {
		if strings.HasSuffix(strings.ToLower(file.Name()), ".jpg") ||
			strings.HasSuffix(strings.ToLower(file.Name()), ".jpeg") {
			contentType = "image/jpeg"
		}

		if strings.HasSuffix(strings.ToLower(file.Name()), ".png") {
			contentType = "image/png"
		}

		if strings.HasSuffix(strings.ToLower(file.Name()), ".txt") {
			contentType = "text/plain"
		}

		if strings.HasSuffix(strings.ToLower(file.Name()), ".webp") {
			contentType = "image/webp"
		}

		if strings.HasSuffix(strings.ToLower(file.Name()), ".webm") {
			contentType = "video/webm"
		}
	}

	return contentType, nil
}

func (r *Request) Call(response interface{}) error {
	var jsonData []byte
	var err error
	var reqBody *bytes.Buffer

	if r.FileName != "" {
		reqBody = &bytes.Buffer{}
		writer := multipart.NewWriter(reqBody)
		file, errFile1 := os.Open(r.FileName)
		if errFile1 != nil {
			return fmt.Errorf("failed to open file: %w", errFile1)
		}
		defer file.Close()

		// Get content type for file
		contentType, err := getFileContentType(file)
		if err != nil {
			return fmt.Errorf("failed to get content type: %w", err)
		}

		// Create form file with content type
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filepath.Base(r.FileName)))
		h.Set("Content-Type", contentType)

		part1, errFile1 := writer.CreatePart(h)
		if errFile1 != nil {
			return fmt.Errorf("failed to create form part: %w", errFile1)
		}

		bytesWritten, errFile1 := io.Copy(part1, file)
		if errFile1 != nil {
			return fmt.Errorf("failed to copy file content: %w", errFile1)
		}
		logger.Debug("Copied bytes to form file", "bytes", bytesWritten)

		// Add form fields
		for _, field := range r.Fields {
			err := writer.WriteField(field.Key, field.Value)
			if err != nil {
				return fmt.Errorf("failed to write field: %w", err)
			}
		}

		err = writer.Close()
		if err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

		// Set the multipart form content type header
		r.AddHeader("Content-Type", writer.FormDataContentType())

	} else if r.IsPost() {
		// JSON post request
		jsonData, err = json.Marshal(r.GetPayload())
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}

		reqBody = bytes.NewBuffer(jsonData)
	}

	if reqBody == nil {
		reqBody = &bytes.Buffer{}
	}

	req, err := http.NewRequest(r.GetMethod(), r.GetUrl(), reqBody)
	if err != nil {
		return fmt.Errorf("failed to create new request: %w", err)
	}

	for _, header := range r.GetHeaders() {
		req.Header.Set(header.Key, header.Value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Birdhole will return a string and not any JSON
	if strPtr, ok := response.(*string); ok {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		*strPtr = string(bodyBytes)
	} else {
		// Handle as JSON for everything else
		err = json.NewDecoder(resp.Body).Decode(response)
		if err != nil {
			logger.Error("Failed to decode JSON response", "error", err)
			return fmt.Errorf("failed to decode JSON response: %w", err)
		}
	}

	return nil
}

func (r *Request) Download() error {
	//Get the response bytes from the url
	response, err := http.Get(r.Url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("received non 200 response code")
	}

	//Create empty file
	file, err := os.Create(r.FileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the field
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func IsImage(url string) bool {
	// Validate URL to prevent SSRF attacks
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		logger.Error("Invalid URL scheme", "url", url)
		return false
	}

	resp, err := http.Head(url)
	if err != nil {
		logger.Error("Error checking image URL", "url", url, "error", err)
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

	// Check if Content-Type header starts with "image/" and Content-Length is less than 5MB
	if strings.HasPrefix(contentType, "image/") && contentLength <= 5*1024*1024 {
		return true
	}

	return false
}
