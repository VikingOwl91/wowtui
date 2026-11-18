//go:build !unix

package fingerprint

import "os"

const readFlags = os.O_RDONLY
