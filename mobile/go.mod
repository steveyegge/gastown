module github.com/steveyegge/gastown/mobile

go 1.24.2

toolchain go1.24.12

require (
	connectrpc.com/connect v1.19.1
	github.com/steveyegge/gastown v0.0.0
	golang.org/x/net v0.48.0
	google.golang.org/protobuf v1.36.9
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	golang.org/x/text v0.33.0 // indirect
)

replace github.com/steveyegge/gastown => ../
