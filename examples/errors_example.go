package main

import (
	"log"

	"github.com/m1z23r/drift/pkg/drift"
)

func main() {
	app := drift.New()

	// ========================================
	// Common HTTP Error Examples
	// ========================================

	// 400 Bad Request
	app.Get("/bad-request", func(c *drift.Context) {
		c.BadRequest("Missing required field: email")
	})

	// 401 Unauthorized
	app.Get("/unauthorized", func(c *drift.Context) {
		c.Unauthorized("Invalid or expired token")
	})

	// 403 Forbidden
	app.Get("/forbidden", func(c *drift.Context) {
		c.Forbidden("You don't have permission to access this resource")
	})

	// 404 Not Found
	app.Get("/not-found", func(c *drift.Context) {
		c.NotFound("User not found")
	})

	// 409 Conflict
	app.Post("/users", func(c *drift.Context) {
		// Simulate duplicate email
		c.Conflict("A user with this email already exists")
	})

	// 422 Unprocessable Entity
	app.Post("/validate", func(c *drift.Context) {
		c.UnprocessableEntity("Validation failed")
	})

	// 429 Too Many Requests
	app.Get("/rate-limited", func(c *drift.Context) {
		c.TooManyRequests("Rate limit exceeded. Try again in 60 seconds")
	})

	// 500 Internal Server Error
	app.Get("/server-error", func(c *drift.Context) {
		c.InternalServerError("Database connection failed")
	})

	// 503 Service Unavailable
	app.Get("/unavailable", func(c *drift.Context) {
		c.ServiceUnavailable("Service is temporarily down for maintenance")
	})

	// ========================================
	// Custom Error Examples
	// ========================================

	// Custom status code with message
	app.Get("/teapot", func(c *drift.Context) {
		c.Error(418, "I'm a teapot - I cannot brew coffee")
	})

	// Custom error with structured data
	app.Post("/register", func(c *drift.Context) {
		c.ErrorWithData(422, map[string]any{
			"error":   "Validation failed",
			"message": "The following fields have errors",
			"fields": map[string]string{
				"email":    "Email format is invalid",
				"password": "Password must be at least 8 characters",
				"age":      "Must be 18 or older",
			},
			"timestamp": "2024-11-30T12:00:00Z",
		})
	})

	// Using HTTPError type directly
	app.Get("/custom-http-error", func(c *drift.Context) {
		err := drift.NewHTTPError(503, "External API is down")
		log.Printf("Error occurred: %v", err.Error())
		c.AbortWithStatusJSON(err.Code, err)
	})

	// ========================================
	// Real-world Usage Examples
	// ========================================

	// Authentication middleware example
	authMiddleware := func(c *drift.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.Unauthorized("Authentication required")
			return
		}
		if token != "Bearer valid-token" {
			c.Forbidden("Invalid token")
			return
		}
		c.Next()
	}

	// Protected endpoint
	app.Get("/protected", authMiddleware, func(c *drift.Context) {
		c.JSON(200, map[string]string{
			"message": "Welcome to the protected area",
		})
	})

	// Validation example
	app.Post("/api/orders", func(c *drift.Context) {
		var order map[string]any
		if err := c.BindJSON(&order); err != nil {
			c.BadRequest("Invalid JSON format")
			return
		}

		// Validate required fields
		if _, ok := order["product_id"]; !ok {
			c.UnprocessableEntity("product_id is required")
			return
		}

		if _, ok := order["quantity"]; !ok {
			c.UnprocessableEntity("quantity is required")
			return
		}

		c.JSON(201, map[string]string{
			"message": "Order created successfully",
		})
	})

	// Resource not found example
	app.Get("/api/posts/:id", func(c *drift.Context) {
		id := c.Param("id")

		// Simulate database lookup
		if id != "1" {
			c.NotFound("Post not found")
			return
		}

		c.JSON(200, map[string]any{
			"id":    id,
			"title": "Example Post",
		})
	})

	// Error with default message (using standard HTTP status text)
	app.Get("/default-error", func(c *drift.Context) {
		// Empty string uses default message "Bad Request"
		c.BadRequest("")
	})

	// ========================================
	// Start Server
	// ========================================

	log.Println("Starting Drift Error Examples on http://localhost:8080")
	log.Println("\nTest the error endpoints:")
	log.Println("  curl http://localhost:8080/bad-request")
	log.Println("  curl http://localhost:8080/unauthorized")
	log.Println("  curl http://localhost:8080/forbidden")
	log.Println("  curl http://localhost:8080/not-found")
	log.Println("  curl http://localhost:8080/server-error")
	log.Println("  curl http://localhost:8080/teapot")
	log.Println("  curl -X POST http://localhost:8080/register")
	log.Println("  curl http://localhost:8080/protected")
	log.Println("  curl -H 'Authorization: Bearer valid-token' http://localhost:8080/protected")

	if err := app.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
