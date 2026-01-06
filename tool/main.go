package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
)

type runFunc func(context.Context, *xrpc.Client, string) ([]*bsky.ActorDefs_ProfileView, error)

var cmds = map[string]runFunc{
	"id":        nil, // exits early, since we're passing the result of this to every other function
	"following": following,
	"followers": followers,
	"mutes":     muted,
	"blocks":    blocked, // app.bsky.graph.getBlocks not yet implemented
}

const (
	// Rate limiting: delay between API calls to avoid hitting rate limits
	apiCallDelay = 1 * time.Second
	// Maximum number of retries for rate-limited requests
	maxRetries = 5
	// Initial backoff duration for retries
	initialBackoff = 2 * time.Second
)

func init() { flag.Parse() }

// retryWithBackoff attempts an API call with exponential backoff on rate limit errors
func retryWithBackoff[T any](ctx context.Context, operation func() (*T, error), operationName string) (*T, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry attempt %d/%d for %s after rate limit, waiting %v", attempt, maxRetries, operationName, backoff)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		result, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err
		// Check if it's a rate limit error (429)
		if xrpcErr, ok := err.(*xrpc.Error); ok && xrpcErr.StatusCode == 429 {
			log.Printf("Rate limit hit for %s (attempt %d/%d)", operationName, attempt+1, maxRetries+1)
			continue
		}

		// For non-rate-limit errors, fail immediately
		return nil, err
	}

	return nil, fmt.Errorf("%s failed after %d retries: %w", operationName, maxRetries, lastErr)
}

func main() {
	if flag.NArg() != 1 {
		panic(fmt.Sprintf("requires 1 positional argument (cmd name); got %d args", flag.NArg()))
	}
	cmd, ok := cmds[flag.Arg(0)]
	if !ok {
		panic("unknown command: " + flag.Arg(0))
	}

	var (
		bskyHandle   = "BSKY_HANDLE"
		bskyAppPwd   = "BSKY_APP_PWD"
		bskyInstance = "BSKY_INSTANCE"
	)
	for _, v := range []*string{&bskyHandle, &bskyAppPwd, &bskyInstance} {
		if vv := os.Getenv(*v); vv == "" {
			panic(*v + " env var not set")
		} else {
			*v = vv
		}
	}
	ctx := context.Background()

	// create a new session
	client := &xrpc.Client{
		Client: util.RobustHTTPClient(),
		Host:   bskyInstance,
		Auth:   &xrpc.AuthInfo{Handle: bskyHandle},
	}

	auth, err := atproto.ServerCreateSession(context.TODO(), client, &atproto.ServerCreateSession_Input{
		Identifier: client.Auth.Handle,
		Password:   bskyAppPwd,
	})
	if err != nil {
		panic(fmt.Errorf("ServerCreateSession failed with %w", err))
	}
	client.Auth.AccessJwt = auth.AccessJwt
	client.Auth.RefreshJwt = auth.RefreshJwt
	client.Auth.Did = auth.Did
	client.Auth.Handle = auth.Handle

	profile, err := selfID(ctx, client, client.Auth.Did)
	if err != nil {
		panic(fmt.Errorf("selfID failed with %w", err))
	}
	if flag.Arg(0) == "id" {
		fmt.Print(profile.Did)
		return
	}

	if err := print(ctx, client, client.Auth.Did, cmd); err != nil {
		panic(err)
	}
}

