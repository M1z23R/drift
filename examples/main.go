package main

import (
	"fmt"
	"log"
	"time"

	"github.com/m1z23r/drift/pkg/drift"
	"github.com/m1z23r/drift/pkg/middleware"
)

func main() {
	// Create a new Drift engine
	app := drift.New()

	// ========================================
	// Global Middleware
	// ========================================

	// CORS middleware
	app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000", "https://example.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
	}))

	// Security headers middleware
	app.Use(middleware.Secure())

	// Body parser middleware
	app.Use(middleware.BodyParser())

	// Rate limiter - 100 requests per minute per IP
	app.Use(middleware.RateLimiter())

	// Custom logger middleware
	app.Use(func(c *drift.Context) {
		start := time.Now()
		path := c.Path()
		method := c.Method()

		c.Next()

		duration := time.Since(start)
		status := c.GetInt("status")
		if status == 0 {
			status = 200
		}
		log.Printf("%s %s - %d - %v", method, path, status, duration)
	})

	// ========================================
	// Basic Routes
	// ========================================

	app.Get("/", func(c *drift.Context) {
		c.JSON(200, map[string]any{
			"message": "Welcome to Drift Framework!",
			"version": "1.0.0",
			"features": []string{
				"HTTP method routing (.Get, .Post, etc.)",
				"Middleware chain",
				"Dynamic parameters",
				"Sub-routers",
				"Context data passing (.Set/.Get)",
				"CORS",
				"Body parsing",
				"Rate limiting",
				"CSRF protection",
				"Security headers",
			},
		})
	})

	app.Get("/hello", func(c *drift.Context) {
		name := c.DefaultQuery("name", "World")
		c.JSON(200, map[string]string{
			"message": fmt.Sprintf("Hello, %s!", name),
		})
	})

	// ========================================
	// Dynamic Parameters
	// ========================================

	app.Get("/users/:id", func(c *drift.Context) {
		id := c.Param("id")
		c.JSON(200, map[string]any{
			"user_id": id,
			"name":    "John Doe",
			"email":   fmt.Sprintf("user%s@example.com", id),
		})
	})

	app.Get("/posts/:category/:id", func(c *drift.Context) {
		category := c.Param("category")
		id := c.Param("id")
		c.JSON(200, map[string]any{
			"category": category,
			"post_id":  id,
			"title":    fmt.Sprintf("Post %s in %s", id, category),
		})
	})

	// Catch-all route
	app.Get("/files/*filepath", func(c *drift.Context) {
		filepath := c.Param("filepath")
		c.JSON(200, map[string]string{
			"filepath": filepath,
			"message":  "File route handler",
		})
	})

	// ========================================
	// Context Data Passing (Set/Get)
	// ========================================

	// Auth middleware that sets user data
	authMiddleware := func(c *drift.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatusJSON(401, map[string]string{
				"error": "Unauthorized - No token provided",
			})
			return
		}

		// Simulate authentication
		c.Set("user_id", "12345")
		c.Set("username", "johndoe")
		c.Set("role", "admin")
		c.Next()
	}

	app.Get("/protected", authMiddleware, func(c *drift.Context) {
		userId := c.GetString("user_id")
		username := c.GetString("username")
		role := c.GetString("role")

		c.JSON(200, map[string]any{
			"message":  "This is a protected route",
			"user_id":  userId,
			"username": username,
			"role":     role,
		})
	})

	// ========================================
	// POST Routes with Body Parsing
	// ========================================

	app.Post("/users", func(c *drift.Context) {
		// Get parsed body from middleware
		body, exists := c.Get("body")
		if !exists {
			c.JSON(400, map[string]string{
				"error": "Invalid request body",
			})
			return
		}

		c.JSON(201, map[string]any{
			"message": "User created successfully",
			"data":    body,
		})
	})

	app.Post("/upload", func(c *drift.Context) {
		// Handle file upload
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, map[string]string{
				"error": "No file uploaded",
			})
			return
		}

		// Save file (in production, you'd save it properly)
		// err = c.SaveUploadedFile(file, "./uploads/"+file.Filename)

		c.JSON(200, map[string]any{
			"message":  "File uploaded successfully",
			"filename": file.Filename,
			"size":     file.Size,
		})
	})

	// ========================================
	// Router Groups (Sub-routers)
	// ========================================

	// API v1 group
	v1 := app.Group("/api/v1")
	{
		// Middleware specific to v1 group
		v1.Use(func(c *drift.Context) {
			c.Header("X-API-Version", "1.0")
			c.Next()
		})

		v1.Get("/status", func(c *drift.Context) {
			c.JSON(200, map[string]string{
				"status":  "ok",
				"version": "1.0",
			})
		})

		// Nested group for admin routes
		admin := v1.Group("/admin", authMiddleware)
		{
			admin.Get("/dashboard", func(c *drift.Context) {
				c.JSON(200, map[string]string{
					"message": "Admin dashboard",
				})
			})

			admin.Get("/users", func(c *drift.Context) {
				c.JSON(200, map[string]any{
					"message": "List of all users",
					"users":   []string{"user1", "user2", "user3"},
				})
			})

			admin.Delete("/users/:id", func(c *drift.Context) {
				id := c.Param("id")
				c.JSON(200, map[string]string{
					"message": fmt.Sprintf("User %s deleted", id),
				})
			})
		}
	}

	// API v2 group
	v2 := app.Group("/api/v2")
	{
		v2.Use(func(c *drift.Context) {
			c.Header("X-API-Version", "2.0")
			c.Next()
		})

		v2.Get("/status", func(c *drift.Context) {
			c.JSON(200, map[string]any{
				"status":  "ok",
				"version": "2.0",
				"features": []string{"improved performance", "new endpoints"},
			})
		})
	}

	// ========================================
	// Per-Route Middleware
	// ========================================

	// Rate limit specific route more strictly (10 requests per minute)
	app.Get("/limited",
		middleware.PerRouteRateLimiter(10, time.Minute),
		func(c *drift.Context) {
			c.JSON(200, map[string]string{
				"message": "This route has strict rate limiting (10 req/min)",
			})
		},
	)

	// ========================================
	// CSRF Protected Routes
	// ========================================

	csrfProtected := app.Group("/csrf")
	csrfProtected.Use(middleware.CSRF())
	{
		csrfProtected.Get("/form", func(c *drift.Context) {
			token := c.GetString("csrf_token")
			c.HTML(200, fmt.Sprintf(`
				<html>
				<body>
					<h1>CSRF Protected Form</h1>
					<form method="POST" action="/csrf/submit">
						<input type="hidden" name="_csrf" value="%s">
						<input type="text" name="message" placeholder="Your message">
						<button type="submit">Submit</button>
					</form>
				</body>
				</html>
			`, token))
		})

		csrfProtected.Post("/submit", func(c *drift.Context) {
			c.JSON(200, map[string]string{
				"message": "Form submitted successfully!",
			})
		})
	}

	// ========================================
	// All HTTP Methods Example
	// ========================================

	app.Get("/resource/:id", func(c *drift.Context) {
		c.JSON(200, map[string]string{"method": "GET", "id": c.Param("id")})
	})

	app.Post("/resource", func(c *drift.Context) {
		c.JSON(201, map[string]string{"method": "POST", "message": "Resource created"})
	})

	app.Put("/resource/:id", func(c *drift.Context) {
		c.JSON(200, map[string]string{"method": "PUT", "id": c.Param("id")})
	})

	app.Patch("/resource/:id", func(c *drift.Context) {
		c.JSON(200, map[string]string{"method": "PATCH", "id": c.Param("id")})
	})

	app.Delete("/resource/:id", func(c *drift.Context) {
		c.JSON(200, map[string]string{"method": "DELETE", "id": c.Param("id")})
	})

	app.Options("/resource/:id", func(c *drift.Context) {
		c.JSON(200, map[string]string{"method": "OPTIONS"})
	})

	app.Head("/resource/:id", func(c *drift.Context) {
		c.Status(200)
	})

	// ========================================
	// Error Handling Examples
	// ========================================

	// Common HTTP errors with default messages
	app.Get("/errors/400", func(c *drift.Context) {
		c.BadRequest("")
	})

	app.Get("/errors/401", func(c *drift.Context) {
		c.Unauthorized("")
	})

	app.Get("/errors/403", func(c *drift.Context) {
		c.Forbidden("")
	})

	app.Get("/errors/404", func(c *drift.Context) {
		c.NotFound("")
	})

	app.Get("/errors/500", func(c *drift.Context) {
		c.InternalServerError("")
	})

	// HTTP errors with custom messages
	app.Get("/errors/custom-message", func(c *drift.Context) {
		c.BadRequest("The request body is missing required fields")
	})

	// Custom error with any status code
	app.Get("/errors/custom-code", func(c *drift.Context) {
		c.Error(418, "I'm a teapot")
	})

	// Custom error with custom data structure
	app.Get("/errors/custom-data", func(c *drift.Context) {
		c.ErrorWithData(422, map[string]any{
			"error": "Validation failed",
			"fields": map[string]string{
				"email":    "Invalid email format",
				"password": "Password must be at least 8 characters",
			},
		})
	})

	// Using HTTPError type directly
	app.Get("/errors/http-error", func(c *drift.Context) {
		err := drift.NewHTTPError(503, "Service temporarily unavailable")
		c.AbortWithStatusJSON(err.Code, err)
	})

	app.Get("/redirect", func(c *drift.Context) {
		c.Redirect(302, "/")
	})

	// ========================================
	// Start Server
	// ========================================

	log.Println("Starting Drift server on http://localhost:8080")
	log.Println("\nAvailable routes:")
	log.Println("  GET    /")
	log.Println("  GET    /hello?name=YourName")
	log.Println("  GET    /users/:id")
	log.Println("  GET    /posts/:category/:id")
	log.Println("  GET    /files/*filepath")
	log.Println("  GET    /protected (requires Authorization header)")
	log.Println("  POST   /users")
	log.Println("  POST   /upload")
	log.Println("  GET    /api/v1/status")
	log.Println("  GET    /api/v1/admin/dashboard")
	log.Println("  GET    /api/v2/status")
	log.Println("  GET    /limited (10 req/min)")
	log.Println("  GET    /csrf/form")
	log.Println("  POST   /csrf/submit")
	log.Println("  GET    /resource/:id")
	log.Println("  POST   /resource")
	log.Println("  PUT    /resource/:id")
	log.Println("  PATCH  /resource/:id")
	log.Println("  DELETE /resource/:id")
	log.Println("  GET    /errors/400 (Bad Request)")
	log.Println("  GET    /errors/401 (Unauthorized)")
	log.Println("  GET    /errors/403 (Forbidden)")
	log.Println("  GET    /errors/404 (Not Found)")
	log.Println("  GET    /errors/500 (Internal Server Error)")
	log.Println("  GET    /errors/custom-message")
	log.Println("  GET    /errors/custom-code")
	log.Println("  GET    /errors/custom-data")
	log.Println("  GET    /errors/http-error")

	if err := app.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
