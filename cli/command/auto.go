package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func autoCmd() *cobra.Command {
	var domain, kind string
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Create a watcher twin, start watch mode, and stream events",
		RunE: func(_ *cobra.Command, _ []string) error {
			// 1. Create a watcher twin
			twinData, _, err := api.Post("/twins", map[string]string{
				"name":   "watcher",
				"kind":   kind,
				"domain": domain,
			})
			if err != nil {
				return fmt.Errorf("create twin: %w", err)
			}
			var twinResp struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(twinData, &twinResp); err != nil {
				return fmt.Errorf("parse twin response: %w", err)
			}
			fmt.Printf("Watcher twin created: %s\n", twinResp.ID)

			// 2. Create a watch objective
			objData, _, err := api.Post("/objectives", map[string]any{
				"title":       "Autonomous watch",
				"description": "Continuous environment monitoring in watch mode",
				"domain":      domain,
				"twin_id":     twinResp.ID,
			})
			if err != nil {
				return fmt.Errorf("create objective: %w", err)
			}
			var objResp struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(objData, &objResp); err != nil {
				return fmt.Errorf("parse objective response: %w", err)
			}
			fmt.Printf("Watch objective created: %s\n", objResp.ID)

			// 3. Start the loop with watch_mode: true
			loopData, _, err := api.Post("/loops", map[string]any{
				"objective_id": objResp.ID,
				"twin_id":      twinResp.ID,
				"watch_mode":   true,
			})
			if err != nil {
				return fmt.Errorf("start loop: %w", err)
			}
			var loopResp struct {
				LoopID string `json:"loop_id"`
			}
			if err := json.Unmarshal(loopData, &loopResp); err != nil {
				return fmt.Errorf("parse loop response: %w", err)
			}
			fmt.Printf("Watch loop started: %s\n", loopResp.LoopID)
			fmt.Printf("Streaming events for objective %s (Ctrl+C to stop)...\n\n", objResp.ID)

			// 4. Stream SSE events from /objectives/<id>/events
			return streamSSE(api.BaseURL+"/objectives/"+objResp.ID+"/events", api.Token)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "software", "Domain for the watcher twin")
	cmd.Flags().StringVar(&kind, "kind", "team", "Twin kind")
	return cmd
}

// streamSSE connects to an SSE endpoint and prints each event to stdout
// until the context is cancelled (Ctrl+C).
func streamSSE(url, token string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to event stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("event stream returned status %d", resp.StatusCode)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)
				// Pretty-print if JSON, else print raw
				var v any
				if json.Unmarshal([]byte(data), &v) == nil {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					_ = enc.Encode(v)
				} else {
					fmt.Println(data)
				}
			}
		}
	}()

	select {
	case <-sigCh:
		fmt.Println("\nInterrupted. Watch mode stopped.")
	case <-done:
		fmt.Println("Event stream closed.")
	}
	return nil
}