func selfID(ctx context.Context, c *xrpc.Client, did string) (*bsky.ActorDefs_ProfileViewDetailed, error) {
	acc, err := bsky.ActorGetProfile(ctx, c, did)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func print(ctx context.Context, c *xrpc.Client, did string, fnc runFunc) error {
	out, err := fnc(ctx, c, did)
	if err != nil {
		return err
	}

	for _, acc := range out {
		fmt.Printf("%s,%s\n", acc.Did, acc.Handle)
	}
	return nil
}

func following(ctx context.Context, c *xrpc.Client, did string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	pageNum := 0

	for {
		pageNum++
		log.Printf("Fetching following page %d (cursor: %s)", pageNum, cursor)

		// Capture cursor in a local variable for the closure
		currentCursor := cursor
		fs, err := retryWithBackoff(ctx, func() (*bsky.GraphGetFollows_Output, error) {
			return bsky.GraphGetFollows(ctx, c, did, currentCursor, 100)
		}, fmt.Sprintf("GraphGetFollows page %d", pageNum))

		if err != nil {
			return nil, err
		}

		accounts = append(accounts, fs.Follows...)
		log.Printf("Fetched %d accounts (total: %d)", len(fs.Follows), len(accounts))

		if fs.Cursor != nil && *fs.Cursor != "" {
			cursor = *fs.Cursor
		} else {
			break
		}
		if len(fs.Follows) == 0 {
			break
		}

		// Rate limiting: sleep between requests to avoid hitting API limits
		time.Sleep(apiCallDelay)
	}

	log.Printf("Total following accounts fetched: %d", len(accounts))
	return accounts, nil
}

func followers(ctx context.Context, c *xrpc.Client, did string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	pageNum := 0

	for {
		pageNum++
		log.Printf("Fetching followers page %d (cursor: %s)", pageNum, cursor)

		// Capture cursor in a local variable for the closure
		currentCursor := cursor
		fs, err := retryWithBackoff(ctx, func() (*bsky.GraphGetFollowers_Output, error) {
			return bsky.GraphGetFollowers(ctx, c, did, currentCursor, 100)
		}, fmt.Sprintf("GraphGetFollowers page %d", pageNum))

		if err != nil {
			return nil, err
		}

		accounts = append(accounts, fs.Followers...)
		log.Printf("Fetched %d accounts (total: %d)", len(fs.Followers), len(accounts))

		if fs.Cursor != nil && *fs.Cursor != "" {
			cursor = *fs.Cursor
		} else {
			break
		}
		if len(fs.Followers) == 0 {
			break
		}

		// Rate limiting: sleep between requests to avoid hitting API limits
		time.Sleep(apiCallDelay)
	}

	log.Printf("Total followers fetched: %d", len(accounts))
	return accounts, nil
}

// app.bsky.graph.getBlocks not yet implemented
func blocked(ctx context.Context, c *xrpc.Client, id string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	pageNum := 0

	for {
		pageNum++
		log.Printf("Fetching blocks page %d (cursor: %s)", pageNum, cursor)

		// Capture cursor in a local variable for the closure
		currentCursor := cursor
		fs, err := retryWithBackoff(ctx, func() (*bsky.GraphGetBlocks_Output, error) {
			return bsky.GraphGetBlocks(ctx, c, currentCursor, 100)
		}, fmt.Sprintf("GraphGetBlocks page %d", pageNum))

		if err != nil {
			return nil, err
		}

		accounts = append(accounts, fs.Blocks...)
		log.Printf("Fetched %d accounts (total: %d)", len(fs.Blocks), len(accounts))

		if fs.Cursor != nil && *fs.Cursor != "" {
			cursor = *fs.Cursor
		} else {
			break
		}
		if len(fs.Blocks) == 0 {
			break
		}

		// Rate limiting: sleep between requests to avoid hitting API limits
		time.Sleep(apiCallDelay)
	}

	log.Printf("Total blocked accounts fetched: %d", len(accounts))
	return accounts, nil
}

func muted(ctx context.Context, c *xrpc.Client, did string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	pageNum := 0

	for {
		pageNum++
		log.Printf("Fetching mutes page %d (cursor: %s)", pageNum, cursor)

		// Capture cursor in a local variable for the closure
		currentCursor := cursor
		fs, err := retryWithBackoff(ctx, func() (*bsky.GraphGetMutes_Output, error) {
			return bsky.GraphGetMutes(ctx, c, currentCursor, 100)
		}, fmt.Sprintf("GraphGetMutes page %d", pageNum))

		if err != nil {
			return nil, err
		}

		accounts = append(accounts, fs.Mutes...)
		log.Printf("Fetched %d accounts (total: %d)", len(fs.Mutes), len(accounts))

		if fs.Cursor != nil && *fs.Cursor != "" {
			cursor = *fs.Cursor
		} else {
			break
		}
		if len(fs.Mutes) == 0 {
			break
		}

		// Rate limiting: sleep between requests to avoid hitting API limits
		time.Sleep(apiCallDelay)
	}

	log.Printf("Total muted accounts fetched: %d", len(accounts))
	return accounts, nil
}
