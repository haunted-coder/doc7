//go:build !darwin

package credentials

import "errors"

func keychainPreferred() bool {
	return false
}

func findKeychainPassword(account string) (Result, error) {
	return Result{}, nil
}

func storeKeychainPassword(account string, apiKey string) (string, error) {
	return "", errors.New("keychain credential store is only supported on macOS")
}
