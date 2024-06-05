package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"final_project/constants"
	"github.com/go-playground/log"
	"github.com/go-playground/log/handlers/console"
	"github.com/sashabaranov/go-openai"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	conn        net.Conn
	username    string
	room        *ChatRoom
	connectedAt time.Time
}

type ChatRoom struct {
	name    string
	clients map[*Client]bool
}

var (
	clients   = make(map[net.Conn]*Client)
	rooms     = make(map[string]*ChatRoom)
	bannedIPs = make(map[string]bool)
	mu        sync.Mutex
)

func broadcastToRoom(room *ChatRoom, message string, sender *Client) {
	mu.Lock()
	defer mu.Unlock()
	for client := range room.clients {
		if client != sender {
			client.conn.Write([]byte(message + "\n"))
		}
	}
}

func getOnlineUsers(room *ChatRoom) string {
	var onlineUsers []string
	for client := range room.clients {
		onlineUsers = append(onlineUsers, client.username)
	}
	return strings.Join(onlineUsers, ", ")
}

func getGPTResponse(query string) (string, error) {
	client := openai.NewClient("sk-proj-Zyfxp7byJzcl6wXohzTtT3BlbkFJ2gYs5HQbvZ9vCgwHtUll")
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: query,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func handleRequest(conn net.Conn) {
	defer conn.Close()

	clientIP := conn.RemoteAddr().String()
	if isBanned(clientIP) {
		conn.Write([]byte("You are banned from this server.\n"))
		return
	}

	reader := bufio.NewReader(conn)
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	conn.Write([]byte("Welcome to the server! Type \"/help\" to get a list of commands.\n"))
	client := &Client{conn: conn, username: username, connectedAt: time.Now()}

	mu.Lock()
	clients[conn] = client
	mu.Unlock()

	log.Infof("New client created: %s", username)

	defer func() {
		mu.Lock()
		delete(clients, conn)
		if client.room != nil {
			delete(client.room.clients, client)
		}
		mu.Unlock()
		log.Infof("Client disconnected: %s", username)
		broadcastMessage("User "+username+" has disconnected.\n", client)
	}()

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		message = strings.TrimSpace(message)

		if strings.HasPrefix(message, "/create ") {
			roomName := strings.TrimPrefix(message, "/create ")
			room := &ChatRoom{name: roomName, clients: make(map[*Client]bool)}
			room.clients[client] = true
			client.room = room
			mu.Lock()
			rooms[roomName] = room
			mu.Unlock()
			conn.Write([]byte("Chat room '" + roomName + "' created.\n"))
			log.Infof("New chat room created: %s", roomName)
		} else if strings.HasPrefix(message, "/join ") {
			roomName := strings.TrimPrefix(message, "/join ")
			mu.Lock()
			room, ok := rooms[roomName]
			mu.Unlock()
			if ok {
				room.clients[client] = true
				client.room = room
				conn.Write([]byte("Joined chat room '" + roomName + "'.\n"))
			} else {
				conn.Write([]byte("Chat room '" + roomName + "' does not exist.\n"))
			}
		} else if message == "/help" {
			response := "/create: command for creating chat rooms\n"
			response += "/join: command for joining chat rooms\n"
			response += "/ban <username>: command for banning users\n"
			response += "/kick <username>: command for kicking users\n"
			_, err := conn.Write([]byte(response))
			if err != nil {
				log.WithError(err).Error("Error sending help message to client")
				break
			}
			continue
		} else if strings.HasPrefix(message, "/ban ") {
			targetUsername := strings.TrimPrefix(message, "/ban ")
			mu.Lock()
			targetClient := findClientByUsername(targetUsername)
			if targetClient != nil {
				bannedIPs[targetClient.conn.RemoteAddr().String()] = true
				targetClient.conn.Write([]byte("You have been banned from the server.\n"))
				targetClient.conn.Close()
				delete(clients, targetClient.conn)
				if targetClient.room != nil {
					delete(targetClient.room.clients, targetClient)
				}
				broadcastMessage("User "+targetUsername+" has been banned.\n", client)
			} else {
				conn.Write([]byte("User '" + targetUsername + "' not found.\n"))
			}
			mu.Unlock()
		} else if strings.HasPrefix(message, "/kick ") {
			targetUsername := strings.TrimPrefix(message, "/kick ")
			mu.Lock()
			targetClient := findClientByUsername(targetUsername)
			if targetClient != nil {
				targetClient.conn.Write([]byte("You have been kicked from the server.\n"))
				targetClient.conn.Close()

				delete(clients, targetClient.conn)
				if targetClient.room != nil {
					delete(targetClient.room.clients, targetClient)
				}
				broadcastMessage("User "+targetUsername+" has been kicked.\n", client)
			} else {
				conn.Write([]byte("User '" + targetUsername + "' not found.\n"))
			}
			mu.Unlock()
		} else if message == "/exit" {
			if client.room != nil {
				broadcastToRoom(client.room, client.username+" is now offline.", client)
				delete(client.room.clients, client)
				log.Infof("Client %s has left room %s", client.username, client.room.name)
				client.room = nil
			}
			break
		} else if strings.HasPrefix(message, "/status typing") {
			if client.room != nil {
				broadcastToRoom(client.room, client.username+" is typing...", client)
			}
		} else if strings.HasPrefix(message, "/status") {
			if client.room != nil {
				onlineUsers := getOnlineUsers(client.room)
				conn.Write([]byte("Online users in room " + client.room.name + ": " + onlineUsers + "\n"))
			}
		} else if strings.HasPrefix(message, "/gpt ") {
			query := strings.TrimPrefix(message, "/gpt ")
			response, err := getGPTResponse(query)
			if err != nil {
				conn.Write([]byte("Error querying GPT: " + err.Error() + "\n"))
			} else {
				conn.Write([]byte("GPT-3 response: " + response + "\n"))
			}
		} else {
			log.Infof("Received message from %s: %s", client.username, message)
			if client.room != nil {
				broadcastToRoom(client.room, "/status typing "+client.username, client)
				time.Sleep(500 * time.Millisecond)
				broadcastToRoom(client.room, client.username+": "+message, client)
			}
		}
	}

	mu.Lock()
	delete(clients, conn)
	mu.Unlock()
	if client.room != nil {
		broadcastToRoom(client.room, client.username+" is now offline.", client)
		log.Infof("Client %s has disconnected from room %s", client.username, client.room.name)
	}
	log.Infof("Client disconnected: %s", client.username)
}

