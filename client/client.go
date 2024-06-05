package main

import (
	"bufio"
	"crypto/tls"
	"final_project/constants"
	"fmt"
	"github.com/go-playground/log"
	"os"
	"strings"
	"time"
)

type Client struct {
	conn       *tls.Conn
	reader     *bufio.Reader
	username   string
	historyLog []string
}

func NewClient(conn *tls.Conn, reader *bufio.Reader, username string) *Client {
	return &Client{
		conn:       conn,
		reader:     reader,
		username:   username,
		historyLog: make([]string, 0),
	}
}

func (c *Client) Send(message string) error {
	_, err := c.conn.Write([]byte(message + "\n"))
	if err != nil {
		return err
	}
	c.historyLog = append(c.historyLog, message)
	return nil
}

func (c *Client) DisplayHistory() {
	fmt.Println("=== Message History ===")
	for _, msg := range c.historyLog {
		fmt.Println(msg)
	}
	fmt.Println("======================")
}

func main() {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial(constants.TYPE, constants.HOST+":"+constants.PORT, conf)
	if err != nil {
		fmt.Println("Dial failed:", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	client := NewClient(conn, reader, username)

	err = client.Send(username)
	if err != nil {
		fmt.Println("Write data failed:", err)
		os.Exit(1)
	}
	welcome, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.WithError(err).Error("Read welcome message failed")
		os.Exit(1)
	}
	fmt.Print(welcome)

	go func() {
		for {
			response, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil {
				log.WithError(err).Error("Read response failed")
				os.Exit(1)
			} else if !strings.HasPrefix(response, "/status") {
				fmt.Print(response)
			}
		}
	}()

	for {
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		if text == "/history" {
			client.DisplayHistory()
			continue
		}

		if text == "/exit" {
			client.Send("/exit")
			fmt.Println("Exiting...")
			conn.Close()
			os.Exit(0)
		}

		if text == "/status" {
			client.Send("/status")
			continue
		}

		if strings.HasPrefix(text, "/gpt ") {
			client.Send(text)
			continue
		}

		err = client.Send("/status typing")
		if err != nil {
			fmt.Println("Write data failed:", err)
			os.Exit(1)
		}

		time.Sleep(500 * time.Millisecond)

		err = client.Send(text)
		if err != nil {
			fmt.Println("Write data failed:", err)
			os.Exit(1)
		}

		time.Sleep(1 * time.Second)
	}
}
