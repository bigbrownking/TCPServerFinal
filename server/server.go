package main

import (
	"bufio"
	"final_project/constants"
	"github.com/go-playground/log"
	"github.com/go-playground/log/handlers/console"
	"net"
	"os"
	"strings"
	"sync"
)

type Client struct {
	conn     net.Conn
	username string
	room     *ChatRoom
}

type ChatRoom struct {
	name    string
	clients map[*Client]bool
}

var (
	clients = make(map[net.Conn]*Client)
	rooms   = make(map[string]*ChatRoom)
	mu      sync.Mutex
)

func handleRequest(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	conn.Write([]byte("Welcome to the server! Type \"/help\" to get a list of commands.\n"))
	client := &Client{conn: conn, username: username}
	mu.Lock()
	clients[conn] = client
	mu.Unlock()
	log.Infof("New client connected: %s", username)
	for {
		message, _ := reader.ReadString('\n')
		message = strings.TrimSpace(message)

		if strings.HasPrefix(message, "/create ") {
			roomName := strings.TrimPrefix(message, "/create ")
			room := &ChatRoom{name: roomName, clients: make(map[*Client]bool)}
			room.clients[client] = true
			mu.Lock()
			rooms[roomName] = room
			mu.Unlock()
			conn.Write([]byte("Chat room '" + roomName + "' created.\n"))
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
		}
		if message == "/help" {
			response := "/create: command for creating chat rooms\n"
			response += "/join: command for joining chat rooms\n"
			_, err := conn.Write([]byte(response))
			if err != nil {
				log.WithError(err).Error("Error sending help message to client")
				break
			}
			continue
		} else {
			log.Infof("Received message from %s: %s", username, message)

			_, err := conn.Write([]byte("Server received the message\n"))
			if err != nil {
				log.WithError(err).Error("Error sending confirmation message to client")
				break
			}
		}

	}
}

func main() {
	log.AddHandler(console.New(true), log.AllLevels...)

	listen, err := net.Listen(constants.TYPE, constants.HOST+":"+constants.PORT)
	if err != nil {
		log.WithError(err).Error("error starting server")
		os.Exit(1)
	}

	defer listen.Close()

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.WithError(err).Error("error accepting connection")
			os.Exit(1)
		}
		go handleRequest(conn)
	}
}
