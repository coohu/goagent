package redis

import (
	"context"
	"time"
)

type Config struct {
	Addr     string
	Password string
	DB       int
}

type Client struct {
	addr     string
	password string
	db       int
}

func New(cfg Config) (*Client, error) {
	c := &Client{addr: cfg.Addr, password: cfg.Password, db: cfg.DB}
	if err := c.ping(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) ping(_ context.Context) error {
	return nil
}

func (c *Client) Set(_ context.Context, key, value string, ttl time.Duration) error {
	return nil
}

func (c *Client) Get(_ context.Context, key string) (string, error) {
	return "", nil
}

func (c *Client) Del(_ context.Context, key string) error {
	return nil
}

func (c *Client) RPush(_ context.Context, key string, values ...string) error {
	return nil
}

func (c *Client) LRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	return nil, nil
}

func (c *Client) Expire(_ context.Context, key string, ttl time.Duration) error {
	return nil
}
