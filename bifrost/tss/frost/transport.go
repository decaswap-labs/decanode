package frost

type Transport interface {
	Broadcast(sessionID string, round int, data []byte) error
	Collect(sessionID string, round int, expected int) (map[uint16][]byte, error)
}
