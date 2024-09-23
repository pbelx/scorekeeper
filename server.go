package main

import (
	"encoding/json"
	"fmt"

	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Define structure to hold team names and scores
type ScoreData struct {
	TeamA      string `json:"teamA"`
	TeamB      string `json:"teamB"`
	TeamAScore int    `json:"teamAScore"`
	TeamBScore int    `json:"teamBScore"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Global variables for storing scores and active WebSocket connections
var scoreData ScoreData
var connections []*websocket.Conn
var mux sync.Mutex
var scoreFile = "scores.json"

// Load scores and team names from JSON file
func loadScores() error {
	file, err := os.ReadFile(scoreFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(file, &scoreData)
	if err != nil {
		return err
	}
	return nil
}

// Save scores and team names to JSON file
func saveScores() error {
	mux.Lock()
	defer mux.Unlock()
	data, err := json.MarshalIndent(scoreData, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(scoreFile, data, 0644)
	return err
}

// Broadcast updated scores to all WebSocket clients
func broadcastScores() {
	mux.Lock()
	defer mux.Unlock()
	message := fmt.Sprintf("%s: %d | %s: %d", scoreData.TeamA, scoreData.TeamAScore, scoreData.TeamB, scoreData.TeamBScore)

	for _, conn := range connections {
		err := conn.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			log.Println("Error broadcasting to client:", err)
			conn.Close()
			removeConnection(conn)
		}
	}
}

// Remove a WebSocket connection from the list
func removeConnection(conn *websocket.Conn) {
	for i, c := range connections {
		if c == conn {
			connections = append(connections[:i], connections[i+1:]...)
			break
		}
	}
}

// Handle WebSocket connections
func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()

	// Add new connection to the list
	connections = append(connections, conn)

	// Immediately send current scores to the newly connected client
	mux.Lock()
	message := fmt.Sprintf("%s: %d | %s: %d", scoreData.TeamA, scoreData.TeamAScore, scoreData.TeamB, scoreData.TeamBScore)
	mux.Unlock()

	err = conn.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		log.Println("Error sending message to new client:", err)
		removeConnection(conn)
		conn.Close()
		return
	}

	// Listen for any further messages from the client
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading from client:", err)
			removeConnection(conn)
			break
		}
	}
}

// Update the score and save it to the file
func updateScore(c *gin.Context) {
	team := c.Param("team")
	action := c.Param("action")

	mux.Lock()
	if team == "a" {
		if action == "add" {
			scoreData.TeamAScore++
		} else if action == "subtract" && scoreData.TeamAScore > 0 {
			scoreData.TeamAScore--
		}
	} else if team == "b" {
		if action == "add" {
			scoreData.TeamBScore++
		} else if action == "subtract" && scoreData.TeamBScore > 0 {
			scoreData.TeamBScore--
		}
	}
	mux.Unlock()

	// Save updated score to file
	err := saveScores()
	if err != nil {
		log.Println("Error saving scores to file:", err)
	}

	// Broadcast updated scores to all WebSocket clients
	broadcastScores()

	c.JSON(http.StatusOK, gin.H{"message": "Score updated"})
}

func main() {
	// Load initial scores and team names from file
	err := loadScores()
	if err != nil {
		log.Println("Error loading scores from file:", err)
		os.Exit(1)
	}

	// Setup Gin router
	r := gin.Default()

	// WebSocket endpoint
	r.GET("/ws", handleWebSocket)

	// Score update endpoints
	r.POST("/score/:team/:action", updateScore)

	// Run the Gin server
	if err := r.Run(":8080"); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
