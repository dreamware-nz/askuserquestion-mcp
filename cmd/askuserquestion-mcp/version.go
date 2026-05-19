package main

// version is the build-time semver string for the askuserquestion-mcp
// binary. It is overridden via -ldflags "-X main.version=<tag>" by the
// release workflow; the in-tree default makes go-installed builds
// identifiable as "dev".
var version = "dev"