func main() {
	log.AddHandler(console.New(true), log.AllLevels...)

	cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		log.WithError(err).Error("error loading key pair")
		os.Exit(1)
	}
	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	listen, err := tls.Listen(constants.TYPE, constants.HOST+":"+constants.PORT, config)
	if err != nil {
		log.WithError(err).Error("error starting server")
		os.Exit(1)
	}

	defer listen.Close()

	go func() {
		http.HandleFunc("/admin", adminPanelHandler)
		http.ListenAndServe(":8080", nil)
	}()

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.WithError(err).Error("error accepting connection")
			continue
		}
		go handleRequest(conn)
	}
}

func isBanned(ip string) bool {
	mu.Lock()
	defer mu.Unlock()
	return bannedIPs[ip]
}

func findClientByUsername(username string) *Client {
	for _, client := range clients {
		if client.username == username {
			return client
		}
	}
	return nil
}

func broadcastMessage(message string, sender *Client) {
	mu.Lock()
	defer mu.Unlock()
	for _, client := range clients {
		if client != sender && client.room == sender.room {
			client.conn.Write([]byte(message))
		}
	}
}

func adminPanelHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	response := "Server Activity:\n"
	for _, client := range clients {
		duration := time.Since(client.connectedAt)
		response += "User: " + client.username + " - Connected for: " + duration.String() + "\n"
	}
	for roomName, room := range rooms {
		response += "Room: " + roomName + " - Users: " + strconv.Itoa(len(room.clients)) + "\n"
	}
	w.Write([]byte(response))
}
