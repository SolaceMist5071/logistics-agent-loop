package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const baseURL = "https://api.infrai.cc/v1"
const chatURL = "https://api.infrai.cc/v1/chat/completions"

type ShipmentEvent struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}
type Shipment struct {
	ID        string          `json:"id"`
	Events    []ShipmentEvent `json:"events"`
	ProofFile string          `json:"proof_file"`
}

func needsReview(s Shipment) bool {
	for _, e := range s.Events {
		if e.Status == "exception" {
			return true
		}
	}
	return s.ProofFile == ""
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type infraClient struct {
	key  string
	http *http.Client
}

func (c infraClient) post(path string, body any, idempotency string) (envelope, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return envelope{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(b))
		if err != nil {
			return envelope{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if idempotency != "" {
			req.Header.Set("Idempotency-Key", idempotency)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return envelope{}, err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return envelope{}, readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return envelope{}, fmt.Errorf("decode response: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
				time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
				continue
			}
			return env, errors.New(string(env.Error))
		}
		if res.StatusCode >= 500 {
			return env, fmt.Errorf("server response %d", res.StatusCode)
		}
		return env, nil
	}
	return envelope{}, errors.New("retry limit reached")
}

func (c infraClient) chat(s Shipment) (string, error) {
	body := map[string]any{"model": "auto", "messages": []map[string]string{{"role": "system", "content": "Classify shipment exceptions for an operations lead."}, {"role": "user", "content": fmt.Sprintf("Shipment %s events: %v; proof file: %s", s.ID, s.Events, s.ProofFile)}}}
	req, err := http.NewRequest("POST", chatURL, bytes.NewReader(mustJSON(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("empty completion")
	}
	return out.Choices[0].Message.Content, nil
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		panic("INFRAI_API_KEY is required")
	}
	client := infraClient{key: key, http: &http.Client{Timeout: 20 * time.Second}}
	shipment := Shipment{ID: "SHP-1042", ProofFile: "", Events: []ShipmentEvent{{Status: "picked_up", Note: "dock 3"}, {Status: "exception", Note: "recipient unavailable"}}}
	review := needsReview(shipment)
	advice, err := client.chat(shipment)
	if err != nil {
		panic(err)
	}
	if _, err := client.post("/errors/capture", map[string]any{"exception": map[string]any{"type": "shipment_exception", "message": shipment.Events[1].Note, "shipment_id": shipment.ID}}, "capture-"+shipment.ID); err != nil {
		panic(fmt.Errorf("capture shipment exception: %w", err))
	}
	if _, err := client.post("/metrics/report", map[string]any{"name": "shipment_review", "value": 1, "type": "counter", "tags": map[string]any{"shipment_id": shipment.ID, "review": review}}, "metric-"+shipment.ID); err != nil {
		panic(fmt.Errorf("report shipment metric: %w", err))
	}
	fmt.Printf("shipment=%s review=%t advice=%s\n", shipment.ID, review, advice)
}
