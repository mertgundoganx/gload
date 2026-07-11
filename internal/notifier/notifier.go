package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type TestResult struct {
	ServiceName string  `json:"service_name"`
	ServiceURL  string  `json:"service_url"`
	Status      string  `json:"status"` // "pass" or "fail"
	TotalReqs   int     `json:"total_reqs"`
	RPS         float64 `json:"rps"`
	AvgLatency  float64 `json:"avg_latency_ms"`
	P95Latency  float64 `json:"p95_latency_ms"`
	P99Latency  float64 `json:"p99_latency_ms"`
	ErrorRate   float64 `json:"error_rate"`
	Duration    float64 `json:"duration_ms"`
}

// SendWebhook sends a POST to the given URL with test results as JSON body.
func SendWebhook(url string, result TestResult) error {
	body, _ := json.Marshal(result)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SendSlack sends a formatted Slack message via incoming webhook URL.
func SendSlack(webhookURL string, result TestResult) error {
	emoji := ":white_check_mark:"
	color := "#10b981"
	if result.Status == "fail" {
		emoji = ":x:"
		color = "#ef4444"
	}

	text := fmt.Sprintf("%s *%s* test completed — *%s*", emoji, result.ServiceName, strings.ToUpper(result.Status))

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"blocks": []map[string]interface{}{
					{"type": "section", "text": map[string]string{"type": "mrkdwn", "text": text}},
					{"type": "section", "fields": []map[string]string{
						{"type": "mrkdwn", "text": fmt.Sprintf("*RPS:* %.1f", result.RPS)},
						{"type": "mrkdwn", "text": fmt.Sprintf("*Avg Latency:* %.1fms", result.AvgLatency)},
						{"type": "mrkdwn", "text": fmt.Sprintf("*P95:* %.1fms", result.P95Latency)},
						{"type": "mrkdwn", "text": fmt.Sprintf("*Error Rate:* %.1f%%", result.ErrorRate)},
						{"type": "mrkdwn", "text": fmt.Sprintf("*Total Requests:* %d", result.TotalReqs)},
						{"type": "mrkdwn", "text": fmt.Sprintf("*Duration:* %.1fs", result.Duration/1000)},
					}},
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SendTeams sends a formatted message to Microsoft Teams via incoming webhook URL.
func SendTeams(webhookURL string, result TestResult) error {
	statusEmoji := "✅"
	themeColor := "10b981"
	if result.Status == "fail" {
		statusEmoji = "❌"
		themeColor = "ef4444"
	}

	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    fmt.Sprintf("gload: %s test %s", result.ServiceName, strings.ToUpper(result.Status)),
		"sections": []map[string]interface{}{
			{
				"activityTitle": fmt.Sprintf("%s **%s** test completed — **%s**", statusEmoji, result.ServiceName, strings.ToUpper(result.Status)),
				"facts": []map[string]string{
					{"name": "URL", "value": result.ServiceURL},
					{"name": "RPS", "value": fmt.Sprintf("%.1f", result.RPS)},
					{"name": "Avg Latency", "value": fmt.Sprintf("%.1f ms", result.AvgLatency)},
					{"name": "P95 Latency", "value": fmt.Sprintf("%.1f ms", result.P95Latency)},
					{"name": "Error Rate", "value": fmt.Sprintf("%.1f%%", result.ErrorRate)},
					{"name": "Total Requests", "value": fmt.Sprintf("%d", result.TotalReqs)},
					{"name": "Duration", "value": fmt.Sprintf("%.1fs", result.Duration/1000)},
				},
				"markdown": true,
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SendDiscord sends a formatted Discord message via webhook URL.
func SendDiscord(webhookURL string, result TestResult) error {
	emoji := ":white_check_mark:"
	color := 0x10b981 // green
	if result.Status == "fail" {
		emoji = ":x:"
		color = 0xef4444 // red
	}

	embed := map[string]interface{}{
		"title":       fmt.Sprintf("%s %s — %s", emoji, result.ServiceName, strings.ToUpper(result.Status)),
		"color":       color,
		"description": fmt.Sprintf("Load test completed for **%s**", result.ServiceURL),
		"fields": []map[string]interface{}{
			{"name": "RPS", "value": fmt.Sprintf("%.1f", result.RPS), "inline": true},
			{"name": "Avg Latency", "value": fmt.Sprintf("%.1fms", result.AvgLatency), "inline": true},
			{"name": "P95 Latency", "value": fmt.Sprintf("%.1fms", result.P95Latency), "inline": true},
			{"name": "Error Rate", "value": fmt.Sprintf("%.1f%%", result.ErrorRate), "inline": true},
			{"name": "Total Requests", "value": fmt.Sprintf("%d", result.TotalReqs), "inline": true},
			{"name": "Duration", "value": fmt.Sprintf("%.1fs", result.Duration/1000), "inline": true},
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"footer":    map[string]string{"text": "gload"},
	}

	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// Notify sends notifications based on configured settings.
func Notify(webhookURL, slackURL, teamsURL, discordURL string, result TestResult) {
	if webhookURL != "" {
		if err := SendWebhook(webhookURL, result); err != nil {
			log.Printf("webhook notification failed: %v", err)
		}
	}
	if slackURL != "" {
		if err := SendSlack(slackURL, result); err != nil {
			log.Printf("slack notification failed: %v", err)
		}
	}
	if teamsURL != "" {
		if err := SendTeams(teamsURL, result); err != nil {
			log.Printf("teams notification failed: %v", err)
		}
	}
	if discordURL != "" {
		if err := SendDiscord(discordURL, result); err != nil {
			log.Printf("discord notification failed: %v", err)
		}
	}
}
