package kvpaxos

const (
	OK       = "OK"
	ErrNoKey = "ErrNoKey"
	PUT      = "PUT"
	GET      = "GET"
)

type Err string

type PutArgs struct {
	// You'll have to add definitions here.
	Key       string
	Value     string
	RequestID int
	ClientID  int
}

type PutReply struct {
	Err Err
}

type GetArgs struct {
	// You'll have to add definitions here.
	Key       string
	RequestID int
	ClientID  int
}

type GetReply struct {
	Err   Err
	Value string
}
