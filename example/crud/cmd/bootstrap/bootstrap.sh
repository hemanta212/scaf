#!/bin/bash
# Bootstrap script for CRUD example
# "It works on my machine" - every developer ever

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCAF_BIN="$(cd "$PROJECT_DIR/../.." && pwd)/bin/scaf"

# Neo4j config - hardcoded because env vars are for people who read documentation
NEO4J_CONTAINER="scaf-crud-neo4j"
NEO4J_USER="neo4j"
NEO4J_PASS="password"  # security theater at its finest

echo "=== CRUD Example Bootstrap ==="
echo "Preparing to mass-delete your data. You did back up, right? lol"
echo ""

# Check if container exists - Docker: making "it works on my machine" everyone's problem
if ! docker ps --format '{{.Names}}' | grep -q "^${NEO4J_CONTAINER}$"; then
    echo "ERROR: Neo4j container '$NEO4J_CONTAINER' not running"
    echo ""
    echo "Skill issue detected. Start the container or cope."
    echo "Protip: docker start $NEO4J_CONTAINER"
    exit 1
fi

# Helper function because typing docker exec is for peasants
cypher() {
    docker exec "$NEO4J_CONTAINER" cypher-shell -u "$NEO4J_USER" -p "$NEO4J_PASS" "$1"
}

echo "Nuking existing CRUD data... (hope you weren't attached to it)"
cypher "MATCH (n) WHERE n:User OR n:Post OR n:Comment DETACH DELETE n"

echo "Creating constraints... (because data integrity is not optional, even if your code quality is)"
cypher "
CREATE CONSTRAINT user_id IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE;
CREATE CONSTRAINT post_id IF NOT EXISTS FOR (p:Post) REQUIRE p.id IS UNIQUE;
CREATE CONSTRAINT comment_id IF NOT EXISTS FOR (c:Comment) REQUIRE c.id IS UNIQUE;
CREATE INDEX user_email IF NOT EXISTS FOR (u:User) ON (u.email);
CREATE INDEX post_created IF NOT EXISTS FOR (p:Post) ON (p.createdAt);
CREATE INDEX comment_created IF NOT EXISTS FOR (c:Comment) ON (c.createdAt);
"

echo "Seeding users... (finally, some NPCs for your sad little app)"
cypher "
CREATE (alice:User {id: 1, name: 'Alice', email: 'alice@example.com', createdAt: 1700000000})
CREATE (bob:User {id: 2, name: 'Bob', email: 'bob@example.com', createdAt: 1700000001})
CREATE (charlie:User {id: 3, name: 'Charlie', email: 'charlie@example.com', createdAt: 1700000002})
"

echo "Creating posts... (content nobody asked for)"
cypher "
CREATE (p1:Post {id: 1, title: 'Hello World', content: 'My first post!', createdAt: 1700000100})
CREATE (p2:Post {id: 2, title: 'Learning Neo4j', content: 'Graph databases are cool', createdAt: 1700000200})
CREATE (p3:Post {id: 3, title: 'Bobs Adventures', content: 'Travel stories', createdAt: 1700000300})
"

echo "Creating comments... (engagement farming at scale)"
cypher "
CREATE (c1:Comment {id: 1, text: 'Great post!', createdAt: 1700000150})
CREATE (c2:Comment {id: 2, text: 'Thanks for sharing', createdAt: 1700000160})
CREATE (c3:Comment {id: 3, text: 'Very interesting', createdAt: 1700000250})
CREATE (c4:Comment {id: 4, text: 'I agree with you!', createdAt: 1700000260})
CREATE (c5:Comment {id: 5, text: 'Good point', createdAt: 1700000270})
"

echo "Linking authors to posts... (establishing who to blame)"
cypher "
MATCH (alice:User {id: 1}), (bob:User {id: 2})
MATCH (p1:Post {id: 1}), (p2:Post {id: 2}), (p3:Post {id: 3})
CREATE (alice)-[:AUTHORED {createdAt: 1700000100}]->(p1)
CREATE (alice)-[:AUTHORED {createdAt: 1700000200}]->(p2)
CREATE (bob)-[:AUTHORED {createdAt: 1700000300}]->(p3)
"

echo "Wiring up comments... (building the argument graph)"
cypher "
MATCH (bob:User {id: 2}), (alice:User {id: 1}), (charlie:User {id: 3})
MATCH (p1:Post {id: 1}), (p2:Post {id: 2})
MATCH (c1:Comment {id: 1}), (c2:Comment {id: 2}), (c3:Comment {id: 3}), (c4:Comment {id: 4}), (c5:Comment {id: 5})
CREATE (bob)-[:WROTE {createdAt: 1700000150}]->(c1)
CREATE (alice)-[:WROTE {createdAt: 1700000160}]->(c2)
CREATE (charlie)-[:WROTE {createdAt: 1700000250}]->(c3)
CREATE (alice)-[:WROTE {createdAt: 1700000260}]->(c4)
CREATE (bob)-[:WROTE {createdAt: 1700000270}]->(c5)
CREATE (p1)-[:HAS {createdAt: 1700000150}]->(c1)
CREATE (p1)-[:HAS {createdAt: 1700000160}]->(c2)
CREATE (p2)-[:HAS {createdAt: 1700000250}]->(c3)
CREATE (c4)-[:REPLIES {createdAt: 1700000260}]->(c1)
CREATE (c5)-[:REPLIES {createdAt: 1700000270}]->(c3)
"

echo ""
echo "Verifying we didn't screw this up..."
echo "Nodes:"
cypher "MATCH (n) RETURN labels(n)[0] as type, count(n) as count"
echo ""
echo "Relationships:"
cypher "MATCH ()-[r]->() RETURN type(r) as type, count(r) as count"

echo ""
echo "Generating Go code... (replacing developers one query at a time)"
cd "$PROJECT_DIR"
"$SCAF_BIN" generate internal/

echo ""
echo "=== Bootstrap Complete ==="
echo "Your database is now full of fake people having fake conversations."
echo "Just like Twitter, but with better data modeling."
echo ""
echo "Next steps:"
echo "  make build        # build the TUI and CLI"
echo "  make test         # pray the tests pass"
echo "  make run-tui      # run the TUI"
echo "  touch grass       # optional but recommended"
