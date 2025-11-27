package main

import (
	"fmt"
	"log"
	"time"

	"github.com/m1z23r/drift/pkg/drift"
	"github.com/m1z23r/drift/pkg/middleware"
)

// This example demonstrates SSE, Recovery, Compression, and Timeout middleware

func main() {
	app := drift.New()

	// ========================================
	// Global Middleware
	// ========================================

	// Recovery middleware - must be first to catch panics in other middleware
	app.Use(middleware.Recovery())

	// Compression middleware - compress responses globally
	// Use middleware.SkipCompression() on routes that shouldn't be compressed
	app.Use(middleware.Compress())

	// CORS
	app.Use(middleware.CORS())

	// ========================================
	// Server-Sent Events (SSE) Examples
	// ========================================

	// Simple SSE endpoint - sends time updates every second
	app.Get("/sse/time", middleware.SkipCompression(), func(c *drift.Context) {
		sse := c.SSE()

		// Send events for 10 seconds
		for i := 0; i < 10; i++ {
			err := sse.Send(time.Now().Format(time.RFC3339), "time-update", fmt.Sprintf("%d", i))
			if err != nil {
				log.Printf("SSE error: %v", err)
				return
			}
			time.Sleep(1 * time.Second)
		}
	})

	// SSE with JSON data
	app.Get("/sse/updates", middleware.SkipCompression(), func(c *drift.Context) {
		sse := c.SSE()

		updates := []map[string]any{
			{"status": "starting", "progress": 0},
			{"status": "processing", "progress": 25},
			{"status": "processing", "progress": 50},
			{"status": "processing", "progress": 75},
			{"status": "completed", "progress": 100},
		}

		for i, update := range updates {
			err := sse.SendJSON(update, "progress", fmt.Sprintf("%d", i))
			if err != nil {
				log.Printf("SSE error: %v", err)
				return
			}
			time.Sleep(2 * time.Second)
		}
	})

	// SSE with keepalive comments
	app.Get("/sse/keepalive", middleware.SkipCompression(), func(c *drift.Context) {
		sse := c.SSE()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		commentTicker := time.NewTicker(30 * time.Second)
		defer commentTicker.Stop()

		timeout := time.After(2 * time.Minute)

		for {
			select {
			case <-timeout:
				sse.Send("Connection closing", "close", "")
				return
			case t := <-ticker.C:
				err := sse.SendJSON(map[string]any{
					"timestamp": t.Unix(),
					"message":   "Heartbeat",
				}, "heartbeat", "")
				if err != nil {
					return
				}
			case <-commentTicker.C:
				// Send a comment to keep connection alive
				sse.SendComment("keepalive")
			}
		}
	})

	// SSE test page
	app.Get("/sse", func(c *drift.Context) {
		c.HTML(200, `
<!DOCTYPE html>
<html>
<head>
	<title>SSE Demo</title>
	<style>
		body { font-family: sans-serif; padding: 20px; }
		.event { padding: 10px; margin: 5px 0; background: #f0f0f0; border-radius: 5px; }
		button { padding: 10px 20px; margin: 5px; cursor: pointer; }
	</style>
</head>
<body>
	<h1>Server-Sent Events Demo</h1>

	<div>
		<button onclick="connectTime()">Time Updates</button>
		<button onclick="connectProgress()">Progress Updates</button>
		<button onclick="connectKeepalive()">Keepalive Demo</button>
		<button onclick="disconnect()">Disconnect</button>
	</div>

	<div id="events"></div>

	<script>
		let eventSource = null;

		function connectTime() {
			disconnect();
			eventSource = new EventSource('/sse/time');
			eventSource.addEventListener('time-update', e => {
				addEvent('Time Update', e.data, e.lastEventId);
			});
			eventSource.onerror = () => addEvent('Error', 'Connection lost');
		}

		function connectProgress() {
			disconnect();
			eventSource = new EventSource('/sse/updates');
			eventSource.addEventListener('progress', e => {
				const data = JSON.parse(e.data);
				addEvent('Progress', JSON.stringify(data, null, 2), e.lastEventId);
			});
			eventSource.onerror = () => addEvent('Error', 'Connection lost');
		}

		function connectKeepalive() {
			disconnect();
			eventSource = new EventSource('/sse/keepalive');
			eventSource.addEventListener('heartbeat', e => {
				const data = JSON.parse(e.data);
				addEvent('Heartbeat', new Date(data.timestamp * 1000).toLocaleTimeString());
			});
			eventSource.addEventListener('close', e => {
				addEvent('Server', e.data);
				disconnect();
			});
			eventSource.onerror = () => addEvent('Error', 'Connection lost');
		}

		function disconnect() {
			if (eventSource) {
				eventSource.close();
				eventSource = null;
				addEvent('Client', 'Disconnected');
			}
		}

		function addEvent(type, data, id = '') {
			const div = document.createElement('div');
			div.className = 'event';
			div.innerHTML = '<strong>' + type + (id ? ' #' + id : '') + ':</strong> ' + data;
			document.getElementById('events').prepend(div);
		}
	</script>
</body>
</html>
		`)
	})

	// ========================================
	// Recovery Middleware Examples
	// ========================================

	// Route that panics - will be caught by recovery middleware
	app.Get("/panic", func(c *drift.Context) {
		panic("Something went terribly wrong!")
	})

	// Route that panics with custom error
	app.Get("/panic-custom", func(c *drift.Context) {
		panic(fmt.Errorf("custom error: database connection failed"))
	})

	// ========================================
	// Compression Examples
	// ========================================

	// Large JSON response that will be compressed
	app.Get("/large-json", func(c *drift.Context) {
		data := make([]map[string]any, 1000)
		for i := 0; i < 1000; i++ {
			data[i] = map[string]any{
				"id":          i,
				"name":        fmt.Sprintf("User %d", i),
				"email":       fmt.Sprintf("user%d@example.com", i),
				"description": "Lorem ipsum dolor sit amet, consectetur adipiscing elit",
			}
		}
		c.JSON(200, data)
	})

	// ========================================
	// Timeout Middleware Examples
	// ========================================

	// Route with timeout - completes quickly
	app.Get("/quick", middleware.TimeoutWithDuration(5*time.Second), func(c *drift.Context) {
		time.Sleep(1 * time.Second)
		c.JSON(200, map[string]string{
			"message": "Completed successfully",
		})
	})

	// Route with timeout - will timeout
	app.Get("/slow", middleware.TimeoutWithDuration(2*time.Second), func(c *drift.Context) {
		time.Sleep(5 * time.Second) // This will timeout
		c.JSON(200, map[string]string{
			"message": "This should not be seen",
		})
	})

	// Route with custom timeout handler
	app.Get("/slow-custom", middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 2 * time.Second,
		Handler: func(c *drift.Context) {
			c.JSON(408, map[string]any{
				"error":   "Request took too long",
				"timeout": "2s",
				"message": "Please try again with a smaller request",
			})
		},
	}), func(c *drift.Context) {
		time.Sleep(5 * time.Second)
		c.JSON(200, map[string]string{"message": "Won't be seen"})
	})

	// ========================================
	// Combined Examples
	// ========================================

	// File streaming with compression
	app.Get("/download", func(c *drift.Context) {
		// Create a large text file in memory for demo
		data := []byte("")
		for i := 0; i < 10000; i++ {
			data = append(data, []byte(fmt.Sprintf("Line %d: This is some example text data\n", i))...)
		}
		c.StreamBytes(200, "text/plain", data)
	})

	// Info endpoint
	app.Get("/", func(c *drift.Context) {
		c.JSON(200, map[string]any{
			"message": "Drift Framework - Advanced Features Demo",
			"endpoints": map[string]string{
				"GET /sse":           "SSE demo page with interactive examples",
				"GET /sse/time":      "SSE endpoint - time updates every second",
				"GET /sse/updates":   "SSE endpoint - progress updates with JSON",
				"GET /sse/keepalive": "SSE endpoint - keepalive demo",
				"GET /panic":         "Test recovery middleware (will panic)",
				"GET /panic-custom":  "Test recovery with custom error",
				"GET /large-json":    "Large JSON response (compressed)",
				"GET /download":      "Download large file (compressed)",
				"GET /quick":         "Quick response (5s timeout)",
				"GET /slow":          "Slow response (will timeout after 2s)",
				"GET /slow-custom":   "Slow response with custom timeout handler",
			},
		})
	})

	// Start server
	log.Println("Starting server with advanced features on http://localhost:8080")
	log.Println("Visit http://localhost:8080/sse for the SSE demo page")
	log.Fatal(app.Run(":8080"))
}
