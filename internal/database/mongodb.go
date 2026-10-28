package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"streamgo/internal/logger"
)

var log = logger.New("mongodb")

// Client wraps the mongo.Client and database instance.
type Client struct {
	Client   *mongo.Client
	Database *mongo.Database
}

// Connect establishes a connection pool to MongoDB and tests connectivity with Ping.
func Connect(ctx context.Context, uri, databaseName string) (*Client, error) {
	if uri == "" {
		return nil, fmt.Errorf("mongo uri is empty")
	}

	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(5 * time.Second).
		SetMinPoolSize(2).
		SetMaxPoolSize(50)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create mongo client: %w", err)
	}

	// Verify connectivity with a quick ping
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping mongodb at %s: %w", uri, err)
	}

	log.Infof("connected successfully to database: %s", databaseName)

	return &Client{
		Client:   client,
		Database: client.Database(databaseName),
	}, nil
}

// Collection returns a handle to the specified collection.
func (c *Client) Collection(name string) *mongo.Collection {
	return c.Database.Collection(name)
}

// Ping checks if the database is responsive.
func (c *Client) Ping(ctx context.Context) error {
	return c.Client.Ping(ctx, readpref.Primary())
}

// Close gracefully closes the connection pool.
func (c *Client) Close(ctx context.Context) error {
	log.Info("disconnecting from database...")
	return c.Client.Disconnect(ctx)
}
