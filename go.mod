module www.theskyscape.com

go 1.24.5

toolchain go1.24.8

require (
	github.com/The-Skyscape/devtools v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/sosedoff/gitkit v0.4.0
	github.com/yuin/goldmark v1.7.13
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/libsql/sqlite-antlr4-parser v0.0.0-20240327125255-dbf53b6cbf06 // indirect
	github.com/tursodatabase/go-libsql v0.0.0-20250912065916-9dd20bb43d31 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8 // indirect
	golang.org/x/sys v0.35.0 // indirect
)

// Use local devtools during development
replace github.com/The-Skyscape/devtools => ../devtools
