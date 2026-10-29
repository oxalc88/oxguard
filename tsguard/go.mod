module github.com/oxalc88/oxguard/tsguard

go 1.26.0

require golang.org/x/term v0.47.0

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/oxalc88/oxguard/contract v0.0.0
	golang.org/x/sys v0.49.0
)

replace github.com/oxalc88/oxguard/contract => ../contract
