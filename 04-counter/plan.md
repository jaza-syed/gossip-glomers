# Challenge 4

Sequential consistency:

1. All nodes operations are in a shared global sequence
1. For each node, it's own operations appear in the global sequence in the
   order that they were issued.
1. There is no cross-node real-time ordering constraint, so operations on one
   node may or may not appear before another nodes operation in the global
   sequence even if it happened earlier in wall-clock time.

This implies that there is no guarantee at all on visibility in time!

From GPT:

```text
Sequential consistency gives you a single coherent global history,
but it does not guarantee eventual visibility or convergence.
So it does not force another node’s completed write to show up in your 
future reads after any amount of real time.

The important nuance is that the other node’s write is not “ignored” or 
absent from the system. It is somewhere in the one global order. What is
not guaranteed is that your later reads will eventually be ordered after it.

```

Shared writes (modifications to the global state) have to be in the same global
order.

```text
Sequential consistency only says:
  whatever results the store returns must be explainable by one global order.
It does not say:
  other nodes’ writes must eventually move before your reads in that order.
```

Sequential consistency is a weak guarantee. In theory, if two operations are
from different nodes, the global order could be such that operations from one
are never seen in operations from the other e.g. all of B before all of A, etc.
There is nothing to force that "Eventually a write from A must become visible
to B" which is what I was waiting for previously with my polling reads.

So: we can get the sequential consistency property by having every node read
and write from a sequentially consistence, that's basically easy with CAS. But
there's no guarantees on any timescale for convergence, which is what we need
for the final read to work. This has to be done via inter-node communication.

CAS loop works because `seq-kv` is a real kv store that does provide writes
from other nodes eventually, and not a pathological adversary that refuses to
provide writes. However, it does **not** guarantee to provide them in any time
interval.

We cannot simply take the maximum of gossiped values, because concurrent
increments under partition will diverge and need to be reconciled.

## Design

GPT Ideas:
```
- State-based CRDT counters
- Operation-based replication - nodes exchange missing events with deduplication
- Anti-entryopy protocols - periodic sync exchanging either full state, compact
  summaries, or "what have I seen so far"
- Version counter / vector clocks per node summaries e.g. increment counts per node
```

Questions I have:
- What triggers repair - how do we know we actually need to do repair? Does
  the repair process only get triggered as a result of not receiving acks from
  other nodes.

- One idea is a version vector: map from version to updates from each' counter. 

Ok: Idea I was missing - we store **each node's** total in the sequential store
with reads and writes. We also keep an in-memory version, that responds to 
periodic broadcasts from the other nodes of each nodes current total and takes
the maximum - if that's higher, we can read from that.
