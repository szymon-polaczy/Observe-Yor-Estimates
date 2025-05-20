module observe-yor-estimates

go 1.24.1

require (
	github.com/gorilla/websocket v1.5.3
	github.com/joho/godotenv v1.5.1
)

require (
	crons/tasks v0.0.0-00010101000000-000000000000
	github.com/mattn/go-sqlite3 v1.14.28
)

replace crons/tasks => ./crons
