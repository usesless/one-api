package controller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"one-api/common"
	"strings"

	"github.com/gin-gonic/gin"
)

func openaiStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*OpenAIErrorWithStatusCode, string) {
	responseText := ""
	// 从c 中获取ocr结果
	ocrResult := c.GetString("ocr_result")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "\n"); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	dataChan := make(chan string)
	stopChan := make(chan bool)
	go func() {
		for scanner.Scan() {
			data := scanner.Text()
			if len(data) < 6 { // ignore blank line or wrong format
				continue
			}
			if data[:6] != "data: " && data[:6] != "[DONE]" {
				continue
			}
			// dataChan <- data
			rawData := data
			data = data[6:]
			if !strings.HasPrefix(data, "[DONE]") {
				switch relayMode {
				case RelayModeChatCompletions:
					var streamResponse ChatCompletionsStreamResponse
					err := json.Unmarshal([]byte(data), &streamResponse)
					if err != nil {
						common.SysError("error unmarshalling stream response: " + err.Error())
						continue // just ignore the error
					}
					for _, choice := range streamResponse.Choices {
						responseText += choice.Delta.Content
						if strings.Contains(choice.FinishReason, `stop`) {
							streamResponse.OcrRawData = ocrResult
							streamData, _ := json.Marshal(streamResponse)
							rawData = `data: ` + string(streamData)
						}
					}
				case RelayModeCompletions:
					var streamResponse CompletionsStreamResponse
					err := json.Unmarshal([]byte(data), &streamResponse)
					if err != nil {
						common.SysError("error unmarshalling stream response: " + err.Error())
						continue
					}
					for _, choice := range streamResponse.Choices {
						responseText += choice.Text
						if strings.Contains(choice.FinishReason, `stop`) {
							streamResponse.OcrRawData = ocrResult
							streamData, _ := json.Marshal(streamResponse)
							rawData = `data: ` + string(streamData)
						}
					}
				}
			}
			dataChan <- rawData
		}
		stopChan <- true
	}()
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			if strings.HasPrefix(data, "data: [DONE]") {
				data = data[:12]
			}
			// some implementations may add \r at the end of data
			data = strings.TrimSuffix(data, "\r")
			c.Render(-1, common.CustomEvent{Data: data})
			return true
		case <-stopChan:
			return false
		}
	})
	err := resp.Body.Close()
	if err != nil {
		return errorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), ""
	}
	return nil, responseText
}

func openaiHandler(c *gin.Context, resp *http.Response, consumeQuota bool) (*OpenAIErrorWithStatusCode, *Usage) {
	var textResponse OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return errorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &textResponse)
	if err != nil {
		return errorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	textResponse.OcrRawData = c.GetString("ocr_result")
	// if consumeQuota {
	newBody, err := json.Marshal(textResponse)
	if err != nil {
		return errorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if textResponse.Error.Type != "" {
		return &OpenAIErrorWithStatusCode{
			OpenAIError: textResponse.Error,
			StatusCode:  resp.StatusCode,
		}, nil
	}

	// Reset response body
	resp.Body = io.NopCloser(bytes.NewBuffer(newBody))
	// }
	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		return errorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return errorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, &textResponse.Usage
}
