package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
)

type TopologyMessage struct {
	Type     string              `json:"type"`
	Topology map[string][]string `json:"topology"`
}

type BroadcastMessage struct {
	Type    string `json:"type"`
	Message int64  `json:"message"`
}

type GossipMessage struct {
	Type    string `json:"type"`
	Message int64  `json:"message"`
}

type ReadMessage struct {
	Type string `json:"type"`
}

const retryInterval = 1

func main() {
	n := maelstrom.NewNode()

	var messagesMtx sync.RWMutex
	var pendingMtx sync.RWMutex
	messages := make(map[int64]struct{})
	pending := make(map[int64]map[string]struct{})
	var neighbors []string

	gossipMessage := func(msg maelstrom.Message, message int64) error {

		// Broadcast gossip to neighbours until all successful
		pendingNodes := make(map[string]struct{}, len(neighbors))
		for _, node := range neighbors {
			pendingNodes[node] = struct{}{}
		}
		pendingMtx.Lock()
		pending[message] = pendingNodes
		pendingMtx.Unlock()

		for len(pendingNodes) > 0 {
			pendingMtx.RLock()
			pendingNodes = pending[message]
			pendingMtx.RUnlock()

			for node := range pendingNodes {
				_ = n.Send(node, map[string]any{
					"type":    "gossip",
					"message": message,
				})
			}

			if len(pendingNodes) > 0 {
				time.Sleep(time.Duration(retryInterval) * time.Second)
			}
		}

		return nil
	}

	n.Handle("topology", func(msg maelstrom.Message) error {
		var body TopologyMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		neighbors = body.Topology[n.ID()]

		return n.Reply(msg, map[string]any{
			"type": "topology_ok",
		})
	})

	n.Handle("broadcast", func(msg maelstrom.Message) error {
		var body BroadcastMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// Receive and acknowledge gossip
		messagesMtx.Lock()
		_, seen := messages[body.Message]
		if !seen {
			messages[body.Message] = struct{}{}
			go gossipMessage(msg, body.Message)
		}
		messagesMtx.Unlock()


		return n.Reply(msg, map[string]any{
			"type": "broadcast_ok",
		})
	})

	n.Handle("gossip", func(msg maelstrom.Message) error {
		var body GossipMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		// Receive and acknowledge gossip
		messagesMtx.Lock()
		_, seen := messages[body.Message]
		if !seen {
			messages[body.Message] = struct{}{}
			go gossipMessage(msg, body.Message)
		}
		messagesMtx.Unlock()

		n.Send(msg.Src, map[string]any{
			"type":    "gossip_ack",
			"message": body.Message,
		})

		return nil
	})

	n.Handle("gossip_ack", func(msg maelstrom.Message) error {
		var body GossipMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		pendingMtx.Lock()
		delete(pending[body.Message], msg.Src)
		pendingMtx.Unlock()
		return nil
	})

	n.Handle("read", func(msg maelstrom.Message) error {
		// Read messages
		messagesMtx.RLock()
		out := make([]int64, 0, len(messages))
		for message := range messages {
			out = append(out, message)
		}
		messagesMtx.RUnlock()

		return n.Reply(msg, map[string]any{
			"type":     "read_ok",
			"messages": out,
		})
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
