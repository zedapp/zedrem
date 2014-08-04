package main

import (
        "fmt"
        "encoding/json"
        "errors"
        "code.google.com/p/go.net/websocket"
)


var editorClients map[string]*EditorClient = make(map[string]*EditorClient)

type EditorClient struct {
        id string
        writeChannels []chan string
}

func GetEditorClientChannel(uuid string) *EditorClient {
        client, ok := editorClients[uuid]
        if !ok {
                client = &EditorClient {
                        id: uuid,
                        writeChannels: make([]chan string, 0),
                }
                editorClients[uuid] = client
        }
        return client
}

func(client *EditorClient) NewChannel() chan string {
        ch := make(chan string, 20)
        client.writeChannels = append(client.writeChannels, ch)
        return ch
}

func (client *EditorClient) Send(editId string) error {
        if len(client.writeChannels) == 0 {
                return errors.New("no-client")
        }
        for _, ch := range client.writeChannels {
                ch <- editId
        }
        return nil
}

func (client *EditorClient) DisconnectChannel(ch chan string) {
        for i, curCh := range client.writeChannels {
                if curCh == ch {
                        client.writeChannels = append(client.writeChannels[:i], client.writeChannels[i+1:]...)
                        close(ch)
                }
        }

        if len(client.writeChannels) == 0 {
                // Delete client object altogether
                delete(editorClients, client.id)
        }
}

var pongBuff []byte = []byte(`{"type": "pong"}`)


func editorSocketServer(ws *websocket.Conn) {
        defer ws.Close()
        buffer := make([]byte, BUFFER_SIZE)
        n, err := ws.Read(buffer)
        var hello HelloMessage
        err = json.Unmarshal(buffer[:n], &hello)
        if err != nil {
                fmt.Println("Could not parse welcome message.")
                return
        }

        fmt.Println("Edit client", hello.UUID, "connected")

        client := GetEditorClientChannel(hello.UUID)
        clientChan := client.NewChannel()

        closed := false

        closeSocket := func() {
                if closed {
                        return
                }
                closed = true
                fmt.Println("Client disconnected", hello.UUID)
                client.DisconnectChannel(clientChan)
        }

        defer closeSocket()

        go func() {
                for {
                        buf := make([]byte, 1024)
                        n, err := ws.Read(buf)
                        if err != nil {
                                closeSocket()
                                return
                        }
                        var message EditSocketMessage
                        err = json.Unmarshal(buf[:n], &message)
                        if message.MessageType == "ping" {
                                ws.Write(pongBuff)
                        }
                }

        }()

        for {
                url, request_ok := <-clientChan
                if !request_ok {
                        return
                }
                messageBuf, err := json.Marshal(EditSocketMessage{"open", url})
                if err != nil {
                        fmt.Println("Couldn't serialize URL")
                        continue
                }
                _, err = ws.Write(messageBuf)
                if err != nil {
                        fmt.Println("Got error", err)
                        return
                }
        }
}
