// keygen-tool — single-shot PASETO v4 asymmetric secret keygen.
// Built static, scp'd to a target host, executed once, then deleted.
// Output format: two lines on stdout, "PRIV=<hex>\nPUB=<hex>\n".
package main

import (
	"fmt"

	"github.com/smackerel/smackerel/internal/auth"
)

func main() {
	priv, pub := auth.GenerateSigningKeypair()
	fmt.Printf("PRIV=%s\n", priv)
	fmt.Printf("PUB=%s\n", pub)
}
