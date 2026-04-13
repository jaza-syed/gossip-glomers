package main

import (
	"maps"
	"context"
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

type ReadMessage struct {
	Type    string `json:"type"`
}

type AddMessage struct {
	Type    string `json:"type"`
	Delta   int64  `json:"delta"`
}

type GossipMessage struct {
	Type   string           `json:"type"`
	Values map[string]int64 `json:"values"`
}

const gossipInterval = 100 * time.Millisecond

func main() {
	n := maelstrom.NewNode()
	kv := maelstrom.NewSeqKV(n)
	nodeIDs := []string{"n0", "n1", "n2"}
	// Cache: current value per-node
	var cacheMtx sync.RWMutex
	cache := map[string]int64{}

	initialize := func() error {
		for n.ID() == "" {
			time.Sleep(10 * time.Millisecond)
		}

		// Initialise cache with values for all nodes
		cacheMtx.Lock()
		for _, nID := range nodeIDs {
			if _, ok := cache[nID]; !ok {
				cache[nID] = 0
			}
		}
		current := cache[n.ID()]
		cacheMtx.Unlock()

		return kv.Write(context.Background(), n.ID(), current)
	}

	go func() {
		if err := initialize(); err != nil {
			log.Print(err)
			return
		}

		ticker := time.NewTicker(gossipInterval)
		defer ticker.Stop()

		for range ticker.C {
			// Gossip full cache to all other nodes
			cacheMtx.RLock()
			values := make(map[string]int64, len(cache))
			maps.Copy(values, cache)
			cacheMtx.RUnlock()
			
			for _, nID := range nodeIDs {
				if nID == n.ID() {
					continue
				}
				n.Send(nID, map[string]any{
					"type":   "gossip",
					"values": values,
				})
			}
		}
	}()

	n.Handle("gossip", func (msg maelstrom.Message) error {
		// Update our current counter values if we get info that
		// They are higher
		var body GossipMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		cacheMtx.Lock()
		for nID, value := range body.Values {
			cache[nID] = max(value, cache[nID])
		}
		cacheMtx.Unlock()

		return nil
	})

	n.Handle("add", func(msg maelstrom.Message) error {
		var body AddMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// Update node in local cache and in seq-kv
		cacheMtx.Lock()
		cache[n.ID()] += body.Delta
		current := cache[n.ID()]
		cacheMtx.Unlock()
		context := context.Background()
		kv.Write(context, n.ID(), current)

		return n.Reply(msg, map[string]any{
			"type": "add_ok",
		})
	})

	n.Handle("read", func(msg maelstrom.Message) error {
		var body ReadMessage
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// See if sequential store has higher values for all nodes
		ctx := context.Background()
		kvValues := make(map[string]int64, len(nodeIDs))
		for _, nID := range nodeIDs {
			value, err := kv.ReadInt(ctx, nID)
			if err != nil {
				value = 0
			}
			kvValues[nID] = int64(value)
		}

		// Update cache with any higher values and compute total
		cacheMtx.Lock()
		value := int64(0)
		for _, nID := range nodeIDs {
			cache[nID] = max(cache[nID], kvValues[nID])
		}
		for _, nodeValue := range cache {
			value += nodeValue
		}
		cacheMtx.Unlock()
		
		return n.Reply(msg, map[string]any{
			"type": "read_ok",
			"value": value,
		})
	})
	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
