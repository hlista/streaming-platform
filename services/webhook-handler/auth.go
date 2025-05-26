package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Path     string `json:"path"`
	IP       string `json:"ip"`
	Action   string `json:"action"`
	Query    string `json:"query"`
}

func authPublishHandler(c *gin.Context) {
	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Auth request: user=%s, path=%s, action=%s, ip=%s",
		req.User, req.Path, req.Action, req.IP)

	// Extract stream key from path (format: /stream/{key})
	parts := strings.Split(strings.Trim(req.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "stream" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid path"})
		return
	}

	streamKey := parts[1]

	// Validate stream key
	ctx := context.Background()
	isValid, err := validateStreamKey(ctx, streamKey, req.Password)
	if err != nil {
		log.Printf("Auth error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if !isValid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Additional checks can be added here:
	// - Check if user is banned
	// - Check if stream slot is available
	// - Rate limiting

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func validateStreamKey(ctx context.Context, streamKey, password string) (bool, error) {
	// Check if stream key exists and matches password
	// This would typically check against a database

	// For now, we'll check Redis for a simple key-value pair
	storedPassword, err := rdb.Get(ctx, fmt.Sprintf("stream_key:%s", streamKey)).Result()
	if err != nil {
		// Key doesn't exist or Redis error
		return false, nil
	}

	return storedPassword == password, nil
}
