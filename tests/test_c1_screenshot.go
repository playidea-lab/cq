package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"
    "encoding/base64"

	"github.com/gorilla/websocket"
)

type HandsRequest struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	ID     uint64      `json:"id"`
}

type HandsResponse struct {
	Result map[string]interface{} `json:"result"`
	Error  string                 `json:"error"`
	ID     uint64                 `json:"id"`
}

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8586", Path: "/"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			
			var resp HandsResponse
			if err := json.Unmarshal(message, &resp); err != nil {
				log.Printf("json error: %v", err)
				continue
			}

			if resp.Result != nil && resp.Result["image_b64"] != nil {
				b64 := resp.Result["image_b64"].(string)
				data, _ := base64.StdEncoding.DecodeString(b64)
				os.WriteFile("tests/screenshot_result.png", data, 0644)
				log.Printf("Screenshot saved to tests/screenshot_result.png (%d bytes)", len(data))
				close(done)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	// Send CaptureScreenshot
	req := HandsRequest{
		Method: "CaptureScreenshot",
		Params: map[string]interface{}{},
		ID: 1,
	}
	reqJSON, _ := json.Marshal(req)
	err = c.WriteMessage(websocket.TextMessage, reqJSON)
	if err != nil {
		log.Println("write:", err)
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	select {
	case <-done:
		return
	case <-time.After(10 * time.Second):
		log.Println("timeout")
	case <-interrupt:
		log.Println("interrupt")
	}
}
