package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type PathInfo struct {
	Name      string     `json:"name"`
	Source    *Source    `json:"source"`
	Readers   []Reader   `json:"readers"`
	Ready     bool       `json:"ready"`
	ReadyTime *time.Time `json:"readyTime"`
}

type Source struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Reader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type PathsList struct {
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
	Items     []PathInfo `json:"items"`
}

var (
	rdb           *redis.Client
	activeStreams = make(map[string]bool)
	viewerCounts  = make(map[string]int)
)

func main() {
	// Initialize Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Start monitoring loop
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	log.Println("Starting MediaMTX monitor...")

	for range ticker.C {
		checkStreams(ctx)
	}
}

func checkStreams(ctx context.Context) {
	resp, err := http.Get("http://mediamtx:9997/v3/paths/list")
	if err != nil {
		log.Printf("Failed to get paths: %v", err)
		return
	}
	defer resp.Body.Close()

	var paths PathsList
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		log.Printf("Failed to decode paths: %v", err)
		return
	}

	currentStreams := make(map[string]bool)

	for _, info := range paths.Items {
		if info.Ready && info.Source != nil {
			streamKey := extractStreamKey(info.Name)
			if streamKey == "" {
				continue
			}

			currentStreams[streamKey] = true

			// Check if this is a new stream
			if !activeStreams[streamKey] {
				log.Printf("Stream started: %s", streamKey)
				onStreamStart(ctx, streamKey)
			}

			// Update viewer count
			newCount := len(info.Readers)
			if oldCount, exists := viewerCounts[streamKey]; !exists || oldCount != newCount {
				viewerCounts[streamKey] = newCount
				updateViewerCount(ctx, streamKey, newCount)
			}
		}
	}

	// Check for stopped streams
	for streamKey := range activeStreams {
		if !currentStreams[streamKey] {
			log.Printf("Stream stopped: %s", streamKey)
			onStreamStop(ctx, streamKey)
			delete(viewerCounts, streamKey)
		}
	}

	activeStreams = currentStreams
}

func extractStreamKey(path string) string {
	// Extract stream key from path like "stream/teststream"
	if len(path) > 7 && path[:7] == "stream/" {
		return path[7:]
	}
	return ""
}

func onStreamStart(ctx context.Context, streamKey string) {
	state := map[string]interface{}{
		"stream_key":   streamKey,
		"start_time":   time.Now(),
		"viewer_count": 0,
		"is_live":      true,
	}

	stateJSON, _ := json.Marshal(state)

	pipe := rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("stream:%s", streamKey), stateJSON, 0)
	pipe.SAdd(ctx, "live_streams", streamKey)
	pipe.Publish(ctx, "stream_events", fmt.Sprintf("start:%s", streamKey))

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Failed to update Redis: %v", err)
	}
}

func onStreamStop(ctx context.Context, streamKey string) {
	pipe := rdb.Pipeline()
	pipe.Del(ctx, fmt.Sprintf("stream:%s", streamKey))
	pipe.SRem(ctx, "live_streams", streamKey)
	pipe.Del(ctx, fmt.Sprintf("viewers:%s", streamKey))
	pipe.Publish(ctx, "stream_events", fmt.Sprintf("stop:%s", streamKey))

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Failed to update Redis: %v", err)
	}
}

func updateViewerCount(ctx context.Context, streamKey string, count int) {
	log.Printf("Viewer count for %s: %d", streamKey, count)

	pipe := rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("viewers:%s:count", streamKey), count, 0)
	pipe.Publish(ctx, "viewer_events", fmt.Sprintf("count:%s:%d", streamKey, count))

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Failed to update viewer count: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
