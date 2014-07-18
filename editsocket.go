package main

import (
        "fmt"
        "encoding/json"
        "code.google.com/p/go.net/websocket"
)


var editorClients map[string]*EditorClient = make(map[string]*EditorClient)

type EditorClient struct {
        writeChannels []chan string
        pendingEditIds []string
}

func GetEditorClientChannel(uuid string) *EditorClient {
        client, ok := editorClients[uuid]
        if !ok {
                client = &EditorClient {
                        writeChannels: make([]chan string, 0),
                        pendingEditIds: make([]string, 0),
                }
                editorClients[uuid] = client
        }
        return client
}

func(client *EditorClient) NewChannel() chan string {
        ch := make(chan string, 20)
        client.writeChannels = append(client.writeChannels, ch)
        // Were any urls pending? Flush them out
        if len(client.pendingEditIds) > 0 {
                for _, editId := range client.pendingEditIds {
                        ch <- editId
                }
                client.pendingEditIds = make([]string, 0)
        }
        return ch
}

func (client *EditorClient) Send(editId string) {
        if len(client.writeChannels) == 0 {
                client.pendingEditIds = append(client.pendingEditIds, editId)
                return
        }
        for _, ch := range client.writeChannels {
                ch <- editId
        }
}

func (client *EditorClient) DisconnectChannel(ch chan string) {
        for i, curCh := range client.writeChannels {
                if curCh == ch {
                        fmt.Println("Removed chan from list")
                        client.writeChannels = append(client.writeChannels[:i], client.writeChannels[i+1:]...)
                        close(ch)
                }
        }
}


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
                        buf := make([]byte, 1)
                        _, err := ws.Read(buf)
                        if err != nil {
                                closeSocket()
                                return
                        }
                }

        }()

        for {
                url, request_ok := <-clientChan
                if !request_ok {
                        fmt.Println("Channel closed, bye!")
                        return
                }
                fmt.Println("Now sending URL", url)
                _, err := ws.Write([]byte(url))
                if err != nil {
                        fmt.Println("Got error", err)
                        return
                }
        }
}
