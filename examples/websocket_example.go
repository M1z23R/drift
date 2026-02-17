// +build ignore

package main

import (
	"fmt"
	"log"

	"github.com/m1z23r/drift/pkg/drift"
	"github.com/m1z23r/drift/pkg/middleware"
	"github.com/m1z23r/drift/pkg/websocket"
)

func main() {
	app := drift.New()

	// Global compression - websocket routes need SkipCompression
	app.Use(middleware.Compress())

	// Regular HTTP endpoint
	app.Get("/", func(c *drift.Context) {
		c.HTML(200, `<!DOCTYPE html>
<html>
<head><title>WebSocket Example</title></head>
<body>
	<h1>WebSocket Echo Server</h1>
	<input type="text" id="msg" placeholder="Type a message">
	<button onclick="send()">Send</button>
	<pre id="log"></pre>
	<script>
		const ws = new WebSocket('ws://' + location.host + '/ws');
		const log = document.getElementById('log');
		ws.onmessage = (e) => { log.textContent += 'Received: ' + e.data + '\n'; };
		ws.onopen = () => { log.textContent += 'Connected!\n'; };
		ws.onclose = () => { log.textContent += 'Disconnected\n'; };
		function send() {
			const msg = document.getElementById('msg').value;
			ws.send(msg);
			log.textContent += 'Sent: ' + msg + '\n';
		}
	</script>
</body>
</html>`)
	})

	// WebSocket endpoint - skip compression for websocket upgrades
	app.Get("/ws", middleware.SkipCompression(), func(c *drift.Context) {
		// Upgrade the connection
		conn, err := websocket.Upgrade(c)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close(websocket.CloseNormalClosure, "bye")

		log.Printf("WebSocket connected from %s", conn.RemoteAddr())

		// Echo loop
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				break
			}

			fmt.Printf("Received [%d]: %s\n", msgType, string(data))

			// Echo back
			if err := conn.WriteMessage(msgType, data); err != nil {
				log.Printf("Write error: %v", err)
				break
			}
		}
	})

	log.Println("Starting server on :8080")
	app.Run(":8080")
}
