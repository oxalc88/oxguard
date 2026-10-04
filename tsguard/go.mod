module github.com/oxalc88/oxguard/tsguard

go 1.25.0

require golang.org/x/term v0.41.0

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

require github.com/oxalc88/oxguard/contract v0.0.0

replace github.com/oxalc88/oxguard/contract => ../contract
