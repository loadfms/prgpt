package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	openAICompletionURL = "https://api.openai.com/v1/chat/completions"
	openAIModel         = "gpt-3.5-turbo-1106"
	CONFIG_FOLDER       = "/.config/openai/"
	FILENAME            = "config.toml"
)

type OpenAIRequest struct {
	Model       string                  `json:"model"`
	Messages    []OpenAIRequestMessages `json:"messages"`
	Temperature float64                 `json:"temperature"`
}

type OpenAIRequestMessages struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIReponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

type FileConfig struct {
	ApiKey struct {
		Key string `toml:"key"`
	} `toml:"apikey"`
	Prompt struct {
		Custom string `toml:"custom"`
	} `toml:"prompt"`
}

func main() {
	var prURL string
	flag.StringVar(&prURL, "pr", "", "URL of the pull request")
	flag.Parse()

	cfg, err := loadConfig()

	if prURL == "" {
		fmt.Println("Usage: pr_review_cli -pr <PR_URL>")
		return
	}

	prDiff, err := getPRDiff(prURL)
	if err != nil {
		fmt.Println("Error fetching PR diff:", err)
		return
	}

	finalConsideration, err := generateFinalConsideration(prDiff, cfg.ApiKey.Key)
	if err != nil {
		fmt.Println("Error generating final consideration:", err)
		return
	}

	fmt.Println(finalConsideration)
}

func getPRDiff(prURL string) (string, error) {
	parts := strings.Split(prURL, "/")
	if len(parts) < 7 {
		return "", fmt.Errorf("invalid PR URL")
	}

	org := parts[3]
	repo := parts[4]
	prNumber := parts[6]

	cmd := exec.Command("gh", "pr", "diff", "-R", org+"/"+repo, prNumber)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running gh pr diff: %v", err)
	}

	return string(output), nil
}

func generateFinalConsideration(prDiff string, apiKey string) (string, error) {
	prompt := prDiff + "\nPlease provide a final consideration for this PR in Markdown format, focusing only on potential issues and ensuring the application's stability. Include an 'Approved: true/false' statement at the end for easy decision-making.Thank you!"

	message := OpenAIRequestMessages{
		Role:    "user",
		Content: prompt,
	}

	reqBody, err := json.Marshal(OpenAIRequest{
		Model:       openAIModel,
		Temperature: 0.5,
		Messages:    []OpenAIRequestMessages{message},
	})

	if err != nil {
		return "", fmt.Errorf("error marshaling OpenAI request: %v", err)
	}

	req, err := http.NewRequest("POST", openAICompletionURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("error creating request to OpenAI API: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to OpenAI API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response from OpenAI API: %v", err)
	}

	var openAIResp OpenAIReponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("error unmarshaling OpenAI response: %v", err)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no response received from OpenAI API")
	}

	return openAIResp.Choices[0].Message.Content, nil
}

func loadConfig() (result FileConfig, err error) {
	currentUser, err := user.Current()
	if err != nil {
		return result, fmt.Errorf("Error getting current user")
	}

	file, err := os.Open(currentUser.HomeDir + CONFIG_FOLDER + FILENAME)
	if err != nil {
		return result, fmt.Errorf("Error opening TOML file: %v", err)
	}
	defer file.Close()

	// Unmarshal the TOML content into a struct
	if err := toml.NewDecoder(file).Decode(&result); err != nil {
		return result, fmt.Errorf("Error parsing TOML file: %v", err)
	}

	return result, nil
}
