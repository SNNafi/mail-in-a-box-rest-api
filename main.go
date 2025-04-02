package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EmailRequest represents the structure of the incoming email request
type EmailRequest struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Content string   `json:"content"`
	Title   string   `json:"title,omitempty"` // it will handle from title e.g Title <sender email> in the receiver's inbox
}

// RateLimiter implements a token bucket rate limiting mechanism
type RateLimiter struct {
	mutex           sync.Mutex
	tokens          map[string]int
	lastRefill      map[string]time.Time
	maxPerSec       int
	bucketSize      int
	cleanupInterval time.Duration
}

// NewRateLimiter creates a new rate limiter with specified rate per second
func NewRateLimiter(maxPerSec int) *RateLimiter {
	// Bucket size is double the rate to allow for some bursting
	bucketSize := maxPerSec * 2

	rl := &RateLimiter{
		tokens:          make(map[string]int),
		lastRefill:      make(map[string]time.Time),
		maxPerSec:       maxPerSec,
		bucketSize:      bucketSize,
		cleanupInterval: 30 * time.Minute, // Clean up every 30 minutes
	}

	// Start the cleanup goroutine
	go rl.periodicCleanup()

	return rl
}

// Allow checks if the user has exceeded their rate limit
func (rl *RateLimiter) Allow(user string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	lastTime, exists := rl.lastRefill[user]

	// Initialize if first request
	if !exists {
		rl.tokens[user] = rl.bucketSize
		rl.lastRefill[user] = now
	} else {
		// Calculate tokens to add based on time elapsed
		elapsed := now.Sub(lastTime).Seconds()
		tokensToAdd := int(elapsed * float64(rl.maxPerSec))

		if tokensToAdd > 0 {
			rl.tokens[user] = min(rl.tokens[user]+tokensToAdd, rl.bucketSize)
			rl.lastRefill[user] = now
		}
	}

	// Check if any tokens available
	if rl.tokens[user] <= 0 {
		return false
	}

	// Consume a token and allow
	rl.tokens[user]--
	return true
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// periodicCleanup runs at regular intervals to remove inactive users
func (rl *RateLimiter) periodicCleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanupInactiveBuckets()
	}
}

// cleanupInactiveBuckets removes user buckets that haven't been used in a while
func (rl *RateLimiter) cleanupInactiveBuckets() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Consider users inactive if they haven't made a request in 1 hour
	inactiveThreshold := time.Now().Add(-1 * time.Hour)

	// Identify inactive users
	var inactiveUsers []string
	for user, lastTime := range rl.lastRefill {
		if lastTime.Before(inactiveThreshold) {
			inactiveUsers = append(inactiveUsers, user)
		}
	}

	// Remove inactive users
	for _, user := range inactiveUsers {
		delete(rl.tokens, user)
		delete(rl.lastRefill, user)
	}

	// Log cleanup results if any users were removed
	if len(inactiveUsers) > 0 {
		log.Printf("Rate limiter cleanup: removed %d inactive users, current user count: %d",
			len(inactiveUsers), len(rl.tokens))
	}
}

// isHTML checks if the content appears to be HTML
func isHTML(content string) bool {
	htmlPattern := regexp.MustCompile(`(?i)<html|<body|<div|<p>|<table|<a\s+href|<img|<span|<h[1-6]|<!DOCTYPE html>`)
	return htmlPattern.MatchString(content)
}

// GetMailHandler creates an HTTP handler for sending emails
func GetMailHandler(rateLimiter *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse Basic Authentication header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Basic ") {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Decode credentials
		credentials, err := base64.StdEncoding.DecodeString(authHeader[6:])
		if err != nil {
			http.Error(w, "Invalid authentication format", http.StatusUnauthorized)
			return
		}

		// Split username and password
		parts := strings.SplitN(string(credentials), ":", 2)
		if len(parts) != 2 {
			http.Error(w, "Invalid authentication format", http.StatusUnauthorized)
			return
		}
		username := parts[0]
		password := parts[1]

		// Check rate limit
		if !rateLimiter.Allow(username) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Parse request body
		var emailReq EmailRequest
		if err := json.NewDecoder(r.Body).Decode(&emailReq); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if len(emailReq.To) == 0 || emailReq.Subject == "" || emailReq.Content == "" {
			http.Error(w, "Missing required fields (to, subject, content)", http.StatusBadRequest)
			return
		}

		// Determine if content is HTML
		isHTMLContent := isHTML(emailReq.Content)

		title := username
		if emailReq.Title != "" {
			// Use the provided from name
			title = fmt.Sprintf("\"%s\" <%s>", emailReq.Title, username)
		} else if strings.Contains(username, "@") {
			// Extract the username part before @ symbol
			parts := strings.Split(username, "@")
			if len(parts) > 0 {
				displayName := strings.Title(parts[0])
				title = fmt.Sprintf("\"%s\" <%s>", displayName, username)
			}
		}

		// Build email message with proper MIME headers
		var msg string
		if isHTMLContent {
			msg = fmt.Sprintf("From: %s\n"+
				"To: %s\n"+
				"Subject: %s\n"+
				"MIME-Version: 1.0\n"+
				"Content-Type: text/html; charset=UTF-8\n\n%s",
				title,
				strings.Join(emailReq.To, ", "),
				emailReq.Subject,
				emailReq.Content)
		} else {
			msg = fmt.Sprintf("From: %s\n"+
				"To: %s\n"+
				"Subject: %s\n"+
				"Content-Type: text/plain; charset=UTF-8\n\n%s",
				title,
				strings.Join(emailReq.To, ", "),
				emailReq.Subject,
				emailReq.Content)
		}

		// Connect to mail server and send email (using localhost since we're on the same server)
		auth := smtp.PlainAuth("", username, password, "box.domain.com")
		err = smtp.SendMail("box.domain.com:587", auth, username, emailReq.To, []byte(msg))
		if err != nil {
			log.Printf("Failed to send email: %v", err)
			http.Error(w, "Failed to send email: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Log success with content type info
		log.Printf("Email sent from %s to %v (HTML: %v)", username, emailReq.To, isHTMLContent)

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Email sent successfully",
		})
	}
}

func main() {
	// Create a rate limiter allowing 10 emails per second per user & a burst of 20
	rateLimiter := NewRateLimiter(10)

	// Register handlers
	http.HandleFunc("/mail/send", GetMailHandler(rateLimiter))

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	port := 1112 // change port if you want
	log.Printf("Starting mail API server on port %d", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
