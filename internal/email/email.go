package email

import (
	"bytes"
	"context"
	"fmt"
	"github.com/sony/gobreaker"
	"net/http"
	"time"
)

type Client struct {
	endpoint string
	http     *http.Client
	cb       *gobreaker.CircuitBreaker
}

func New(endpoint string) *Client {
	return &Client{endpoint: endpoint, http: &http.Client{Timeout: 3 * time.Second}, cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "email"})}
}
func (c *Client) SendInvite(ctx context.Context, email string) error {
	const methodCtx = "email.SendInvite"
	if c.endpoint == "" {
		return nil
	}
	_, err := c.cb.Execute(func() (any, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(email))
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return nil
}
