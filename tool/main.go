package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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

func init() { flag.Parse() }

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
	for {
		fs, err := bsky.GraphGetFollows(ctx, c, did, cursor, 100)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, fs.Follows...)
		if fs.Cursor != nil {
			cursor = *fs.Cursor
		}
		if len(fs.Follows) == 0 {
			break
		}
	}
	return accounts, nil
}

func followers(ctx context.Context, c *xrpc.Client, did string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	for {
		fs, err := bsky.GraphGetFollowers(ctx, c, did, cursor, 100)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, fs.Followers...)
		if fs.Cursor != nil {
			cursor = *fs.Cursor
		}
		if len(fs.Followers) == 0 {
			break
		}
	}
	return accounts, nil
}

// app.bsky.graph.getBlocks not yet implemented
func blocked(ctx context.Context, c *xrpc.Client, id string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	for {
		fs, err := bsky.GraphGetBlocks(ctx, c, cursor, 100) // using a local function until the library is updated.
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, fs.Blocks...)
		if fs.Cursor != nil {
			cursor = *fs.Cursor
		}
		if len(fs.Blocks) == 0 {
			break
		}
	}
	return accounts, nil
}

func muted(ctx context.Context, c *xrpc.Client, did string) ([]*bsky.ActorDefs_ProfileView, error) {
	var accounts []*bsky.ActorDefs_ProfileView
	var cursor string
	for {
		fs, err := bsky.GraphGetMutes(ctx, c, cursor, 100)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, fs.Mutes...)
		if fs.Cursor != nil {
			cursor = *fs.Cursor
		}
		if len(fs.Mutes) == 0 {
			break
		}
	}
	return accounts, nil
}
