module github.com/caddyserver/circuitbreaker

go 1.14

require (
	github.com/caddyserver/caddy/v2 v2.0.0-test.4
	github.com/diamondburned/oxy v1.1.1-0.20200502024248-e47851599193
	gopkg.in/ahmetb/go-linq.v3 v3.1.0 // indirect
)

// see https://github.com/caddyserver/caddy/issues/3331 for why this is here
replace gopkg.in/ahmetb/go-linq.v3 => github.com/ahmetb/go-linq/v3 v3.1.0
