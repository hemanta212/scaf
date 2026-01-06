package internal

import "github.com/rlch/neogo"

type User struct {
	neogo.Node `neo4j:"User"`

	ID        int    `json:"id" neo4j:"id"`
	Name      string `json:"name" neo4j:"name"`
	Email     string `json:"email" neo4j:"email"`
	CreatedAt int    `json:"createdAt,omitempty" neo4j:"createdAt"`

	Authored neogo.Many[Authored] `neo4j:"->"`
	Wrote    neogo.Many[Wrote]    `neo4j:"->"`
}

type Post struct {
	neogo.Node `neo4j:"Post"`

	ID         int    `json:"id" neo4j:"id"`
	Title      string `json:"title" neo4j:"title"`
	Content    string `json:"content" neo4j:"content"`
	CreatedAt  int    `json:"createdAt,omitempty" neo4j:"createdAt"`
	AuthorName string `json:"authorName,omitempty" neo4j:"-"`

	Has neogo.Many[Has] `neo4j:"->"`
}

type Comment struct {
	neogo.Node `neo4j:"Comment"`

	ID         int    `json:"id" neo4j:"id"`
	Text       string `json:"text" neo4j:"text"`
	CreatedAt  int    `json:"createdAt,omitempty" neo4j:"createdAt"`
	AuthorName string `json:"authorName,omitempty" neo4j:"-"`
	PostTitle  string `json:"postTitle,omitempty" neo4j:"-"`

	Replies neogo.Many[Replies] `neo4j:"->"`
}

type Authored struct {
	neogo.Relationship `neo4j:"AUTHORED"`

	User *User `neo4j:"startNode"`
	Post *Post `neo4j:"endNode"`
}

type Wrote struct {
	neogo.Relationship `neo4j:"WROTE"`

	User    *User    `neo4j:"startNode"`
	Comment *Comment `neo4j:"endNode"`
}

type Has struct {
	neogo.Relationship `neo4j:"HAS"`

	Post    *Post    `neo4j:"startNode"`
	Comment *Comment `neo4j:"endNode"`
}

type Replies struct {
	neogo.Relationship `neo4j:"REPLIES"`

	Reply  *Comment `neo4j:"startNode"`
	Parent *Comment `neo4j:"endNode"`
}
