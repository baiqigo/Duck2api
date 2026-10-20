package initialize

import (
	"aurora/httpclient"
	"aurora/httpclient/bogdanfinn"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const duckDuckGoModelsURL = "https://duck.ai/duckchat/v1/models"

type duckDuckGoModel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type duckDuckGoModelsResponse struct {
	Models []duckDuckGoModel `json:"models"`
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

func fetchDuckDuckGoModels(proxyURL string) (openAIModelsResponse, int, error) {
	client := bogdanfinn.NewStdClient()
	if proxyURL != "" {
		if err := client.SetProxy(proxyURL); err != nil {
			return openAIModelsResponse{}, http.StatusBadGateway, fmt.Errorf("configure models proxy: %w", err)
		}
	}

	headers := make(httpclient.AuroraHeaders)
	headers.Set("accept", "application/json")
	headers.Set("origin", "https://duck.ai")
	headers.Set("referer", "https://duck.ai/")
	headers.Set("user-agent", bogdanfinn.ChromeUA)

	response, err := client.Request(httpclient.GET, duckDuckGoModelsURL, headers, nil, nil)
	if err != nil {
		return openAIModelsResponse{}, http.StatusBadGateway, fmt.Errorf("request DuckDuckGo models: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return openAIModelsResponse{}, http.StatusBadGateway, fmt.Errorf("read DuckDuckGo models response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return openAIModelsResponse{}, response.StatusCode, fmt.Errorf("DuckDuckGo models returned %s: %s", response.Status, string(body))
	}

	models, err := parseDuckDuckGoModels(body, time.Now().Unix())
	if err != nil {
		return openAIModelsResponse{}, http.StatusBadGateway, err
	}
	return models, http.StatusOK, nil
}

func parseDuckDuckGoModels(body []byte, created int64) (openAIModelsResponse, error) {
	var upstream duckDuckGoModelsResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return openAIModelsResponse{}, fmt.Errorf("decode DuckDuckGo models response: %w", err)
	}

	result := openAIModelsResponse{
		Object: "list",
		Data:   make([]openAIModel, 0, len(upstream.Models)),
	}
	for _, model := range upstream.Models {
		if model.ID == "" {
			continue
		}
		result.Data = append(result.Data, openAIModel{
			ID:      model.ID,
			Object:  "model",
			Created: created,
			OwnedBy: model.Provider,
		})
	}
	return result, nil
}
