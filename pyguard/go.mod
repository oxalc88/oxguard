module github.com/oxDevelop/oxguard/pyguard

go 1.26.11

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/oxalc88/oxguard/contract v0.0.0
	golang.org/x/sys v0.50.0
	golang.org/x/term v0.48.0
)

replace github.com/oxalc88/oxguard/contract => ../contract
