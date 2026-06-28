package conn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectionManager_RegisterGetDelete(t *testing.T) {
	cm := NewConnectionManager()

	// Register
	client := &ClientConnection{UserID: 1}
	cm.Register(1, client)

	// Get
	got, ok := cm.Get(1)
	assert.True(t, ok)
	assert.Equal(t, client, got)

	// Delete
	cm.Delete(1)
	_, ok = cm.Get(1)
	assert.False(t, ok)
}

func TestConnectionManager_KickOld(t *testing.T) {
	cm := NewConnectionManager()

	oldClient := &ClientConnection{UserID: 1, SendCh: make(chan []byte, 256), CloseCh: make(chan struct{})}
	cm.Register(1, oldClient)

	newClient := &ClientConnection{UserID: 1, SendCh: make(chan []byte, 256), CloseCh: make(chan struct{})}
	cm.KickOld(1, newClient)

	// Old connection should receive kick message
	select {
	case msg := <-oldClient.SendCh:
		assert.Contains(t, string(msg), "kick")
		assert.Contains(t, string(msg), "new_login")
	default:
		t.Fatal("old client should receive kick message")
	}

	// Old connection's CloseCh should be closed
	select {
	case <-oldClient.CloseCh:
		// expected: CloseCh was closed
	default:
		t.Fatal("old client's CloseCh should be closed")
	}

	// New connection should be registered
	got, ok := cm.Get(1)
	assert.True(t, ok)
	assert.Equal(t, newClient, got)
}

func TestConnectionManager_KickOld_NoExisting(t *testing.T) {
	cm := NewConnectionManager()

	// No existing connection — KickOld should just register the new one
	newClient := &ClientConnection{UserID: 1, SendCh: make(chan []byte, 256), CloseCh: make(chan struct{})}
	cm.KickOld(1, newClient)

	got, ok := cm.Get(1)
	assert.True(t, ok)
	assert.Equal(t, newClient, got)
}
