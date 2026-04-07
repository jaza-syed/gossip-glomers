package main

import (
	"encoding/json"
	"fmt"
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
	Type     string  `json:"type"`
	Messages []int64 `json:"messages"`
}

const gossipInterval = 75 * time.Millisecond
const groupCount = 5
const groupSize = 5

func clusteredNeighbors(nodeID string, nodeIDs []string) ([]string, error) {
	if len(nodeIDs) != groupCount*groupSize {
		return nil, fmt.Errorf("expected %d nodes, got %d", groupCount*groupSize, len(nodeIDs))
	}

	nodeIndex := -1
	for i, id := range nodeIDs {
		if id == nodeID {
			nodeIndex = i
			break
		}
	}
	if nodeIndex == -1 {
		return nil, fmt.Errorf("unknown node %q", nodeID)
	}

	groupIndex := nodeIndex / groupSize
	groupStart := groupIndex * groupSize
	hubID := nodeIDs[groupStart]

	if nodeIndex == groupStart {
		neighbors := make([]string, 0, (groupSize-1)+(groupCount-1))
		for i := 1; i < groupSize; i++ {
			neighbors = append(neighbors, nodeIDs[groupStart+i])
		}
		for i := 0; i < groupCount; i++ {
			otherHub := nodeIDs[i*groupSize]
			if otherHub != nodeID {
				neighbors = append(neighbors, otherHub)
			}
		}
		return neighbors, nil
	}

	return []string{hubID}, nil
}

func main() {
	n := maelstrom.NewNode()

	var messagesMtx sync.RWMutex
	messages := make(map[int64]struct{})
	var pendingMtx sync.RWMutex
	pending := make(map[string]map[int64]struct{})
	var neighbors []string
	topologyReady := make(chan struct{})

	storeMessages := func(in []int64) []int64 {
		messagesMtx.Lock()
		defer messagesMtx.Unlock()

		out := make([]int64, 0, len(in))
		for _, message := range in {
			if _, seen := messages[message]; seen {
				continue
			}
			messages[message] = struct{}{}
			out = append(out, message)
		}

		return out
	}

	queuePending := func(in []int64, src string) {
		if len(in) == 0 {
			return
		}

		pendingMtx.Lock()
		defer pendingMtx.Unlock()

		for _, node := range neighbors {
			if node == src {
				continue
			}
			if pending[node] == nil {
				pending[node] = make(map[int64]struct{})
			}
			for _, message := range in {
				pending[node][message] = struct{}{}
			}
		}
	}

	go func() {
		<-topologyReady

		ticker := time.NewTicker(gossipInterval)
		defer ticker.Stop()

		for range ticker.C {
			for _, neighbor := range neighbors {
				pendingMtx.RLock()
				pendingMessages := pending[neighbor]
				batch := make([]int64, 0, len(pendingMessages))
				for message := range pendingMessages {
					batch = append(batch, message)
				}
				pendingMtx.RUnlock()

				if len(batch) == 0 {
					continue
				}

				_ = n.Send(neighbor, GossipMessage{
					Type:     "gossip",
					Messages: batch,
				})
			}
		}
	}()

	n.Handle("topology", func(msg maelstrom.Message) error {
		var body TopologyMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		var err error
		neighbors, err = clusteredNeighbors(n.ID(), n.NodeIDs())
		if err != nil {
			return err
		}
		for _, neighbor := range neighbors {
			pending[neighbor] = make(map[int64]struct{})
		}
		close(topologyReady)

		return n.Reply(msg, map[string]any{
			"type": "topology_ok",
		})
	})

	n.Handle("broadcast", func(msg maelstrom.Message) error {
		var body BroadcastMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		newMessages := storeMessages([]int64{body.Message})
		if len(newMessages) > 0 {
			queuePending(newMessages, msg.Src)
		}

		return n.Reply(msg, map[string]any{
			"type": "broadcast_ok",
		})
	})

	n.Handle("gossip", func(msg maelstrom.Message) error {
		var body GossipMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		newMessages := storeMessages(body.Messages)
		if len(newMessages) > 0 {
			queuePending(newMessages, msg.Src)
		}

		return n.Send(msg.Src, GossipMessage{
			Type:     "gossip_ok",
			Messages: body.Messages,
		})
	})

	n.Handle("gossip_ok", func(msg maelstrom.Message) error {
		var body GossipMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		pendingMtx.Lock()
		for _, message := range body.Messages {
			delete(pending[msg.Src], message)
		}
		pendingMtx.Unlock()
		return nil
	})

	n.Handle("read", func(msg maelstrom.Message) error {
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
