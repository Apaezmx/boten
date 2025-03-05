package models

import "os"

type Provider struct {
	Name         string
	Models       []Model
	ApiKey       string
	GetApiKeyURL string
	GetOutputFn  func(any) string
	UrlFn        func(string, string) string
	AuthFn       func(string, string) string
	BodyFn       func(string, string, string) string
}

type Model struct {
	Name      string
	Code      string
	URLSuffix string
}

var Models = map[string]Provider{
	"Gemini": {
		Name:   "Gemini",
		ApiKey: os.Getenv("GEMINI_API_KEY"),
		Models: []Model{
			{
				Name:      "Gemini 2.0 Flash",
				Code:      "gemini-2.0-flash",
				URLSuffix: "gemini-2.0-flash",
			},
			{
				Name:      "Gemini 2.0 Flash-Lite Preview",
				Code:      "gemini-2.0-flash-lite-preview-02-05",
				URLSuffix: "gemini-2.0-flash-lite",
			},
			{
				Name:      "Gemini 1.5 Flash",
				Code:      "gemini-1.5-flash",
				URLSuffix: "gemini-1.5-flash",
			},
			{
				Name:      "Gemini 1.5 Flash-8B",
				Code:      "gemini-1.5-flash-8b",
				URLSuffix: "gemini-1.5-flash-8b",
			},
			{
				Name:      "Gemini 1.5 Pro",
				Code:      "gemini-1.5-pro",
				URLSuffix: "gemini-1.5-pro",
			},
		},
		GetApiKeyURL: "https://aistudio.google.com/apikey",
		GetOutputFn: func(x any) string {
			return x.(map[string]any)["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
		},
		UrlFn: func(apiKey, model string) string {
			return "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent?key=" + apiKey
		},
		AuthFn: func(apiKey, model string) string {
			return ""
		},
		BodyFn: func(apiKey, model, prompt string) string {
			return `{
				"contents": [
				  {
					"parts": [
					  {
						"text": "` + prompt + `"
					  }
					]
				  }
				]
			  }`
		},
	},
	"OpenAI": {
		Name:         "OpenAI",
		Models:       []Model{},
		ApiKey:       os.Getenv("OPENAI_API_KEY"),
		GetApiKeyURL: "https://platform.openai.com/settings/organization/api-keys",
		GetOutputFn: func(x any) string {
			return x.(map[string]any)["choices"].([]any)[0].(map[string]any)["text"].(string)
		},
		UrlFn: func(apiKey, model string) string {
			return "https://api.openai.com/v1/chat/completions"
		},
		AuthFn: func(apiKey, model string) string {
			return "Bearer " + apiKey
		},
		BodyFn: func(apiKey, model, prompt string) string {
			return `{
				"model": "` + model + `",
				"temperature": 0.7,
				"messages": [
				  {
					"role": "user",
					"content": "` + prompt + `"
				  }
				]
			  }`
		},
	},
}
