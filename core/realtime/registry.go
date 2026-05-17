package realtime

type Registry interface {
	Add(client *Client) error
	Remove(clientID string) error
	Get(clientID string) (*Client, bool)
	GetUserClients(userID string) []*Client
	GetRoomClients(roomID string) []*Client
}
