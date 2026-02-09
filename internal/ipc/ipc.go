package ipc

import (
	"net/http"
)

// Client handles communication with PHP workers
type Client struct {
	// connection details
}

// NewClient creates a new IPC client
func NewClient() *Client {
	return &Client{}
}

// ForwardRequest sends the HTTP request to a PHP worker and returns the response
func (c *Client) ForwardRequest(w http.ResponseWriter, r *http.Request) error {
	// TODO: Implement FastCGI or custom protocol here
	return nil
}
