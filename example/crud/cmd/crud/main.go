package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rlch/neogo"
	"github.com/rlch/scaf/example/crud/service"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "crud",
		Usage: "CRUD operations for users, posts, comments",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "uri",
				Usage:   "Neo4j connection URI",
				Value:   "bolt://localhost:7689",
				Sources: cli.EnvVars("NEO4J_URI"),
			},
			&cli.StringFlag{
				Name:    "user",
				Usage:   "Neo4j username",
				Value:   "neo4j",
				Sources: cli.EnvVars("NEO4J_USER"),
			},
			&cli.StringFlag{
				Name:    "pass",
				Usage:   "Neo4j password",
				Value:   "password",
				Sources: cli.EnvVars("NEO4J_PASS"),
			},
		},
		Commands: []*cli.Command{
			userCommands(),
			commentCommands(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func getDB(ctx context.Context, cmd *cli.Command) (neogo.Driver, error) {
	uri := cmd.String("uri")
	user := cmd.String("user")
	pass := cmd.String("pass")

	return neogo.New(uri, neo4j.BasicAuth(user, pass, ""))
}

func userCommands() *cli.Command {
	return &cli.Command{
		Name:    "user",
		Aliases: []string{"u"},
		Usage:   "User operations",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a new user",
				ArgsUsage: "<name> <email>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 2 {
						return fmt.Errorf("usage: crud user create <name> <email>")
					}
					name := cmd.Args().Get(0)
					email := cmd.Args().Get(1)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewUserService(db)
					user, err := svc.Create(ctx, name, email)
					if err != nil {
						return err
					}

					return printJSON(user)
				},
			},
			{
				Name:      "get",
				Usage:     "Get user by ID",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud user get <id>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewUserService(db)
					user, err := svc.GetByID(ctx, id)
					if err != nil {
						return err
					}

					return printJSON(user)
				},
			},
			{
				Name:  "list",
				Usage: "List all users",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewUserService(db)
					users, err := svc.List(ctx)
					if err != nil {
						return err
					}

					return printJSON(users)
				},
			},
			{
				Name:      "update",
				Usage:     "Update user",
				ArgsUsage: "<id> <name> <email>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 3 {
						return fmt.Errorf("usage: crud user update <id> <name> <email>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)
					name := cmd.Args().Get(1)
					email := cmd.Args().Get(2)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewUserService(db)
					user, err := svc.Update(ctx, id, name, email)
					if err != nil {
						return err
					}

					return printJSON(user)
				},
			},
			{
				Name:      "delete",
				Usage:     "Delete user",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud user delete <id>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewUserService(db)
					if err := svc.Delete(ctx, id); err != nil {
						return err
					}

					fmt.Println("deleted")
					return nil
				},
			},
		},
	}
}

func commentCommands() *cli.Command {
	return &cli.Command{
		Name:    "comment",
		Aliases: []string{"c"},
		Usage:   "Comment operations",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a new comment on a post",
				ArgsUsage: "<text> <author_id> <post_id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 3 {
						return fmt.Errorf("usage: crud comment create <text> <author_id> <post_id>")
					}
					text := cmd.Args().Get(0)
					var authorID, postID int
					fmt.Sscanf(cmd.Args().Get(1), "%d", &authorID)
					fmt.Sscanf(cmd.Args().Get(2), "%d", &postID)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					comment, err := svc.Create(ctx, text, authorID, postID)
					if err != nil {
						return err
					}
					return printJSON(comment)
				},
			},
			{
				Name:      "get",
				Usage:     "Get comment by ID",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud comment get <id>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					comment, err := svc.GetByID(ctx, id)
					if err != nil {
						return err
					}
					return printJSON(comment)
				},
			},
			{
				Name:      "list",
				Usage:     "List comments by post",
				ArgsUsage: "<post_id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud comment list <post_id>")
					}
					var postID int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &postID)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					comments, err := svc.GetByPost(ctx, postID)
					if err != nil {
						return err
					}
					return printJSON(comments)
				},
			},
			{
				Name:      "reply",
				Usage:     "Reply to a comment",
				ArgsUsage: "<text> <author_id> <parent_comment_id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 3 {
						return fmt.Errorf("usage: crud comment reply <text> <author_id> <parent_id>")
					}
					text := cmd.Args().Get(0)
					var authorID, parentID int
					fmt.Sscanf(cmd.Args().Get(1), "%d", &authorID)
					fmt.Sscanf(cmd.Args().Get(2), "%d", &parentID)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					reply, err := svc.Reply(ctx, text, authorID, parentID)
					if err != nil {
						return err
					}
					return printJSON(reply)
				},
			},
			{
				Name:      "replies",
				Usage:     "Get replies to a comment",
				ArgsUsage: "<comment_id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud comment replies <comment_id>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					replies, err := svc.GetReplies(ctx, id)
					if err != nil {
						return err
					}
					return printJSON(replies)
				},
			},
			{
				Name:      "delete",
				Usage:     "Delete a comment",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return fmt.Errorf("usage: crud comment delete <id>")
					}
					var id int
					fmt.Sscanf(cmd.Args().Get(0), "%d", &id)

					db, err := getDB(ctx, cmd)
					if err != nil {
						return err
					}
					defer db.DB().Close(ctx)

					svc := service.NewCommentService(db)
					if err := svc.Delete(ctx, id); err != nil {
						return err
					}
					fmt.Println("deleted")
					return nil
				},
			},
		},
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
