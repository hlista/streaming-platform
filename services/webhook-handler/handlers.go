package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

type StreamEvent struct {
	StreamKey string `json:"stream_key"`
	Path      string `json:"path"`
	Query     string `json:"query"`
	IP        string `json:"ip"`
}

type StreamStartEvent struct {
	StreamKey string `json:"stream_key"`
	Path      string `json:"path"`
}

type StreamState struct {
	StreamKey   string    `json:"stream_key"`
	StartTime   time.Time `json:"start_time"`
	ViewerCount int       `json:"viewer_count"`
	IsLive      bool      `json:"is_live"`
}

func streamStartHandler(c *gin.Context) {
	timer := prometheus.NewTimer(webhookDuration.WithLabelValues("stream_start"))
	defer timer.ObserveDuration()

	var event StreamStartEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Stream started: %s", event.StreamKey)

	ctx := context.Background()

	// Create stream state
	state := StreamState{
		StreamKey:   event.StreamKey,
		StartTime:   time.Now(),
		ViewerCount: 0,
		IsLive:      true,
	}

	stateJSON, _ := json.Marshal(state)

	// Store in Redis
	pipe := rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("stream:%s", event.StreamKey), stateJSON, 0)
	pipe.SAdd(ctx, "live_streams", event.StreamKey)
	pipe.Publish(ctx, "stream_events", fmt.Sprintf("start:%s", event.StreamKey))

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Failed to update Redis: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Update metrics
	streamStartCounter.WithLabelValues(event.StreamKey).Inc()

	// Notify CDN coordinator
	go notifyCDN("stream_start", event.StreamKey)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func streamStopHandler(c *gin.Context) {
	timer := prometheus.NewTimer(webhookDuration.WithLabelValues("stream_stop"))
	defer timer.ObserveDuration()

	var event StreamEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Stream stopped: %s", event.StreamKey)

	ctx := context.Background()

	// Update stream state
	stateKey := fmt.Sprintf("stream:%s", event.StreamKey)
	stateJSON, err := rdb.Get(ctx, stateKey).Result()
	if err != nil {
		log.Printf("Failed to get stream state: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}

	var state StreamState
	json.Unmarshal([]byte(stateJSON), &state)
	state.IsLive = false

	updatedStateJSON, _ := json.Marshal(state)

	// Update Redis
	pipe := rdb.Pipeline()
	pipe.Set(ctx, stateKey, updatedStateJSON, 24*time.Hour) // Keep for 24h
	pipe.SRem(ctx, "live_streams", event.StreamKey)
	pipe.Del(ctx, fmt.Sprintf("viewers:%s", event.StreamKey))
	pipe.Publish(ctx, "stream_events", fmt.Sprintf("stop:%s", event.StreamKey))

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Failed to update Redis: %v", err)
	}

	// Update metrics
	concurrentViewers.WithLabelValues(event.StreamKey).Set(0)

	// Notify CDN coordinator
	go notifyCDN("stream_stop", event.StreamKey)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func viewerJoinHandler(c *gin.Context) {
	timer := prometheus.NewTimer(webhookDuration.WithLabelValues("viewer_join"))
	defer timer.ObserveDuration()

	var event StreamEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Add viewer to set
	viewerKey := fmt.Sprintf("viewers:%s", event.StreamKey)
	rdb.SAdd(ctx, viewerKey, event.IP)

	// Get viewer count
	count, _ := rdb.SCard(ctx, viewerKey).Result()

	// Update metrics
	concurrentViewers.WithLabelValues(event.StreamKey).Set(float64(count))

	// Publish event
	rdb.Publish(ctx, "viewer_events", fmt.Sprintf("join:%s:%s", event.StreamKey, event.IP))

	c.JSON(http.StatusOK, gin.H{"status": "ok", "viewer_count": count})
}

func viewerLeaveHandler(c *gin.Context) {
	timer := prometheus.NewTimer(webhookDuration.WithLabelValues("viewer_leave"))
	defer timer.ObserveDuration()

	var event StreamEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Remove viewer from set
	viewerKey := fmt.Sprintf("viewers:%s", event.StreamKey)
	rdb.SRem(ctx, viewerKey, event.IP)

	// Get viewer count
	count, _ := rdb.SCard(ctx, viewerKey).Result()

	// Update metrics
	concurrentViewers.WithLabelValues(event.StreamKey).Set(float64(count))

	// Publish event
	rdb.Publish(ctx, "viewer_events", fmt.Sprintf("leave:%s:%s", event.StreamKey, event.IP))

	c.JSON(http.StatusOK, gin.H{"status": "ok", "viewer_count": count})
}

func notifyCDN(eventType, streamKey string) {
	// This would notify the CDN coordinator service
	// Implementation depends on your CDN setup
	log.Printf("CDN notification: %s for stream %s", eventType, streamKey)
}
