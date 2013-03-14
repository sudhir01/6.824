package paxos

// acceptor's state:
//   n_p (highest prepare seen)
//   n_a, v_a (highest accept seen)
type Instance struct {
  //With this optimization, an acceptor needs to remember only the highest
  //numbered proposal that it has ever accepted and the number of the highest
  //numbered prepare request to which it has responded.
  highestAccepted int
  highestResponded int
  agreed bool
  value interface{}
}

//constructor for instances
func MakeInstance() Instance {
  instance := Instance{}
  instance.highestAccepted = -1
  instance.highestResponded = -1
  instance.agreed = false
  instance.value = nil
  return instance
}

// Phase 1. 
// (a) A proposer selects a proposal number n and sends a prepare
//     request with number n to a majority of acceptors.
// (b) If an acceptor receives a prepare request with number n greater
//     than that of any prepare request to which it has already responded,
// 	   then it responds to the request with a promise not to accept any more
//     proposals numbered less than n and with the highest-numbered proposal (if any) that it has accepted.
// Phase 2.
// (a) If the proposer receives a response to its prepare requests
//     (numbered n) from a majority of acceptors, then it sends an accept
//     request to each of those acceptors for a proposal numbered n with a
//     value v, where v is the value of the highest-numbered proposal among
//     the responses, or is any value if the responses reported no proposals.
// (b) If an acceptor receives an accept request for a proposal numbered
//     n, it accepts the proposal unless it has already responded to a prepare
//     request having a number greater than n.

type PrepareArgs struct {
  Instance int //paxos instance this belongs to
  Proposal int //proposal number
  Done int //max done seen
  Me string //me!
}

type PrepareReply struct {
  MaxProposalAcceptedSoFar int
  NextProposalNumber int
  OK bool
  Value interface{}
}

type AcceptArgs struct {
  Instance int
  Proposal int
  Value interface{}
}

type AcceptReply struct {
  OK bool
  Proposal int
  NextProposalNumber int
}

type DecidedArgs struct {
  Instance int
  Proposal int
  Value interface{}
}

type DecidedReply struct {
  OK bool
  NextProposalNumber int
}
