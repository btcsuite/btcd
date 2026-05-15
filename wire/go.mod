module github.com/btcsuite/btcd/wire/v2

go 1.23.2

require (
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.40.0
)

require (
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// TODO(guggero): Remove once we have a tagged version of the chainhash package.
replace github.com/btcsuite/btcd/chainhash/v2 => ../chainhash
