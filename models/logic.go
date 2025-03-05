package models

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CallLLM calls the language model with the given board, provider, model, availableMoves, and invalidMoves.
// Returns the move and the reason.
func CallLLM(board, provider, model string, availableMoves, invalidMoves []string) (string, string, error) {
	move, reason, err := callLLM(board, provider, model, availableMoves, invalidMoves)
	if err != nil {
		return "", "", fmt.Errorf("LLM call returned an error: %v", err)
	}
	return move, reason, nil
}

func callLLM(board, provider, model string, availableMoves, invalidMoves []string) (string, string, error) {
	p, ok := Models[provider]
	if !ok {
		return "", "", fmt.Errorf("Provider %s not found", provider)
	}
	modelURL := p.UrlFn(p.ApiKey, model)
	parsedURL, err := url.Parse(modelURL)
	if err != nil {
		return "", "", fmt.Errorf("error in parsing URL: %v", err)
	}
	auth := p.AuthFn(p.ApiKey, model)
	prompt := fmt.Sprintf(`
	You are playing chess against the human. Act as if you were a chess grandmaster of the highest ELO rating.
	Here is the current fen string representation of the board:
	%s
	Here are all the possible moves:
	%s
	Please return the next move you would take to win the match using SAN notation. ON YOUR REPONSE FIRST LINE WRITE THE MOVE ONLY.
	In a second line add at most a paragraph about why you chose that move. These are INVALID moves:
	%s
	`, board, strings.Join(availableMoves, ", "), strings.Join(invalidMoves, ", "))
	body := p.BodyFn(p.ApiKey, model, prompt)
	fmt.Println("Prompting llm: ", prompt)

	req, err := http.NewRequest(http.MethodPost, parsedURL.String(), strings.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("error in creating LLM request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("error in calling LLM: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("error in calling LLM: %v", res.Status)
	}

	var out any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("error in decoding LLM response: %v", err)
	}
	text := p.GetOutputFn(out)
	move := strings.Trim(strings.Replace(strings.Split(text, "\n")[0], "...", "", -1), " \n\t")
	fmt.Println("Move ", move)
	reason := strings.Trim(text[len(move):], " \n\t")
	fmt.Println("Reason ", reason)
	return move, reason, nil
}
