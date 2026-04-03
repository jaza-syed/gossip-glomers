package main
import (
	"encoding/json"
	"log"

	maelstrom "github.com/jepsen-io/maelstrom/demo/go"
	uuid "github.com/google/uuid"
)

func main() {
	n:= maelstrom.NewNode()

	// Handle an echo message
	n.Handle("generate", func(msg maelstrom.Message) error {
		// Unmarshal the message body as a loosely-typed map
		var body map[string]any
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}

		// Update the message type to return back
		body["type"] = "generate_ok"
		body["id"] = uuid.New()

		// Echo the original message back with the updated message type
		return n.Reply(msg, body)
	})

	if err := n.Run(); err != nil {
		log.Fatal(err)
	}
}
