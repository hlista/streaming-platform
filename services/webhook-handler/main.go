package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var (
	rdb *redis.Client

	// Metrics
	streamStartCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stream_starts_total",
			Help: "Total number of stream starts",
		},
		[]string{"stream_key"},
	)

	concurrentViewers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "concurrent_viewers",
			Help: "Number of concurrent viewers",
		},
		[]string{"stream_key"},
	)

	webhookDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "webhook_duration_seconds",
			Help: "Webhook processing duration",
		},
		[]string{"webhook_type"},
	)
)

func init() {
	prometheus.MustRegister(streamStartCounter)
	prometheus.MustRegister(concurrentViewers)
	prometheus.MustRegister(webhookDuration)
}

func main() {
	// Initialize Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Setup Gin router
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Authentication endpoint
	r.POST("/auth/publish", authPublishHandler)

	// Webhook endpoints
	webhooks := r.Group("/webhooks")
	{
		webhooks.POST("/stream/start", streamStartHandler)
		webhooks.POST("/stream/stop", streamStopHandler)
		webhooks.POST("/viewer/join", viewerJoinHandler)
		webhooks.POST("/viewer/leave", viewerLeaveHandler)
	}

	// Start server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
