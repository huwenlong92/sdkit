package memory

import (
	"context"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
)

type Store struct {
	mu sync.RWMutex

	clients     map[string]*realtime.Client
	userClients map[string]map[string]struct{}
	rooms       map[string]map[string]struct{}
	clientRooms map[string]map[string]struct{}
	heartbeats  map[string]time.Time
}

func New() *Store {
	return &Store{
		clients:     make(map[string]*realtime.Client),
		userClients: make(map[string]map[string]struct{}),
		rooms:       make(map[string]map[string]struct{}),
		clientRooms: make(map[string]map[string]struct{}),
		heartbeats:  make(map[string]time.Time),
	}
}

func NewRegistry() *Store {
	return New()
}

func (s *Store) Add(client *realtime.Client) error {
	if client == nil {
		return realtime.ErrNilClient
	}
	if client.ID == "" {
		return realtime.ErrEmptyClientID
	}
	if client.Ch == nil {
		client.Ch = make(chan realtime.Event, 64)
	}
	now := time.Now()
	if client.CreatedAt.IsZero() {
		client.CreatedAt = now
	}
	client.LastActiveAt = now
	identity := client.CurrentIdentity()
	client.Identity = identity
	if identity != nil {
		client.UserID = identity.ID
		client.TenantID = identity.TenantID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.clients[client.ID]; current != nil {
		s.removeIndexesLocked(current)
	}
	s.clients[client.ID] = client
	s.addUserIndexLocked(client)
	s.heartbeats[client.ID] = now
	return nil
}

func (s *Store) Remove(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	client := s.clients[clientID]
	if client == nil {
		return nil
	}
	s.removeIndexesLocked(client)
	delete(s.clients, clientID)
	delete(s.heartbeats, clientID)
	return nil
}

func (s *Store) Get(clientID string) (*realtime.Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	client, ok := s.clients[clientID]
	return client, ok
}

func (s *Store) GetUserClients(userID string) []*realtime.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.userClients[userID]
	clients := make([]*realtime.Client, 0, len(ids))
	for clientID := range ids {
		if client := s.clients[clientID]; client != nil {
			clients = append(clients, client)
		}
	}
	return clients
}

func (s *Store) GetRoomClients(roomID string) []*realtime.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if roomID == "" {
		clients := make([]*realtime.Client, 0, len(s.clients))
		for _, client := range s.clients {
			clients = append(clients, client)
		}
		return clients
	}
	ids := s.rooms[roomID]
	clients := make([]*realtime.Client, 0, len(ids))
	for clientID := range ids {
		if client := s.clients[clientID]; client != nil {
			clients = append(clients, client)
		}
	}
	return clients
}

func (s *Store) Online(ctx context.Context, clientID string, identity *realtime.Identity) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if clientID == "" {
		return realtime.ErrEmptyClientID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client := s.clients[clientID]
	if client == nil {
		client = &realtime.Client{ID: clientID, Identity: identity}
		s.clients[clientID] = client
	} else {
		s.removeIndexesLocked(client)
		client.Identity = identity
	}
	if identity != nil {
		identity = identity.Normalize()
		client.Identity = identity
		client.UserID = identity.ID
		client.TenantID = identity.TenantID
	}
	s.addUserIndexLocked(client)
	s.heartbeats[clientID] = time.Now()
	return nil
}

func (s *Store) Offline(ctx context.Context, clientID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.Remove(clientID)
}

func (s *Store) Heartbeat(ctx context.Context, clientID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if clientID == "" {
		return realtime.ErrEmptyClientID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if client := s.clients[clientID]; client != nil {
		client.LastActiveAt = time.Now()
	}
	s.heartbeats[clientID] = time.Now()
	return nil
}

func (s *Store) IsOnline(ctx context.Context, clientID string) (bool, error) {
	if err := ctxErr(ctx); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.clients[clientID]
	return ok, nil
}

func (s *Store) Join(ctx context.Context, roomID string, clientID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if roomID == "" {
		return realtime.ErrEmptyEvent
	}
	if clientID == "" {
		return realtime.ErrEmptyClientID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[roomID] == nil {
		s.rooms[roomID] = make(map[string]struct{})
	}
	if s.clientRooms[clientID] == nil {
		s.clientRooms[clientID] = make(map[string]struct{})
	}
	s.rooms[roomID][clientID] = struct{}{}
	s.clientRooms[clientID][roomID] = struct{}{}
	return nil
}

func (s *Store) Leave(ctx context.Context, roomID string, clientID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if roomID != "" {
		delete(s.rooms[roomID], clientID)
		if len(s.rooms[roomID]) == 0 {
			delete(s.rooms, roomID)
		}
		delete(s.clientRooms[clientID], roomID)
		if len(s.clientRooms[clientID]) == 0 {
			delete(s.clientRooms, clientID)
		}
		return nil
	}
	for joinedRoom := range s.clientRooms[clientID] {
		delete(s.rooms[joinedRoom], clientID)
		if len(s.rooms[joinedRoom]) == 0 {
			delete(s.rooms, joinedRoom)
		}
	}
	delete(s.clientRooms, clientID)
	return nil
}

func (s *Store) Clients(ctx context.Context, roomID string) ([]string, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.rooms[roomID]
	out := make([]string, 0, len(ids))
	for clientID := range ids {
		out = append(out, clientID)
	}
	return out, nil
}

func (s *Store) Rooms(ctx context.Context, clientID string) ([]string, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rooms := s.clientRooms[clientID]
	out := make([]string, 0, len(rooms))
	for roomID := range rooms {
		out = append(out, roomID)
	}
	return out, nil
}

func (s *Store) addUserIndexLocked(client *realtime.Client) {
	userID := clientUserID(client)
	if userID == "" {
		return
	}
	if s.userClients[userID] == nil {
		s.userClients[userID] = make(map[string]struct{})
	}
	s.userClients[userID][client.ID] = struct{}{}
}

func (s *Store) removeIndexesLocked(client *realtime.Client) {
	if client == nil {
		return
	}
	userID := clientUserID(client)
	if userID != "" {
		delete(s.userClients[userID], client.ID)
		if len(s.userClients[userID]) == 0 {
			delete(s.userClients, userID)
		}
	}
	for roomID := range s.clientRooms[client.ID] {
		delete(s.rooms[roomID], client.ID)
		if len(s.rooms[roomID]) == 0 {
			delete(s.rooms, roomID)
		}
	}
	delete(s.clientRooms, client.ID)
}

func clientUserID(client *realtime.Client) string {
	if client == nil {
		return ""
	}
	if client.Identity != nil {
		if id := client.Identity.Normalize().ID; id != "" {
			return id
		}
	}
	return client.UserID
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

var (
	_ realtime.Registry = (*Store)(nil)
	_ realtime.Presence = (*Store)(nil)
	_ realtime.Room     = (*Store)(nil)
)
