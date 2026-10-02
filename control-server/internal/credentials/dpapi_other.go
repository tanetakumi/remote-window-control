//go:build !windows

package credentials

import "errors"

func unprotect([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is only available on Windows")
}
