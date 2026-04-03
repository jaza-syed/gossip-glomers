# Challenge 3

## Challenge 3b)

- We receive `broadcast` messages to one node, but read messages should 
  eventually be consistent on all nodes. We therefore need to tell other nodes
  about messages we recieve. We will do this by implementing a new message 
  `gossip` to send a message to other nodes and a handler for it on each node.
  Broadcast messages are unique, so nodes only get a subset of the messages.

- Ok: simplest implementation, we literally we receive a number we also send
  a gossip with that number to each neighbout. If it has it already it doesn't
  do anything, if it doesn't it adds it. How do we avoid cycles? 
  - Message can travel through the system with a list of nodes it's been to?
    This is the simplest method to understand, but the message will get heavy.
  - Node can avoid stop gossiping the number if it receives a message it already
    had?

- Handlers are goroutines which can run concurrently - it's not javascript!!
  So we need to use mutexes!
- When we do `n.Send` we do have to potentially deal with the response!

## Challenge 3c)

- Network partitions are introduced now, so the network will be temporarily
  made disjoint to simulate a failing network. I guess the simplest way to do
  this is to introduce retries, so we need to track whether sends to a node
  are successful and retry until they are.
- Currently, `n.Send` does not actually fail when there is a network partition.
  We need to receive an ack **before** we mark a number as having been sent. 
  But we may be gossiping multiple messages. So we actually need to maintain
  a some sort of map number->pending. 
- Principle: I want to keep sending until i receive an ack. so I need to keep
  track of whether I've received an ack for each number I've got. My ack 
  handler will mark it as received. The data structure this requires is
  Set "pending" which `gossipMessage` creates of the neighbours, for that 
  message. Ack. will remove the message from that list. So:

```go
type Pending struct {
    Mu sync.RWMutex
    Pending map[int][]string
}
```

- `gossipMessage` inserts (receivedMessage -> list of neighbours) with a
  write lock. It then makes a copy of the list for that message with a read 
  lock before every set of retries. GossipMessage sends `gossip_ok` after
  it inserts the message into it's own messages.
- `gossip_ok` handler will remove the node it's been received from from pending.
- However , what if the partition happens before `gossip_ok` is sent?
  it'll just do a redundant resend, which I think is fine?.
- Last 


## Chat with George

- missing the goroutine on the call

Categories of solution:
- Push: - Most naive version: is what I was doing: trying to push to everyone
- Pull: - Most naive version: constantly ask neighbours for what nodes they  have
- Hybrid: - Most naive version: 
  - broadcast once and don't retry
  - constantly read rpc every second from neighbours
  - this is naive because you're doing the full read every time
